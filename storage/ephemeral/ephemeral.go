// Package ephemeral defines a storage.Store implementation that stores
// messages in an in-memory data structure.
package ephemeral

import (
	"io"
	"zenhack.net/go/irc-idler/irc"
	"zenhack.net/go/irc-idler/storage"
)

type store struct {
	// Our store implementation closely mirrors the interface: we have a map
	// mapping chnanel names to slices of messages. If there are no messages
	// for a given channel that entry in the map will be nil.
	channels map[string][]*irc.Message
}

type channelLog struct {
	// Pointer to the store. We can't just store the slice itself, since
	// it may be nil, and if the user calls Clear(), we need to *set it*
	// to nil.
	store *store

	// name of the channel
	name string
}

type cursor struct {
	// messages in the log
	msgs []*irc.Message

	// index into msgs
	i int
}

// Return a new memory-backed Store.
func NewStore() storage.Store {
	return &store{channels: make(map[string][]*irc.Message)}
}

func (s *store) GetChannel(name string) (storage.ChannelLog, error) {
	return &channelLog{s, name}, nil
}

func (l *channelLog) LogMessage(msg *irc.Message) error {
	if l.store.channels[l.name] == nil {
		l.store.channels[l.name] = []*irc.Message{msg}
	} else {
		l.store.channels[l.name] = append(l.store.channels[l.name], msg)
	}
	return nil
}

func (l *channelLog) Clear() error {
	delete(l.store.channels, l.name)
	return nil
}

func (l *channelLog) Replay() (storage.LogCursor, error) {
	if l.store.channels[l.name] == nil {
		return storage.EmptyCursor, nil
	} else {
		return &cursor{l.store.channels[l.name], 0}, nil
	}
}

func (c *cursor) Next() {
	c.i++
}

func (c *cursor) Get() (*irc.Message, error) {
	if c.i >= len(c.msgs) {
		return nil, io.EOF
	} else {
		return c.msgs[c.i], nil
	}
}

func (c *cursor) Close() error {
	return nil
}
