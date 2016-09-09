package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"zenhack.net/go/irc-idler/irc"
)

var (
	raddr = flag.String("raddr", "", "Remote address to connect to")
	laddr = flag.String("laddr", "", "Local address to listen on")
)

func checkFatal(err error) {
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}
}

func pingPong(rw irc.ReadWriter, w irc.Writer) {
	for {
		msg, err := rw.ReadMessage()
		checkFatal(err)
		if msg.Command == "PING" {
			msg.Command = "PONG"
			checkFatal(rw.WriteMessage(msg))
		} else {
			checkFatal(w.WriteMessage(msg))
		}
	}
}

func dumbCopy(r irc.Reader, w irc.Writer) {
	for {
		msg, err := r.ReadMessage()
		checkFatal(err)
		checkFatal(w.WriteMessage(msg))
	}
}

func mainLoop(conn net.Conn) {
	peer := irc.NewReadWriteCloser(conn)
	stdin := irc.NewReader(os.Stdin)
	stdout := irc.NewWriter(os.Stdout)

	go pingPong(peer, stdout)
	dumbCopy(stdin, peer)
	conn.Close()
}

func client() {
	conn, err := net.Dial("tcp", *raddr)
	checkFatal(err)
	mainLoop(conn)
}

func server() {
	l, err := net.Listen("tcp", *laddr)
	checkFatal(err)
	for {
		conn, err := l.Accept()
		if err == nil {
			fmt.Println("Got connection")
			mainLoop(conn)
		} else {
			fmt.Println("Accept failed")
		}
	}
}

func main() {
	flag.Parse()

	if *raddr == "" && *laddr == "" || *raddr != "" && *laddr != "" {
		fmt.Fprintln(os.Stderr, "You must specify exactly one of -laddr, -raddr")
		os.Exit(1)
	}

	if *laddr == "" {
		client()
	} else {
		server()
	}
}
