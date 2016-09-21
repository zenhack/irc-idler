package state

import (
	"strings"
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
	case irc.RPL_NAMEREPLY:
		// TODO: store this in the state:
		// mode := msg.Params[1]
		users := strings.Split(msg.Params[3], " ")

		for _, user := range users {

			user = strings.Trim(user, " \r\n")
			// XXX: we're accepting full clientIDs + flag here,
			// but only nick + flag is legal.
			clientID, err := irc.ParseClientID(user)
			var nick string
			if err != nil {
				// TODO: report this. our API doesn't give us a clear
				// way to complain, so we just leave it unparsed,
				// which is... wrong. nick = user
			} else {
				nick = clientID.Nick
			}

			s.InitialUsers[nick] = true
		}
	}
}
