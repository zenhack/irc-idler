package main

import (
	"fmt"
	"github.com/Sirupsen/logrus"
	"golang.org/x/net/context"
	"io"
	"log"
	"os"
	"zenhack.net/go/irc-idler/irc"
	"zenhack.net/go/irc-idler/proxy"
	"zenhack.net/go/irc-idler/sandstorm/webui"
	grain_capnp "zenhack.net/go/sandstorm/capnp/grain"
	ip_capnp "zenhack.net/go/sandstorm/capnp/ip"
	"zenhack.net/go/sandstorm/grain"
	"zenhack.net/go/sandstorm/ip"
	"zombiezen.com/go/capnproto2"
)

func main() {
	logger := logrus.New()
	logger.Level = logrus.DebugLevel

	backend := &webui.Backend{
		IpNetworkCaps:   make(chan capnp.Pointer),
		GetServerConfig: make(chan webui.ServerConfig),
		SetServerConfig: make(chan webui.ServerConfig),
		HaveNetwork:     make(chan bool),
		ClientConns:     make(chan io.ReadWriteCloser),
	}
	var (
		serverConfig      webui.ServerConfig
		daemon            *proxy.Proxy
		daemonClientConns chan irc.ReadWriteCloser
		ipNetwork         *ip_capnp.IpNetwork
	)
	ctx := context.Background()
	uiView := &webui.UiView{
		Ctx:     ctx,
		Backend: backend,
	}

	api, err := grain.ConnectAPI(ctx, uiView)

	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: ", err)
		os.Exit(1)
	}
	log.Println("Got API: ", api)
	log.Println("Going to try to stay awake...")
	api.StayAwake(ctx, nil).Handle()
	log.Println("Got the wake lock.")

	// Stop the running proxy daemon (if any) and start a new one.
	newDaemon := func() {
		if daemon != nil {
			logger.Debugln("Stopping daemon")
			daemon.Stop()
			daemon = nil
		}
		daemonClientConns = make(chan irc.ReadWriteCloser)
		daemon = proxy.NewProxy(
			daemonClientConns,
			&proxy.DialerConnector{
				Dialer:  &ip.IpNetworkDialer{ctx, *ipNetwork},
				Network: "tcp",
				Addr:    serverConfig.String(),
			},
			logger,
		)
		go daemon.Run()
	}
	for {
		select {
		case ipNetworkCap := <-backend.IpNetworkCaps:
			fmt.Println("got ipNetwork cap: ", ipNetworkCap)

			// TODO: actually put the resulting token somewhere for future use.
			api.Save(
				ctx,
				func(p grain_capnp.SandstormApi_save_Params) error {
					p.SetCap(ipNetworkCap)
					return nil
				},
			).Struct()

			ipNetwork = &ip_capnp.IpNetwork{capnp.ToInterface(ipNetworkCap).Client()}

			if serverConfig.Port != 0 {
				newDaemon()
			}
		case serverConfig = <-backend.SetServerConfig:
			fmt.Println("got server config: ", serverConfig)
			if ipNetwork != nil {
				newDaemon()
			}
		case conn := <-backend.ClientConns:
			if daemon == nil {
				// The daemon isn't running, probably because we don't have
				// a network capability; we can't connect to the  server.
				// TODO: give the client some useful error message.
				logger.Debugln("Got client connection, but daemon isn't running")
				conn.Close()
			} else {
				logger.Debugln("Sending client connection to daemon.")
				daemonClientConns <- irc.NewReadWriteCloser(conn)
			}
		case backend.GetServerConfig <- serverConfig:
		case backend.HaveNetwork <- ipNetwork != nil:
		}
	}
}
