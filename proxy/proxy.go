package proxy

// Proxy IRC server. It's a state machine, with a similar implementation to
// the lexer Rob Pike describes here:
//
//     https://youtu.be/WIxQ-KvzwpM?t=735
//
// Briefly, each state is a stateFn, which takes the proxy as an argument, and
// returns the next state.

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
	passPhase                      // Waiting for PASS (or NICK) command
	nickPhase                      // Waiting for NICK command
	userPhase                      // Waiting for USER command
	readyPhase                     // handshake complete
)

type phase int

type stateFn func(p *Proxy) stateFn

type Proxy struct {
	// listens for client connections:
	listener net.Listener
	//used by asyncAccept; read the comments there:
	acceptChan <-chan net.Conn

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
	phase
}

func (c *connection) shutdown() {
	c.Close()

	// Make sure the message queue is empty, otherwise we'll leak the goroutine
	for ok := true; c.Chan != nil && ok; {
		_, ok = <-c.Chan
	}

	*c = &connection{}
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
		listener: l,
		addr:     addr,
		client:   &connection{},
		server:   &connection{},
		logger:   logger,
	}
}

func (p *Proxy) Run() {
	go p.acceptLoop()
	p.serve()
}

func (c *connection) setupClient(conn net.Conn) {
	c.Closer = conn
	c.ReadWriter = irc.NewReadWriter(conn)
	c.Chan = irc.ReadAll(c)
	c.phase = passPhase
}

func (c *connection) setupServer(conn net.Conn) {
	c.Closer = conn
	c.ReadWriter = irc.AutoPong(irc.NewReadWriter(conn))
	c.Chan = irc.ReadAll(c)
	c.phase = passPhase
}

func (p *Proxy) acceptLoop() {
	for {
		conn, err := p.listener.Accept()
		if err != nil {
			time.Sleep(0.1 * time.Second)
			continue
		}
		p.acceptChan <- conn
	}
}

func (p *Proxy) dialServer() (net.Conn, error) {
	return net.Dial("tcp", p.addr)
}

func (p *Proxy) serve() {
	var (
		msg      *irc.Message
		ok       bool
		eventSrc *connection
	)
	p.asyncAccept()
	for {
		select {
		case msg, ok = <-p.client.Chan:
			eventSrc = p.client
		case msg, ok = <-p.server.Chan:
			eventSrc = p.server
		case clientConn := <-p.acceptChan:
			// A client connected. We boot the old one, if any:
			p.client.shutdown()

			p.client.setupClient(clientConn)

			// If we were totally disconnected, we need to reconnect to the server:
			if p.server.phase == disconnectedPhase {
				serverConn, err := p.dialServer()
				if err != nil {
					// Server connection failed. Boot the client and let
					// them deal with it:
					p.client.shutdown()
					p.server.shutdown()
				}
				p.server.setupServer(serverConn)
			}
			continue
		}

		if err := msg.Validate(); err != nil {
			// TODO: report the error to the relevant party or such. (what to do if
			// it's the server?
			p.logger.Println("Recieved Invalid message: %v\n", err)
			continue
		}
		switch {
		case eventSrc == p.client && !ok:
			// Client disconnected
			p.client.shutdown()
		case eventSrc == p.server && !ok:
			// Server disconnect. We boot the client and start all over.
			// TODO: might be nice to attempt a reconnect with cached credentials.
			p.client.shutdown()
			p.server.shutdown()
		case eventSrc == p.client && p.client.phase == passPhase:
			switch msg.Command {
			case "PASS":
				p.logger.Println("TODO (PASS): handle PASS messages.")
			case "NICK":
				p.logger.Println("TODO (PASS): handle NICK messages.")
			}
		}
	}
}

// State: starting up. Will accept a client connection and then connect to
// the server.
func start(p *Proxy) stateFn {
	p.logger.Println("Entering start state.")
	p.acceptClient()
	if p.err != nil {
		return cleanUp
	}
	p.dialServer()
	if p.err != nil {
		return cleanUp
	}
	return relaying
}

// State: shutting down. Clean up resources and exit.
func cleanUp(p *Proxy) stateFn {
	p.logger.Println("Entering cleanUp state.")
	if p.client.Closer != nil {
		p.client.Close()
	}
	if p.server.Closer != nil {
		p.server.Close()
	}
	return nil
}

// State: connected to both client and server, relaying messages
// between the two.
func relaying(p *Proxy) stateFn {
	p.logger.Println("Entering relaying state.")
	select {
	case msg, ok := <-p.server.Chan:
		if !ok {
			return cleanUp
		}
		p.err = p.client.WriteMessage(msg)
	case msg, ok := <-p.client.Chan:
		if !ok {
			return logging
		}
		switch msg.Command {
		case "QUIT":
			p.client.Close()
			return logging
		default:
			p.err = p.server.WriteMessage(msg)
		}
	}
	if p.err != nil {
		return cleanUp
	}
	return relaying
}

// State: client is disconnected; logging messages from the server
// for later delivery.
func logging(p *Proxy) stateFn {
	p.logger.Println("Entering logging state.")
	p.asyncAccept()

	select {
	case msg, ok := <-p.server.Chan:
		if !ok {
			return cleanUp
		}
		p.messagelog = append(p.messagelog, msg)
		return logging
	case conn := <-p.acceptChan:
		p.acceptChan = nil // reset for next time
		p.client.Closer = conn
		p.client.ReadWriter = irc.NewReadWriter(conn)
		p.client.Chan = irc.ReadAll(p.client)
		return reconnecting
	}
}

// State: client has established a new tcp connection, and is trying to do the
// handshake again. From the server's standpoint we're already logged in, so we
// throw out these messages.
func reconnecting(p *Proxy) stateFn {
	p.logger.Println("Entering reconnecting state.")

	select {
	case msg, ok := <-p.server.Chan:
		if !ok {
			return cleanUp
		}
		// Keep logging things until we've actually finished the reconnect:
		p.messagelog = append(p.messagelog, msg)
		return reconnecting
	case msg, ok := <-p.client.Chan:
		if !ok {
			return logging
		}
		switch msg.Command {
		case "QUIT":
			p.client.Close()
			return logging
		case "NICK":
			if len(msg.Params) == 0 {
				p.logger.Println("Client sent NICK message with no params.")
			} else {
				p.nick = msg.Params[0]
			}
			return reconnecting
		case "USER":
			// user has sent the last handshake message.
			p.err = p.client.WriteMessage(&irc.Message{
				Command: irc.RPL_WELCOME,
				Params:  []string{p.nick},
			})
			if p.err != nil {
				// client disconnect
				return logging
			}
			return dumpLog
		default:
			return reconnecting
		}
	}
}

// State: client has reconnected, dumping the log
func dumpLog(p *Proxy) stateFn {
	p.logger.Println("Entering dumpLog state.")
	for _, v := range p.messagelog {
		p.err = p.client.WriteMessage(v)
		if p.err != nil {
			// probably a client disconnect; back to logging mode.
			return logging
		}
	}
	p.messagelog = p.messagelog[:0]
	return relaying
}
