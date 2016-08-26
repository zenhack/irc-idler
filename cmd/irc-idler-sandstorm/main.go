package main

import (
	"fmt"
	"golang.org/x/net/context"
	"log"
	"os"
	"zenhack.net/go/irc-idler/sandstorm/webui"
	grain_capnp "zenhack.net/go/sandstorm/capnp/grain"
	"zenhack.net/go/sandstorm/grain"
	"zombiezen.com/go/capnproto2"
)

func main() {
	backend := &webui.Backend{
		IpNetworkCaps: make(chan capnp.Pointer),
		ServerConfigs: make(chan webui.ServerConfig),
	}
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
			_, err := api.Save(
				ctx,
				func(p grain_capnp.SandstormApi_save_Params) error {
					p.SetCap(ipNetworkCap)
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
