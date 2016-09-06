package irc

import (
	"fmt"
	"strings"
)

// A client identifier. RFC 2812 defines the syntax of this in section 2.3.1,
// in the latter half of the "prefix" production. The description of RPL_WELCOME
// suggests (but actually doesn't explicitly say) that this is the syntax for
// its client field.
type ClientID struct {
	Nick, User, Host string
}

func (id ClientID) String() string {
	ret := id.Nick
	if id.Host == "" {
		return ret
	}
	if id.User != "" {
		ret += "!" + id.User
	}
	ret += "@" + id.Host
	return ret
}

type clientIDParseError string

func (e clientIDParseError) Error() string {
	return fmt.Sprintf("Error parsing client id: %v", string(e))
}

func ParseClientID(text string) (ClientID, error) {
	var ret ClientID
	nickHostParts := strings.Split(text, "@")
	switch len(nickHostParts) {
	case 1:
		ret = ClientID{Nick: text}
	case 2:
		nickUserParts := strings.Split(nickHostParts[0], "!")
		switch len(nickUserParts) {
		case 1:
			ret = ClientID{
				Nick: nickHostParts[0],
				Host: nickHostParts[1],
			}
		case 2:
			ret = ClientID{
				Nick: nickUserParts[0],
				User: nickUserParts[1],
				Host: nickHostParts[1],
			}
		default:
			return ClientID{}, clientIDParseError("More than one '!' char in client id")
		}
	default:
		return ClientID{}, clientIDParseError("More than one '@' char in client id")
	}

	if ret.Nick == "" {
		return ClientID{}, clientIDParseError("No nick in client ID.")
	}
	if ret.User != "" && ret.Host == "" {
		return ClientID{}, clientIDParseError("User but no host in client ID.")
	}

	return ret, nil
}
