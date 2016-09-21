package state

import (
	"zenhack.net/go/irc-idler/irc"
)

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

func (s *ChannelState) UpdateFromClient(msg *irc.Message) {

}

func (s *ChannelState) UpdateFromServer(msg *irc.Message) {
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
