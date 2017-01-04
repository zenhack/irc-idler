package filters

import (
	"golang.org/x/net/context"
	"testing"
	"time"
	"zenhack.net/go/irc-idler/irc"
)

// Basic test for RateLimit; send a bunch of messages through the filter, and
// verify that it takes at least as long as the rate-limiting would require.
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

// Basic test for AutoPong; send some messages, make sure the right responses
// come back on the right channels.
func TestAutoPong(t *testing.T) {
	in := make(chan *irc.Message)
	forward := make(chan *irc.Message)
	reply := make(chan *irc.Message)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go AutoPong(ctx, in, forward, reply)

	// Something other than a PING should get forwarded through:
	msgIn := &irc.Message{Command: "One", Params: []string{}}
	in <- msgIn
	msgFwd := <-forward
	if !msgIn.Eq(msgFwd) {
		t.Fatalf("Unexpected forwarded message: %q", msgFwd)
	}

	// A PING should result in a PONG being sent on reply:
	in <- &irc.Message{Command: "PING", Params: []string{"One"}}
	msgReply := <-reply
	if !msgReply.Eq(&irc.Message{Command: "PONG", Params: []string{"One"}}) {
		t.Fatal("Unexpected message on reply channel: %q\n")
	}

	// More things other than PINGs:
	for _, cmd := range []string{"Two", "Three", "Four"} {
		msgIn = &irc.Message{Command: cmd, Params: []string{}}
		in <- msgIn
		msgFwd = <-forward
		if !msgIn.Eq(msgFwd) {
			t.Fatalf("Unexpected forwarded message: %q", msgFwd)
		}
	}
}

func TestAutoPing(t *testing.T) {
	in := make(chan *irc.Message)
	forward := make(chan *irc.Message)
	reply := make(chan *irc.Message)

	pingTime := time.Second / 5

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go AutoPing(ctx, cancel, pingTime, in, forward, reply)

	// Wait for a PING and respond:
	select {
	case msg := <-reply:
		if msg.Command != "PING" {
			t.Fatalf("AutoPong sent something other than PING on reply: %q", msg)
		}
		msg.Command = "PONG"
		in <- msg
	case <-time.After(pingTime * 2):
		t.Fatal("AutoPing didn't send us a PING.")
	case <-ctx.Done():
		t.Fatal("AutoPing disconnected us without sending PING.")
	}

	// Wait for a PING and ignore it:
	select {
	case msg := <-reply:
		if msg.Command != "PING" {
			t.Fatalf("AutoPong sent something other than PING on reply: %q", msg)
		}
	case <-time.After(pingTime * 2):
		t.Fatal("AutoPing didn't send us a PING.")
	case <-ctx.Done():
		t.Fatal("AutoPing disconnected us without sending PING.")
	}

	// Wait for the disconnect:
	select {
	case msg := <-reply:
		t.Fatalf("AutoPing sent us another message after the PING: %q", msg)
	case <-time.After(pingTime * 2):
		t.Fatalf("AutoPing should have disconnected us by now.")
	case <-ctx.Done():
		// Good, we've been disconnected.
	}
}

// Test AutoPong against AutoPing; we hook them up to one another and make
// sure AutoPing doesn't disconnect us for a little while.
func TestAutoPingPong(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pingIn := make(chan *irc.Message)
	pingReply := make(chan *irc.Message)
	pingForward := make(chan *irc.Message)

	pongForward := make(chan *irc.Message)

	go AutoPong(ctx, pingReply, pongForward, pingIn)
	go AutoPing(ctx, cancel, time.Second/10, pingIn, pingForward, pingReply)

	select {
	case <-ctx.Done():
		t.Fatal("Context was canceled; AutoPong didn't do its job.")
	case msg := <-pingForward:
		t.Fatalf("AutoPing incorrectly forwarded a message: %q", msg)
	case msg := <-pongForward:
		t.Fatalf("AutoPong incorrectly forwarded a message: %q", msg)
	case <-time.After(3 * time.Second):
		// Test passed
		return
	}
}
