package main

import (
	"fmt"
	"golang.org/x/net/context"
	"log"
	"net/http"
	"os"
	"zenhack.net/go/sandstorm/grain"
)

func main() {
	ctx := context.Background()
	api, err := grain.ConnectAPI(ctx, UiView{})
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: ", err)
		os.Exit(1)
	}
	log.Println("Got API: ", api)
	log.Println("Going to try to stay awake...")
	api.StayAwake(ctx, nil).Handle()
	log.Println("Got the wake lock.")
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello!")
	})
	http.ListenAndServe(":8000", nil)
}
