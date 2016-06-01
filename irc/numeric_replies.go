package irc

// Map from symbolic names (e.g. RPY_WELCOME) to numeric codes (as strings).
// Note that this is incomplete; it only includes the replies we've needed
// thus far.
var Replies = map[string]string{
	"RPL_WELCOME": "001",
}
