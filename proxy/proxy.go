// Package proxy implements the IRC Idler proxy daemon proper.
package proxy

import (
	"errors"
	log "github.com/Sirupsen/logrus"
	"io"
	"io/ioutil"
	"net"
	"strings"
	"time"
	"zenhack.net/go/irc-idler/internal/netextra"
	"zenhack.net/go/irc-idler/irc"
	"zenhack.net/go/irc-idler/proxy/state"
	"zenhack.net/go/irc-idler/storage"
)

var (
	// Amount of time after which to send a PING, or to disconnect after
	// a PING has been sent without a corresponding PONG.
	//
	// This is a var instead of a const so that we can reduce it during
	// testing; a reasonable ping time for production is a long time to
	// wait during a test.
	pingTime = 30 * time.Second
)

var (
	errConnectionClosed = errors.New("Connection Closed")
)

type Connector interface {
	Connect() (irc.ReadWriteCloser, error)
}

type DialerConnector struct {
	netextra.Dialer
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

// A Proxy is a daemon implementing the core IRC Idler proxying functionality.
type Proxy struct {
	// Incomming client connections:
	clientConns <-chan irc.ReadWriteCloser

	client          *connection
	server          *connection
	serverConnector Connector
	err             error

	// Per-channel IRC messages recieved while client is not in the channel.
	messagelogs storage.Store

	// State of the session before messages in the logs have been recieved.
	// TODO: this needs to be persistent if messagelogs is.
	preLogSession *state.Session

	// recorded server responses; if the server already thinks we're logged in, we
	// can't get it to send these again, so we record them the first
	// time to use when the client reconnects:
	haveMsgCache bool
	msgCache     struct {
		yourhost string
		created  string
		myinfo   []string
	}
	serverPrefix string // The prefix for messages from the server.

	logger *log.Logger // Informational logging (nothing to do with messagelog).

	// send indicates the server should shut down.
	stop chan struct{}
}

// A connection is an IRC connection. This implements both session state
// tracking and communication.
//
// Note that a connection may be in a "disconnected" state.
type connection struct {
	irc.ReadWriteCloser
	Chan <-chan *irc.Message
	*state.Session

	// Disconnect if we don't recieve a message first. Only valid if PingSent
	// is true.
	DropDeadline time.Time

	// Send PING if we don't recieve a message first. Only valid if PingSent
	// is false.
	PingDeadline time.Time

	// True if we have sent a PING and are waiting on a response.
	PingSent bool
}

// Return a fresh connection in the "disconnected" state.
func emptyConnection() *connection {
	return &connection{
		Session: state.NewSession(),
	}
}

// Returns true if and only if the connection is closed.
func (c *connection) IsClosed() bool {
	return c == nil || c.ReadWriteCloser == nil || c.Chan == nil
}

// Update the deadlines for sending PING messages and/or dropping the
// connection, based on having received a message at time.Now().
func (c *connection) updateDeadlines() {
	c.PingDeadline = time.Now().Add(pingTime)
	c.PingSent = false
}

// Tear down the connection. If it is not currently active, this is a noop.
func (c *connection) shutdown() {
	if c.IsClosed() {
		return
	}
	c.Close()

	// Make sure the message queue is empty, otherwise we'll leak the goroutine
	for ok := true; c.Chan != nil && ok; {
		_, ok = <-c.Chan
	}

	*c = *emptyConnection()
}

// Create a new proxy.
//
// parameters:
//
// `logger`, if non-nil, will be used for informational logging.
// `store` is the Store to use for persistent data.
// `clientConns` is a channel on which incoming client connections are sent.
// `serverConnector` is a `Connector` to be used to connect to the server.
func NewProxy(
	logger *log.Logger,
	store storage.Store,
	clientConns <-chan irc.ReadWriteCloser,
	serverConnector Connector) *Proxy {

	if logger == nil {
		logger = log.New()
		logger.Out = ioutil.Discard
	}
	return &Proxy{
		clientConns:     clientConns,
		serverConnector: serverConnector,
		client:          emptyConnection(),
		server:          emptyConnection(),
		logger:          logger,
		messagelogs:     store,
		preLogSession:   state.NewSession(),
		stop:            make(chan struct{}),
	}
}

// Run the proxy daemon. Returns when the daemon shuts down.
func (p *Proxy) Run() {
	p.serve()
}

// Shuts down the daemon, which must be already running. Does not wait for
// the daemon to shut down completely before returning.
func (p *Proxy) Stop() {
	p.stop <- struct{}{}
}

// Set up the connection, using `conn` as the transport.
func (c *connection) setup(conn irc.ReadWriteCloser) {
	c.ReadWriteCloser = conn
	c.Chan = irc.ReadAll(conn)
	c.Session = state.NewSession()
	c.updateDeadlines()
}

// Accept connections from `l`, and send them on `acceptChan`.
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
	} else {
		p.server.UpdateFromClient(msg)
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
	} else {
		p.client.UpdateFromServer(msg)

		// FIXME: We clear the log only when everything is finsihed,
		// but we update this incrementally. This could cause an
		// inconsistency if the client disconnects during the replay.
		p.preLogSession.UpdateFromServer(msg)
	}
	return err
}

// main server loop
func (p *Proxy) serve() {
	p.logger.Infoln("Proxy starting up")
	ticker := time.NewTicker(pingTime)
	defer ticker.Stop()
	for {
		p.logger.Debugln("serve(): Top of loop")
		select {
		case <-p.stop:
			p.logger.Infoln("Proxy shutting down")
			p.reset()
			return
		case msg, ok := <-p.client.Chan:
			p.logger.Debugln("serve(): Got client event")
			p.client.updateDeadlines()
			p.handleClientEvent(msg, ok)
		case msg, ok := <-p.server.Chan:
			p.logger.Debugln("serve(): Got server event")
			p.server.updateDeadlines()
			p.handleServerEvent(msg, ok)
		case <-ticker.C:
			p.checkTimeout(
				p.client,
				func() { p.dropClient() },
				func(msg *irc.Message) { p.sendClient(msg) })
			p.checkTimeout(
				p.server,
				func() { p.reset() },
				func(msg *irc.Message) { p.sendServer(msg) })
		case clientConn := <-p.clientConns:
			p.logger.Debugln("serve(): Got client connection")
			// A client connected. We boot the old one, if any:
			p.dropClient()

			p.client.setup(clientConn)

			if p.server.IsClosed() {
				p.logger.Debugln("Connecting to server...")
				serverConn, err := p.serverConnector.Connect()
				if err != nil {
					p.logger.Debugln("Server connection failed:", err)
					// Server connection failed. Boot the client and let
					// them deal with it:
					p.dropClient()
				} else {
					p.logger.Debugln("Established connection to server")
					p.server.setup(serverConn)
				}
			}
		}
	}
}

// Send PINGs or drop the connection due to timeout, if needed.
//
// `conn` is the connection to query.
// `drop` is the function to call if the connection should be dropped.
// `send` is the function to call if a PING message needs to be sent.
func (p *Proxy) checkTimeout(conn *connection, drop func(), send func(msg *irc.Message)) {
	if conn.IsClosed() {
		return
	}
	now := time.Now()
	if conn.PingSent && now.After(conn.DropDeadline) {
		p.logger.Infoln("PING timeout; dropping connection.")
		drop()
	} else if !conn.PingSent && now.After(conn.PingDeadline) {
		send(&irc.Message{Command: "PING", Params: []string{"irc-idler"}})
		conn.PingSent = true
		conn.DropDeadline = now.Add(pingTime)
	}
}

// Handle a message sent by the client during a handshake.
func (p *Proxy) handleHandshakeMessage(msg *irc.Message) {
	switch msg.Command {
	case "PASS", "USER", "NICK":
		// XXX: The client should only be sending a PASS before NICK
		// and USER. we're not checking this, and just forwarding to the
		// server. Might be nice to do a bit more validation ourselves.

		if !p.server.Handshake.Done() {
			// Client and server agree on the handshake state, so just pass
			// the message through:
			p.sendServer(msg)

			// One of two things will be the true here:
			//
			// 1. sendServer failed, in which case it will have reset the connections.
			// 2. The client and server still agree on the handshake state, so we
			//    don't need to make any adjustments ourselves.
			//
			// In both cases we can just return.
			return
		}

		// XXX: we ought to do at least a little sanity checking here. e.g.
		// what if the client sends a nick other than what we have on file?

		if p.server.Handshake.Done() && p.client.Handshake.WantsWelcome() {
			// Server already thinks we're done; it won't send the welcome sequence,
			// so we need to do it ourselves.

			if !p.haveMsgCache {
				// We don't have a cached welcome message to send! This is probably
				// a bug. TODO: We should report it to the user in a more
				// comprehensible way.
				p.logger.Errorln("no message cache on client reconnect!")
				p.reset()
				return
			}

			nick := p.server.Session.ClientID.Nick
			messages := []*irc.Message{
				{
					Prefix:  p.serverPrefix,
					Command: irc.RPL_WELCOME,
					Params: []string{
						nick,
						"Welcome back to IRC Idler, " +
							p.server.Session.ClientID.String(),
					},
				},
				{
					Prefix:  p.serverPrefix,
					Command: irc.RPL_YOURHOST,
					Params: []string{
						nick,
						p.msgCache.yourhost,
					},
				},
				{
					Prefix:  p.serverPrefix,
					Command: irc.RPL_CREATED,
					Params: []string{
						nick,
						p.msgCache.created,
					},
				},
				{
					Prefix:  p.serverPrefix,
					Command: irc.RPL_MYINFO,
					Params:  append([]string{nick}, p.msgCache.myinfo...),
				},
			}
			for _, m := range messages {
				if p.sendClient(m) != nil {
					return
				}
			}
			p.client.Session.ClientID = p.server.Session.ClientID
			// Trigger a message of the day response; once that completes
			// the client will be ready.
			p.sendServer(&irc.Message{Command: "MOTD", Params: []string{}})
		}
	}
}

// Handle an event from the client
//
// The parameters are those returned by the receive on the client's channel;
// `ok` indicates whether a message was successfully received. If it is falls,
// the client has disconnected.
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
		return
	}

	p.client.UpdateFromClient(msg)

	if !p.client.Handshake.Done() {
		p.handleHandshakeMessage(msg)
		return
	}

	switch msg.Command {
	case "PING":
		msg.Prefix = ""
		msg.Command = "PONG"
		p.sendClient(msg)
	case "PONG":
		// We just ignore this one; the keepalive logic is centralized.
	case "QUIT":
		p.logger.Debugln("Client sent quit; disconnecting.")
		p.dropClient()
	case "JOIN":
		channelName := msg.Params[0]

		p.logger.Debugf("Got join for channel %q\n", channelName)

		if p.client.Session.HaveChannel(channelName) {
			p.logger.Infoln("Client already in channel " + channelName)
			// Some clients (e.g. Pidgin) will send a JOIN message when
			// the user tries to a join a channel, even if they're already in the
			// channel. Pidgin ends up with duplicate windows/tabs for that
			// channel if we actually respond to the extra messages, so we don't.
			return
		}

		if p.server.Session.HaveChannel(channelName) {
			p.logger.Infoln("Rejoining channel " + channelName)
			p.rejoinChannel(channelName, p.preLogSession.GetChannel(channelName))
		} else {
			p.sendServer(msg)
		}
	default:
		// TODO: we should restrict the list of commands used here to known-safe.
		// We also need to inspect a lot of these and adjust our own state.
		p.sendServer(msg)
	}
}

// Handle the case where the client has just requested to join channel that we
// are already in on the server side. This replays message logs and updates
// state as necessary.
func (p *Proxy) rejoinChannel(channelName string, preLogState *state.ChannelState) {
	joinMessage := &irc.Message{
		Prefix:  p.client.Session.ClientID.String(),
		Command: "JOIN",
		Params:  []string{channelName},
	}
	if p.sendClient(joinMessage) != nil {
		return
	}
	if preLogState.Topic != "" {
		rplTopic := &irc.Message{
			Prefix:  p.serverPrefix,
			Command: irc.RPL_TOPIC,
			Params: []string{
				p.client.Session.ClientID.String(),
				channelName,
				preLogState.Topic,
			},
		}
		if p.sendClient(rplTopic) != nil {
			return
		}
	}

	clientState := p.client.Session.GetChannel(channelName)

	myNick := p.server.Session.ClientID.Nick
	for _, nick := range preLogState.Users() {
		rplNamreply := &irc.Message{
			Prefix:  p.serverPrefix,
			Command: irc.RPL_NAMEREPLY,
			// FIXME: The "=" denotes a public channel. at some point
			// we should actually check this.
			Params: []string{myNick, "=", channelName, nick},
		}
		if p.sendClient(rplNamreply) != nil {
			return
		}
		clientState.AddUser(nick)
	}
	if p.sendClient(&irc.Message{
		Prefix:  p.serverPrefix,
		Command: irc.RPL_ENDOFNAMES,
		Params: []string{
			myNick, channelName, "End of NAMES list",
		}}) == nil {
		p.replayLog(channelName)
	}
}

// Like handleClientEvent, but for events from the server.
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

	p.server.UpdateFromServer(msg)

	switch msg.Command {
	case "PING":
		msg.Prefix = ""
		msg.Command = "PONG"
		p.sendServer(msg)
	case "PONG":
		// We just ignore this one; the keepalive logic is centralized.

	// Things we can pass through to the client without any extra handling:
	case
		irc.RPL_MOTDSTART,
		irc.RPL_MOTD,
		irc.RPL_NAMEREPLY,
		irc.RPL_TOPIC,

		// Various nick related errors. TODO: we should be more careful;
		// at least based on the RFC, NICKCOLLISION could potnetially happen without
		// any action on our part, so we'd have to somehow update our state.
		// For the most part however, this is just the client having done
		// something that's failed, and we just need to forward the error.
		irc.ERR_NONICKNAMEGIVEN,
		irc.ERR_ERRONEUSNICKNAME,
		irc.ERR_NICKNAMEINUSE,
		irc.ERR_NICKCOLLISION:

		p.sendClient(msg)
	case irc.RPL_WELCOME:
		p.serverPrefix = msg.Prefix

		// Extract the client ID. annoyingly, this isn't its own argument, so we
		// have to pull it out of the welcome message manually.
		parts := strings.Split(msg.Params[1], " ")
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

		p.server.Session.ClientID = clientID
		if p.sendClient(msg) == nil {
			p.client.Session.ClientID = clientID
		}

	// We can mostly just pass these through to the client and it will do the right
	// thing, but we need to save them for when the client disconnects and then
	// reconnects, becasue the server won't send them again if it thinks the client
	// is already connected:
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
	case irc.RPL_ENDOFMOTD, irc.ERR_NOMOTD:
		p.sendClient(msg)
		// If we're just reconnecting, this is the appropriate point to send
		// buffered messages addressed directly to us. If not, that log should
		// be empty anyway:
		p.replayLog(p.client.Session.ClientID.Nick)
	case irc.RPL_ENDOFNAMES:
		p.sendClient(msg)
		// One of two things has just happened:
		//
		// 1. We've just joined a channel for the first time since connecting to
		//    the server. We should replay the log in case we have logged
		//    messages from a previous connnection.
		// 2. The user specifically sent a NAMES request, in which case they're
		//    presumably already in the channel, so there should be no log, and
		//    therefore it is safe to replay it.
		p.replayLog(msg.Params[1])
	case "PRIVMSG", "NOTICE":
		targetName := msg.Params[0]
		if p.client.Session.HaveChannel(targetName) || p.client.Session.IsMe(targetName) {
			if p.sendClient(msg) != nil {
				p.logMessage(msg)
			}
		} else {
			p.logMessage(msg)
		}
	case "JOIN", "KICK", "PART", "QUIT", "NICK":
		if !p.client.Handshake.Done() || p.sendClient(msg) != nil {
			// Can't send the message to the client, so log it.
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
	p.logger.Debugln("dropClient(): dropping client connection.")
	p.client.shutdown()
	if !p.server.Handshake.Done() {
		p.logger.Debugln("dropClient(): handshake incomplete; dropping server connection.")
		p.server.shutdown()
	}
}

// Drop both connections.
func (p *Proxy) reset() {
	p.logger.Debugln("Dropping connections.")
	p.client.shutdown()
	p.server.shutdown()
}

// Replay the message log for channel `channelName`.
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
			if p.sendClient(msg) != nil {
				return
			}
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

// Log the message `msg`. Note that not all message types are logged.
func (p *Proxy) logMessage(msg *irc.Message) {
	p.logger.Debugln("logMessage(%q)\n", msg)

	switch msg.Command {
	case "QUIT":
		// FIXME: we need to store this, since otherwise we don't
		// hear about users leaving channels by disconnecting from the
		// network entirely. However, before we cand do that we need to
		// do a bit of refactoring, since QUIT doesn't include channel
		// information. For now, we just punt.
		return
	case "PRIVMSG", "NOTICE", "JOIN", "KICK", "PART", "NICK":
	default:
		// Don't log anything we don't specificially whitelist above.
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
