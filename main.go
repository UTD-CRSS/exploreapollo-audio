package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strconv"
)

var workingDir string = path.Join(os.TempDir(), "apollo-audio")
var clipDir string = path.Join(workingDir, "clips")
var AAC string = "aac"
var OGG string = "ogg"

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return true, err
}

func makeDir(dir string) {
	dirExists, err := exists(dir)
	check(err)
	if !dirExists {
		err := os.Mkdir(dir, 0777)
		check(err)
	}
}

func downloadFromS3AndSave(filename string) string {
	clipPath := path.Join(clipDir, filename)
	if _, err := os.Stat(clipPath); err == nil {
		fmt.Println("file exists; skipping")
		return clipPath
	}
	fmt.Println(clipPath)
	fmt.Println("debug")
	out, err := os.Create(clipPath)
	check(err)
	defer out.Close()
	resp, err := http.Get("http://exploreapollo-tmp.s3.amazonaws.com/audio/Tape885_20July_20-07-00_HR2U_LunarLanding/" + filename)
	// resp, err := http.Get(filename)
	check(err)
	defer resp.Body.Close()
	_, err = io.Copy(out, resp.Body)
	check(err)
	return clipPath
}

type flushWriter struct {
	f http.Flusher
	w io.Writer
}

func (fw *flushWriter) Write(p []byte) (n int, err error) {
	n, err = fw.w.Write(p)
	if fw.f != nil {
		fw.f.Flush()
	}
	return
}

func handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-type", "audio/mpeg")
	c1 := downloadFromS3AndSave("c1.wav")
	c2 := downloadFromS3AndSave("c2.wav")
	sox, err := exec.LookPath("sox")
	check(err)
	fmt.Println("using sox " + sox)
	ffmpeg, err := exec.LookPath("ffmpeg")
	check(err)
	fmt.Println("using ffmpeg " + ffmpeg)
	soxArgs := []string{"-t", "wav", "-m", c1, c2, "-p"}
	soxCommand := exec.Command(sox, soxArgs...)
	ffmpegArgs := []string{"-i", "-", "-f", "mp3", "-ab", "256k", "pipe:"}
	ffmpegCommand := exec.Command(ffmpeg, ffmpegArgs...)
	fw := flushWriter{w: w}
	if f, ok := w.(http.Flusher); ok {
		fw.f = f
	}
	ffmpegCommand.Stdin, _ = soxCommand.StdoutPipe()
	ffmpegCommand.Stdout = &fw
	ffmpegCommand.Stderr = os.Stdout
	ffmpegCommand.Start()
	soxCommand.Run()
	ffmpegCommand.Wait()
	fmt.Println("done")
}

func getFileLocation(mission int, chanNum int, start int, end int) []string {
	/*
		will have to make sure file locations are acquired in order u kno...sheeeeeit
		my relational algebra is rusty.

		channeldb :=
		select url, start_met, end_met from channel_chunks
		where channel = chann

		fileloc1 :=
		select url, end_met from channeldb
		where start_met <= start
		and end_met > start

		if fileloc1.end_met < end:

		fileloc2 :=
		select url from channeldb
		where start_met < end
		and end_met <= end

	*/
	var urls []string
	switch chanNum {
		case 14:
			urls = append(urls, "14_SPACE-ENVIRONMENT_20July_20-07-00.wav")
		case 16:
			urls = append(urls, "16_SPAN_20July_20-07-00.wav")
		case 18:
			urls = append(urls, "18_BOOSTER-C_20July_20-07-00.wav")
		case 19:
			urls = append(urls, "19_BOOSTER-R_20July_20-07-00.wav")
		case 20:
			urls = append(urls, "20_3-FLIGHT-DIRECTOR-LOOP_20July_20-07-00.wav")
		case 21:
			urls = append(urls, "21_3-AFD-CONF-LOOP_20July_20-07-00.wav")
		case 22:
			urls = append(urls, "22_3-GOSS-2-LOOP_20July_20-07-00.wav")
		case 23:
			urls = append(urls, "23_ALSEP-EAO-2_20July_20-07-00.wav")
		case 24:
			urls = append(urls, "24_3-MOCR-DYN-LOOP_20July_20-07-00.wav")
		default:
			panic("unknown or unavailable channel")
	}
	return urls
}

func endMET(start int, duration int) int {
	// assuming met is in milliseconds, duration is given in seconds
	return start + duration * 1000

}


// http://localhost:5000/stream?mission=14&channel=14&channel=18&channel=24&format=aac&t=369300000&len=600
func streamHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-type", "audio/mpeg")
	// TODO: validate: filename.wav, filename.trs exist
	// TODO: validate: format is aac or ogg
	// TODO: validate: t is accepted MET HHHDDMM

	var audioFiles []string
	r.ParseForm()

	// use tracks parameter to query DB and find where appropriate track(s) live
	// may need multple tracks of the same channel in case length exceeds end of file MET
	mission, err := strconv.Atoi(r.Form["mission"][0])
	check(err)
	channels := r.Form["channel"]
	format := r.Form["format"][0]
	start, err := strconv.Atoi(r.Form["t"][0])
	check(err)
	duration, err := strconv.Atoi(r.Form["len"][0])
	check(err)
	end := endMET(start, duration)
	// TODO: convert MET timecode to start second in appropriate file
	fmt.Fprintf(w, "debug\n -- format: %s --\n -- startms: %d --\n -- endms: %d --\n", format, start, end)

	// retrive files to be mixed
	// TODO: refactoring needed OMG
	for n := range channels {
		var chunks []string
		chanNum, err := strconv.Atoi(channels[n])
		check(err)
		output := fmt.Sprintf("channel%d.wav", chanNum)
		chunkLoci := getFileLocation(mission, chanNum, start, end)

		// queerrry
		for c := range chunkLoci {
			fmt.Println("Downloading " + chunkLoci[c])
			fp := downloadFromS3AndSave(chunkLoci[c])
			chunks = append(chunks, fp)
		}

		if len(chunks) == 0 {
			panic("no corresponding file(s) found. something's wrong.")
		} else if len(chunks) == 1 {
			audioFiles = append(audioFiles, chunks[0])
		} else {
			sox, err := exec.LookPath("sox")
			check(err)
			fmt.Println("using sox " + sox)
			soxArgs := []string{"-t", "wav", "--combine", "concatenate"}
			soxArgs = append(soxArgs, chunks...)
			soxArgs = append(soxArgs, output)
			soxCommand := exec.Command(sox, soxArgs...)

			soxCommand.Run()

			audioFiles = append(audioFiles, output)
		}

	}

	// mmmmmmagic
	sox, err := exec.LookPath("sox")
	check(err)
	fmt.Println("using sox " + sox)
	ffmpeg, err := exec.LookPath("ffmpeg")
	check(err)
	fmt.Println("using ffmpeg " + ffmpeg)

	// mix the tracks
	// there's probably a better way to do this. halp.
	// TODO: time stuff
	soxArgs := []string{"-t", "wav", "-m"}
	soxArgs = append(soxArgs, audioFiles...)
	soxArgs = append(soxArgs, "-p")
	soxCommand := exec.Command(sox, soxArgs...)

	// convert the thing
	var ffmpegArgs []string
	if format == AAC {
		// ffmpegArgs = []string{"-i", "-", "-strict", "2", "-c:a", "aac", "-b:a", "240k", "-f", "m4a", "pipe:"}
		ffmpegArgs = []string{"-i", "-", "-c:a", "libfdk_aac", "-b:a", "256k", "-f", "m4a", "pipe:"}
		// dued idek wut encoder
		// works, but gotta compile ffmpeg on server with special options
	} else if format == OGG {
		ffmpegArgs = []string{"-i", "-", "-c:a", "libvorbis", "-qscale:a", "6", "-f", "ogg", "pipe:"}
	} else {
		fmt.Println("unsupported output format requested")
		// break it
		ffmpegArgs = []string{"-i", "-", "-f", "mp3", "-ab", "256k", "pipe:"}
	}
	ffmpegCommand := exec.Command(ffmpeg, ffmpegArgs...)

	fw := flushWriter{w: w}
	if f, ok := w.(http.Flusher); ok {
		fw.f = f
	}

	ffmpegCommand.Stdin, _ = soxCommand.StdoutPipe()
	ffmpegCommand.Stdout = &fw
	ffmpegCommand.Stderr = os.Stdout

	ffmpegCommand.Start()
	soxCommand.Run()
	ffmpegCommand.Wait()

	fmt.Println("done")
}


func main() {
	makeDir(workingDir)
	makeDir(clipDir)
	http.HandleFunc("/stream", streamHandler)
	ServerPort := "5000" // default port
	if len(os.Getenv("PORT")) > 0 {
		ServerPort = os.Getenv("PORT")
	}
	fmt.Println("Starting server on " + ServerPort)
	http.ListenAndServe(":"+ServerPort, nil)
}
