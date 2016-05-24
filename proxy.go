package main

import (
	"log"
	"net"
	"zenhack.net/go/irc-idler/irc"
)

func serverToClient(sReader *irc.Reader, cWriter, sWriter *irc.Writer) {
	for {
		msg, err := sReader.ReadMessage()
		if err != nil {
			log.Println("Reading message from server: ", err)
			return
		}
		if msg.Command == "PING" {
			msg.Command = "PONG"
			msg.Prefix = ""
			err = sWriter.WriteMessage(msg)
			if err != nil {
				log.Println("Writing PONG to server: ", err)
				return
			}
		} else {
			err = cWriter.WriteMessage(msg)
			if err != nil {
				log.Println("Writing message to client: ", err)
				return
			}
		}
	}
}

func clientToServer(cReader *irc.Reader, sWriter *irc.Writer) {
	for {
		msg, err := cReader.ReadMessage()
		if err != nil {
			log.Println("Reading message from client: ", err)
			return
		}
		err = sWriter.WriteMessage(msg)
		if err != nil {
			log.Println("Writing message to server: ", err)
			return
		}
	}
}

func proxy(server, client net.Conn) {
	defer server.Close()
	defer client.Close()
	cReader := irc.NewReader(client)
	sReader := irc.NewReader(server)
	cWriter := irc.NewWriter(client)
	sWriter := irc.NewWriter(server)
	go serverToClient(sReader, cWriter, sWriter)
	clientToServer(cReader, sWriter)
}
