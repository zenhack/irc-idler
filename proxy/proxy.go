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

type session struct {
	phase
	nick string
}

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
		p.logger.Printf("Accept: (%v, %v)", conn, err)
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}
		p.acceptChan <- conn
		p.logger.Println("Sent connection.")
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
	for {
		p.logger.Println("Top of serve() loop")
		select {
		case msg, ok = <-p.client.Chan:
			eventSrc = p.client
			p.logger.Println("Got client event")
		case msg, ok = <-p.server.Chan:
			eventSrc = p.server
			p.logger.Println("Got server event")
		case clientConn := <-p.acceptChan:
			p.logger.Println("Got client connection")
			// A client connected. We boot the old one, if any:
			p.client.shutdown()

			p.client.setupClient(clientConn)

			// If we're not done with the handshake, restart the server connection too.
			if p.server.phase != readyPhase {
				p.server.shutdown()
				serverConn, err := p.dialServer()
				if err != nil {
					// Server connection failed. Boot the client and let
					// them deal with it:
					p.client.shutdown()
				}
				p.server.setupServer(serverConn)
			}
			continue
		}

		if err := msg.Validate(); err != nil {
			// TODO: report the error to the relevant party or such. (what to do if
			// it's the server?
			p.logger.Println("Got invalid message: %v\n", err)
			continue
		}
		p.logger.Printf("Got valid message: %v\n", msg)
		switch {
		case eventSrc == p.client && (!ok || msg.Command == "QUIT"):
			// Client disconnect. Shut down the connection, and if we weren't
			// done with the handshake, close te server connection too.
			p.client.shutdown()
			if p.server.phase != readyPhase {
				p.server.shutdown()
			}
		case eventSrc == p.server && !ok:
			// Server disconnect. We boot the client and start all over.
			// TODO: might be nice to attempt a reconnect with cached credentials.
			p.client.shutdown()
			p.server.shutdown()
		case eventSrc == p.client && p.client.phase == readyPhase:
			err := p.server.WriteMessage(msg)
			if err != nil {
				// server disconnect
				p.server.shutdown()
				p.client.shutdown()
			}
		case eventSrc == p.server && p.client.phase == readyPhase:
			err := p.client.WriteMessage(msg)
			if err != nil {
				// client disconnect. Make sure to log te dropped message.
				p.client.shutdown()
				p.messagelog = append(p.messagelog, msg)
			}
		case eventSrc == p.client && p.client.phase == passPhase && msg.Command == "NICK":
			// FIXME: This logic is wrong.
			p.client.session.nick = msg.Params[0]
			p.server.session.nick = msg.Params[0]
			err := p.server.WriteMessage(msg)
			if err != nil {
				p.logger.Printf("Failed to write message to server: %v\n", err)
				p.client.shutdown()
				p.server.shutdown()
				continue
			}
			p.client.phase = userPhase
		case eventSrc == p.client && p.client.phase == userPhase && msg.Command == "USER":
			// FIXME: this is dubious at best
			// user has sent the last handshake message. First give them the welcome:
			err := p.client.WriteMessage(&irc.Message{
				Command: irc.RPL_WELCOME,
				Params:  []string{p.server.session.nick},
			})
			if err != nil {
				p.client.shutdown()
				continue
			}

			// then replay the message log:
			for _, v := range p.messagelog {
				err = p.client.WriteMessage(v)
				if err != nil {
					// client disconnect; back to logging.
					p.client.shutdown()
					break
				}
			}
			if err == nil {
				// replaying the log completed successfully; clear it and
				// move the client to the next phase.
				p.messagelog = p.messagelog[:0]
				p.client.phase = readyPhase
			}
		case eventSrc == p.server && p.client.phase == disconnectedPhase:
			p.messagelog = append(p.messagelog, msg)
		}
	}
}
