package main

import (
	"main/src/client"
	"main/src/server"
	"main/src/test_client"
	"os"
	"strconv"
)

func main() {
	url := "localhost"
	port := 8000

	arg := os.Args[1]
	if arg == "client" {
		client := client.NewClient(url, port)
		client.Start()
	} else if arg == "server" {
		// Uso: main server <wfq|sp|fifo>

		queuePolicy := os.Args[2]
		server := server.NewServer("0.0.0.0", port, queuePolicy)
		server.Start()
	} else if arg == "test-client" {
		// Uso: main test-client [ip] parallelism baseLatency

		if len(os.Args) > 1 {
			url = os.Args[2]
		}
		parallelism := 128
		if len(os.Args) > 2 {
			parallelism, _ = strconv.Atoi(os.Args[3])
		}
		baseLatency := 250
		if len(os.Args) > 3 {
			baseLatency, _ = strconv.Atoi(os.Args[4])
		}

		test_client.StartTestClient(url, port, parallelism, baseLatency)
	}
}
