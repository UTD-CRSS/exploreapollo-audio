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

/* container for the request variables */
type RequestVars struct {
	mission  int
	channels []int
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

type DatabaseVars struct {
	DB_HOST     string `json:"DB_HOST"`
	DB_PORT     int    `json:"DB_PORT"`
	DB_USER     string `json:"DB_USER"`
	DB_PASSWORD string `json:"DB_PASSWORD"`
	DB_NAME     string `json:"DB_NAME"`
}

func parseParameters(r *http.Request) (RequestVars, error) {
	var rv RequestVars

	r.ParseForm()

	// Handle empty request
	if len(r.Form) == 0 {
		log.Println("Received empty request")
		return rv, errors.New("empty request")
	}

	tempMission, err := strconv.Atoi(r.Form["mission"][0])
	check(err)
	rv.mission = tempMission

	var tempChannels []int
	for n := range r.Form["channel"] {
		ch, err := strconv.Atoi(r.Form["channel"][n])
		check(err)
		tempChannels = append(tempChannels, ch)
	}
	rv.channels = tempChannels

	tempFormat := r.Form["format"][0]
	rv.format = tempFormat

	tempStart, err := strconv.Atoi(r.Form["t"][0])
	check(err)
	rv.start = tempStart

	tempDuration, err := strconv.Atoi(r.Form["len"][0])
	check(err)
	rv.duration = tempDuration

	return rv, nil
}

func getLocations(rv RequestVars) []TimeSlice {

	var slices []TimeSlice

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

	stmt, err := db.Prepare("CREATE TABLE chunks_a AS SELECT * FROM channel_chunks WHERE met_end > $1")
	check(err)
	_, err = stmt.Exec(rv.start)
	check(err)

	stmt, err = db.Prepare("CREATE TABLE chunks_b AS SELECT * FROM chunks_a WHERE met_start < $1")
	check(err)
	reqEnd := rv.start + rv.duration
	_, err = stmt.Exec(reqEnd)
	check(err)

	_, err = db.Query("DROP TABLE chunks_a")
	check(err)

	rows, err := db.Query("SELECT DISTINCT met_start, met_end FROM chunks_b ORDER BY met_start")

	for rows.Next() {
		var metstart int
		var metend int
		var loc []string
		err = rows.Scan(&metstart, &metend)
		slice := TimeSlice{start: metstart, end: metend, location: loc}
		slices = append(slices, slice)
	}

	for ch := range rv.channels {
		rows, err := db.Query("SELECT url, met_start, met_end from channel_chunks cb WHERE cb.channel = $1", rv.channels[ch])
		check(err)

		for rows.Next() {
			var url string
			var startTime int
			var endTime int
			err = rows.Scan(&url, &startTime, &endTime)
			fmt.Println(url)
			check(err)

			filename := fmt.Sprintf("mission%dchannel%d%d%d.wav", rv.mission, rv.channels[ch], rv.start, rv.duration)
			loc := downloadFromS3AndSave(url, filename)

			for ts := range slices {
				if startTime == slices[ts].start && endTime == slices[ts].end {
					slices[ts].location = append(slices[ts].location, loc)
				}
			}
		}
	}

	_, err = db.Query("DROP TABLE chunks_b;")
	check(err)
	return slices
}

func streamHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-type", "audio/mpeg")

	/* PARRRAMETERS */
	rv, err := parseParameters(r)
	// Handle bad params
	if err != nil {
		http.Error(w, http.StatusText(500), 500)
		return
	}

	/* DEEBEE */
	timeslices := getLocations(rv)

	sox, err := exec.LookPath("sox")
	check(err)
	fmt.Println("using sox " + sox)
	ffmpeg, err := exec.LookPath("ffmpeg")
	check(err)
	fmt.Println("using ffmpeg " + ffmpeg)

	for s := range timeslices {

		chunkFiles := timeslices[s].location

		soxArgs := []string{"-t", "wav", "-m"}
		soxArgs = append(soxArgs, chunkFiles...)
		soxArgs = append(soxArgs, "-p")
		soxCommand := exec.Command(sox, soxArgs...)

		// convert the thing
		var ffmpegArgs []string
		if rv.format == AAC || rv.format == M4A {
			ffmpegArgs = []string{"-i", "-", "-c:a", "libfdk_aac", "-b:a", "256k", "-f", M4A, "pipe:"}
			// works, but gotta compile ffmpeg on server with special options
		} else if rv.format == OGG {
			ffmpegArgs = []string{"-i", "-", "-c:a", "libvorbis", "-qscale:a", "6", "-f", OGG, "pipe:"}
		} else {
			fmt.Println("unsupported output format requested. break some rools.")
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

	}

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
