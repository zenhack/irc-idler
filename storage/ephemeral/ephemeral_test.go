package ephemeral

import (
	"testing"
	stest "zenhack.net/go/irc-idler/storage/testing"
)

func TestEphemeral(t *testing.T) {
	stest.RandTest(t, NewStore)
}
