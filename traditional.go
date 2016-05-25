package main

import (
	"encoding/json"
	"net"
	"os"
)

// Entry point for running in a "traditional" environment, i.e. not Sandstorm.

func traditionalMain() {
	config, err := readConfig("config.json")
	checkFatal(err)
	serve(config)
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
	proxy := &Proxy{
		config:   config,
		listener: l,
	}
	proxy.run()
}
