package irc

import (
	"testing"
)

type testCase struct {
	text string
	ClientID
	err bool
}

var cases = []testCase{
	testCase{"", ClientID{}, true},
	testCase{"nick!user@host", ClientID{"nick", "user", "host"}, false},
	testCase{"nick", ClientID{"nick", "", ""}, false},
}

// For each of our test cases, verify that:
//
// 1. Parsing the string produces the expected result (either the correct
//    client ID or an error)
// 2. If the result is not an error, converting the ClientID back to a
//    string yields the input string
func TestParse(t *testing.T) {
	for _, v := range cases {
		t.Logf("TestParse: {%q, {%q, %q, %q}, %v}",
			v.text,
			v.ClientID.Nick, v.ClientID.User, v.ClientID.Host,
			v.err,
		)

		// first check ParseClientID()
		clientID, err := ParseClientID(v.text)

		if err != nil && !v.err {
			t.Fatalf("Got unexpected error %q.", err)
		}
		if err == nil && v.err {
			t.Fatalf("Expected an error but got success.")
		}

		// If the above gives an error, the remaining tests are not meaningful:
		if err != nil {
			continue
		}

		if clientID != v.ClientID {
			t.Fatalf(
				"ParseClientID() test failed: expected "+
					"ClientID{%q, %q, %q} but got "+
					"ClientID{%q, %q, %q}.",
				v.ClientID.Nick, v.ClientID.User, v.ClientID.Host,
				clientID.Nick, clientID.User, clientID.Host,
			)
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
