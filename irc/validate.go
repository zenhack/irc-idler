package irc

// A wrapper around Message that implements the error interface.
//
// This allows it to be used as an error, and also to be a reply for erroneous
// messages sent by peers.
type MessageError Message

func (me *MessageError) Error() string {
	return me.String()
}

func (me *MessageError) String() string {
	m := (*Message)(me)
	return m.String()
}

var minParams = map[string]int{
	"PASS":      1,
	"NICK":      1,
	"USER":      4,
	RPL_WELCOME: 1, // XXX: maybe 2 according to the spec? 2nd is for humans
}

// Validate the message m. This performs various checks:
//
// * A command is supplied
// * Minimum number of arguments for the command are supplied, if the command
//   is known.
// * The number of parameters does not exceed the limit imposed by the rfc (15).
//
// Returns nil for a valid message. For an invalid message, return a suitable
// reply error message.
//
// Note that this method does not check for errors that cannot occur in a message
// read off the wire, e.g. Params being nil (as opposed to []string{}).
func (m *Message) Validate() *MessageError {
	switch {
	case m.Command == "":
		return &MessageError{
			Command: ERR_UNKNOWNCOMMAND,
			Params:  []string{"Unknown command: \"\""},
		}
	case len(m.Params) > 15:
		// XXX: ERR_UNKNOWNCOMMAND isn't really a good fit for this, but the RFC
		// doesn't seem to define someting obviously better.
		return &MessageError{
			Command: ERR_UNKNOWNCOMMAND,
			Params:  []string{"Too many parameters (max 15)"},
		}
	case len(m.Params) < minParams[m.Command]:
		return &MessageError{
			Command: ERR_NEEDMOREPARAMS,
			Params:  []string{"Not enough parameters"},
		}
	}
	return nil
}
