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
	"zenhack.net/go/irc-idler/proxy/internal/session"
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
		welcome  []string
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
	*session.Session
}

func emptyConnection() *connection {
	return &connection{
		Session: session.NewSession(),
	}
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

	*c = *emptyConnection()
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
		client:          emptyConnection(),
		server:          emptyConnection(),
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
	c.Session = session.NewSession()
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
	} else if !p.server.Handshake.Done() {
		p.server.Handshake.Update(msg)
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
	} else if !p.client.Handshake.Done() {
		p.client.Handshake.Update(msg)
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
			if !p.server.Handshake.Done() {
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

			clientID := p.server.Session.ClientID.String()
			messages := []*irc.Message{
				&irc.Message{
					Command: irc.RPL_WELCOME,
					Params:  p.msgCache.welcome,
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
			p.client.Session.ClientID, _ = irc.ParseClientID(clientID)
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
		return
	}

	if !p.client.Handshake.Done() {
		p.client.Handshake.Update(msg)
		p.handleHandshakeMessage(msg)
		return
	}

	switch msg.Command {
	case "QUIT":
		p.logger.Debugln("Client sent quit; disconnecting.")
		p.dropClient()
	case "JOIN":
		channelName := msg.Params[0]
		if p.server.Session.HaveChannel(channelName) {
			p.rejoinChannel(channelName, p.server.Session.GetChannel(channelName))
		} else {
			p.sendServer(msg)
		}
	default:
		// TODO: we should restrict the list of commands used here to known-safe.
		// We also need to inspect a lot of these and adjust our own state.
		p.sendServer(msg)
	}
}

func (p *Proxy) rejoinChannel(channelName string, serverState *session.ChannelState) {
	joinMessage := &irc.Message{
		Prefix:  p.client.Session.ClientID.String(),
		Command: "JOIN",
		Params:  []string{channelName},
	}
	if p.sendClient(joinMessage) != nil {
		return
	}
	if serverState.Topic != "" {
		rplTopic := &irc.Message{
			Command: irc.RPL_TOPIC,
			Params: []string{
				p.client.Session.ClientID.String(),
				channelName,
				serverState.Topic,
			},
		}
		if p.sendClient(rplTopic) != nil {
			return
		}
	}

	clientState := p.client.Session.GetChannel(channelName)

	myNick := p.server.Session.ClientID.Nick
	for nick, _ := range serverState.InitialUsers {
		rplNamreply := &irc.Message{
			Command: irc.RPL_NAMEREPLY,
			// FIXME: The "=" denotes a public channel. at some point
			// we should actually check this.
			Params: []string{myNick, "=", channelName, nick},
		}
		if p.sendClient(rplNamreply) != nil {
			return
		}
		clientState.InitialUsers[nick] = true
	}
	if p.sendClient(&irc.Message{Command: irc.RPL_ENDOFNAMES, Params: []string{
		myNick, channelName, "End of NAMES list",
	}}) == nil {
		p.replayLog(channelName)
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

	if !p.server.Handshake.Done() {
		p.server.Handshake.Update(msg)
	}

	switch msg.Command {
	case "PING":
		msg.Prefix = ""
		msg.Command = "PONG"
		p.sendServer(msg)

	// Things we can pass through to the client without any extra handling:
	case
		irc.RPL_MOTDSTART,
		irc.RPL_MOTD,

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
		p.msgCache.welcome = msg.Params

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
		p.sendServer(&irc.Message{Command: "MOTD", Params: []string{}})
	case irc.RPL_TOPIC:
		channelName, topic := msg.Params[1], msg.Params[2]
		if !p.server.Session.HaveChannel(channelName) {
			// Something weird is going on; the server shouldn't be
			// sending us one of these for a channel we're not in.
			p.logger.Warnln(
				"Server sent RPL_TOPIC for a channel we're not in: %q",
				msg,
			)
			return
		}
		p.server.Session.GetChannel(channelName).Topic = topic
		if p.client.Session.HaveChannel(channelName) && p.sendClient(msg) != nil {
			p.client.Session.GetChannel(channelName).Topic = topic
		}
	case irc.RPL_NAMEREPLY:
		// TODO: store this in the state:
		// mode := msg.Params[1]
		channelName := msg.Params[2]
		nicks := strings.Split(msg.Params[3], " ")

		if p.sendClient(msg) == nil {

			serverState := p.server.Session.GetChannel(channelName)
			clientState := p.client.Session.GetChannel(channelName)
			for _, nick := range nicks {
				nick = strings.Trim(nick, " \r\n")
				serverState.InitialUsers[nick] = true
				clientState.InitialUsers[nick] = true
			}
		}

	case irc.RPL_ENDOFMOTD, irc.ERR_NOMOTD:
		p.sendClient(msg)
		// If we're just reconnecting, this is the appropriate point to send
		// buffered messages addressed directly to us. If not, that log should
		// be empty anyway:
		p.replayLog(p.client.Session.ClientID.Nick)
	case "PRIVMSG", "NOTICE":
		targetName := msg.Params[0]
		if p.client.Session.HaveChannel(targetName) || p.client.Session.IsMe(targetName) {
			if p.sendClient(msg) != nil {
				p.logMessage(msg)
			}
		} else {
			p.logMessage(msg)
		}
	case "JOIN", "KICK", "PART", "QUIT":
		p.server.Session.Update(msg)

		p.logger.Debugf("Got %s message for channel %q.", msg.Command, msg.Params[0])

		if !p.client.Handshake.Done() || p.sendClient(msg) != nil {
			// Can't send the message to the client, so log it.
			p.logMessage(msg)
		} else {
			// client knows about the change; update their state.
			p.client.Session.Update(msg)
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
			if p.sendClient(msg) != nil {
				return
			}
		} else if err == io.EOF {
			p.logger.Debugf("Done replaying log for %q.", channelName)
			// TODO: update server's initialUsers to match client.
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
		p.client.Session.GetChannel(channelName).Update(msg)
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
