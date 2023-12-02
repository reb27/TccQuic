package main

import (
	"main/src/client"
	"main/src/server"
	"main/src/test_client"
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
	} else if arg == "test-client" {
		client := test_client.NewClient(url, port)
		client.Start()
	}
}
