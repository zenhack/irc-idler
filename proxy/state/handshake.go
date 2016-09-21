package state

import (
	"zenhack.net/go/irc-idler/irc"
)

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

func (h *Handshake) UpdateFromClient(msg *irc.Message) {
	if h.Done() {
		return
	}
	switch msg.Command {
	case "USER":
		h.haveUser = true
	case "NICK":
		h.haveNick = true
	}
}

func (h *Handshake) UpdateFromServer(msg *irc.Message) {
	if h.Done() {
		return
	}
	switch msg.Command {
	case irc.ERR_NONICKNAMEGIVEN, irc.ERR_ERRONEUSNICKNAME, irc.ERR_NICKNAMEINUSE,
		irc.ERR_NICKCOLLISION:
		// Server rejected our NICK message, we'll need to send another before
		// we're done.
		h.haveNick = false
	case irc.RPL_ENDOFMOTD, irc.ERR_NOMOTD:
		h.haveMOTD = true
	}
}
