package audio

import (
	"io"
	"log"
	"net/http"
	"os"
	"sync"
)

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

func DownloadAllAudio(timeSlices []TimeSlice) {
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
				downloadUrlAndSave(item.url, item.localPath)
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

func downloadUrlAndSave(url string, filename string) string {
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
	log.Println("Saved", filename)
	return clipPath
}
