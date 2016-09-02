package proxy

import (
	"errors"
	log "github.com/Sirupsen/logrus"
	"io"
	"io/ioutil"
	"net"
	"strings"
	"time"
	"zenhack.net/go/irc-idler/irc"
	"zenhack.net/go/irc-idler/storage"
	"zenhack.net/go/irc-idler/storage/ephemeral"

	"golang.org/x/net/proxy"
)

var (
	errConnectionClosed = errors.New("Connection Closed")
)

type Connector interface {
	Connect() (irc.ReadWriteCloser, error)
}

type DialerConnector struct {
	proxy.Dialer
	Network string
	Addr    string
}

func (dc *DialerConnector) Connect() (irc.ReadWriteCloser, error) {
	conn, err := dc.Dial(dc.Network, dc.Addr)
	if err != nil {
		return nil, err
	}
	return irc.NewReadWriteCloser(conn), err
}

type Proxy struct {
	// Incomming client connections:
	clientConns <-chan irc.ReadWriteCloser

	client          *connection
	server          *connection
	serverConnector Connector
	err             error

	// Per-channel IRC messages recieved while client is not in the channel.
	messagelogs storage.Store

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

	// send indicates the server should shut down.
	stop chan struct{}
}

type connection struct {
	irc.ReadWriteCloser
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

// Return true if the prefix identifies the user associated with this session,
// false otherwise.
func (s *session) IsMe(prefix string) bool {
	clientID, err := irc.ParseClientID(prefix)
	if err != nil {
		// TODO: debug logging here. Control flow makes it a bit hard.
		// We should probably start using contexts.
		return false
	}
	return clientID.Nick == s.ClientID.Nick
}

// Tear down the connection. If it is not currently active, this is a noop.
func (c *connection) shutdown() {
	if c == nil || c.ReadWriteCloser == nil || c.Chan == nil {
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
// `dialer` is a proxy.Dialer to be used to establish the connection.
// `addr` is the tcp address of the server
// `logger`, if non-nil, will be used for informational logging. Note that the logging
//  preformed is very noisy; it is mostly meant for debugging.
func NewProxy(clientConns <-chan irc.ReadWriteCloser, serverConnector Connector, logger *log.Logger) *Proxy {
	if logger == nil {
		logger = log.New()
		logger.Out = ioutil.Discard
	}
	return &Proxy{
		clientConns:     clientConns,
		serverConnector: serverConnector,
		client:          &connection{},
		server:          &connection{},
		logger:          logger,
		messagelogs:     ephemeral.NewStore(),
		stop:            make(chan struct{}),
	}
}

func (p *Proxy) Run() {
	p.serve()
}

func (p *Proxy) Stop() {
	p.stop <- struct{}{}
}

func (c *connection) setup(conn irc.ReadWriteCloser) {
	c.ReadWriteCloser = conn
	c.Chan = irc.ReadAll(conn)
	c.session.channels = make(map[string]bool)
}

func AcceptLoop(l net.Listener, acceptChan chan<- irc.ReadWriteCloser, logger *log.Logger) {
	for {
		conn, err := l.Accept()
		logger.Debugf("AcceptLoop(): Accept: (%v, %v)", conn, err)
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}
		acceptChan <- irc.NewReadWriteCloser(conn)
		logger.Debugln("AcceptLoop(): Sent connection.")
	}
}

// Send a message to the server. On failure, call p.reset()
func (p *Proxy) sendServer(msg *irc.Message) error {
	p.logger.Debugf("sendServer(): sending message: %q\n", msg)
	if p.server.ReadWriteCloser == nil {
		return errConnectionClosed
	}
	err := p.server.WriteMessage(msg)
	if err != nil {
		p.logger.Errorf("sendServer(): error: %v.\n", err)
		p.reset()
	}
	return err
}

// Send a message to the client. On failure, call p.dropClient()
func (p *Proxy) sendClient(msg *irc.Message) error {
	p.logger.Debugf("sendClient(): sending message: %q\n", msg)
	if p.client.ReadWriteCloser == nil {
		return errConnectionClosed
	}
	err := p.client.WriteMessage(msg)
	if err != nil {
		p.logger.Errorf("sendClient(): error: %v.\n", err)
		p.dropClient()
	}
	return err
}

func (p *Proxy) serve() {
	p.logger.Infoln("Proxy starting up")
	for {
		p.logger.Debugln("serve(): Top of loop")
		select {
		case <-p.stop:
			p.logger.Infoln("Proxy shutting down")
			p.reset()
			return
		case msg, ok := <-p.client.Chan:
			p.logger.Debugln("serve(): Got client event")
			p.handleClientEvent(msg, ok)
		case msg, ok := <-p.server.Chan:
			p.logger.Debugln("serve(): Got server event")
			p.handleServerEvent(msg, ok)
		case clientConn := <-p.clientConns:
			p.logger.Debugln("serve(): Got client connection")
			// A client connected. We boot the old one, if any:
			p.client.shutdown()

			p.client.setup(clientConn)

			// If we're not done with the handshake, restart the server connection too.
			if p.server.inHandshake() {
				p.server.shutdown()
				p.logger.Debugln("Connecting to server...")
				serverConn, err := p.serverConnector.Connect()
				if err != nil {
					p.logger.Debugln("Server connection failed.")
					// Server connection failed. Boot the client and let
					// them deal with it:
					p.client.shutdown()
				} else {
					p.logger.Debugln("Established connection to server")
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

func (p *Proxy) handleHandshakeMessage(msg *irc.Message) {
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
			p.logger.Errorln("no message cache on client reconnect!")
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
	}
}

func (p *Proxy) handleClientEvent(msg *irc.Message, ok bool) {
	if !ok {
		p.logger.Debugln("Client disconnected")
		p.dropClient()
		return
	}
	p.logger.Debugf("handleClientEvent(): Recieved message: %q\n", msg)
	if err := msg.Validate(); err != nil {
		p.sendClient((*irc.Message)(err))
		p.dropClient()
	}

	if p.client.inHandshake() {
		p.handleHandshakeMessage(msg)
		return
	}

	switch msg.Command {
	case "QUIT":
		p.logger.Debugln("Client sent quit; disconnecting.")
		p.dropClient()
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

func (p *Proxy) handleServerEvent(msg *irc.Message, ok bool) {
	if ok {
		if err := msg.Validate(); err != nil {
			p.logger.Errorf("handleServerEvent(): Got an invalid message"+
				"from server: %q (error: %q), disconnecting.\n", msg, err)
			p.reset()
			return
		}
		p.logger.Debugf("handleServerEvent(): RecievedMessage: %q\n", msg)
	} else {
		// Server disconnect. We boot the client and start all over.
		// TODO: might be nice to attempt a reconnect with cached credentials.
		p.logger.Errorf("Server disconnected")
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
			p.logger.Errorf(
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
			p.client.session.IsMe(msg.Params[0]) {

			if p.sendClient(msg) != nil {
				p.logMessage(msg)
			}
		} else {
			p.logMessage(msg)
		}
	case "JOIN", "KICK", "PART", "QUIT":
		// Set our presence for the channel according to the message; if it's not
		// about us, nothing changes. otherwise, for a JOIN message we mark
		// ourselves present, and otherwise we mark ourselves absent.
		setPresence := func(m map[string]bool) {
			if !p.server.session.IsMe(msg.Prefix) {
				return
			}
			if msg.Command == "JOIN" {
				m[msg.Params[0]] = true
			} else {
				delete(m, msg.Params[0])
			}
		}
		setPresence(p.server.session.channels)

		p.logger.Debugf("Got %s message for channel %q.", msg.Command, msg.Params[0])

		if p.client.inHandshake() || p.sendClient(msg) != nil {
			// Can't send the message to the client, so log it.
			p.logMessage(msg)
		} else {
			// client knows about the change; update their state.
			setPresence(p.client.session.channels)
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
	p.logger.Debugln("dropClient(): dropping client connection.")
	p.client.shutdown()
	if p.server.inHandshake() {
		p.logger.Debugln("dropClient(): handshake incomplete; dropping server connection.")
		p.server.shutdown()
	}
}

func (p *Proxy) reset() {
	p.logger.Debugln("Dropping connections.")
	p.client.shutdown()
	p.server.shutdown()
}

func (p *Proxy) replayLog(channelName string) {
	p.logger.Debugf("replayLog(%q)\n", channelName)
	chLog, err := p.messagelogs.GetChannel(channelName)
	if err != nil {
		p.logger.Debugln("messagelogs.GetChannel(): %v", err)
		return
	}

	cursor, err := chLog.Replay()
	if err != nil {
		p.logger.Errorf(
			"Got an error replaying the log for %q: %q. ",
			channelName, err)
		return
	}
	defer cursor.Close()
	for {
		msg, err := cursor.Get()
		if err == nil {
			p.sendClient(msg)
		} else if err == io.EOF {
			p.logger.Debugf("Done replaying log for %q.", channelName)
			chLog.Clear()
			return
		} else {
			p.logger.Errorf(
				"Got an error replaying the log for %q: %q. "+
					"Not clearing logs, just in case; "+
					"this may result in duplicate messages.\n",
				channelName, err,
			)
			return
		}
		cursor.Next()
	}
}

func (p *Proxy) logMessage(msg *irc.Message) {
	p.logger.Debugln("logMessage(%q)\n", msg)
	// For now we only log messages. we'll want to add to this list in
	// the future.
	switch msg.Command {
	case "PRIVMSG", "NOTICE":
	default:
		return
	}

	channelName := msg.Params[0]
	chLog, err := p.messagelogs.GetChannel(channelName)
	if err != nil {
		p.logger.Errorf("Failed to get log for %q: %q.\n", channelName, err)
		return
	}
	err = chLog.LogMessage(msg)
	if err != nil {
		p.logger.Errorf("Failed log message %q: %q.\n", msg, err)
	}
}
