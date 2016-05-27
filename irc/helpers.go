package irc

// This file defines helpers that make working with messages easier.

// AutoPong returns a ReadWriterer which which filters incomming PING
// commands, responding to them automatically. Note that this will
// *only* happen when ReadMessage is called; no separate goroutine
// is created.
func AutoPong(rw ReadWriter) ReadWriter {
	return autoPonger{rw}
}

// type returned by AutoPong
type autoPonger struct {
	rw ReadWriter
}

func (ap autoPonger) ReadMessage() (*Message, error) {
	for {
		msg, err := ap.rw.ReadMessage()
		if err != nil {
			return nil, err
		}
		if msg.Command != "PING" {
			return msg, err
		}
		msg.Prefix = ""
		msg.Command = "PONG"
		err = ap.WriteMessage(msg)
		if err != nil {
			return nil, err
		}
	}
}

func (ap autoPonger) WriteMessage(msg *Message) error {
	return ap.rw.WriteMessage(msg)
}

// Read all messages from r in a separate go routine. returns a channel via
// which the messages may be recieved.
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
