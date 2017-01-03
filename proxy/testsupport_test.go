package proxy

// Support code for tests.
//
// The main thing this is intended to support verifying traces of
// expected behavior, e.g.
//
// TraceTest(t, ExpectMany{
// 	ClientConnect{},
// 	ConnectServer{},
// 	FromClient(&irc.Message{Command: "NICK", Params: []string{"bob"}}),
// 	ToServer(&irc.Message{Command: "NICK", Params: []string{"bob"}}),
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
	"os"
	"testing"
	"time"
	"zenhack.net/go/irc-idler/irc"
	"zenhack.net/go/irc-idler/storage/ephemeral"
)

var (
	ErrTimeout              = errors.New("Timeout")
	ErrUnexpectedDisconnect = errors.New("Unexpected Disconnect")
	ErrExpectedDisconnect   = errors.New("Expected Disconnect")

	// The timeout passed to each Expect() call. This can be overridden
	// by setting the variable II_TEST_TIMEOUT to a string that can be
	// parsed by time.ParseDuration
	//
	// This is useful for e.g. single stepping in a debugger, as
	// otherwise the timeout prevents inspecting things.
	TimeoutLength = 10 * time.Second
)

func init() {
	pingTime = TimeoutLength / 10
	durationEnv := os.Getenv("II_TEST_TIMEOUT")
	if durationEnv == "" {
		return
	}
	duration, err := time.ParseDuration(durationEnv)
	if err != nil {
		panic(err)
	}
	TimeoutLength = duration
	pingTime = TimeoutLength / 10

}

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
		FromClient(msg),
		ToServer(msg),
	}
}

func ForwardS2C(msg *irc.Message) ProxyAction {
	return ExpectMany{
		FromServer(msg),
		ToClient(msg),
	}
}

func (e *NestedError) Error() string {
	return fmt.Sprintf("Error in action #%d (%v):\n\n %v", e.Index, e.Ctx, e.Err)
}

func ExpectFunc(label string, fn func(state *ProxyState, timeout time.Duration) error) ProxyAction {
	return expectFunc{
		fn:    fn,
		label: label,
	}
}

type expectFunc struct {
	label string
	fn    func(state *ProxyState, timeout time.Duration) error
}

func (ef expectFunc) String() string {
	return fmt.Sprintf("ExpectFunc(%q)", ef.label)
}

func (ef expectFunc) Expect(state *ProxyState, timeout time.Duration) error {
	return ef.fn(state, timeout)
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
	ToChans         [numEndpoints]<-chan *irc.Message
	FromChans       [numEndpoints]chan<- *irc.Message
	ConnectChans    [numEndpoints]chan<- irc.ReadWriteCloser
	ConnectRequests <-chan struct{}
	DropChans       [numEndpoints]<-chan struct{}
	Disconnect      [numEndpoints]context.CancelFunc
}

type Endpoint int

const (
	Client Endpoint = iota
	Server
	numEndpoints
)

func (e Endpoint) String() string {
	return map[Endpoint]string{
		Client: "Client",
		Server: "Server",
	}[e]
}

func Drop(endpoint Endpoint) ProxyAction {
	return ExpectFunc(fmt.Sprintf("Drop(%v)", endpoint),
		func(state *ProxyState, timeout time.Duration) error {
			select {
			case <-time.After(timeout):
				return ErrTimeout
			case <-state.DropChans[endpoint]:
				return nil
			}
		})
}

func Sleep(sleepTime time.Duration) ProxyAction {
	return ExpectFunc(fmt.Sprintf("Sleep(%v)", sleepTime),
		func(state *ProxyState, timeout time.Duration) error {
			time.Sleep(sleepTime)
			return nil
		})
}

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

func Disconnect(endpoint Endpoint) ProxyAction {
	return ExpectFunc(fmt.Sprintf("Disconnect(%v)", endpoint),
		func(state *ProxyState, timeout time.Duration) error {
			state.Disconnect[endpoint]()
			return nil
		})
}

func (state *ProxyState) initEndpoint(endpoint Endpoint, timeout time.Duration) error {
	to, from, rwc := NewRWC(context.TODO())

	select {
	case state.ConnectChans[endpoint] <- rwc:
		state.ToChans[endpoint] = to
		state.FromChans[endpoint] = from
		state.DropChans[endpoint] = rwc.Context.Done()
		state.Disconnect[endpoint] = rwc.CancelFunc
		return nil
	case <-time.After(timeout):
		return ErrTimeout
	}
}

func Connect(endpoint Endpoint) ProxyAction {
	return ExpectFunc(fmt.Sprintf("Connect(%v)", endpoint),
		func(state *ProxyState, timeout time.Duration) error {
			if endpoint == Server {
				// wait for the daemon to ask for a connection
				select {
				case <-time.After(timeout):
					return ErrTimeout
				case <-state.ConnectRequests:
				}
			}
			return state.initEndpoint(endpoint, timeout)
		})
}

func fromMsgExpect(msg *irc.Message, msgChan chan<- *irc.Message, timeout time.Duration) error {
	select {
	case <-time.After(timeout):
		return ErrTimeout
	case msgChan <- msg:
		return nil
	}
}

func To(endpoint Endpoint, expected *irc.Message) ProxyAction {
	label := fmt.Sprintf("To(%s, %q)", endpoint, expected)
	return ExpectFunc(label, func(state *ProxyState, timeout time.Duration) error {
		select {
		case <-time.After(timeout):
			return ErrTimeout
		case actual := <-state.ToChans[endpoint]:
			if !expected.Eq(actual) {
				return &MsgsDiffer{
					Expected: expected,
					Actual:   actual,
				}
			}
		}
		return nil
	})
}

func From(endpoint Endpoint, msg *irc.Message) ProxyAction {
	label := fmt.Sprintf("From(%v, %q)", endpoint, msg)
	return ExpectFunc(label, func(state *ProxyState, timeout time.Duration) error {
		return fromMsgExpect(msg, state.FromChans[endpoint], timeout)
	})
}

func UnorderedTo(endpoint Endpoint, expected []*irc.Message) ProxyAction {
	return ExpectFunc(fmt.Sprintf("UnorderedTo(%v, %v)", endpoint, expected),
		func(state *ProxyState, timeout time.Duration) error {
			have := make(map[string]bool, len(expected))
			for range expected {
				select {
				case <-time.After(timeout):
					return ErrTimeout
				case msg := <-state.ToChans[endpoint]:
					have[msg.String()] = true
				}
			}
			for _, msg := range expected {
				if !have[msg.String()] {
					return fmt.Errorf(
						"Did not receive expected message: %v", msg)
				}
			}
			return nil
		})
}

func ToServer(expected *irc.Message) ProxyAction { return To(Server, expected) }
func ToClient(expected *irc.Message) ProxyAction { return To(Client, expected) }
func FromServer(msg *irc.Message) ProxyAction    { return From(Server, msg) }
func FromClient(msg *irc.Message) ProxyAction    { return From(Client, msg) }

func ManyMsg(convert func(msg *irc.Message) ProxyAction, msgs []*irc.Message) ProxyAction {
	ret := make(ExpectMany, len(msgs))
	for i, v := range msgs {
		ret[i] = convert(v)
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
	proxy := NewProxy(
		logger,
		ephemeral.NewStore(),
		clientConns,
		connector)
	go proxy.Run()

	state := &ProxyState{
		ConnectRequests: connectRequests,
	}
	state.ConnectChans[Client] = clientConns
	state.ConnectChans[Server] = connectResponses
	return state
}

func TraceTest(t *testing.T, action ProxyAction) {
	state := StartTestProxy()
	err := action.Expect(state, TimeoutLength)
	if err != nil {
		t.Fatal(err)
	}
}
