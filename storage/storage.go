package storage

import (
	"io"
	"zenhack.net/go/irc-idler/irc"
)

var (
	EmptyCursor LogCursor = emptyCursor{}
)

type Store interface {
	GetChannel(name string) (ChannelLog, error)
}

type ChannelLog interface {
	LogMessage(msg *irc.Message) error
	Replay() (LogCursor, error)
	Clear() error
}

type LogCursor interface {
	Get() (*irc.Message, error)
	Next()
	Close() error
}

type emptyCursor struct{}

func (c emptyCursor) Get() (*irc.Message, error) {
	return nil, io.EOF
}

func (c emptyCursor) Next() {
}

func (c emptyCursor) Close() error {
	return nil
}
