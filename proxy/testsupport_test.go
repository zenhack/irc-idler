package proxy

// Support code for tests.
//
// This is incomplete, but the main thing this is intended to support
// is doing things like:
//
// Expect(state, timeout,
// 	&ConnectClient{},
// 	&ConnectServer{},
// 	&FromClient{Command: "NICK", Params: []string{"bob"}},
// 	&ToServer{Command: "NICK", Params: []string{"bob"}},
// 	...
//
// i.e. We verify traces of expected behavior.

// TODO: general concern: we're using io.EOF in a lot of places where it's
// arguably inappropriate

import (
	"errors"
	"fmt"
	"io"
	//"github.com/Sirupsen/logrus"
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
	Send      chan<- *irc.Message
	Recv      <-chan *irc.Message
	CloseChan chan struct{}
}

type ChanConnector struct {
	Requests  chan<- struct{}
	Responses <-chan irc.ReadWriteCloser
}

func (c *ChanConnector) Connect() (irc.ReadWriteCloser, error) {
	c.Requests <- struct{}{}
	ret, ok := <-c.Responses
	if !ok {
		return nil, io.EOF
	}
	return ret, nil
}

func NewChanConn() (*ChanRWC, *ChanRWC) {
	to := make(chan *irc.Message)
	from := make(chan *irc.Message)
	closeChan := make(chan struct{}, 1)

	left := &ChanRWC{
		Send:      to,
		Recv:      from,
		CloseChan: closeChan,
	}
	right := &ChanRWC{
		Send:      from,
		Recv:      to,
		CloseChan: closeChan,
	}
	return left, right
}

func (c *ChanRWC) Close() error {
	c.CloseChan <- struct{}{}
	return nil
}

func (c *ChanRWC) ReadMessage() (*irc.Message, error) {
	select {
	case msg := <-c.Recv:
		return msg, nil
	case <-c.CloseChan:
		return nil, io.EOF
	}
}

func (c *ChanRWC) WriteMessage(msg *irc.Message) error {
	select {
	case c.Send <- msg:
		return nil
	case <-c.CloseChan:
		return io.EOF
	}
}

type ProxyAction interface {
	Expect(state *ProxyState, timeout time.Duration) error
}

type ProxyState struct {
	ToServer, ToClient     <-chan *irc.Message
	FromServer, FromClient chan<- *irc.Message
	ConnectClient          chan<- irc.ReadWriteCloser
	ConnectServer          <-chan irc.ReadWriteCloser
	ConnectRequests        <-chan struct{}
	ClientClose            chan struct{}
	ServerClose            chan struct{}
}

type (
	ToClient         irc.Message
	ToServer         irc.Message
	FromClient       irc.Message
	FromServer       irc.Message
	DropClient       struct{}
	DropServer       struct{}
	ClientConnect    struct{}
	ClientDisconnect struct{}
	ConnectServer    struct{}
	ServerDisconnect struct{}
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

func fromMsgExpect(msg *irc.Message, msgChan chan<- *irc.Message, timeout time.Duration) error {
	select {
	case <-time.After(timeout):
		return Timeout
	case msgChan <- msg:
		return nil
	}
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
	case _, ok := <-msgChan:
		if ok {
			return ExpectedDisconnect
		}
		return nil
	}
}

func (dc DropClient) Expect(state *ProxyState, timeout time.Duration) error {
	return dropExpect(state.ToClient, timeout)
}

func (ds DropServer) Expect(state *ProxyState, timeout time.Duration) error {
	return dropExpect(state.ToServer, timeout)
}

func (ts *ToServer) Expect(state *ProxyState, timeout time.Duration) error {
	return toMsgExpect((*irc.Message)(ts), state.ToServer, timeout)
}

func (tc *ToClient) Expect(state *ProxyState, timeout time.Duration) error {
	return toMsgExpect((*irc.Message)(tc), state.ToClient, timeout)
}

func (fs *FromServer) Expect(state *ProxyState, timeout time.Duration) error {
	return fromMsgExpect((*irc.Message)(fs), state.FromServer, timeout)
}

func (fc *FromClient) Expect(state *ProxyState, timeout time.Duration) error {
	return fromMsgExpect((*irc.Message)(fc), state.FromClient, timeout)
}

func Expect(state *ProxyState, timeout time.Duration, actions ...ProxyAction) error {
	for _, action := range actions {
		if err := action.Expect(state, timeout); err != nil {
			return err
		}
	}
	return nil
}

func StartTestProxy() *ProxyState {
	connectRequests := make(chan struct{})
	connectResponses := make(chan irc.ReadWriteCloser)
	clientConns := make(chan irc.ReadWriteCloser)

	connector := &ChanConnector{
		Requests:  connectRequests,
		Responses: connectResponses,
	}

	proxy := NewProxy(clientConns, connector, nil) // TODO: pass a logger.
	go proxy.Run()

	return &ProxyState{
		ConnectServer:   connectResponses,
		ConnectRequests: connectRequests,
		ConnectClient:   clientConns,
	}
}
