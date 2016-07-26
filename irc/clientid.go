package irc

import (
	"fmt"
	"strings"
)

// A client identifier. The RFC suggests (but actually doesn't explicitly say)
// that this is of the form `<nick>!<user>@<host>`. We accept either this or
// just `<nick>` in which case the value of the other two fields will be `""`.
type ClientID struct {
	Nick, User, Host string
}

func (id ClientID) String() string {
	if id.User == "" || id.Host == "" {
		return id.Nick
	} else {
		return id.Nick + "!" + id.User + "@" + id.Host
	}
}

type clientIDParseError string

func (e clientIDParseError) Error() string {
	return fmt.Sprintf("Error parsing client id: %v", string(e))
}

func ParseClientID(text string) (ClientID, error) {
	var ret ClientID
	parts := strings.Split(text, "!")
	if len(parts) == 1 {
		return ClientID{parts[0], "", ""}, nil
	} else if len(parts) != 2 {
		return ClientID{}, clientIDParseError("More than one '!' character")
	}
	ret.Nick = parts[0]
	parts = strings.Split(parts[1], "@")
	if len(parts) != 2 {
		return ClientID{}, clientIDParseError("Have user field but no host.")
	}
	ret.User = parts[0]
	ret.Host = parts[1]
	return ret, nil
}
