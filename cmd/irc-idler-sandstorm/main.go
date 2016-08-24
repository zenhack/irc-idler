package main

import (
	"fmt"
	"golang.org/x/net/context"
	"log"
	"os"
	"zenhack.net/go/irc-idler/sandstorm/webui"
	grain_capnp "zenhack.net/go/sandstorm/capnp/grain"
	"zenhack.net/go/sandstorm/grain"
	"zenhack.net/go/sandstorm/websession"
)

func main() {
	backend := &webui.Backend{
		IpNetworkCaps: make(chan []byte),
		ServerConfigs: make(chan webui.ServerConfig),
	}
	handler, err := webui.NewHandler(backend)
	ctx := context.Background()
	api, err := grain.ConnectAPI(ctx, websession.FromHandler(ctx, handler))

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
			_, err := api.Restore(
				ctx,
				func(args grain_capnp.SandstormApi_restore_Params) error {
					args.SetToken(ipNetworkCap)
					return nil
				},
			).Struct()
			if err != nil {
				log.Println("Error restoring capability: ", err)
			}
		case config := <-backend.ServerConfigs:
			fmt.Println("got server config: ", config)
		}
	}
}
