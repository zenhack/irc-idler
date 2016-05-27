package proxy

// Proxy IRC server. It's a state machine, with a similar implementation
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
	listener net.Listener
	client   *connection
	server   *connection
	addr     string // address of IRC server to connect to.
	err      error
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

func start(p *Proxy) stateFn {
	clientConn, err := p.listener.Accept()
	if err != nil {
		p.err = err
		return cleanUp
	}

	serverConn, err := net.Dial("tcp", p.addr)
	if err != nil {
		// TODO: try again? backoff?
		p.err = err
		return cleanUp
	}
	p.client.Closer = clientConn
	p.server.Closer = serverConn

	p.client.ReadWriter = irc.NewReadWriter(clientConn)
	p.server.ReadWriter = irc.AutoPong(irc.NewReadWriter(serverConn))

	p.client.Chan = irc.ReadAll(p.client)
	p.server.Chan = irc.ReadAll(p.server)
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
	// TODO: This whole thing is super messy and broken in important ways.
	log := []*irc.Message{}
	connChan := make(chan net.Conn)
	go func() {
		for {
			conn, err := p.listener.Accept()
			if err == nil {
				connChan <- conn
				break
			}
		}
	}()
	select {
	case msg, ok := <-p.server.Chan:
		if !ok {
			return cleanUp
		}
		log = append(log, msg)
	case conn := <-connChan:
		p.client.Closer = conn
		p.client.ReadWriter = irc.NewReadWriter(conn)
		p.client.Chan = irc.ReadAll(p.client)
		// TODO: dump log to the client
		return withClient
	}
	return sansClient
}
