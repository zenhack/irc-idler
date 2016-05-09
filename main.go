package main

import (
	"fmt"
	"golang.org/x/net/context"
	"log"
	"net"
	"net/http"
	"os"
	"zenhack.net/go/sandstorm/capnp/sandstorm/grain"
	"zombiezen.com/go/capnproto2/rpc"
)

func getApi(ctx context.Context) (grain.SandstormApi, error) {
	////	Eventually it would be nice to get it right from FD #3,
	////    rather than using sandstorm-http-bridge:
	//	file := os.NewFile(3, "<sandstorm-api>")
	//	conn, err := net.FileConn(file)
	conn, err := net.Dial("unix", "/tmp/sandstorm-api")
	if err != nil {
		return grain.SandstormApi{}, err
	}
	transport := rpc.StreamTransport(conn)
	client := rpc.NewConn(transport).Bootstrap(ctx)
	return grain.SandstormApi{Client: client}, nil
}

func main() {
	ctx := context.Background()
	log.Println("Getting api...")
	api, err := getApi(ctx)
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
