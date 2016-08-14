package sql

import (
	"database/sql"
	"zenhack.net/go/irc-idler/irc"
	"zenhack.net/go/irc-idler/storage"
)

type store struct {
	db *sql.DB
}

type channelLog struct {
	db   *sql.DB
	name string
}

type cursor struct {
	rows sql.Rows
}

func NewStore(db *sql.DB) storage.Store {
	return &store{db}
}

func (s *store) GetChannel(name string) (storage.ChannelLog, error) {
	return &channelLog{
		db:   s.db,
		name: name,
	}, nil
}

func (l *channelLog) LogMessage(msg *irc.Message) error {
	panic("not implemented!")
}

func (l *channelLog) Replay() (storage.LogCursor, error) {
	panic("not implemented!")
}

func (l *channelLog) Clear() error {
	panic("not implemented!")
}

func (c *cursor) Next() {
	panic("not implemented!")
}

func (c *cursor) Close() error {
	return c.rows.Close()
}
