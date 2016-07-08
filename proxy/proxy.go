package proxy

import (
	"io"
	"io/ioutil"
	"log"
	"net"
	"strings"
	"time"
	"zenhack.net/go/irc-idler/irc"
)

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

	// recorded server responses; if the server already thinks we're logged in, we
	// can't get it to send these again, so we record them the first
	// time to use when the client reconnects:
	haveMsgCache bool
	msgCache     struct {
		welcome  string
		yourhost string
		created  string
		myinfo   []string
	}

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
	irc.ClientID
	// whether the handshake's NICK and USER messages have been recieved:
	nickRecieved, userRecieved bool
}

func (s *session) inHandshake() bool {
	return !(s.nickRecieved && s.userRecieved)
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

// Send a message to the server. On failure, call p.reset()
func (p *Proxy) sendServer(msg *irc.Message) error {
	p.logger.Printf("sendServer(): sending message: %q\n", msg)
	err := p.server.WriteMessage(msg)
	if err != nil {
		p.logger.Printf("sendServer(): error: %v.\n")
		p.reset()
	}
	return err
}

// Send a message to the client. On failure, call p.dropClient()
func (p *Proxy) sendClient(msg *irc.Message) error {
	p.logger.Printf("sendClient(): sending message: %q\n", msg)
	err := p.client.WriteMessage(msg)
	if err != nil {
		p.logger.Printf("sendClient(): error: %v.\n")
		p.dropClient()
	}
	return err
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
			if p.server.inHandshake() {
				p.server.shutdown()
				serverConn, err := p.dialServer()
				if err != nil {
					// Server connection failed. Boot the client and let
					// them deal with it:
					p.client.shutdown()
				} else {
					p.server.setup(serverConn)
				}
			}
			continue
		}
	}
}

func (p *Proxy) advanceHandshake(command string) {
	for _, c := range []*connection{p.client, p.server} {
		switch command {
		case "NICK":
			c.session.nickRecieved = true
		case "USER":
			c.session.userRecieved = true
		default:
			panic("advanceHandshake() called with something other " +
				"than USER or NICK.")
		}
	}
}

func (p *Proxy) handleClientEvent(msg *irc.Message, ok bool) {
	if ok {
		p.logger.Printf("handleClientEvent(): Recieved message: %q\n", msg)
		if err := msg.Validate(); err != nil {
			p.sendClient((*irc.Message)(err))
			p.dropClient()
		}
	}

	switch {
	case !ok || msg.Command == "QUIT":
		p.dropClient()
	case p.client.inHandshake() && msg.Command == "PASS":
		if p.server.inHandshake() {
			// XXX: The client should only be sending a PASS before NICK
			// and USER. we're not checking this, and just forwarding to the
			// server. Might be nice to do a bit more validation ourselves.
			p.sendServer(msg)
		}
	case p.client.inHandshake() && (msg.Command == "USER" || msg.Command == "NICK"):
		if p.server.inHandshake() {
			p.sendServer(msg)
			p.advanceHandshake(msg.Command)

			// One of two things will be the case here:
			//
			// 1. We're still not done with the handshake, in which case we return
			//    and wait for further messages
			// 2. We've just finished the handshake on both sides, so the server will
			//    take care of the welcome messages itself.
			//
			// In both cases we can just return
			return
		}

		// XXX: we ought to do at least a little sanity checking here. e.g.
		// what if the client sends a nick other than what we have on file?
		p.advanceHandshake(msg.Command)
		if p.client.inHandshake() {
			// Client still has more handshaking to do; we can return and wait for
			// more messages.
			return
		}

		// The server thinks the handshake is done, so we need to produce the welcome
		// messages ourselves.

		if !p.haveMsgCache {
			// This is probably a bug. TODO: We should report it to the user in a
			// more comprehensible way.
			p.logger.Println("ERROR: no message cache on client reconnect!")
			p.reset()
		} else {
			// Server already thinks we're done; it won't send the welcome sequence,
			// so we need to do it ourselves.
			clientID := p.server.session.ClientID.String()
			messages := []*irc.Message{
				&irc.Message{
					Command: irc.RPL_WELCOME,
					Params: []string{
						clientID,
						p.msgCache.welcome,
					},
				},
				&irc.Message{
					Command: irc.RPL_YOURHOST,
					Params: []string{
						clientID,
						p.msgCache.yourhost,
					},
				},
				&irc.Message{
					Command: irc.RPL_CREATED,
					Params: []string{
						clientID,
						p.msgCache.created,
					},
				},
				&irc.Message{
					Command: irc.RPL_MYINFO,
					Params:  append([]string{clientID}, p.msgCache.myinfo...),
				},
			}
			for _, m := range messages {
				if p.sendClient(m) != nil {
					return
				}
			}
			p.client.session.ClientID, _ = irc.ParseClientID(clientID)
			// Trigger a message of the day response; once that completes
			// the client will be ready.
			p.sendServer(&irc.Message{Command: "MOTD", Params: []string{}})
		}
		for _, c := range []*connection{p.client, p.server} {
			switch msg.Command {
			case "USER":
				c.session.userRecieved = true
			case "NICK":
				c.session.nickRecieved = true
			}
		}
	case !p.client.inHandshake():
		// TODO: we should restrict the list of commands used here to known-safe.
		// We also need to inspect a lot of these and adjust our own state.
		p.sendServer(msg)
	}
}

func (p *Proxy) handleServerEvent(msg *irc.Message, ok bool) {
	if ok {
		if err := msg.Validate(); err != nil {
			p.logger.Printf("handleServerEvent(): Got an invalid message"+
				"from server: %q (error: %q), disconnecting.\n", msg, err)
			p.reset()
		}
		p.logger.Printf("handleServerEvent(): RecievedMessage: %q\n", msg)
	}
	switch {
	case !ok:
		// Server disconnect. We boot the client and start all over.
		// TODO: might be nice to attempt a reconnect with cached credentials.
		p.reset()
	case msg.Command == "PING":
		msg.Prefix = ""
		msg.Command = "PONG"
		p.sendServer(msg)
	case p.client.inHandshake() && (msg.Command == irc.ERR_NONICKNAMEGIVEN ||
		msg.Command == irc.ERR_ERRONEUSNICKNAME ||
		msg.Command == irc.ERR_NICKNAMEINUSE ||
		msg.Command == irc.ERR_NICKCOLLISION):
		// The client sent a NICK which was rejected by the server. We unset the
		// nickRecieved bit (and of course forward the message):
		p.client.session.nickRecieved = false
		p.sendClient(msg)
	case msg.Command == irc.RPL_WELCOME:
		p.msgCache.welcome = msg.Params[1]

		// Extract the client ID. annoyingly, this isn't its own argument, so we
		// have to pull it out of the welcome message manually.
		parts := strings.Split(p.msgCache.welcome, " ")
		clientIDString := parts[len(parts)-1]
		clientID, err := irc.ParseClientID(clientIDString)

		if err != nil {
			p.logger.Printf(
				"Server sent a welcome message with an invalid "+
					"client id: %q (%v). Dropping connections.",
				clientIDString, err,
			)
			p.reset()
			return
		}

		p.server.session.ClientID = clientID
		if p.sendClient(msg) == nil {
			p.client.session.ClientID = clientID
		}
	case msg.Command == irc.RPL_YOURHOST:
		p.msgCache.yourhost = msg.Params[1]
		p.sendClient(msg)
	case msg.Command == irc.RPL_CREATED:
		p.msgCache.created = msg.Params[1]
		p.sendClient(msg)
	case msg.Command == irc.RPL_MYINFO:
		p.msgCache.myinfo = msg.Params[1:]
		p.sendClient(msg)
		p.haveMsgCache = true
	case msg.Command == irc.RPL_MOTDSTART || msg.Command == irc.RPL_MOTD:
		p.sendClient(msg)
	case msg.Command == irc.RPL_ENDOFMOTD:
		if p.sendClient(msg) == nil {
			p.replayLog()
		}
	case p.client.inHandshake():
		p.logMessage(msg)
	default:
		// TODO: be a bit more methodical; there's probably a pretty finite list
		// of things that can come through, and we want to make sure nothing is
		// going to get us out of sync with the client.
		if p.sendClient(msg) != nil {
			p.logMessage(msg)
		}
	}
}

// Disconnect the client. If the handshake isn't done, disconnect the server too.
func (p *Proxy) dropClient() {
	p.logger.Println("dropClient(): dropping client connection.")
	p.client.shutdown()
	if p.server.inHandshake() {
		p.logger.Println("dropClient(): handshake incomplete; dropping server connection.")
		p.server.shutdown()
	}
}

func (p *Proxy) reset() {
	p.logger.Println("Dropping connections.")
	p.client.shutdown()
	p.server.shutdown()
}

func (p *Proxy) logMessage(m *irc.Message) {
	p.messagelog = append(p.messagelog, m)
}

func (p *Proxy) replayLog() {
	p.logger.Println("replayLog()")
	for _, v := range p.messagelog {
		p.sendClient(v)
	}
	p.messagelog = p.messagelog[:0]
}
