package main

import (
	"log"
	"net"
	"zenhack.net/go/irc-idler/irc"
)

func serverToClient(client, server net.Conn) {
	sReader := irc.NewReader(server)
	for {
		msg, err := sReader.ReadMessage()
		if err != nil {
			log.Println("Reading message from server: ", err)
			return
		}
		if msg.Command == "PING" {
			msg.Command = "PONG"
			msg.Prefix = ""
			_, err = msg.WriteTo(server)
			if err != nil {
				log.Println("Writing PONG to server: ", err)
				return
			}
		} else {
			_, err = msg.WriteTo(client)
			if err != nil {
				log.Println("Writing message to client: ", err)
				return
			}
		}
	}
}

func clientToServer(server, client net.Conn) {
	cReader := irc.NewReader(client)
	for {
		msg, err := cReader.ReadMessage()
		if err != nil {
			log.Println("Reading message from client: ", err)
			return
		}
		_, err = msg.WriteTo(server)
		if err != nil {
			log.Println("Writing message to server: ", err)
			return
		}
	}
}

func proxy(server, client net.Conn) {
	defer server.Close()
	defer client.Close()
	go serverToClient(client, server)
	clientToServer(server, client)
}
