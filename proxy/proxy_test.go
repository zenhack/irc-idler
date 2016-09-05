package proxy

import (
	"testing"
	"time"
	"zenhack.net/go/irc-idler/irc"
)

var (
	initialConnect = ExpectMany{
		ClientConnect{},
		ConnectServer{},
		ForwardC2S(&irc.Message{Command: "NICK", Params: []string{"alice"}}),
		ForwardC2S(&irc.Message{Command: "USER", Params: []string{"alice", "0", "*", ":Alice"}}),
		welcomeSequence,
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

	welcomeSequence = ExpectMany{
		ForwardS2C(&irc.Message{
			Command: irc.RPL_WELCOME,
			Params:  []string{"alice", "alice", "Welcome to a mock irc server!"},
		}),
		ForwardS2C(&irc.Message{
			Command: irc.RPL_YOURHOST,
			Params:  []string{"alice", "Your host is testing.example.com"},
		}),
		ForwardS2C(&irc.Message{
			Command: irc.RPL_CREATED,
			Params:  []string{"alice", "This server was started now-ish."},
		}),
		ForwardS2C(&irc.Message{
			Command: irc.RPL_MYINFO,
			Params:  []string{"alice", "Some info about your host"},
		}),
		motd,
	}

	reconnect = ExpectMany{
		&ClientConnect{},
		&FromClient{Command: "NICK", Params: []string{"alice"}},
		&FromClient{Command: "USER", Params: []string{"alice", "0", "*", ":Alice"}},
	}
)

func TestConnectDisconnect(t *testing.T) {
	state := StartTestProxy()
	err := Expect(state, time.Second,
		ClientConnect{},
		ConnectServer{},
		ClientDisconnect{},
		// Handshake isn't done:
		DropServer{},
	)
	if err != nil {
		t.Fatal(err)
	}
}

// Regression tests for https://github.com/zenhack/irc-idler/issues/4
func TestNickInUse(t *testing.T) {
	state := StartTestProxy()
	err := Expect(state, time.Second,
		ClientConnect{},
		ConnectServer{},
		&FromClient{Command: "NICK", Params: []string{"alice"}},
		&ToServer{Command: "NICK", Params: []string{"alice"}},
		&FromServer{Command: irc.ERR_NICKNAMEINUSE},
		&ToClient{Command: irc.ERR_NICKNAMEINUSE},
	)
	if err != nil {
		t.Fatal(err)
	}
}

func TestInitialLogin(t *testing.T) {
	state := StartTestProxy()
	err := Expect(state, time.Second,
		initialConnect,
	)
	if err != nil {
		t.Fatal(err)
	}
}
