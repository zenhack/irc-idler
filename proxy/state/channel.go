package state

import (
	"strings"
	"zenhack.net/go/irc-idler/irc"
)

// State of the channel
type ChannelState struct {
	Topic string // the topic for the channel, if any.

	// Users in the channel. If the client is connected, this is
	// modified as users enter and leave the channel, but if the client
	// is disconnected, this is left unchanged. In this case we save
	// JOIN/PART messages to the log, and update this as we replay them.
	// This is important since it avoids the log e.g. conveying a PRIVMSG
	// for a user who is not in the channel, which might confuse the client.
	// putting these users in RPL_NAMREPLY and then replaying the log
	// should get us to the correct final state.
	users map[string]bool
}

func NewChannelState(topic string) *ChannelState {
	return &ChannelState{
		Topic: topic,
		users: make(map[string]bool),
	}
}

func (s *ChannelState) UpdateFromClient(msg *irc.Message) {

}

// Return a slice of nicks for users in the channel
func (s *ChannelState) Users() []string {
	ret := make([]string, 0, len(s.users))
	for nick, _ := range s.users {
		ret = append(ret, nick)
	}
	return ret
}

func (s *ChannelState) AddUser(nick string) {
	s.users[nick] = true
}

func (s *ChannelState) RemoveUser(nick string) {
	delete(s.users, nick)
}

func (s *ChannelState) HaveUser(nick string) bool {
	return s.users[nick]
}

func (s *ChannelState) UpdateFromServer(msg *irc.Message) {
	// TODO: report the errors from ParseClientID somehow.
	switch msg.Command {
	case "JOIN":
		clientID, err := irc.ParseClientID(msg.Prefix)
		if err != nil {
			return
		}
		s.AddUser(clientID.Nick)
	case "PART", "KICK", "QUIT":
		// TODO: we need to specially handle the case were *we* are leaving.
		clientID, err := irc.ParseClientID(msg.Prefix)
		if err != nil {
			return
		}
		s.RemoveUser(clientID.Nick)
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

			s.AddUser(nick)
		}
	case "NICK":
		newNick := msg.Params[0]
		clientID, err := irc.ParseClientID(msg.Prefix)
		if err != nil {
			return
		}
		if s.HaveUser(clientID.Nick) {
			s.RemoveUser(clientID.Nick)
			s.AddUser(newNick)
		}
	}
}
