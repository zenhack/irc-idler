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
	"net"
	"zenhack.net/go/irc-idler/irc"
)

type stateFn func(p *Proxy) stateFn

type Proxy struct {
	// listens for client connections:
	listener net.Listener
	//used by asyncAccept; read the comments there:
	acceptChan <-chan net.Conn

	client *connection
	server *connection
	addr   string // address of IRC server to connect to.
	err    error
	log    []*irc.Message // messages recieved while client is disconnected.
}

type connection struct {
	io.Closer
	irc.ReadWriter
	Chan <-chan *irc.Message
}

func NewProxy(l net.Listener, addr string) *Proxy {
	return &Proxy{
		listener: l,
		addr:     addr,
		client:   &connection{},
		server:   &connection{},
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

func start(p *Proxy) stateFn {
	p.acceptClient()
	if p.err != nil {
		return cleanUp
	}
	p.dialServer()
	if p.err != nil {
		return cleanUp
	}
	return withClient
}

func cleanUp(p *Proxy) stateFn {
	if p.client.Closer != nil {
		p.client.Close()
	}
	if p.server.Closer != nil {
		p.server.Close()
	}
	return nil
}

func withClient(p *Proxy) stateFn {
	select {
	case msg, ok := <-p.server.Chan:
		if !ok {
			return cleanUp
		}
		p.err = p.client.WriteMessage(msg)
	case msg, ok := <-p.client.Chan:
		if !ok {
			return sansClient
		}
		switch msg.Command {
		case "QUIT":
			p.client.Close()
			return sansClient
		default:
			p.err = p.server.WriteMessage(msg)
		}
	}
	if p.err != nil {
		return cleanUp
	}
	return withClient
}

func sansClient(p *Proxy) stateFn {
	p.asyncAccept()

	select {
	case msg, ok := <-p.server.Chan:
		if !ok {
			return cleanUp
		}
		p.log = append(p.log, msg)
		return sansClient
	case conn := <-p.acceptChan:
		p.acceptChan = nil // reset for next time
		p.client.Closer = conn
		p.client.ReadWriter = irc.NewReadWriter(conn)
		p.client.Chan = irc.ReadAll(p.client)
		// TODO: dump log to the client
		return withClient
	}
}
