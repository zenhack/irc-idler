package irc

import (
	"fmt"
	"strings"
)

type ClientID struct {
	Nick, User, Host string
}

func (id ClientID) String() string {
	return id.Nick + "!" + id.User + "@" + id.Host
}

type clientIDParseError string

func (e clientIDParseError) Error() string {
	return fmt.Sprintf(
		"Error parsing client id: there must be exactly one %q character.",
		string(e),
	)
}

func ParseClientID(text string) (ClientID, error) {
	var ret ClientID
	parts := strings.Split(text, "!")
	if len(parts) != 2 {
		return ClientID{}, clientIDParseError("!")
	}
	ret.Nick = parts[0]
	parts = strings.Split(parts[1], "@")
	if len(parts) != 2 {
		return ClientID{}, clientIDParseError("@")
	}
	ret.User = parts[0]
	ret.Host = parts[1]
	return ret, nil
}
