package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"zenhack.net/go/irc-idler/proxy"
)

var (
	laddr = flag.String("laddr", ":6667", "Local address to listen on")
	raddr = flag.String("raddr", "", "Remote address to connect to")
)

func checkFatal(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: ", err)
		os.Exit(1)
	}
}

// Entry point for running in a "traditional" environment, i.e. not Sandstorm.

func main() {
	flag.Parse()
	l, err := net.Listen("tcp", *laddr)
	checkFatal(err)
	checkFatal(proxy.NewProxy(l, *raddr).Run())
}
