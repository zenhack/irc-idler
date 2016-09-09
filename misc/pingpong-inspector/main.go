package main

import (
	"flag"
	"log"
	"net"
	"os"
	"zenhack.net/go/irc-idler/irc"
)

var (
	raddr = flag.String("raddr", ":6667", "Remote address to connect to")
)

func checkFatal(err error) {
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}
}

func main() {
	flag.Parse()

	conn, err := net.Dial("tcp", *raddr)
	checkFatal(err)

	server := irc.NewReadWriteCloser(conn)
	stdin := irc.NewReader(os.Stdin)
	stdout := irc.NewWriter(os.Stdout)

	go func() {
		for {
			msg, err := server.ReadMessage()
			checkFatal(err)
			if msg.Command == "PING" {
				msg.Command = "PONG"
				checkFatal(server.WriteMessage(msg))
			} else {
				checkFatal(stdout.WriteMessage(msg))
			}
		}
	}()

	for {
		msg, err := stdin.ReadMessage()
		checkFatal(err)
		checkFatal(server.WriteMessage(msg))
	}
}
