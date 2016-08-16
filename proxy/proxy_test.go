package proxy

import (
	"zenhack.net/go/irc-idler/irc"
)

type eventType int

const (
	AcceptClient = iota
	DialServer
	DropClient
	DropServer
	ToClient
	FromClient
	ToServer
	FromServer
)

var tests = [][]struct {
	Type eventType
	Msg  *irc.Message
}{
	{
		{Type: AcceptClient},
		{Type: DialServer},
	},
}
