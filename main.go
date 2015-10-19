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

func thyme(timecode string) int {
	// TODO: validate: in expected range, ie. that time exists
	var startSecond int
	// days, err := strconv.Atoi(timecode[0:3])
	// hrs, err := strconv.Atoi(timecode[3:5])
	min, err := strconv.Atoi(timecode[5:7])
	sec, err := strconv.Atoi(timecode[7:9])
	if err != nil {
		panic(err)
	}
	startSecond = min * 60 + sec // lol math
	return startSecond

}

func streamHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-type", "audio/mpeg")
	// .../stream?track=14_SPACE-ENVIRONMENT_20July_20-07-00&track=16_SPAN_20July_20-07-00&track=18_BOOSTER-C_20July_20-07-00&format=aac&t=1023255&len=300
	// TODO: validate: filename.wav, filename.trs exist
	// TODO: validate: format is aac or ogg
	// TODO: validate: t is accepted MET HHHDDMM

	var audioFiles []string
	r.ParseForm()

	// use tracks parameter to query DB and find where appropriate track(s) live
	// may need multple tracks of the same channel in case length exceeds end of file MET
	tracks := r.Form["track"]
	for n := range tracks {
		tmpStr1 := fmt.Sprintf("%s.wav", tracks[n])
		fmt.Println("Pretending to download " + tmpStr1)
		fp := downloadFromS3AndSave(tmpStr1)
		audioFiles = append(audioFiles, fp)
	}

	format := r.Form["format"][0]
	timecode := r.Form["t"][0]
	// TODO: convert MET timecode to start second in appropriate file
	fmt.Fprintf(w, "debug\n -- format: %s --\n -- timecode: %s --\n -- startsecond: %s --", format, timecode, thyme(timecode))

	// mmmmmmagic
	sox, err := exec.LookPath("sox")
	check(err)
	fmt.Println("using sox " + sox)
	ffmpeg, err := exec.LookPath("ffmpeg")
	check(err)
	fmt.Println("using ffmpeg " + ffmpeg)

	// there's probably a better way to do this. halp.
	// TODO: time stuff
	soxArgs := []string{"-t", "wav", "-m"}
	soxArgs = append(soxArgs, audioFiles...)
	soxArgs = append(soxArgs, "-p")
	soxCommand := exec.Command(sox, soxArgs...)

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
