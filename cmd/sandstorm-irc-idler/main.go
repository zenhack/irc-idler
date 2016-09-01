package main

import (
	"encoding/json"
	"github.com/Sirupsen/logrus"
	"golang.org/x/net/context"
	"io"
	"io/ioutil"
	"zenhack.net/go/irc-idler/irc"
	"zenhack.net/go/irc-idler/proxy"
	"zenhack.net/go/irc-idler/sandstorm/webui"
	grain_capnp "zenhack.net/go/sandstorm/capnp/grain"
	ip_capnp "zenhack.net/go/sandstorm/capnp/ip"
	"zenhack.net/go/sandstorm/grain"
	"zenhack.net/go/sandstorm/ip"
	"zombiezen.com/go/capnproto2"
)

const (
	ipNetworkCapFile = "/var/ipNetworkCap"
	serverConfigFile = "/var/server-config.json"
)

func saveServerConfig(cfg webui.ServerConfig) error {
	buf, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(serverConfigFile, buf, 0600)
}

func loadServerConfig() (webui.ServerConfig, error) {
	ret := webui.ServerConfig{}
	buf, err := ioutil.ReadFile(serverConfigFile)
	if err != nil {
		return ret, err
	}
	err = json.Unmarshal(buf, &ret)
	if err != nil {
		// Make sure we return the zero value on failure:
		return webui.ServerConfig{}, err
	} else {
		return ret, nil
	}
}

func saveIpNetwork(ctx context.Context, api grain_capnp.SandstormApi, ipNetworkCap capnp.Pointer) error {
	results, err := api.Save(
		ctx,
		func(p grain_capnp.SandstormApi_save_Params) error {
			p.SetCap(ipNetworkCap)
			// TODO: set Label
			return nil
		},
	).Struct()
	if err != nil {
		return err
	}
	token, err := results.Token()
	if err != nil {
		return err
	}
	return ioutil.WriteFile(ipNetworkCapFile, token, 0600)
}

func loadIpNetwork(ctx context.Context, api grain_capnp.SandstormApi) (*ip_capnp.IpNetwork, error) {
	token, err := ioutil.ReadFile(ipNetworkCapFile)
	if err != nil {
		return nil, err
	}
	capability, err := api.Restore(ctx,
		func(p grain_capnp.SandstormApi_restore_Params) error {
			p.SetToken(token)
			return nil
		}).Cap().Struct()
	if err != nil {
		return nil, err
	}
	return &ip_capnp.IpNetwork{capnp.ToInterface(capability).Client()}, nil
}

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
		err               error
	)
	ctx := context.Background()
	uiView := &webui.UiView{
		Ctx:     ctx,
		Backend: backend,
	}

	api, err := grain.ConnectAPI(ctx, uiView)

	if err != nil {
		logger.Fatalln("Error: ", err)
	}
	logger.Debugln("Got API: ", api)
	logger.Debugln("Going to try to stay awake...")
	api.StayAwake(ctx, nil).Handle()
	logger.Debugln("Got the wake lock.")

	ipNetwork, err = loadIpNetwork(ctx, api)
	if err != nil {
		logger.Infoln("Failed to load ipNetwork capability:", err)
	}

	serverConfig, err = loadServerConfig()
	if err != nil {
		logger.Infoln("Failed to load server config:", err)
	}

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
			logger.Debugln("got ipNetwork cap: ", ipNetworkCap)

			if err := saveIpNetwork(ctx, api, ipNetworkCap); err != nil {
				logger.Warnln("Failed to save ipNetwork capability:", err)
			}

			ipNetwork = &ip_capnp.IpNetwork{capnp.ToInterface(ipNetworkCap).Client()}

			if serverConfig.Port != 0 {
				newDaemon()
			}
		case serverConfig = <-backend.SetServerConfig:
			logger.Debugln("got server config: ", serverConfig)
			err = saveServerConfig(serverConfig)
			if err != nil {
				logger.Warnln("Failed to save server config:", err)
			}
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
