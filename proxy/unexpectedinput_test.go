package proxy

// Tests for behavior on unexpected messages.

import (
	"testing"
	"zenhack.net/go/irc-idler/irc"
)

func TestUnexpected_RPL_TOPIC(t *testing.T) {
	TraceTest(t, ExpectMany{
		ClientConnect{},
		ConnectServer{},
		FromServer(&irc.Message{
			Command: irc.RPL_TOPIC,
			Params:  []string{"alice", "#unexpected", "unexpected topic!"},
		}),

		// Should ignore it and keep on trucking:
		FromClient(&irc.Message{Command: "NICK", Params: []string{"alice"}}),
	})
}

func TestUnexpected_RPL_NAMEREPLY(t *testing.T) {
	TraceTest(t, ExpectMany{
		ClientConnect{},
		ConnectServer{},
		FromServer(&irc.Message{
			Command: irc.RPL_NAMEREPLY,
			Params: []string{
				"alice", "=", "#unexpected", "unexpected users",
			},
		}),

		// Should ignore it and keep on trucking:
		FromClient(&irc.Message{Command: "NICK", Params: []string{"alice"}}),
	})
}
