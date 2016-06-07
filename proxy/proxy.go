package proxy

import (
	"io"
	"io/ioutil"
	"log"
	"net"
	"time"
	"zenhack.net/go/irc-idler/irc"
)

const (
	// phases of connection setup
	disconnectedPhase phase = iota // No tcp connection
	passPhase                      // Server waiting for PASS (or NICK) command
	nickPhase                      // Server waiting for NICK command
	userPhase                      // Server waiting for USER command
	welcomePhase                   // Client waiting for RPL_WELCOME
	readyPhase                     // Handshake complete
)

type phase int

type Proxy struct {
	// listens for client connections:
	listener net.Listener

	// Incomming connections from acceptLoop:
	acceptChan chan net.Conn

	client     *connection
	server     *connection
	addr       string // address of IRC server to connect to.
	err        error
	messagelog []*irc.Message // IRC messages recieved while client is disconnected.

	// Nick on the server. Not always set, only used by the reconnecting. Basically
	// a hack to be able to give the user the right name in the welcome message on
	// reconnect:
	nick string

	logger *log.Logger // Informational logging (nothing to do with messagelog).
}

type connection struct {
	io.Closer
	irc.ReadWriter
	Chan <-chan *irc.Message
	session
}

// Information about the state of the connection. Note that we store one of these
// indepentently for both client and server; their views may not always align.
type session struct {
	phase
	nick string // User's current nick.
}

// Tear down the connection. If it is not currently active, this is a noop.
func (c *connection) shutdown() {
	if c == nil || c.Closer == nil || c.Chan == nil {
		return
	}
	c.Close()

	// Make sure the message queue is empty, otherwise we'll leak the goroutine
	for ok := true; c.Chan != nil && ok; {
		_, ok = <-c.Chan
	}

	*c = connection{}
}

// Create a new proxy.
//
// parameters:
//
// `l` will be used to listen for client connections.
// `addr` is the tcp address of the server
// `logger`, if non-nil, will be used for informational logging. Note that the logging
//  preformed is very noisy; it is mostly meant for debugging.
func NewProxy(l net.Listener, addr string, logger *log.Logger) *Proxy {
	if logger == nil {
		logger = log.New(ioutil.Discard, log.Prefix(), log.Flags())
	}
	return &Proxy{
		listener:   l,
		addr:       addr,
		client:     &connection{},
		server:     &connection{},
		logger:     logger,
		acceptChan: make(chan net.Conn),
	}
}

func (p *Proxy) Run() {
	go p.acceptLoop()
	p.serve()
}

func (c *connection) setup(conn net.Conn) {
	c.Closer = conn
	c.ReadWriter = irc.NewReadWriter(conn)
	c.Chan = irc.ReadAll(c)
	c.phase = passPhase
}

func (p *Proxy) acceptLoop() {
	for {
		conn, err := p.listener.Accept()
		p.logger.Printf("acceptLoop(): Accept: (%v, %v)", conn, err)
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}
		p.acceptChan <- conn
		p.logger.Println("acceptLoop(): Sent connection.")
	}
}

func (p *Proxy) dialServer() (net.Conn, error) {
	return net.Dial("tcp", p.addr)
}

func (p *Proxy) serve() {
	for {
		p.logger.Println("serve(): Top of loop")
		select {
		case msg, ok := <-p.client.Chan:
			p.logger.Println("serve(): Got client event")
			p.handleClientEvent(msg, ok)
		case msg, ok := <-p.server.Chan:
			p.logger.Println("serve(): Got server event")
			p.handleServerEvent(msg, ok)
		case clientConn := <-p.acceptChan:
			p.logger.Println("serve(): Got client connection")
			// A client connected. We boot the old one, if any:
			p.client.shutdown()

			p.client.setup(clientConn)

			// If we're not done with the handshake, restart the server connection too.
			if p.server.phase != readyPhase {
				p.server.shutdown()
				serverConn, err := p.dialServer()
				if err != nil {
					// Server connection failed. Boot the client and let
					// them deal with it:
					p.client.shutdown()
				}
				p.server.setup(serverConn)
			}
			continue
		}
	}
}

func (p *Proxy) handleClientEvent(msg *irc.Message, ok bool) {
	if ok {
		p.logger.Printf("handleClientEvent(): Recieved message: %q\n", msg)
		if err := msg.Validate(); err != nil {
			p.client.WriteMessage((*irc.Message)(err))
			p.dropClient()
		}
	}

	phase := &p.client.phase
	switch {
	case !ok || msg.Command == "QUIT":
		p.dropClient()
	case *phase == passPhase && msg.Command == "PASS":
		if p.server.phase == passPhase {
			// FIXME: how do we advance the server phase? We shouldn't assume
			// the server does this automatically.
			if err := p.server.WriteMessage(msg); err != nil {
				p.reset()
				return
			}
		}
		*phase = nickPhase
	case *phase == passPhase && msg.Command == "NICK":
		*phase = nickPhase
		fallthrough
	case (*phase == nickPhase || *phase == userPhase) && msg.Command == "NICK":
		// FIXME: we should check if the server is done with the handshake and thinks we
		// have a different nick.
		if err := p.server.WriteMessage(msg); err != nil {
			p.reset()
		}
		*phase = userPhase
		// NOTE: we do *not* set the session's nick field now; that
		// happens when the server replies.
		if p.server.phase == readyPhase {
			// Server already thinks we're done; it won't send the welcome message,
			// so we need to do it ourselves.
			err := p.client.WriteMessage(&irc.Message{
				Command: irc.RPL_WELCOME,
				Params:  []string{p.server.session.nick},
			})
			if err != nil {
				p.dropClient()
			}
			*phase = readyPhase
			p.replayLog()
		}
	case *phase == userPhase && msg.Command == "USER":
		if err := p.server.WriteMessage(msg); err != nil {
			p.reset()
		}
		*phase = welcomePhase
	case *phase == readyPhase:
		// TODO: we should restrict the list of commands used here to known-safe.
		// We also need to inspect a lot of these and adjust our own state.
		if err := p.server.WriteMessage(msg); err != nil {
			p.reset()
		}
	}
}

func (p *Proxy) handleServerEvent(msg *irc.Message, ok bool) {
	phase := &p.server.phase
	session := &p.server.session
	if ok {
		if err := msg.Validate(); err != nil {
			p.logger.Printf("handleServerEvent(): Got an invalid message"+
				"from server: %q (error: %q), disconnecting.\n", msg, err)
			p.reset()
		}
		p.logger.Printf("handleServertEvent(): RecievedMessage: %q\n", msg)
	}
	switch {
	case !ok:
		// Server disconnect. We boot the client and start all over.
		// TODO: might be nice to attempt a reconnect with cached credentials.
		p.reset()
	case msg.Command == "PING":
		msg.Prefix = ""
		msg.Command = "PONG"
		if err := p.server.WriteMessage(msg); err != nil {
			p.reset()
		}
	case msg.Command == irc.RPL_WELCOME:
		session.nick = msg.Params[0]
		*phase = readyPhase
		if err := p.client.WriteMessage(msg); err != nil {
			p.dropClient()
			return
		}
		p.client.phase = readyPhase
		p.client.session.nick = session.nick
	case p.client.phase != readyPhase:
		p.logMessage(msg)
	default:
		// TODO: be a bit more methodical; there's probably a pretty finite list
		// of things that can come through, and we want to make sure nothing is
		// going to get us out of sync with the client.
		if err := p.client.WriteMessage(msg); err != nil {
			p.logMessage(msg)
			p.dropClient()
		}
	}
}

// Disconnect the client. If the handshake isn't done, disconnect the server too.
func (p *Proxy) dropClient() {
	p.client.shutdown()
	if p.server.phase != readyPhase {
		p.server.shutdown()
	}
}

func (p *Proxy) reset() {
	p.client.shutdown()
	p.server.shutdown()
}

func (p *Proxy) logMessage(m *irc.Message) {
	p.messagelog = append(p.messagelog, m)
}

func (p *Proxy) replayLog() {
	for _, v := range p.messagelog {
		err := p.client.WriteMessage(v)
		if err != nil {
			p.dropClient()
			return
		}
	}
	p.messagelog = p.messagelog[:0]
}
