package state

import (
	"zenhack.net/go/irc-idler/irc"
)

// Information about the state of the connection. Note that we store one of these
// indepentently for both client and server; their views may not always align.
type Session struct {
	irc.ClientID

	Handshake

	channels map[string]*ChannelState // State of channels we're in.
}

// Return a newly initialized session
func NewSession() *Session {
	return &Session{
		channels: make(map[string]*ChannelState),
	}
}

// Return true if the prefix identifies the user associated with this session,
// false otherwise.
func (s *Session) IsMe(prefix string) bool {
	clientID, err := irc.ParseClientID(prefix)
	if err != nil {
		// TODO: debug logging here. Control flow makes it a bit hard.
		// We should probably start using contexts.
		return false
	}
	return clientID.Nick == s.ClientID.Nick
}

// Return true if we're in the channel `channelName`, false otherwise.
func (s *Session) HaveChannel(channelName string) bool {
	_, ok := s.channels[channelName]
	return ok
}

// Get the state for channel `channelName`. If we're not already marked as in
// the channel, this adds the channel to our list and returns a fresh state.
func (s *Session) GetChannel(channelName string) *ChannelState {
	if !s.HaveChannel(channelName) {
		s.channels[channelName] = &ChannelState{
			Topic:        "",
			InitialUsers: make(map[string]bool),
		}
	}
	return s.channels[channelName]
}

func (s *Session) UpdateFromClient(msg *irc.Message) {
	s.Handshake.UpdateFromClient(msg)
}

func (s *Session) UpdateFromServer(msg *irc.Message) {
	s.Handshake.UpdateFromServer(msg)

	if s.IsMe(msg.Prefix) {
		// The message is about us:
		switch msg.Command {
		case "KICK", "PART":
			// we left a channel
			delete(s.channels, msg.Params[0])
		case "JOIN":
			// we entered a channel
			s.GetChannel(msg.Params[0]).UpdateFromServer(msg)
		case "NICK":
			// we changed our nick
			s.ClientID.Nick = msg.Params[0]
		}
	} else {
		switch msg.Command {
		case "KICK", "PART", "JOIN", "QUIT":
			// Some other user's state in a channel changed.
			s.GetChannel(msg.Params[0]).UpdateFromServer(msg)
		}
	}
}
