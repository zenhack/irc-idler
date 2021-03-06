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
	closeLastDB := func() {
		// Close the db from the previous test. We want a fresh db every time, so we
		// re-create it during the test. We can't easily close it afterwards, since
		// RandTest calls Fatal, so we close the previous test's DB, and then get the
		// last one in a top-level defer.
		if db != nil {
			db.Close()
		}
	}
	defer closeLastDB()

	stest.RandTest(t, func() storage.Store {
		closeLastDB()

		var err error
		db, err = sql.Open("sqlite3", ":memory:")
		if err != nil {
			panic(err)
		}
		return NewStore(db)
	})
}
