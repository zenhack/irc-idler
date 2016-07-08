package irc

import (
	"testing"
)

type testCase struct {
	text string
	ClientID
	err error
}

var cases = []testCase{
	testCase{"", ClientID{}, clientIDParseError("!")},
	testCase{"nick!user@host", ClientID{"nick", "user", "host"}, nil},
}

// For each of our test cases, verify that:
//
// 1. Parsing the string produces the expected result (either the correct
//    client ID or an error)
// 2. If the result is not an error, converting the ClientID back to a
//    string yields the input string
func TestParse(t *testing.T) {
	for _, v := range cases {

		// first check ParseClientID()
		clientID, err := ParseClientID(v.text)
		if clientID != v.ClientID || err != v.err {
			t.Fatalf(
				"ParseClientID() test failed: expected "+
					"(ClientID{%q, %q, %q}, %q) but got "+
					"(ClientID{%q, %q, %q}, %q).",
				v.ClientID.Nick, v.ClientID.User, v.ClientID.Host, v.err,
				clientID.Nick, clientID.User, clientID.Host, err,
			)
		}

		// If the above gives an error, the next test is not meaningful:
		if err != nil {
			continue
		}

		// Now test String():
		text := clientID.String()
		if text != v.text {
			t.Fatalf(
				"ClientID.String() test failed: expected %q but got %q.",
				v.text, text,
			)
		}
	}
}
