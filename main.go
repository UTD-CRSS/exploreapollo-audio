package main

import (
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/UTD-CRSS/audio.exploreapollo.org/audio"
)

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

func parseParameters(r *http.Request) (rv audio.RequestVars, err error) {
	qs := r.URL.Query()
	log.Println("Got request", qs)

	// Handle empty request
	if len(qs) == 0 {
		log.Println("Not enough request params")
		return rv, errors.New("bad request")
	}

	missionId := qs.Get("mission")
	rv.Mission, err = strconv.Atoi(missionId)
	if err != nil {
		log.Println("invalid mission id:", missionId)
		return rv, err
	}

	rv.Channels = strings.Split(qs.Get("channels"), ",")
	// Validate all channel numbers
	for _, a := range rv.Channels {
		if _, err := strconv.Atoi(a); err != nil {
			log.Println("invalid channel:", a)
			return rv, err
		}
	}

	rv.Format = qs.Get("format")

	stStr := qs.Get("start")
	rv.Start, err = strconv.Atoi(stStr)
	if err != nil {
		log.Println("invalid start:", stStr)
		return rv, err
	}

	durStr := qs.Get("duration")
	rv.Duration, err = strconv.Atoi(durStr)
	if err != nil {
		log.Println("invalid duration:", durStr)
		return rv, err
	}

	return rv, nil
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
	slices := audio.GetRequestSlices(rv)

	// Check for no audio
	if len(slices) == 0 {
		log.Println("No data found for request")
		http.Error(w, http.StatusText(404), 404)
		return
	}

	fw := flushWriter{w: w}
	if f, ok := w.(http.Flusher); ok {
		fw.f = f
	}

	audio.DownloadAndStream(slices, rv, &fw)
	log.Println("done")
}

func main() {
	audio.InitDirs()
	http.HandleFunc("/stream", streamHandler)
	ServerPort := "5000" // default port
	if len(os.Getenv("PORT")) > 0 {
		ServerPort = os.Getenv("PORT")
	}
	log.Println("Starting server on " + ServerPort)
	http.ListenAndServe(":"+ServerPort, nil)
}
