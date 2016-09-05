package proxy

import (
	"testing"
	"time"
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
