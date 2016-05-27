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
	listener                 net.Listener
	clientClose, serverClose io.Closer
	clientIO, serverIO       irc.ReadWriter
	clientChan, serverChan   <-chan *irc.Message
	addr                     string // address of IRC server to connect to.
	err                      error
}

func NewProxy(l net.Listener, addr string) *Proxy {
	return &Proxy{
		listener: l,
		addr:     addr,
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
	p.clientClose = clientConn
	p.serverClose = serverConn

	p.clientIO = irc.NewReadWriter(clientConn)
	p.serverIO = irc.AutoPong(irc.NewReadWriter(serverConn))

	p.clientChan = irc.ReadAll(p.clientIO)
	p.serverChan = irc.ReadAll(p.serverIO)
	return withClient
}

func cleanUp(p *Proxy) stateFn {
	if p.clientClose != nil {
		p.clientClose.Close()
	}
	if p.serverClose != nil {
		p.serverClose.Close()
	}
	return nil
}

func withClient(p *Proxy) stateFn {
	select {
	case msg, ok := <-p.serverChan:
		if !ok {
			return cleanUp
		}
		p.err = p.clientIO.WriteMessage(msg)
	case msg, ok := <-p.clientChan:
		if !ok {
			return cleanUp
		}
		p.err = p.serverIO.WriteMessage(msg)
	}
	if p.err != nil {
		return cleanUp
	}
	return withClient
}
