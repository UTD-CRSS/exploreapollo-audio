package main

import {
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strconv"
	"database/sql"
	_ "github.com/lib/pq"
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

type RequestVars struct {
	mission int
	channels []int
	format string
	start int
	duration int
}

type TimeSlice struct {
	ch int
	start int
	end int
	urls []string
}

func parseParameters(r *http.Request) *RequestVars {
	rv = new(RequestVars)
	r.ParseForm()

	rv.mission, err := strconv.Atoi(r.Form["mission"][0])
	check(err)
	for n in range r.Form["channel"] {
		ch, err := strconv.Atoi(r.Form["channel"][n])
		check(err)
		rv.channels = append(channels, ch)
	}
	rv.format := r.Form["format"][0]
	rv.start, err := strconv.Atoi(r.Form["t"][0])
	check(err)
	rv.duation, err := strconv.Atoi(r.Form["len"][0])
	check(err)

	return rv
}

func getURLS(rv *RequestVars) []TimeSlice {
	for ch in range rv.channels {

	}
}

func streamHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-type", "audio/mpeg")

	/* PARRRAMETERS */
	rv := parseParameters(r)

	/* DEEBEE */



}
