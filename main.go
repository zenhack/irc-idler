package main

import (
	"encoding/json"
	"fmt"
	"golang.org/x/net/context"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"time"
	"zenhack.net/go/sandstorm/grain"
	"zenhack.net/go/sandstorm/websession"
)

type Config struct {
	Listen string `json:"listen"`
	Dial   string `json:"dial"`
}

func checkFatal(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: ", err)
		os.Exit(1)
	}
}

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello!")
	})
	if os.Getenv("SANDSTORM") == "1" {
		sandstormMain()
	} else {
		normalMain()
	}
}

func readConfig(filename string) (*Config, error) {
	var config Config
	file, err := os.Open(filename)
	checkFatal(err)
	defer file.Close()
	d := json.NewDecoder(file)
	err = d.Decode(&config)
	checkFatal(err)
	return &config, err
}

func doCopy(ch chan<- bool, dst io.Writer, src io.Reader) {
	io.Copy(dst, src)
	ch <- false
}

func serve(config *Config) {
	l, err := net.Listen("tcp", config.Listen)
	checkFatal(err)
	for {
		clientConn, err := l.Accept()
		if err != nil {
			// TODO: handle this? net/http does some backoff stuff, need to
			// investigate/understand that better.
			continue
		}
		srvConn, err := net.Dial("tcp", config.Dial)
		ch := make(chan bool, 2)
		go doCopy(ch, clientConn, srvConn)
		go doCopy(ch, srvConn, clientConn)
		<-ch
		<-ch
		// TODO: should keep the server connection open. Need to wrangle
		// something to deal with the traffic while the client is offline.
		clientConn.Close()
		srvConn.Close()
	}
}

func normalMain() {
	config, err := readConfig("config.json")
	checkFatal(err)
	serve(config)
}

func sandstormMain() {
	ctx := context.Background()
	api, err := grain.ConnectAPI(ctx, websession.FromHandler(http.DefaultServeMux))

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
