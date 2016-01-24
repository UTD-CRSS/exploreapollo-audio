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
	"sync"

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
	start  int
	end    int
	chunks map[int]AudioChunk
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

func NewTimeSlice(start, end int) TimeSlice {
	chunks := make(map[int]AudioChunk)
	return TimeSlice{start, end, chunks}
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

func downloadAllAudio(timeSlices []TimeSlice) {
	// Parallel download
	var wg sync.WaitGroup
	itemQ := make(chan AudioChunk)
	workerCount := 2
	// launch workers
	for a := 0; a < workerCount; a++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range itemQ {
				downloadFromS3AndSave(item.url, item.localPath)
			}
		}()
	}
	// Add items
	for _, a := range timeSlices {
		for _, b := range a.chunks {
			itemQ <- b
		}
	}
	close(itemQ)
	wg.Wait()
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

func connectDb() *sql.DB {
	// Use env default
	dbStr := os.Getenv("DATABASE_URL")
	// Read config if no url
	if len(dbStr) == 0 {
		var dbvars DatabaseVars
		log.Println("Loading db config file")
		dbjson, err := ioutil.ReadFile("./config.json")
		check(err)
		err = json.Unmarshal(dbjson, &dbvars)
		check(err)
		dbStr = fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", dbvars.DB_HOST, dbvars.DB_PORT, dbvars.DB_USER, dbvars.DB_PASSWORD, dbvars.DB_NAME)
	}
	// Connect
	db, err := sql.Open("postgres", dbStr)
	check(err)
	return db
}

func getRequestSlices(rv RequestVars) []TimeSlice {
	var slices []TimeSlice
	// Get db
	db := connectDb()
	defer db.Close()

	// Psql array for ANY query
	channelString := fmt.Sprintf("{%s}", strings.Join(rv.channels, ","))
	reqEnd := rv.start + rv.duration

	// Query
	stmt, err := db.Prepare("SELECT met_start, met_end, url, channel FROM channel_chunks WHERE channel = ANY($1::integer[]) AND met_end > $2 AND met_start < $3 ORDER BY met_start")
	check(err)
	rows, err := stmt.Query(channelString, rv.start, reqEnd)
	check(err)
	defer rows.Close()

	//Scan results and build slices
	lastStart := -1
	for rows.Next() {
		var chunk AudioChunk
		err = rows.Scan(&chunk.start, &chunk.end, &chunk.url, &chunk.channel)
		if err != nil {
			log.Println("Error reading row", err)
			continue
		}
		//Check for new timeslice
		if chunk.start > lastStart {
			slices = append(slices, NewTimeSlice(chunk.start, chunk.end))
			lastStart = chunk.start
		}
		// Set local name
		loc := fmt.Sprintf("mission_%d_channel_%d_%d.wav", rv.mission, chunk.channel, chunk.start)
		chunk.localPath = path.Join(clipDir, loc)

		// Add to proper slice
		for i, a := range slices {
			if chunk.start == a.start && chunk.end == a.end {
				slices[i].chunks[chunk.channel] = chunk
				break
			}
		}
	}
	return slices
}

func getSoxTrimArgs(i int, rv RequestVars, slices []TimeSlice) (args []string) {
	slice := slices[i]
	trimOffset := 0
	if i == 0 && rv.start > slice.start {
		trimOffset = rv.start - slice.start
		offset := float64(trimOffset) / 1000.0
		offStr := strconv.FormatFloat(offset, 'f', 4, 64)
		log.Println("Trimming first slice by", offStr)
		args = append(args, "trim", offStr)
	}
	reqEnd := rv.start + rv.duration
	if i == len(slices)-1 && slice.end > reqEnd {
		var duration int
		if rv.start > slice.start {
			duration = reqEnd - rv.start
		} else {
			duration = reqEnd - slice.start
		}
		df := float64(duration) / 1000.0
		durStr := strconv.FormatFloat(df, 'f', 4, 64)
		log.Println("Trimming last slice by", durStr)

		// Need starting offset if not already set
		if trimOffset == 0 {
			args = append(args, "trim", "0", durStr)
		} else {
			args = append(args, durStr)
		}
	}
	return args
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
	slices := getRequestSlices(rv)

	// Check for no audio
	if len(slices) == 0 {
		log.Println("No data found for request")
		http.Error(w, http.StatusText(404), 404)
		return
	}
	// Download all here. Could be done concurrently while prev slice is streaming
	downloadAllAudio(slices)

	// Build cmds
	sox, err := exec.LookPath("sox")
	check(err)
	log.Println("using sox " + sox)
	ffmpeg, err := exec.LookPath("ffmpeg")
	check(err)
	log.Println("using ffmpeg " + ffmpeg)

	// Process and stream each chunk
	for i, slice := range slices {
		var chunkPaths []string
		// Gather paths
		for _, ch := range slice.chunks {
			chunkPaths = append(chunkPaths, ch.localPath)
		}
		// Merge the channels
		soxArgs := []string{"-t", "wav"}
		// Only merge if there are multiple files
		if len(chunkPaths) > 1 {
			soxArgs = append(soxArgs, "-m")
		}
		soxArgs = append(soxArgs, chunkPaths...)
		soxArgs = append(soxArgs, "-p")

		// Handle trim cases on start and end
		soxArgs = append(soxArgs, getSoxTrimArgs(i, rv, slices)...)

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
	}

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
