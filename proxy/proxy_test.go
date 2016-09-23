package proxy

import (
	"testing"
	"zenhack.net/go/irc-idler/irc"
)

var (
	motd = ExpectMany{
		&ToServer{Command: "MOTD"},
		ForwardS2C(&irc.Message{
			Command: irc.RPL_MOTDSTART,
			Params:  []string{"motd for test server"},
		}),
		ForwardS2C(&irc.Message{
			Command: irc.RPL_MOTD,
			Params:  []string{"Hello, World"},
		}),
		ForwardS2C(&irc.Message{
			Command: irc.RPL_ENDOFMOTD,
			Params:  []string{"End MOTD."},
		}),
	}
)

func initialConnect(nick string) ProxyAction {
	return ExpectMany{
		ClientConnect{},
		ConnectServer{},
		ForwardC2S(&irc.Message{Command: "NICK", Params: []string{nick}}),
		ForwardC2S(&irc.Message{Command: "USER", Params: []string{nick, "0", "*", "Alice"}}),
		ForwardS2C(&irc.Message{
			Command: irc.RPL_WELCOME,
			Params:  []string{nick, "Welcome to a mock irc server alice"},
		}),
		ManyMsg(ForwardS2C, welcomeSequence(nick)),
		motd,
	}
}

// The welcome sequence, omitting the actual RPL_WELCOME at the beginning, since
// that is different between the initial connect and reconnect.
func welcomeSequence(nick string) []*irc.Message {
	return []*irc.Message{
		{
			Command: irc.RPL_YOURHOST,
			Params:  []string{nick, "Your host is testing.example.com"},
		},
		{
			Command: irc.RPL_CREATED,
			Params:  []string{nick, "This server was started now-ish."},
		},
		{
			Command: irc.RPL_MYINFO,
			Params: []string{
				nick,
				"testing.example.com",
				"mock-0.1",
				// TODO: these might actually matter someday:
				"0",
				"0",
			},
		},
	}
}

func reconnect(nick string) ProxyAction {
	return ExpectMany{
		&ClientConnect{},
		&FromClient{Command: "NICK", Params: []string{nick}},
		&FromClient{Command: "USER", Params: []string{nick, "0", "*", "Alice"}},
		&ToClient{
			Command: irc.RPL_WELCOME,
			Params:  []string{nick, "Welcome back to IRC Idler, " + nick},
		},
		ManyToClient(welcomeSequence(nick)),
		motd,
	}
}

func joinSeq(nick string) []*irc.Message {
	return []*irc.Message{
		&irc.Message{Prefix: nick, Command: "JOIN", Params: []string{"#sandstorm"}},
		&irc.Message{Command: irc.RPL_TOPIC, Params: []string{
			nick, "#sandstorm", "Welcome to #sandstorm!",
		}},
		&irc.Message{Command: irc.RPL_NAMEREPLY, Params: []string{
			nick, "=", "#sandstorm", nick,
		}},
		&irc.Message{Command: irc.RPL_NAMEREPLY, Params: []string{
			nick, "=", "#sandstorm", "bob",
		}},
		&irc.Message{Command: irc.RPL_ENDOFNAMES, Params: []string{
			nick, "#sandstorm", "End of NAMES list",
		}},
	}
}

func TestConnectDisconnect(t *testing.T) {
	TraceTest(t, ExpectMany{
		ClientConnect{},
		ConnectServer{},
		ClientDisconnect{},
		// Handshake isn't done:
		DropServer{},
	})
}

// Regression tests for https://github.com/zenhack/irc-idler/issues/4
func TestNickInUse(t *testing.T) {
	TraceTest(t, ExpectMany{
		ClientConnect{},
		ConnectServer{},
		&FromClient{Command: "NICK", Params: []string{"alice"}},
		&ToServer{Command: "NICK", Params: []string{"alice"}},
		&FromServer{Command: irc.ERR_NICKNAMEINUSE},
		&ToClient{Command: irc.ERR_NICKNAMEINUSE},
	})
}

func TestInitialLogin(t *testing.T) {
	TraceTest(t, initialConnect("alice"))
}

func TestBasicReconnect(t *testing.T) {
	TraceTest(t, ExpectMany{
		initialConnect("alice"),
		ClientDisconnect{},
		reconnect("alice"),
	})
}

func TestChannelRejoinNoBackLog(t *testing.T) {
	TraceTest(t, ExpectMany{
		initialConnect("alice"),
		ForwardC2S(&irc.Message{Command: "JOIN", Params: []string{"#sandstorm"}}),
		ManyMsg(ForwardS2C, joinSeq("alice")),
		ClientDisconnect{},
		reconnect("alice"),
		&FromClient{Command: "JOIN", Params: []string{"#sandstorm"}},
		ManyToClient(joinSeq("alice")),
	})
}

func TestChangeNickRejoin(t *testing.T) {
	TraceTest(t, ExpectMany{
		initialConnect("alice"),
		ForwardC2S(&irc.Message{Command: "JOIN", Params: []string{"#sandstorm"}}),
		ManyMsg(ForwardS2C, joinSeq("alice")),
		ForwardC2S(&irc.Message{Command: "NICK", Params: []string{"eve"}}),
		ForwardS2C(&irc.Message{Prefix: "alice", Command: "NICK", Params: []string{"eve"}}),
		ClientDisconnect{},
		reconnect("eve"),
		&FromClient{Command: "JOIN", Params: []string{"#sandstorm"}},
		ManyToClient(joinSeq("eve")),
	})
}

func TestClientPingDrop(t *testing.T) {
	TraceTest(t, ExpectMany{
		initialConnect("alice"),

		Sleep(pingTime),
		&ToClient{Command: "PING", Params: []string{"irc-idler"}},
		&ToServer{Command: "PING", Params: []string{"irc-idler"}},

		&FromServer{Command: "PONG", Params: []string{"irc-idler"}},
		Sleep(pingTime),

		&DropClient{},
		&ToServer{Command: "PING", Params: []string{"irc-idler"}},
	})
}
