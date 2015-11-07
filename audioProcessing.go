package main

import (
	"database/sql"
	_ "github.com/lib/pq"
)

const (
	DB_USER = "postgres"
	DB_PASSWORD = "postgres"
	DB_NAME = "test"
)

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

func getChannelPath(rmission int, rchan int, rstart int, rend int) []string {

	dbinfo := fmt.Sprintf("user=%s password=%s dbname=%s sslmode=disable", DB_USER, DB_PASSWORD, DB_NAME)
	db, err := sql.Open("postgres", dbinfo)
	check(err)
	defer db.Close()

	var queries []string
	var res Result
	// get channel ids of channels that belong to the requested mission rmission
	queries := append(queries, "CREATE TABLE mission_chans AS SELECT c.id FROM channels c WHERE c.mission IN (SELECT m.id FROM missions m WHERE m.name=$1")
	// get channel chunk ids of chunks on the requested channel rchan
	queries = append(queries, "CREATE TABLE chan AS SELECT mc.id FROM mission_chans mc WHERE mc.name=$1")
	// get chunk info corresponding to aforementioned chunk ids
	queries = append(queries, "CREATE TABLE chunks_a AS SELECT * FROM channel_chunks cc WHERE cc.channel in chan.id")
	// gets all chunks that occur after the requested start time rstart
	queries = append(queries, "CREATE TABLE chunks_b AS SELECT * FROM chunks_a WHERE end_met > $1")
	// gets chunks from previous selection that begin before the
	// requested end time, and bob's your uncle rend
	queries = append(queries, "CREATE TABLE chunks AS SELECT * FROM chunks_b WHERE met_start < $1")
	args := [rmission, rchan, nil, rstart, rend]

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
