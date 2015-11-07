package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strconv"
	"queries"
)

var workingDir string = path.Join(os.TempDir(), "apollo-audio")
var clipDir string = path.Join(workingDir, "clips")
var AAC string = "aac"
var M4A string = "m4a"
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

func downloadFromS3AndSave(url string, filename string) string {
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
	resp, err := http.Get(url)
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

func streamHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-type", "audio/mpeg")

	var audioFiles []string
	r.ParseForm()

	mission, err := strconv.Atoi(r.Form["mission"][0])
	check(err)
	var channels []int
	for n := range r.Form["channel"] {
		ch, err := strconv.Atoi(r.Form["channel"][n])
		check(err)
		channels = append(channels, ch)
	}
	
	format := r.Form["format"][0]
	start, err := strconv.Atoi(r.Form["t"][0])
	check(err)
	duration, err := strconv.Atoi(r.Form["len"][0])
	check(err)

	// make audioFiles a bunch of paths to wav files of individ channels
	for n := range channels {
		audioFiles = append(audioFiles, getChannelPath(mission, channels[n], start, start+duration))
	}
	


	// mmmmmmagic
	sox, err := exec.LookPath("sox")
	check(err)
	fmt.Println("using sox " + sox)
	ffmpeg, err := exec.LookPath("ffmpeg")
	check(err)
	fmt.Println("using ffmpeg " + ffmpeg)

	// merge channels
	soxArgs := []string{"-t", "wav", "-m"}
	soxArgs = append(soxArgs, audioFiles...)
	soxArgs = append(soxArgs, "-p")
	soxCommand := exec.Command(sox, soxArgs...)

	// convert the thing
	var ffmpegArgs []string
	if format == AAC || format == M4A {
		ffmpegArgs = []string{"-i", "-", "-c:a", "libfdk_aac", "-b:a", "256k", "-f", M4A, "pipe:"}
		// works, but gotta compile ffmpeg on server with special options
	} else if format == OGG {
		ffmpegArgs = []string{"-i", "-", "-c:a", "libvorbis", "-qscale:a", "6", "-f", OGG, "pipe:"}
	} else {
		fmt.Println("unsupported output format requested")
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
