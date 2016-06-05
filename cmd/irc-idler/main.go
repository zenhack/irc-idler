package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"zenhack.net/go/irc-idler/proxy"
)

var (
	laddr    = flag.String("laddr", ":6667", "Local address to listen on")
	raddr    = flag.String("raddr", "", "Remote address to connect to")
	debuglog = flag.Bool("debuglog", false, "Enable debug logging.")
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
	var logger *log.Logger
	if *debuglog {
		logger = log.New(os.Stderr, log.Prefix(), log.Flags())
	}
	l, err := net.Listen("tcp", *laddr)
	checkFatal(err)
	proxy.NewProxy(l, *raddr, logger).Run()
}
