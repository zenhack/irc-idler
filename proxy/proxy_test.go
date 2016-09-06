package proxy

import (
	"testing"
	"zenhack.net/go/irc-idler/irc"
)

var (
	initialConnect = ExpectMany{
		ClientConnect{},
		ConnectServer{},
		ForwardC2S(&irc.Message{Command: "NICK", Params: []string{"alice"}}),
		ForwardC2S(&irc.Message{Command: "USER", Params: []string{"alice", "0", "*", "Alice"}}),
		ManyMsg(ForwardS2C, welcomeSequence),
		motd,
	}

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

	welcomeSequence = []*irc.Message{
		{
			Command: irc.RPL_WELCOME,
			Params:  []string{"alice", "Welcome to a mock irc server alice"},
		},
		{
			Command: irc.RPL_YOURHOST,
			Params:  []string{"alice", "Your host is testing.example.com"},
		},
		{
			Command: irc.RPL_CREATED,
			Params:  []string{"alice", "This server was started now-ish."},
		},
		{
			Command: irc.RPL_MYINFO,
			Params: []string{
				"alice",
				"testing.example.com",
				"mock-0.1",
				// TODO: these might actually matter someday:
				"0",
				"0",
			},
		},
	}

	reconnect = ExpectMany{
		&ClientConnect{},
		&FromClient{Command: "NICK", Params: []string{"alice"}},
		&FromClient{Command: "USER", Params: []string{"alice", "0", "*", "Alice"}},
		ManyToClient(welcomeSequence),
		motd,
	}
)

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
	TraceTest(t, initialConnect)
}

func TestBasicReconnect(t *testing.T) {
	TraceTest(t, ExpectMany{
		initialConnect,
		ClientDisconnect{},
		reconnect,
	})
}

func TestChannelRejoinNoBackLog(t *testing.T) {
	joinSeq := []*irc.Message{
		&irc.Message{Prefix: "alice", Command: "JOIN", Params: []string{"#sandstorm"}},
		&irc.Message{Command: irc.RPL_TOPIC, Params: []string{
			"alice", "#sandstorm", "Welcome to #sandstorm!",
		}},
		&irc.Message{Command: irc.RPL_NAMEREPLY, Params: []string{
			"alice", "=", "#sandstorm", "alice",
		}},
		&irc.Message{Command: irc.RPL_NAMEREPLY, Params: []string{
			"alice", "=", "#sandstorm", "bob",
		}},
	}
	TraceTest(t, ExpectMany{
		initialConnect,
		ForwardC2S(&irc.Message{Command: "JOIN", Params: []string{"#sandstorm"}}),
		ManyMsg(ForwardS2C, joinSeq),
		ClientDisconnect{},
		reconnect,
		&FromClient{Command: "JOIN", Params: []string{"#sandstorm"}},
		ManyToClient(joinSeq),
	})
}
