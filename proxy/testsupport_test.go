package proxy

// Support code for tests.
//
// The main thing this is intended to support verifying traces of
// expected behavior, e.g.
//
// TraceTest(t, ExpectMany{
// 	ClientConnect{},
// 	ConnectServer{},
// 	&FromClient{Command: "NICK", Params: []string{"bob"}},
// 	&ToServer{Command: "NICK", Params: []string{"bob"}},
// 	...
// })

// TODO: general concern: we're using io.EOF in a lot of places where it's
// arguably inappropriate

import (
	"errors"
	"fmt"
	"github.com/Sirupsen/logrus"
	"golang.org/x/net/context"
	"io"
	"testing"
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
	context.Context
	context.CancelFunc
}

func (c *ChanRWC) Close() error {
	c.CancelFunc()
	return nil
}

func (c *ChanRWC) ReadMessage() (*irc.Message, error) {
	select {
	case msg := <-c.Recv:
		return msg, nil
	case <-c.Context.Done():
		return nil, c.Context.Err()
	}
}

func (c *ChanRWC) WriteMessage(msg *irc.Message) error {
	select {
	case c.Send <- msg:
		return nil
	case <-c.Context.Done():
		return c.Context.Err()
	}
}

type NestedError struct {
	Index int
	Ctx   interface{}
	Err   error
}

func ForwardC2S(msg *irc.Message) ProxyAction {
	return ExpectMany{
		(*FromClient)(msg),
		(*ToServer)(msg),
	}
}

func ForwardS2C(msg *irc.Message) ProxyAction {
	return ExpectMany{
		(*FromServer)(msg),
		(*ToClient)(msg),
	}
}

func (e *NestedError) Error() string {
	return fmt.Sprintf("Error in action #%d (%v):\n\n %v", e.Index, e.Ctx, e.Err)
}

type ExpectMany []ProxyAction

func (e ExpectMany) Expect(state *ProxyState, timeout time.Duration) error {
	for i, v := range e {
		err := v.Expect(state, timeout)
		if err != nil {
			return &NestedError{
				Index: i,
				Ctx:   v,
				Err:   err,
			}
		}
	}
	return nil
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

type ProxyAction interface {
	Expect(state *ProxyState, timeout time.Duration) error
}

type ProxyState struct {
	ToServer, ToClient                 <-chan *irc.Message
	FromServer, FromClient             chan<- *irc.Message
	ConnectClient, ConnectServer       chan<- irc.ReadWriteCloser
	ConnectRequests                    <-chan struct{}
	DropClient, DropServer             <-chan struct{}
	ClientDisconnect, ServerDisconnect context.CancelFunc
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

func (a *DropClient) String() string       { return "&DropClient{}" }
func (a *ClientConnect) String() string    { return "&ClientConnect{}" }
func (a *ClientDisconnect) String() string { return "&ClientDisconnect{}" }
func (a *ConnectServer) String() string    { return "&ConnectServer{}" }
func (a *ServerDisconnect) String() string { return "&ServerDisconnect{}" }
func (a *DropServer) String() string       { return "&DropServer{}" }

func (m *ToClient) String() string   { return "&ToClient(" + (*irc.Message)(m).String() + ")" }
func (m *ToServer) String() string   { return "&ToServer(" + (*irc.Message)(m).String() + ")" }
func (m *FromClient) String() string { return "&FromClient(" + (*irc.Message)(m).String() + ")" }
func (m *FromServer) String() string { return "&FromServer(" + (*irc.Message)(m).String() + ")" }

type MsgsDiffer struct {
	Expected, Actual *irc.Message
}

func (e *MsgsDiffer) Error() string {
	return fmt.Sprintf("Messages differ; epected %q but got %q.",
		e.Expected,
		e.Actual,
	)
}

func NewRWC(oldCtx context.Context) (to, from chan *irc.Message, rwc *ChanRWC) {
	to = make(chan *irc.Message)
	from = make(chan *irc.Message)
	ctx, cancel := context.WithCancel(oldCtx)
	rwc = &ChanRWC{
		Send:       to,
		Recv:       from,
		Context:    ctx,
		CancelFunc: cancel,
	}
	return
}

func (cd ClientDisconnect) Expect(state *ProxyState, timeout time.Duration) error {
	state.ClientDisconnect()
	return nil
}

func (sd ServerDisconnect) Expect(state *ProxyState, timeout time.Duration) error {
	state.ServerDisconnect()
	return nil
}

func (cc ClientConnect) Expect(state *ProxyState, timeout time.Duration) error {
	toClient, fromClient, rwc := NewRWC(context.TODO())

	select {
	case state.ConnectClient <- rwc:
		state.ToClient = toClient
		state.FromClient = fromClient
		state.DropClient = rwc.Context.Done()
		state.ClientDisconnect = rwc.CancelFunc
		return nil
	case <-time.After(timeout):
		return Timeout
	}
}

func (cs ConnectServer) Expect(state *ProxyState, timeout time.Duration) error {
	select {
	case <-time.After(timeout):
		return Timeout
	case <-state.ConnectRequests:
		toServer, fromServer, rwc := NewRWC(context.TODO())

		select {
		case <-time.After(timeout):
			return Timeout
		case state.ConnectServer <- rwc:
			state.ToServer = toServer
			state.FromServer = fromServer
			state.DropServer = rwc.Context.Done()
			state.ServerDisconnect = rwc.CancelFunc
			return nil
		}
	}
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
	case actual := <-msgChan:
		if !expected.Eq(actual) {
			return &MsgsDiffer{
				Expected: expected,
				Actual:   actual,
			}
		}
	}
	return nil
}

func dropExpect(closeChan <-chan struct{}, timeout time.Duration) error {
	select {
	case <-time.After(timeout):
		return Timeout
	case <-closeChan:
		return nil
	}
}

func (dc DropClient) Expect(state *ProxyState, timeout time.Duration) error {
	return dropExpect(state.DropClient, timeout)
}

func (ds DropServer) Expect(state *ProxyState, timeout time.Duration) error {
	return dropExpect(state.DropServer, timeout)
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

func ManyMsg(convert func(msg *irc.Message) ProxyAction, msgs []*irc.Message) ProxyAction {
	ret := make(ExpectMany, len(msgs))
	for i, v := range msgs {
		ret[i] = convert(v)
	}
	return ret
}

func ManyToClient(msgs []*irc.Message) ProxyAction {
	ret := make(ExpectMany, len(msgs))
	for i, v := range msgs {
		ret[i] = (*ToClient)(v)
	}
	return ret
}

func StartTestProxy() *ProxyState {
	connectRequests := make(chan struct{})
	connectResponses := make(chan irc.ReadWriteCloser)
	clientConns := make(chan irc.ReadWriteCloser)

	connector := &ChanConnector{
		Requests:  connectRequests,
		Responses: connectResponses,
	}

	logger := logrus.New()
	logger.Level = logrus.DebugLevel
	proxy := NewProxy(clientConns, connector, logger)
	go proxy.Run()

	return &ProxyState{
		ConnectServer:   connectResponses,
		ConnectRequests: connectRequests,
		ConnectClient:   clientConns,
	}
}

func TraceTest(t *testing.T, action ProxyAction) {
	state := StartTestProxy()
	err := action.Expect(state, time.Second)
	if err != nil {
		t.Fatal(err)
	}
}
