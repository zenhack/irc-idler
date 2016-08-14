// Package storage defines the interface to the message log.
//
// irc-idler must store messages when the user is disconnected; this package
// defines the interfaces required for a storage backend. It does not itself
// provide impelmentations of Stores (but see EmptyCursor). Various backends
// can be found in the subdirectories of this package.
//
// No requirements for thread safety are imposed on implementations; Clients
// of these interfaces must handle synchronization themselves.
//
// In general, if error is non-nil then any other return values may be nil.
package storage

import (
	"io"
	"zenhack.net/go/irc-idler/irc"
)

var (
	// An "empty" cursor, whose Get() method always returns (nil, io.EOF).
	// Its Close() returns nil and does nothing.
	EmptyCursor LogCursor = emptyCursor{}
)

// A data store for logged messages
type Store interface {
	// Get a ChannelLog for the named channel
	GetChannel(name string) (ChannelLog, error)
}

// A (sequential) log for a particular channel.
type ChannelLog interface {

	// Append a message to the end of log.
	LogMessage(msg *irc.Message) error

	// Replay the log. Returns a cursor pointing at the first message in the log
	Replay() (LogCursor, error)

	// Delete all of the messages in the log
	Clear() error
}

// A cursor into the log.
type LogCursor interface {
	// Get the current message pointed to by the cursor. If the cursor is past the end of the
	// log, returns (nil, io.EOF).
	Get() (*irc.Message, error)

	// Advance the cursor to the next message in the log.
	Next()

	// Destroy the cursor and clean up any associated resources.
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
