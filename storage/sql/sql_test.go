package sql

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	"testing"
	"zenhack.net/go/irc-idler/storage"
	stest "zenhack.net/go/irc-idler/storage/testing"
)

func TestStore(t *testing.T) {
	var db *sql.DB
	stest.RandTest(t, func() storage.Store {
		if db != nil {
			// Close the db from last time. We'll leak one at the end, but this will
			// keep the leaks to a minimum. TODO: would be nice to have a less-hacky
			// way of cleaning up.
			db.Close()
		}
		var err error
		db, err = sql.Open("sqlite3", ":memory:")
		if err != nil {
			panic(err)
		}
		return NewStore(db)
	})
}
