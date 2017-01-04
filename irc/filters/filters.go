// Package filters provides "filters" for IRC messages, useful for common processing tasks.
package filters

import (
	"golang.org/x/net/context"
	"time"
	"zenhack.net/go/irc-idler/irc"
)

// RateLimit copies src to dst, rate-limiting the flow, as follows:
//
// * A quota is maintained with initial value initQuota.
// * Once per `refresh`, the quota is incremented by 1, to a maximum of maxQuota.
// * Each time a message is copied, the quota is decremented by 1.
// * If the quota is zero, no new messages will be copied until the quota
//   increases.
//
// When src is closed, RateLimit will close dst and then return.
func RateLimit(src <-chan *irc.Message, dst chan<- *irc.Message, initQuota, maxQuota int, refresh time.Duration) {
	left := maxQuota
	ticker := time.NewTicker(refresh)
	defer ticker.Stop()
	for {
		if left > maxQuota {
			left = maxQuota
		}
		select {
		case <-ticker.C:
			left++
		case msg, ok := <-src:
			if !ok {
				close(dst)
				return
			}
			dst <- msg
			left--
			if left == 0 {
				<-ticker.C
				left++
			}
		}
	}
}

// AutoPong automatically responds to PINGs sent on `in`, with PONGs sent on `reply`. It forwards
// other messages on `forward`.
func AutoPong(ctx context.Context, in <-chan *irc.Message, forward, reply chan<- *irc.Message) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-in:
			var dest chan<- *irc.Message
			if msg.Command == "PING" {
				dest = reply
				msg.Command = "PONG"
			} else {
				dest = forward
			}
			select {
			case <-ctx.Done():
				return
			case dest <- msg:
			}
		}
	}
}

// AutoPing forwards messages from `in` to `forward`. If no message is received on `in`
// for at least `pingTime`, a PING will be sent on `reply`. If after another duration of
// `pingTime`, there have still been no messages recevied, disconnect will be called.
// PONG messages will *not* be forwarded.
func AutoPing(ctx context.Context, disconnect context.CancelFunc, pingTime time.Duration,
	in <-chan *irc.Message, forward, reply chan<- *irc.Message) {
	pingSent := false
	timer := time.NewTimer(pingTime)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-in:
			if !timer.Stop() {
				<-timer.C
			}
			timer.Reset(pingTime)
			pingSent = false
			if msg.Command == "PONG" {
				continue
			}
			select {
			case <-ctx.Done():
				return
			case forward <- msg:
			}
		case <-timer.C:
			if pingSent {
				disconnect()
				return
			}
			select {
			case <-ctx.Done():
				return
			case reply <- &irc.Message{
				Command: "PING",
				Params:  []string{"ping"},
			}:
				timer.Reset(pingTime)
				pingSent = true
			}
		}
	}

}
