package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"

	_ "github.com/lib/pq"
)

var workingDir string = path.Join(os.TempDir(), "apollo-audio")
var clipDir string = path.Join(workingDir, "clips")
var AAC string = "aac"
var M4A string = "m4a"
var OGG string = "ogg"

/* container for the request variables */
type RequestVars struct {
	mission  int
	channels []string
	format   string
	start    int
	duration int
}

/* local paths to audio files for all channels belonging to particular time slice */
type TimeSlice struct {
	start    int
	end      int
	location []string
}

type AudioChunk struct {
	start     int
	end       int
	url       string
	localPath string
	channel   int
}

type DatabaseVars struct {
	DB_HOST     string `json:"DB_HOST"`
	DB_PORT     int    `json:"DB_PORT"`
	DB_USER     string `json:"DB_USER"`
	DB_PASSWORD string `json:"DB_PASSWORD"`
	DB_NAME     string `json:"DB_NAME"`
}

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

func downloadAllAudio(chunkMap map[int][]AudioChunk) {
	for _, a := range chunkMap {
		for _, b := range a {
			downloadFromS3AndSave(b.url, b.localPath)
		}
	}
}

func downloadFromS3AndSave(url string, filename string) string {
	log.Println("Downloading", url, "to", filename)
	clipPath := filename
	if _, err := os.Stat(clipPath); err == nil {
		log.Println("file exists; skipping")
		return clipPath
	}

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

func parseParameters(r *http.Request) (RequestVars, error) {
	var rv RequestVars

	r.ParseForm()

	log.Println(r.Form)

	// Handle empty request
	if len(r.Form) == 0 {
		log.Println("Not enough request params")
		return rv, errors.New("bad request")
	}

	missionId := r.PostFormValue("mission")
	tempMission, err := strconv.Atoi(missionId)
	if err != nil {
		log.Println("invalid mission id:", missionId)
		return rv, err
	}
	rv.mission = tempMission

	rv.channels = r.Form["channels"]

	tempFormat := r.PostFormValue("format")
	rv.format = tempFormat

	tempStart, err := strconv.Atoi(r.PostFormValue("start"))
	check(err)
	rv.start = tempStart

	tempDuration, err := strconv.Atoi(r.PostFormValue("duration"))
	check(err)
	rv.duration = tempDuration

	return rv, nil
}

func getLocations(rv RequestVars) map[int][]AudioChunk {

	//var slices []TimeSlice
	chunkMap := make(map[int][]AudioChunk)

	dbjson, err := ioutil.ReadFile("./config.json")
	check(err)

	var dbvars DatabaseVars
	err = json.Unmarshal(dbjson, &dbvars)
	check(err)

	dbcreds := dbvars
	dbinfo := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", dbcreds.DB_HOST, dbcreds.DB_PORT, dbcreds.DB_USER, dbcreds.DB_PASSWORD, dbcreds.DB_NAME)
	db, err := sql.Open("postgres", dbinfo)
	check(err)
	defer db.Close()

	//reqEnd := rv.start + rv.duration
	channelString := fmt.Sprintf("{%s}", strings.Join(rv.channels, ","))

	stmt, err := db.Prepare("SELECT met_start, met_end, url, channel FROM channel_chunks WHERE channel = ANY($1::integer[])") // WHERE met_end > $1 AND met_start < $2")
	check(err)

	rows, err := stmt.Query(channelString) //rv.start, reqEnd)
	check(err)

	defer rows.Close()

	for rows.Next() {
		var chunk AudioChunk
		err = rows.Scan(&chunk.start, &chunk.end, &chunk.url, &chunk.channel)
		if err == nil {
			//log.Println(chunk)
			// Set local name
			loc := fmt.Sprintf("mission_%d_channel_%d_%d.wav", rv.mission, chunk.channel, chunk.start)
			chunk.localPath = path.Join(clipDir, loc)
			chunkMap[chunk.channel] = append(chunkMap[chunk.channel], chunk)
		}
	}
	//log.Println(chunkMap)
	return chunkMap
}

func streamHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-type", "audio/mpeg")

	/* PARRRAMETERS */
	rv, err := parseParameters(r)
	// Handle bad params
	if err != nil {
		log.Println("Error processing params:", err)
		http.Error(w, http.StatusText(500), 500)
		return
	}
	// All clear
	log.Println("Handling request for ", rv)

	/* DEEBEE */
	chunkMap := getLocations(rv)
	downloadAllAudio(chunkMap)
	//log.Println(chunkMap)

	sox, err := exec.LookPath("sox")
	check(err)
	log.Println("using sox " + sox)
	ffmpeg, err := exec.LookPath("ffmpeg")
	check(err)
	log.Println("using ffmpeg " + ffmpeg)

	var chunkPaths []string
	// concat chunks of the same channel
	for _, a := range chunkMap {
		// Grab first one for now
		chunkPaths = append(chunkPaths, a[0].localPath)
	}

	// Merge the channels
	soxArgs := []string{"-t", "wav"}
	// Only merge if there are multiple files
	if len(chunkPaths) > 1 {
		soxArgs = append(soxArgs, "-m")
	}
	soxArgs = append(soxArgs, chunkPaths...)
	soxArgs = append(soxArgs, "-p")
	log.Println("running sox", strings.Join(soxArgs, " "))
	soxCommand := exec.Command(sox, soxArgs...)

	// Transcode the result
	var ffmpegArgs []string
	if rv.format == AAC || rv.format == M4A {
		ffmpegArgs = []string{"-i", "-", "-c:a", "libfdk_aac", "-b:a", "256k", "-f", M4A, "pipe:"}
		// works, but gotta compile ffmpeg on server with special options
	} else if rv.format == OGG {
		ffmpegArgs = []string{"-i", "-", "-c:a", "libvorbis", "-qscale:a", "6", "-f", OGG, "pipe:"}
	} else {
		log.Println("unsupported output format requested. break some rools.")
		ffmpegArgs = []string{"-i", "-", "-f", "mp3", "-ab", "256k", "pipe:"}
	}
	log.Println("running ffmpeg", strings.Join(ffmpegArgs, " "))
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

	log.Println("done")
}

func main() {
	makeDir(workingDir)
	makeDir(clipDir)
	http.HandleFunc("/stream", streamHandler)
	ServerPort := "5000" // default port
	if len(os.Getenv("PORT")) > 0 {
		ServerPort = os.Getenv("PORT")
	}
	log.Println("Starting server on " + ServerPort)
	http.ListenAndServe(":"+ServerPort, nil)
}
