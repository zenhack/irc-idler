package main

// Proxy IRC server. It's a state machine, with a similar implementation
// the lexer Rob Pike describes here:
//
//     https://youtu.be/WIxQ-KvzwpM?t=735
//
// Briefly, each state is a stateFn, which takes the proxy as an argument, and
// returns the next state.

import (
	"log"
	"net"
	"zenhack.net/go/irc-idler/irc"
)

type stateFn func(p *Proxy) stateFn

type Proxy struct {
	listener               net.Listener
	clientConn, serverConn net.Conn
	clientChan, serverChan <-chan *irc.Message
	config                 *Config
	err                    error
}

func (p *Proxy) run() error {
	for state := start; state != nil; {
		state = state(p)
	}
	return p.err
}

func start(p *Proxy) stateFn {
	client, err := p.listener.Accept()
	if err != nil {
		p.err = err
		return cleanUp
	}
	p.clientConn = client

	serverConn, err := net.Dial("tcp", p.config.Dial)
	if err != nil {
		// TODO: try again? backoff?
		p.clientConn.Close()
		p.err = err
		return cleanUp
	}
	p.serverConn = serverConn

	p.clientChan = readMessages(p.clientConn)
	p.serverChan = readMessages(p.serverConn)
	return withClient
}

func cleanUp(p *Proxy) stateFn {
	if p.clientConn != nil {
		p.clientConn.Close()
	}
	if p.serverConn != nil {
		p.serverConn.Close()
	}
	return nil
}

func withClient(p *Proxy) stateFn {
	select {
	case msg, ok := <-p.serverChan:
		if !ok {
			return cleanUp
		}
		return handleServerMessageWithClient(msg, p)
	case msg, ok := <-p.clientChan:
		if !ok {
			return cleanUp
		}
		return handleClientMessage(msg, p)
	}
}

func readMessages(conn net.Conn) <-chan *irc.Message {
	ch := make(chan *irc.Message)
	reader := irc.NewReader(conn)
	go func() {
		for {
			msg, err := reader.ReadMessage()
			if err != nil {
				log.Println("Error in readMessages: ", err)
				break
			}
			ch <- msg
		}
		close(ch)
	}()
	return ch
}

func handleServerMessageWithClient(msg *irc.Message, p *Proxy) stateFn {
	var err error
	switch msg.Command {
	case "PING":
		msg.Command = "PONG"
		msg.Prefix = ""
		_, err = msg.WriteTo(p.serverConn)
	default:
		_, err = msg.WriteTo(p.clientConn)
	}
	p.err = err
	if err != nil {
		return cleanUp
	} else {
		return withClient
	}
}

func handleClientMessage(msg *irc.Message, p *Proxy) stateFn {
	_, err := msg.WriteTo(p.serverConn)
	p.err = err
	if err != nil {
		return cleanUp
	} else {
		return withClient
	}
}
