package main

import (
	"fmt"
	"golang.org/x/net/context"
	"log"
	"os"
	"zenhack.net/go/irc-idler/sandstorm/webui"
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
		case config := <-backend.ServerConfigs:
			fmt.Println("got server config: ", config)
		}
	}
}
