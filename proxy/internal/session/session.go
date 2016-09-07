package session

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

func (s *Session) Update(msg *irc.Message) {
	if s.IsMe(msg.Prefix) {
		// The message is about us:
		switch msg.Command {
		case "KICK", "PART":
			// we left a channel
			delete(s.channels, msg.Params[0])
		case "JOIN":
			// we entered a channel
			s.GetChannel(msg.Params[0]).Update(msg)
		case "NICK":
			// we changed our nick
			s.ClientID.Nick = msg.Params[0]
		}
	} else {
		switch msg.Command {
		case "KICK", "PART", "JOIN", "QUIT":
			// Some other user's state in a channel changed.
			s.GetChannel(msg.Params[0]).Update(msg)
		}
	}
}

// State of the channel
type ChannelState struct {
	Topic string // the topic for the channel, if any.

	// Initial users in the channel. If the client is connected, this is
	// modified as users enter and leave the channel, but if the client
	// is disconnected, this is left unchanged. In this case we save
	// JOIN/PART messages to the log, and update this as we replay them.
	// This is important since it avoids the log e.g. conveying a PRIVMSG
	// for a user who is not in the channel, which might confuse the client.
	// putting these users in RPL_NAMREPLY and then replaying the log
	// should get us to the correct final state.
	InitialUsers map[string]bool
}

func (s *ChannelState) Update(msg *irc.Message) {
	// TODO: report the errors from ParseClientID somehow.
	switch msg.Command {
	case "JOIN":
		clientID, err := irc.ParseClientID(msg.Prefix)
		if err != nil {
			return
		}
		s.InitialUsers[clientID.Nick] = true
	case "PART", "KICK", "QUIT":
		// TODO: we need to specially handle the case were *we* are leaving.
		clientID, err := irc.ParseClientID(msg.Prefix)
		if err != nil {
			return
		}
		delete(s.InitialUsers, clientID.Nick)
	}
}

// State of the initial handshake. The handshake consist of
//
// 1. Client sends NICK and USER messages
// 2. Server does not reject the NICK (if so, client needs to resend)
// 3. Server sends welcome sequence up through the MOTD.
type Handshake struct {
	haveNick, haveUser bool // The client has sent the NICK/USER mesage.

	// The client has received the full MOTD; this is the last thing the
	// server sends as part of the initial welcome sequence.
	haveMOTD bool
}

// Return true if the handshake is complete, false otherwise.
func (h Handshake) Done() bool {
	return h.haveNick && h.haveUser && h.haveMOTD
}

// Return true if the handshake is complete on the client side, but still
// waiting for (some of) the server's welcome sequence.
func (h Handshake) WantsWelcome() bool {
	return h.haveNick && h.haveUser && !h.haveMOTD
}

// Update the state to be consistent with `msg` having just been transferred.
// Note that we don't have to specify whether this is the state for the
// client or server, or whether the message was sent or received, because
// the way the handshake works, these things are unambiguous from the message
// itself.
//
// Note that it is *not* safe to call this regardless of whether the handshake
// is already complete.
func (h *Handshake) Update(msg *irc.Message) {
	switch msg.Command {
	case "USER":
		h.haveUser = true
	case "NICK":
		h.haveNick = true
	case irc.ERR_NONICKNAMEGIVEN, irc.ERR_ERRONEUSNICKNAME, irc.ERR_NICKNAMEINUSE,
		irc.ERR_NICKCOLLISION:
		// Server rejected our NICK message, we'll need to send another before
		// we're done.
		h.haveNick = false
	case irc.RPL_ENDOFMOTD, irc.ERR_NOMOTD:
		h.haveMOTD = true
	}
}
