package irc

import (
	"bytes"
	"testing"
)

// Compare m1 and m2 for equality. We can't just use (==), as it
// doesn't work on string slices (Message.Params).
func msgEq(m1, m2 *Message) bool {
	if m1.Prefix != m2.Prefix || m1.Command != m2.Command {
		return false
	}
	if len(m1.Params) != len(m2.Params) {
		return false
	}
	for i := range m1.Params {
		if m1.Params[i] != m2.Params[i] {
			return false
		}
	}
	return true
}

// example data for the tests
var sampleMessages = []*Message{
	&Message{Command: "PRIVMSG", Params: []string{"##cool_topic", "Hello!"}},
	&Message{Command: "PING", Params: []string{}},
	&Message{Prefix: "bob", Command: "STUFF", Params: []string{"THINGS"}},
}

// Verify that writing out msg and reading it back results in the same value.
func checkReadBack(t *testing.T, msg *Message) {
	buf := &bytes.Buffer{}
	msg.WriteTo(buf)
	result, err := NewReader(buf).ReadMessage()
	if err != nil {
		t.Fatalf("Error reading back message: %v\n", err)
	} else if !msgEq(msg, result) {
		t.Fatalf(
			"Read message %v differs from written %v.\n",
			result,
			msg,
		)
	}
}

// Call checkReadBack on each of the messages in sampleMessages.
func TestReadBack(t *testing.T) {
	for _, m := range sampleMessages {
		checkReadBack(t, m)
	}
}
