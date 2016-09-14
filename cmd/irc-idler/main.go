package main

import (
	"database/sql"
	"flag"
	"fmt"
	log "github.com/Sirupsen/logrus"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/net/proxy"
	"net"
	"os"
	"zenhack.net/go/irc-idler/internal/netextra"
	"zenhack.net/go/irc-idler/irc"
	ircproxy "zenhack.net/go/irc-idler/proxy"
	sqlstore "zenhack.net/go/irc-idler/storage/sql"
)

var (
	laddr  = flag.String("laddr", ":6667", "Local address to listen on")
	raddr  = flag.String("raddr", "", "Remote address to connect to")
	dbpath = flag.String("dbpath", ":memory:", "Path to SQLite database. Uses an in "+
		"memory database if unspecified")
	loglevel = flag.String("loglevel", "info", "Log level {debug,info,warn,error,fatal,panic}")

	// TODO: default should probably be `true`.
	useTLS = flag.Bool("tls", false, "Connect via tls.")
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

	db, err := sql.Open("sqlite3", *dbpath)
	if err != nil {
		logger.Fatalln(err)
	}
	if err = db.Ping(); err != nil {
		logger.Fatalln(err)
	}

	defer db.Close()

	var dialer proxy.Dialer
	if *useTLS {
		dialer = &netextra.TLSDialer{proxy.Direct}
	} else {
		dialer = proxy.Direct
	}
	l, err := net.Listen("tcp", *laddr)
	if err != nil {
		logger.Fatal(err)
	}

	clientConns := make(chan irc.ReadWriteCloser)
	connector := &ircproxy.DialerConnector{
		Dialer:  dialer,
		Network: "tcp",
		Addr:    *raddr,
	}
	go ircproxy.AcceptLoop(l, clientConns, logger)
	ircproxy.NewProxy(logger, sqlstore.NewStore(db), clientConns, connector).Run()
}
