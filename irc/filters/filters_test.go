package filters

import (
	"testing"
	"time"
	"zenhack.net/go/irc-idler/irc"
)

func TestRateLimit(t *testing.T) {
	in := make(chan *irc.Message)
	out := make(chan *irc.Message)
	initQuota := 10
	maxQuota := 10
	numMessages := 30
	refresh := time.Second / 10
	go RateLimit(in, out, initQuota, maxQuota, refresh)
	start := time.Now()
	go func() {
		for i := 0; i < numMessages; i++ {
			in <- &irc.Message{Command: "PING", Params: []string{}}
		}
		close(in)
	}()
	ok := true
	for ok {
		_, ok = <-out
	}
	end := time.Now()
	elapsed := end.Sub(start)
	if elapsed < refresh*time.Duration(numMessages-initQuota) {
		t.Fatal("Test completed too quickly!")
	}
}
