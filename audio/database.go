package audio

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	_ "github.com/lib/pq"
)

type DatabaseVars struct {
	DB_HOST     string `json:"DB_HOST"`
	DB_PORT     int    `json:"DB_PORT"`
	DB_USER     string `json:"DB_USER"`
	DB_PASSWORD string `json:"DB_PASSWORD"`
	DB_NAME     string `json:"DB_NAME"`
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
