package main

import (
	"fmt"
	"golang.org/x/net/context"
	"log"
	"net/http"
	"os"
	"time"
	"zenhack.net/go/sandstorm/grain"
	"zenhack.net/go/sandstorm/websession"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte(
			"Hey! IRC idler doesn't have a web interface yet, but this " +
				"placeholder page seems to be working."))
	})
	ctx := context.Background()
	api, err := grain.ConnectAPI(ctx, websession.FromHandler(ctx, http.DefaultServeMux))

	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: ", err)
		os.Exit(1)
	}
	log.Println("Got API: ", api)
	log.Println("Going to try to stay awake...")
	api.StayAwake(ctx, nil).Handle()
	log.Println("Got the wake lock.")
	for {
		// If we don't do this we'll exit the process. We don't have anything
		// else to do in main, but we need to stay running to keep our websesion
		// available
		time.Sleep(30 * time.Second)
		fmt.Println("Still running")
	}
}
