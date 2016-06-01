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
	"zenhack.net/go/irc-idler/irc"
)

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

	logger *log.Logger // Informational logging (nothing to do with messagelog).
}

type connection struct {
	io.Closer
	irc.ReadWriter
	Chan <-chan *irc.Message
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

func (p *Proxy) Run() error {
	for state := start; state != nil; {
		state = state(p)
	}
	return p.err
}

func (p *Proxy) acceptClient() {
	clientConn, err := p.listener.Accept()
	p.err = err
	if err != nil {
		return
	}
	p.client.Closer = clientConn
	p.client.ReadWriter = irc.NewReadWriter(clientConn)
	p.client.Chan = irc.ReadAll(p.client)
}

// Accept a client connection in a separate goroutine.
//
// if p.acceptChan is not nil, this is a noop. otherwise,
// it create a new channel, assign it to p.acceptChan, and launch a
// separate goroutine listening for a client connection. When
// one is recieved, it will be send down p.acceptChan, and the
// channel will be closed.
func (p *Proxy) asyncAccept() {
	if p.acceptChan != nil {
		// There's one of these already running; ignore.
		return
	}
	acceptChan := make(chan net.Conn)
	go func() {
		conn, err := p.listener.Accept()
		p.err = err
		if err == nil {
			acceptChan <- conn
		}
		close(acceptChan)
	}()
	p.acceptChan = acceptChan
}

func (p *Proxy) dialServer() {
	serverConn, err := net.Dial("tcp", p.addr)
	p.err = err
	if err != nil {
		return
	}
	p.server.Closer = serverConn
	p.server.ReadWriter = irc.AutoPong(irc.NewReadWriter(serverConn))
	p.server.Chan = irc.ReadAll(p.server)
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
		return dumpLog
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
