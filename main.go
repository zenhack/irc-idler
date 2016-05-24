package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
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
		serverConn, err := net.Dial("tcp", config.Dial)
		proxy(serverConn, clientConn)
	}
}

func normalMain() {
	config, err := readConfig("config.json")
	checkFatal(err)
	serve(config)
}
