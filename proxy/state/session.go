package state

import (
	"zenhack.net/go/irc-idler/irc"
)

// Information about the state of the connection. Note that we store one of these
// indepentently for both client and server; their views may not always align.
type Session struct {
	irc.ClientID

	Handshake

	channels AllChannelStates
}

// Return a newly initialized session
func NewSession() *Session {
	return &Session{
		channels: &mapChannelStates{make(map[string]*ChannelState)},
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
	return s.channels.HaveChannel(channelName)
}

// Get the state for channel `channelName`. If we're not already marked as in
// the channel, this adds the channel to our list and returns a fresh state.
func (s *Session) GetChannel(channelName string) *ChannelState {
	return s.channels.GetChannel(channelName)
}

func (s *Session) UpdateFromClient(msg *irc.Message) {
	s.Handshake.UpdateFromClient(msg)
}

func (s *Session) UpdateFromServer(msg *irc.Message) {
	s.Handshake.UpdateFromServer(msg)
	s.channels.UpdateFromServer(msg)

	if s.IsMe(msg.Prefix) {
		// The message is about us:
		switch msg.Command {
		case "KICK", "PART":
			// we left a channel
			s.channels.DeleteChannel(msg.Params[0])
		case "NICK":
			// we changed our nick
			s.ClientID.Nick = msg.Params[0]
		}
	}
}
