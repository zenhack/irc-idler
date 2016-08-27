package main

import (
	"fmt"
	"golang.org/x/net/context"
	"log"
	"net"
	"os"
	"zenhack.net/go/irc-idler/sandstorm/webui"
	grain_capnp "zenhack.net/go/sandstorm/capnp/grain"
	ip_capnp "zenhack.net/go/sandstorm/capnp/ip"
	"zenhack.net/go/sandstorm/grain"
	"zenhack.net/go/sandstorm/ip"
	"zombiezen.com/go/capnproto2"
)

func main() {
	backend := &webui.Backend{
		IpNetworkCaps: make(chan capnp.Pointer),
		ServerConfigs: make(chan webui.ServerConfig),
	}
	serverConfig := webui.ServerConfig{}
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
	for {
		select {
		case ipNetworkCap := <-backend.IpNetworkCaps:
			fmt.Println("got ipNetwork cap: ", ipNetworkCap)

			// TODO: actually put the resulting token somewhere for future use.
			_, err := api.Save(
				ctx,
				func(p grain_capnp.SandstormApi_save_Params) error {
					p.SetCap(ipNetworkCap)
					return nil
				},
			).Struct()

			ipNetwork := ip_capnp.IpNetwork{capnp.ToInterface(ipNetworkCap).Client()}
			dialer := ip.IpNetworkDialer{ctx, ipNetwork}
			conn, err := dialer.Dial("tcp", net.JoinHostPort(
				serverConfig.Host,
				fmt.Sprintf("%d", serverConfig.Port),
			))
			if err != nil {
				fmt.Println(err)
				continue
			}
			conn.Write([]byte("Hello\n"))
			conn.Close()
		case serverConfig = <-backend.ServerConfigs:
			fmt.Println("got server config: ", serverConfig)
		}
	}
}
