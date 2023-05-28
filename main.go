package main

import (
	"main/src/client"
	"main/src/server"
	"os"
)

func main() {
	url := "localhost"
	port := 8000

	arg := os.Args[1]
	if arg == "client" {
		client := client.NewClient(url, port)
		client.Start()
	} else if arg == "server" {
		queuePolicy := os.Args[2]
		server := server.NewServer(url, port, queuePolicy)
		server.Start()
	}
}
