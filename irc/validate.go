package irc

import (
	"errors"
)

var (
	// Errors for Validate. TODO: would be nice to define a type implementing
	// error that provides a suitable irc-protocol error message, so users
	// of the library can in many cases just send back the error.
	E_NO_COMMAND      = errors.New("Message has no command")
	E_TOO_MANY_PARAMS = errors.New("Message has too many arguments")
	E_TOO_FEW_PARAMS  = errors.New("Message has too few arguments")

	// This should never happen with a message we're reading off the wire:
	E_PARAMS_IS_NIL = errors.New("Params is nil!")
)

var minParams = map[string]int{
	"PASS": 1,
	"NICK": 1,
}

func (m *Message) Validate() error {
	switch {
	case m.Params == nil:
		return E_PARAMS_IS_NIL
	case m.Command == "":
		return E_NO_COMMAND
	case len(m.Params) > 15: // TODO: double check the RFC; is it > or >=?
		return E_TOO_MANY_PARAMS
	case len(m.Params) < minParams[m.Command]:
		return E_TOO_FEW_PARAMS
	}
	return nil
}
