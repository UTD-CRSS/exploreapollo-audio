package main

import (
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
		stitched := getChannelPath(mission, channels[n], start, start+duration)
		audioFiles = append(audioFiles, stitched)
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


const (
	DB_USER = "user"
	DB_PASSWORD = "password"
	DB_NAME = "postgres"
)

func getChannelPath(rmission int, rchan int, rstart int, rend int) []string {

	dbinfo := fmt.Sprintf("user=%s password=%s dbname=%s sslmode=disable", DB_USER, DB_PASSWORD, DB_NAME)
	db, err := sql.Open("postgres", dbinfo)
	check(err)
	defer db.Close()

	var queries []string
	var res Result
	// get channel ids of channels that belong to the requested mission rmission
	queries = append(queries, "CREATE TABLE mission_chans AS SELECT c.id FROM channels c WHERE c.mission IN (SELECT m.id FROM missions m WHERE m.name=$1")
	// get channel chunk ids of chunks on the requested channel rchan
	queries = append(queries, "CREATE TABLE chan AS SELECT mc.id FROM mission_chans mc WHERE mc.name=$1")
	// get chunk info corresponding to aforementioned chunk ids
	queries = append(queries, "CREATE TABLE chunks_a AS SELECT * FROM channel_chunks cc WHERE cc.channel in chan.id")
	// gets all chunks that occur after the requested start time rstart
	queries = append(queries, "CREATE TABLE chunks_b AS SELECT * FROM chunks_a WHERE end_met > $1")
	// gets chunks from previous selection that begin before the
	// requested end time, and bob's your uncle rend
	queries = append(queries, "CREATE TABLE chunks AS SELECT * FROM chunks_b WHERE met_start < $1")
	args := [rmission, rchan, "", rstart, rend]

	for n := range queries {
		res, err := queries[n].Exec(args[n])
		check(err)
	}

	// cleanup
	var cleanup []string
	cleanup = append(cleanup, "DROP TABLE mission_chans")
	cleanup = append(cleanup, "DROP TABLE chan")
	cleanup = append(cleanup, "DROP TABLE chunks_a")
	cleanup = append(cleanup, "DROP TABLE chunks_b")

	for n := range cleanup {
		rows, err := db.Query(cleanup[n])
		check(err)
	}

	rows, err = db.Query("SELECT * FROM chunks")
	check(err)

	var chunkIds []string
	var urls []string
	var startTimes []int
	var endTimes []int
	
	for rows.Next() {
		var cid int
		var chanid int
		var url string
		var name string
		var start int
		var end int
		err = rows.Scan(&cid, &chanid, &url, &name, &start, &end)
		check(err)
		chunkIds = append(chunkIds, cid)
		urls = append(urls, url)
		startTimes = append(startTimes, start)
		endTimes = append(endTimes, end)
	}
	
	return stitch(rmission, rchan, chunkIds, urls, startTimes, endTimes)
	
}

func stitch(missionNumber int, channelNumber int, urls []string, met_starts []int, met_ends []int) string {
	output := fmt.Sprintf("mission%dchannel%d%d%d.wav", missionNumber, channelNumber, met_starts[0], met_ends[met_ends.length])
	outPath := path.Join(clipDir, output)
	var files []string
	if len(urls) == 0 {
		panic("No corresponding file(s) found. Something's wrong.")
	} else if len(urls) == 1 {
		return urls[0]
	} else {
		for n := range(urls) {
			filename := fmt.Sprintf("mission%dchannel%d%d%d", missionNumber, channelNumber, met_starts[n], met_ends[n])
			files = append(files, downloadFromS3AndSave(url))
		}
	}
	sox, err := exec.LookPath("sox")
	check(err)
	fmt.Println("using sox " + sox)
	soxArgs := []string{"-t", "wav", "--combine", "concatenate"}
	soxArgs = append(soxArgs, urls...)
	soxArgs = append(soxArgs, output)
	soxCommand := exec.Command(sox, soxArgs...)

	soxCommand.Run()

	return output
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
