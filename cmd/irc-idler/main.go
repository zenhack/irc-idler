package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"golang.org/x/net/proxy"
	"net"
	"os"
	ircproxy "zenhack.net/go/irc-idler/proxy"
)

var (
	laddr    = flag.String("laddr", ":6667", "Local address to listen on")
	raddr    = flag.String("raddr", "", "Remote address to connect to")
	loglevel = flag.String("loglevel", "info", "Log level {debug,info,warn,error,fatal,panic}")

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

	level, err := log.ParseLevel(*loglevel)
	if err != nil {
		// We don't just print the error from logrus, since it talks about
		// "logrus" levels, and I (zenhack) would prefer to keep that level
		// of detail out of messages logged above debug level; end users
		// shouldn't care what log package we're using.
		fmt.Fprintf(os.Stderr, "Error: %q is not a valid log level.\n", *loglevel)
		os.Exit(1)
	}
	logger := log.New()
	logger.Level = level

	var dialer proxy.Dialer
	if *useTLS {
		dialer = (*TLSDialer)(nil)
	} else {
		dialer = proxy.Direct
	}

	l, err := net.Listen("tcp", *laddr)
	if err != nil {
		logger.Fatal(err)
	}
	ircproxy.NewProxy(l, dialer, *raddr, logger).Run()
}
