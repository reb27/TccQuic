package main

import (
	"main/src/client"
	"main/src/server"
	"main/src/server/metrics"
	"main/src/test_client"
	"os"
	"os/signal"
	"strconv"
	"syscall"
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

		// O harness do Mininet encerra o servidor com SIGTERM após os clientes
		// terminarem. Sem capturar o sinal, o processo morre antes de gravar o
		// server_summary.csv. Capturamos SIGTERM/SIGINT para liberar o resumo
		// (idempotente via closeOnce) antes de sair.
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigs
			metrics.M().WriteSummaryAndClose()
			os.Exit(0)
		}()

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
