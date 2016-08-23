package proxy

// Support code for tests.
//
// This is incomplete, but the main thing this is intended to support
// is doing things like (pseudocode):
//
// Trigger(ConnectClient)
// Expect(timeout, ConnectServer)
// Trigger(FromClient("NICK bob"))
// Expect(ToServer("NICK bob"))
// ...
//
// i.e. We verify traces of expected behavior.

import (
	"errors"
	"fmt"
	"reflect"
	"time"
	"zenhack.net/go/irc-idler/irc"
)

var (
	Timeout              = errors.New("Timeout")
	UnexpectedDisconnect = errors.New("Unexpected Disconnect")
	ExpectedDisconnect   = errors.New("Expected Disconnect")
)

type ChanRWC struct {
	Send chan<- *irc.Message
	Recv <-chan *irc.Message
}

func (c *ChanRWC) Close() error {
	// TODO: this is wrong. we need to work out a way for
	// it to be safe to close one of these from both ends.
	close(c.Send)
	return nil
}

func (c *ChanRWC) ReadMessage() (*irc.Message, error) {
	msg, ok := <-c.Recv
	if !ok {
		return nil, io.EOF
	}
	return msg, nil
}

type ProxyAction interface {
	Expect(state *ProxyState, timeout time.Duration) error
}

type ProxyState struct {
	ToServer, ToClient     <-chan *irc.Message
	FromServer, FromClient chan<- *irc.Message
	ConnectClient          chan<- *irc.ReadWriteCloser
	ConnectServer          <-chan *irc.ReadWriteCloser
}

type (
	ToClient   irc.Message
	ToServer   irc.Message
	DropClient struct{}
	DropServer struct{}
)

type MsgsDiffer struct {
	Expected, Actual *irc.Message
}

func (e *MsgsDiffer) Error() string {
	return fmt.Sprintf("Messages differ; epected %q but got %q.",
		e.Expected,
		e.Actual,
	)
}

func toMsgExpect(expected *irc.Message, msgChan <-chan *irc.Message, timeout time.Duration) error {
	select {
	case <-time.After(timeout):
		return Timeout
	case actual, ok := <-msgChan:
		if !ok {
			return UnexpectedDisconnect
		}
		if !reflect.DeepEqual(expected, actual) {
			return &MsgsDiffer{
				Expected: expected,
				Actual:   actual,
			}
		}
	}
	return nil
}

func dropExpect(msgChan <-chan *irc.Message, timeout time.Duration) error {
	select {
	case <-time.After(timeout):
		return Timeout
	case msg, ok := <-msgChan:
		if ok {
			return ExpectedDisconnect
		}
	}
}

func (dc DropClient) Expect(state *ProxyState, timeout time.Duration) error {
	dropExpect(state.ToCleint, timeout)
}

func (ds DropServer) Expect(state *ProxyState, timeout time.Duration) error {
	dropExpect(state.ToServer, timeout)
}

func (ts *ToServer) Expect(state *ProxyState, timeout time.Duration) error {
	return toMsgExpect((*irc.Message)(ts), state.ToServer, timeout)
}

func (tc *ToClient) Expect(state *ProxyState, timeout time.Duration) error {
	return toMsgExpect((*irc.Message)(tc), state.ToClient, timeout)
}

func Expect(state *ProxyState, timeout time.Duration, actions ...ProxyAction) error {
	for _, action := range actions {
		if err := action.Expect(state, timeout); err != nil {
			return err
		}
	}
	return nil
}
