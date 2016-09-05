package proxy

import (
	"testing"
	"time"
	"zenhack.net/go/irc-idler/irc"
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
