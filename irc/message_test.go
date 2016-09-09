package irc

import (
	"bytes"
	"fmt"
	"testing"
	"testing/quick"
)

// example data for the tests
var sampleMessages = []*Message{
	&Message{Command: "PRIVMSG", Params: []string{"##cool_topic", "Hello!"}},
	&Message{Command: "PING", Params: []string{}},
	&Message{Prefix: "bob", Command: "STUFF", Params: []string{"THINGS"}},
}

var sampleUnparsedMessages = []string{
	":alice PRIVMSG ##crypto :Hey Bob!\r\n",
	":bob PRIVMSG ##crypto :Hey!\r\n",
	":bob PRIVMSG ##crypto Hey!\r\n",
}

// Verify that writing out msg and reading it back results in the same value.
func checkReadBack(msg *Message) bool {
	buf := &bytes.Buffer{}
	msg.WriteTo(buf)
	result, err := NewReader(buf).ReadMessage()
	if err != nil {
		fmt.Printf("Error reading back message: %v\n", err)
		return false
	} else if !msg.Eq(result) {
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

// Make sure Message.Len computes the actual length of the serialized message.
func TestLen(t *testing.T) {
	for _, m := range sampleMessages {
		str := m.String()
		expected := len(str)
		actual := m.Len()
		if expected != actual {
			t.Fatalf("TestLen: Expected length %d for message %q, but got %d.",
				expected, str, actual)
		}
	}
}

// Make sure ParseMessage obeys the same rules as checkReadBack is checking.
func TestParseStringReadBack(t *testing.T) {
	err := quick.Check(func(msg1 *Message) bool {
		str1 := msg1.String()
		msg2, err := ParseMessage(msg1.String())
		if err != nil {
			fmt.Printf("%q", err)
			return false
		}
		if !msg1.Eq(msg2) {
			fmt.Printf("Messages differ: msg1: %v vs msg2: %v\n", msg1, msg2)
			return false
		}
		str2 := msg2.String()
		if str1 != str2 {
			fmt.Printf("Strings differ: str1: %q vs str2: %q\n", str1, str2)
			return false
		}
		return true
	}, nil)

	if err != nil {
		t.Fatal(err)
	}
}

// Make sure that parsing and then re-serializing a message doesn't result in
// a longer message. This can happen without otherwise generating an invalid
// or semantically different message, but it is important that we don't
// overflow the maximum message size, so we want to ensure our implementation
// doesn't have this property
func TestMessageDoesntGrow(t *testing.T) {
	for _, str := range sampleUnparsedMessages {
		msg, err := ParseMessage(str)
		if err != nil {
			panic(err)
		}
		newStr := msg.String()
		if len(newStr) > len(str) {
			t.Fatalf("Message %q (length %d) grew to message %q (length %d)",
				str, len(str), newStr, len(newStr))
		}
	}
}

// Make sure ParseMessage doesn't accept strings with more than one message
func TestParseStringOneMessageOnly(t *testing.T) {
	_, err := ParseMessage("PING foo\r\nPONG foo\r\n")
	if err == nil {
		t.Fatal("ParseMessage() did not return an error on a string " +
			"with two messages.")
	}
}
