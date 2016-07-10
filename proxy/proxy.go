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

	client *connection
	server *connection
	addr   string // address of IRC server to connect to.
	err    error

	// Per-channel IRC messages recieved while client is not in the channel.
	messagelogs map[string][]*irc.Message

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

	channels map[string]bool // List of channels we're in.
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
		listener:    l,
		addr:        addr,
		client:      &connection{},
		server:      &connection{},
		logger:      logger,
		acceptChan:  make(chan net.Conn),
		messagelogs: make(map[string][]*irc.Message),
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
	c.session.channels = make(map[string]bool)
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

	if !ok || msg.Command == "QUIT" {
		p.dropClient()
		return
	}

	if p.client.inHandshake() {
		switch msg.Command {
		case "PASS":
			if p.server.inHandshake() {
				// XXX: The client should only be sending a PASS before NICK
				// and USER. we're not checking this, and just forwarding to the
				// server. Might be nice to do a bit more validation ourselves.
				p.sendServer(msg)
			}
		case "USER", "NICK":
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
		}
	} else {
		switch msg.Command {
		case "JOIN":
			if p.server.session.channels[msg.Params[0]] {
				msg.Prefix = p.client.session.ClientID.String()
				if p.sendClient(msg) == nil {
					p.client.session.channels[msg.Params[0]] = true
					p.replayLog(msg.Params[0])
				}
			} else {
				p.sendServer(msg)
			}
		default:
			// TODO: we should restrict the list of commands used here to known-safe.
			// We also need to inspect a lot of these and adjust our own state.
			p.sendServer(msg)
		}
	}
}

func (p *Proxy) handleServerEvent(msg *irc.Message, ok bool) {
	if ok {
		if err := msg.Validate(); err != nil {
			p.logger.Printf("handleServerEvent(): Got an invalid message"+
				"from server: %q (error: %q), disconnecting.\n", msg, err)
			p.reset()
			return
		}
		p.logger.Printf("handleServerEvent(): RecievedMessage: %q\n", msg)
	} else {
		// Server disconnect. We boot the client and start all over.
		// TODO: might be nice to attempt a reconnect with cached credentials.
		p.reset()
		return
	}
	switch msg.Command {
	case "PING":
		msg.Prefix = ""
		msg.Command = "PONG"
		p.sendServer(msg)
	case irc.ERR_NONICKNAMEGIVEN, irc.ERR_ERRONEUSNICKNAME, irc.ERR_NICKNAMEINUSE,
		irc.ERR_NICKCOLLISION:
		if p.client.inHandshake() {
			// The client sent a NICK which was rejected by the server. We unset the
			// nickRecieved bit (and of course forward the message):
			p.client.session.nickRecieved = false
			p.sendClient(msg)
		}
	case irc.RPL_WELCOME:
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
	case irc.RPL_YOURHOST:
		p.msgCache.yourhost = msg.Params[1]
		p.sendClient(msg)
	case irc.RPL_CREATED:
		p.msgCache.created = msg.Params[1]
		p.sendClient(msg)
	case irc.RPL_MYINFO:
		p.msgCache.myinfo = msg.Params[1:]
		p.sendClient(msg)
		p.haveMsgCache = true
	case irc.RPL_MOTDSTART, irc.RPL_MOTD:
		p.sendClient(msg)
	case irc.RPL_ENDOFMOTD, irc.ERR_NOMOTD:
		p.sendClient(msg)
		// If we're just reconnecting, this is the appropriate point to send
		// buffered messages addressed directly to us. If not, that log should
		// be empty anyway:
		p.replayLog(p.client.session.ClientID.Nick)
	case "PRIVMSG", "NOTICE":
		if p.client.session.channels[msg.Params[0]] ||
			msg.Params[0] == p.client.session.ClientID.Nick {

			if p.sendClient(msg) != nil {
				p.logMessage(msg)
			}
		} else {
			p.logMessage(msg)
		}
	case "JOIN", "KICK", "PART", "QUIT":
		var setPresence func(m map[string]bool)
		if msg.Command == "JOIN" {
			p.logger.Printf("Got JOIN message for channel %q.", msg.Params[0])
			setPresence = func(m map[string]bool) {
				m[msg.Params[0]] = true
			}
		} else {
			setPresence = func(m map[string]bool) {
				delete(m, msg.Params[0])
			}
		}

		clientid, err := irc.ParseClientID(msg.Prefix)
		isMe := err == nil && clientid.Nick == p.server.session.ClientID.Nick
		if isMe {
			setPresence(p.server.session.channels)
		}
		if p.client.inHandshake() {
			p.logMessage(msg)
		} else if p.sendClient(msg) == nil {
			if isMe {
				setPresence(p.client.session.channels)
			}
		} else {
			p.logMessage(msg)
		}
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

func (p *Proxy) replayLog(channelName string) {
	p.logger.Printf("replayLog(%q)\n", channelName)
	if p.messagelogs[channelName] == nil {
		p.logger.Printf("No log for channel.")
		return
	}
	for _, v := range p.messagelogs[channelName] {
		p.sendClient(v)
	}
	delete(p.messagelogs, channelName)
}

func (p *Proxy) logMessage(msg *irc.Message) {
	p.logger.Printf("logMessage(%q)\n", msg)
	// For now we only log messages. we'll want to add to this list in
	// the future.
	switch msg.Command {
	case "PRIVMSG", "NOTICE":
	default:
		return
	}

	channelName := msg.Params[0]
	if p.messagelogs[channelName] == nil {
		p.messagelogs[channelName] = []*irc.Message{msg}
	} else {
		p.messagelogs[channelName] = append(p.messagelogs[channelName], msg)
	}
}
