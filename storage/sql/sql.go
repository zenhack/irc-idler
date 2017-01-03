// Package sql provides an implementation of storage.Store on top of an SQL
// database.
package sql

import (
	"database/sql"
	"io"
	"zenhack.net/go/irc-idler/irc"
	"zenhack.net/go/irc-idler/storage"
)

type store struct {
	db         *sql.DB
	haveSchema bool
}

type channelLog struct {
	db   *sql.DB
	name string
}

type cursor struct {
	rows *sql.Rows
	err  error
	msg  *irc.Message
}

// NewStore returns a new store backed by the database `db`.
func NewStore(db *sql.DB) storage.Store {
	return &store{db: db}
}

func (s *store) GetChannel(name string) (storage.ChannelLog, error) {
	if !s.haveSchema {
		_, err := s.db.Exec(
			`CREATE TABLE IF NOT EXISTS messages (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				channel VARCHAR(512) NOT NULL,
				message VARCHAR(512) NOT NULL
			)`,
		)
		if err != nil {
			return nil, err
		}
		s.haveSchema = true
	}
	return &channelLog{
		db:   s.db,
		name: name,
	}, nil
}

func (l *channelLog) LogMessage(msg *irc.Message) error {
	_, err := l.db.Exec(
		"INSERT INTO messages(channel, message) VALUES (?, ?)",
		l.name, msg.String(),
	)
	return err
}

func (l *channelLog) Replay() (storage.LogCursor, error) {
	rows, err := l.db.Query(
		"SELECT message FROM messages WHERE channel = ? ORDER BY id",
		l.name)
	if err != nil {
		return nil, err
	}
	ret := &cursor{
		rows: rows,
		err:  nil,
		msg:  nil,
	}
	ret.Next()
	return ret, nil
}

func (l *channelLog) Clear() error {
	_, err := l.db.Exec("DELETE FROM messages WHERE channel = ?", l.name)
	return err
}

func (c *cursor) Get() (*irc.Message, error) {
	return c.msg, c.err
}

func (c *cursor) Next() {
	if !c.rows.Next() {
		c.err = c.rows.Err()
		if c.err == nil {
			c.err = io.EOF
		}
		c.Close()
		return
	}
	var str string
	c.err = c.rows.Scan(&str)
	if c.err != nil {
		c.Close()
		return
	}
	c.msg, c.err = irc.ParseMessage(str)
}

func (c *cursor) Close() error {
	return c.rows.Close()
}
