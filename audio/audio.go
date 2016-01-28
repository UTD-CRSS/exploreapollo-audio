package audio

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
)

var workingDir string = path.Join(os.TempDir(), "apollo-audio")
var clipDir string = path.Join(workingDir, "clips")
var AAC string = "aac"
var M4A string = "m4a"
var OGG string = "ogg"

/* container for the request variables */
type RequestVars struct {
	Mission  int
	Channels []string
	Format   string
	Start    int
	Duration int
}

/* local paths to audio files for all channels belonging to particular time slice */
type TimeSlice struct {
	start  int
	end    int
	chunks map[int]AudioChunk
}

func NewTimeSlice(start, end int) TimeSlice {
	chunks := make(map[int]AudioChunk)
	return TimeSlice{start, end, chunks}
}

type AudioChunk struct {
	start     int
	end       int
	url       string
	localPath string
	channel   int
}

func InitDirs() {
	makeDir(workingDir)
	makeDir(clipDir)
}

func GetRequestSlices(rv RequestVars) []TimeSlice {
	var slices []TimeSlice
	// Get db
	db := connectDb()
	defer db.Close()

	// Psql array for ANY query
	channelString := fmt.Sprintf("{%s}", strings.Join(rv.Channels, ","))
	reqEnd := rv.Start + rv.Duration

	// Query
	stmt, err := db.Prepare("SELECT met_start, met_end, url, channel FROM channel_chunks WHERE channel = ANY($1::integer[]) AND met_end > $2 AND met_start < $3 ORDER BY met_start")
	check(err)
	rows, err := stmt.Query(channelString, rv.Start, reqEnd)
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
		loc := fmt.Sprintf("mission_%d_channel_%d_%d.wav", rv.Mission, chunk.channel, chunk.start)
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
	if i == 0 && rv.Start > slice.start {
		trimOffset = rv.Start - slice.start
		offset := float64(trimOffset) / 1000.0
		offStr := strconv.FormatFloat(offset, 'f', 4, 64)
		log.Println("Trimming first slice by", offStr)
		args = append(args, "trim", offStr)
	}
	reqEnd := rv.Start + rv.Duration
	if i == len(slices)-1 && slice.end > reqEnd {
		var duration int
		if rv.Start > slice.start {
			duration = reqEnd - rv.Start
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

func DownloadAndStream(slices []TimeSlice, rv RequestVars, w io.Writer) {
	// Download all here. Could be done concurrently while prev slice is streaming
	DownloadAllAudio(slices)

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
		if rv.Format == AAC || rv.Format == M4A {
			ffmpegArgs = []string{"-i", "-", "-c:a", "libfdk_aac", "-b:a", "256k", "-f", M4A, "pipe:"}
			// works, but gotta compile ffmpeg on server with special options
		} else if rv.Format == OGG {
			ffmpegArgs = []string{"-i", "-", "-c:a", "libvorbis", "-qscale:a", "6", "-f", OGG, "pipe:"}
		} else {
			log.Println("unsupported output format requested. break some rools.")
			ffmpegArgs = []string{"-i", "-", "-f", "mp3", "-ab", "256k", "pipe:"}
		}
		log.Println("running ffmpeg", strings.Join(ffmpegArgs, " "))
		ffmpegCommand := exec.Command(ffmpeg, ffmpegArgs...)

		ffmpegCommand.Stdin, _ = soxCommand.StdoutPipe()
		ffmpegCommand.Stdout = w
		ffmpegCommand.Stderr = os.Stdout

		ffmpegCommand.Start()
		soxCommand.Run()
		ffmpegCommand.Wait()
	}
}
