package proxy

import (
	"testing"
	"zenhack.net/go/irc-idler/irc"
)

var (
	motd = ExpectMany{
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
		Connect(Client),
		Connect(Server),
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
		Connect(Client),
		FromClient(&irc.Message{Command: "NICK", Params: []string{nick}}),
		FromClient(&irc.Message{Command: "USER", Params: []string{nick, "0", "*", "Alice"}}),
		ToClient(&irc.Message{
			Command: irc.RPL_WELCOME,
			Params:  []string{nick, "Welcome back to IRC Idler, " + nick},
		}),
		ManyMsg(ToClient, welcomeSequence(nick)),
		ToServer(&irc.Message{Command: "MOTD"}),
		motd,
	}
}

func joinSeq(forward bool, nick string) ProxyAction {
	var (
		namerepliesAction ProxyAction
		convert           func(*irc.Message) ProxyAction
	)
	namereplyMsgs := []*irc.Message{
		{Command: irc.RPL_NAMEREPLY, Params: []string{
			nick, "=", "#sandstorm", nick,
		}},
		{Command: irc.RPL_NAMEREPLY, Params: []string{
			nick, "=", "#sandstorm", "bob",
		}},
	}
	if forward {
		convert = ForwardS2C
		namerepliesAction = ManyMsg(convert, namereplyMsgs)
	} else {
		convert = ToClient
		namerepliesAction = UnorderedTo(Client, namereplyMsgs)
	}
	return ExpectMany{
		convert(&irc.Message{Prefix: nick, Command: "JOIN", Params: []string{"#sandstorm"}}),
		convert(&irc.Message{Command: irc.RPL_TOPIC, Params: []string{
			nick, "#sandstorm", "Welcome to #sandstorm!",
		}}),
		namerepliesAction,
		convert(&irc.Message{Command: irc.RPL_ENDOFNAMES, Params: []string{
			nick, "#sandstorm", "End of NAMES list",
		}}),
	}
}

func TestConnectDisconnect(t *testing.T) {
	TraceTest(t, ExpectMany{
		Connect(Client),
		Connect(Server),
		Disconnect(Client),
		// Handshake isn't done:
		Drop(Server),
	})
}

// Regression tests for https://github.com/zenhack/irc-idler/issues/4
func TestNickInUse(t *testing.T) {
	TraceTest(t, ExpectMany{
		Connect(Client),
		Connect(Server),
		FromClient(&irc.Message{Command: "NICK", Params: []string{"alice"}}),
		ToServer(&irc.Message{Command: "NICK", Params: []string{"alice"}}),
		FromServer(&irc.Message{Command: irc.ERR_NICKNAMEINUSE}),
		ToClient(&irc.Message{Command: irc.ERR_NICKNAMEINUSE}),
	})
}

func TestInitialLogin(t *testing.T) {
	TraceTest(t, initialConnect("alice"))
}

func TestBasicReconnect(t *testing.T) {
	TraceTest(t, ExpectMany{
		initialConnect("alice"),
		Disconnect(Client),
		reconnect("alice"),
	})
}

func TestChannelRejoinNoBackLog(t *testing.T) {
	TraceTest(t, ExpectMany{
		initialConnect("alice"),
		ForwardC2S(&irc.Message{Command: "JOIN", Params: []string{"#sandstorm"}}),
		joinSeq(true, "alice"),
		Disconnect(Client),
		reconnect("alice"),
		FromClient(&irc.Message{Command: "JOIN", Params: []string{"#sandstorm"}}),
		joinSeq(false, "alice"),
	})
}

func TestChangeNickRejoin(t *testing.T) {
	TraceTest(t, ExpectMany{
		initialConnect("alice"),
		ForwardC2S(&irc.Message{Command: "JOIN", Params: []string{"#sandstorm"}}),
		joinSeq(true, "alice"),
		ForwardC2S(&irc.Message{Command: "NICK", Params: []string{"eve"}}),
		ForwardS2C(&irc.Message{Prefix: "alice", Command: "NICK", Params: []string{"eve"}}),
		Disconnect(Client),
		reconnect("eve"),
		FromClient(&irc.Message{Command: "JOIN", Params: []string{"#sandstorm"}}),
		joinSeq(false, "eve"),
	})
}

func TestClientPingDrop(t *testing.T) {
	TraceTest(t, ExpectMany{
		initialConnect("alice"),

		Sleep(pingTime),
		ToClient(&irc.Message{Command: "PING", Params: []string{"irc-idler"}}),
		ToServer(&irc.Message{Command: "PING", Params: []string{"irc-idler"}}),

		FromServer(&irc.Message{Command: "PONG", Params: []string{"irc-idler"}}),
		Sleep(pingTime),

		Drop(Client),
		ToServer(&irc.Message{Command: "PING", Params: []string{"irc-idler"}}),
	})
}
