package irc

import (
	"testing"
	"time"
)

func TestRateLimit(t *testing.T) {
	in := make(chan *Message)
	out := make(chan *Message)
	initQuota := 10
	maxQuota := 10
	numMessages := 30
	refresh := time.Second / 10
	go RateLimit(in, out, initQuota, maxQuota, refresh)
	start := time.Now()
	go func() {
		for i := 0; i < numMessages; i++ {
			in <- &Message{Command: "PING", Params: []string{}}
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
