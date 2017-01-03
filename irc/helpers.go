package irc

// This file defines helpers that make working with messages easier.

// ReadAll reads all messages from r in a separate go routine. returns a
// channel via which the messages may be received.
func ReadAll(r Reader) <-chan *Message {
	ch := make(chan *Message)
	go func() {
		for {
			msg, err := r.ReadMessage()
			if err != nil {
				// TODO: would be nice if we were logging the
				// error somehow (at least if it's not io.EOF).
				// Don't want a hardcoded logging statement in
				// library code though; will need to parametrize.
				break
			}
			ch <- msg
		}
		close(ch)
	}()
	return ch
}
