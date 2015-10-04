package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
)

var workingDir string = path.Join(os.TempDir(), "apollo-audio")
var clipDir string = path.Join(workingDir, "clips")

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func makeDir(dir string) {
	src, err := os.Stat(dir)
	check(err)
	if !src.IsDir() {
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
	resp, err := http.Get("http://austinpray.s3.amazonaws.com/static/apolloclips/" + filename)
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
	ffmpeg, err := exec.LookPath("ffmpeg")
	check(err)
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
	ffmpegCommand.Start()
	soxCommand.Run()
	ffmpegCommand.Wait()
}

func main() {
	makeDir(workingDir)
	makeDir(clipDir)
	http.HandleFunc("/stream.mp3", handler)
	http.ListenAndServe(":8080", nil)
}
