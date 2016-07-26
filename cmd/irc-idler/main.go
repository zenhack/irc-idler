package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"golang.org/x/net/proxy"
	"log"
	"net"
	"os"
	ircproxy "zenhack.net/go/irc-idler/proxy"
)

var (
	laddr    = flag.String("laddr", ":6667", "Local address to listen on")
	raddr    = flag.String("raddr", "", "Remote address to connect to")
	debuglog = flag.Bool("debuglog", false, "Enable debug logging.")

	// TODO: default should probably be `true`.
	useTLS = flag.Bool("tls", false, "Connect via tls.")
)

type TLSDialer tls.Config

func (cfg *TLSDialer) Dial(network, addr string) (net.Conn, error) {
	return tls.Dial(network, addr, (*tls.Config)(cfg))
}

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
	var dialer proxy.Dialer
	if *debuglog {
		logger = log.New(os.Stderr, log.Prefix(), log.Flags())
	}
	if *useTLS {
		dialer = (*TLSDialer)(nil)
	} else {
		dialer = proxy.Direct
	}
	l, err := net.Listen("tcp", *laddr)
	checkFatal(err)
	ircproxy.NewProxy(l, dialer, *raddr, logger).Run()
}
