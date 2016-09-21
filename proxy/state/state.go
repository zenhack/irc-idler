// Package state provides support for modeling the state of
// objects affected by IRC messages
package state

import (
	"zenhack.net/go/irc-idler/irc"
)

type State interface {
	// Update the state when the client sends `msg`.
	UpdateFromClient(msg *irc.Message)

	// Update the state when the server sends `msg`.
	UpdateFromServer(msg *irc.Message)
}
