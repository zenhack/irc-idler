// Package filters provides "filters" for IRC messages, useful for common processing tasks.
package filters

import (
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
