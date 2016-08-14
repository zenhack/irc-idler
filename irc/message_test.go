package irc

import (
	"bytes"
	"fmt"
	"testing"
	"testing/quick"
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
func checkReadBack(msg *Message) bool {
	buf := &bytes.Buffer{}
	msg.WriteTo(buf)
	result, err := NewReader(buf).ReadMessage()
	if err != nil {
		fmt.Printf("Error reading back message: %v\n", err)
		return false
	} else if !msgEq(msg, result) {
		fmt.Printf(
			"Read message %v differs from written %v.\n",
			result,
			msg,
		)
		return false
	}
	return true
}

// Call checkReadBack on each of the messages in sampleMessages, as well
// as some randomized messages.
func TestReadBack(t *testing.T) {
	for _, m := range sampleMessages {
		if !checkReadBack(m) {
			t.FailNow()
		}
	}
	if err := quick.Check(checkReadBack, nil); err != nil {
		t.Fatal(err)
	}
}
