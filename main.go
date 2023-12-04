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
		server := server.NewServer("0.0.0.0", port, queuePolicy)
		server.Start()
	} else if arg == "test-client" {
		if len(os.Args) > 1 {
			url = os.Args[2]
		}

		client := test_client.NewClient(url, port)
		client.Start()
	}
}
