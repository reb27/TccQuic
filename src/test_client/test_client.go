// Basic client for testing the server functionality

package test_client

import (
	"encoding/json"
	"fmt"
	"log"
	"main/src/model"
	"main/src/test_client/netstats"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
)

// If pipeline = true, use the same stream for all requests.
// If pipeline = false, use one stream for each request.
const pipeline = false

// Proportion of medium priority
const mediumPriorityRatio = 0.0

// Proportion of high priority
const highPriorityRatio = 0.3

func StartTestClient(serverURL string, serverPort int, parallelism int, baseLatencyMs int) {
	client := NewClient(ClientOptions{
		Pipeline:   pipeline,
		ServerURL:  serverURL,
		ServerPort: serverPort,
	})

	log.Println("Base latency =", baseLatencyMs)

	err := client.Connect()
	if err != nil {
		log.Println("failed to connect")
		return
	}

	statisticsPath := fmt.Sprintf("statistics-%d.csv", os.Getpid())

	statisticsLogger := NewStatisticsLogger(statisticsPath)
	runTestIteration(client, parallelism, baseLatencyMs, statisticsLogger)
	statisticsLogger.Close()
}

func runTestIteration(client *Client, parallelism int, baseLatencyMs int,
	statisticsLogger *StatisticsLogger) {
	var wg sync.WaitGroup

	startTime := time.Now()

	segmentDuration := 1 * time.Second
	baseLatency := time.Duration(baseLatencyMs) * time.Millisecond
	firstSegment := 100
	lastSegment := 177
	playbackSimulator := NewPlaybackSimulator(
		segmentDuration,
		baseLatency,
		firstSegment,
		lastSegment,
	)

	counter := 0
	counterMediumPriority := 0
	counterHighPriority := 0

	parallelismSemaphore := NewSemaphore(parallelism)

	log.Printf("Test started with parallelism = %d", parallelism)
	fmt.Printf("Test started with parallelism = %d\n", parallelism)

	playbackSimulator.Start()

	// Inicia o pacote de coleta de dados da rede com uma window size
	collector := netstats.New(177)

	for iSegment := firstSegment; iSegment <= lastSegment; iSegment++ {
		log.Printf("Processing segment %d", iSegment)

		for iTile := 1; iTile <= 120; iTile++ {
			tile, segment := iTile, iSegment

			priority := model.LOW_PRIORITY

			// Classifica a prioridade do segmento (algoritmo de adaptação)
			if float64(counterHighPriority)/float64(counter+1) < highPriorityRatio {
				priority = model.HIGH_PRIORITY
				counterHighPriority++
			} else if float64(counterMediumPriority)/float64(counter+1) < mediumPriorityRatio {
				priority = model.MEDIUM_PRIORITY
				counterMediumPriority++
			}
			counter++

			parallelismSemaphore.Acquire()
			wg.Add(1)

			go func() {
				defer func() {
					parallelismSemaphore.Release()
					wg.Done()
				}()

				timeToReceive := playbackSimulator.GetTimeToReceive(segment)

				request := model.VideoPacketRequest{
					ID:       uuid.Must(uuid.New(), nil),
					Priority: priority,
					Bitrate:  model.HIGH_BITRATE,
					Segment:  segment,
					Tile:     tile,
					Timeout:  int(timeToReceive.Milliseconds()),
				}

				// Log de envio da requisição
				fmt.Printf("Sending request for segment %d, tile %d with priority %d\n", segment, tile, priority)

				// Obtém o tamanho da request em bytes
				requestBytes, err := json.Marshal(request)
				if err != nil {
					return
				}

				sizeInBytes := len(requestBytes)

				// Registra a request, o tamanho e realiza o cálculo da vazão instantenea
				_, instaThroughput := collector.RecordRecv(request.ID, sizeInBytes)
				//avgThroughput := collector.AvgThroughput()

				if timeToReceive == 0 {
					fmt.Printf("Skipped (timeout) segment %d, tile %d\n", segment, tile)
					if statisticsLogger != nil {
						statisticsLogger.Log(time.Since(startTime), request,
							baseLatency+segmentDuration, true, true, false, instaThroughput)
					}
					return
				}

				requestTime := time.Since(startTime)
				response := client.Request(request, timeToReceive)
				responseTime := time.Since(startTime)

				// Remove a request do mapa de pendentes
				collector.RecordSend(request.ID)

				var timedOut bool
				if response == nil {
					fmt.Printf("Timeout: no response for segment %d, tile %d\n", segment, tile)
					timedOut = true
				} else {
					if len(response.Data) == 0 {
						log.Panicf("Empty response for (%d, %d)", segment, tile)
					}

					if playbackSimulator.GetTimeToReceive(segment) == 0 {
						fmt.Printf("Late response for segment %d, tile %d\n", segment, tile)
						timedOut = true
					} else {
						fmt.Printf("Received response for segment %d, tile %d\n", segment, tile)
						timedOut = false
					}
				}

				if statisticsLogger != nil {
					statisticsLogger.Log(requestTime, request,
						responseTime-requestTime, timedOut, false, !timedOut, instaThroughput)
				}
			}()
		}

		playbackSimulator.WaitForPlaybackStart(iSegment)
	}

	log.Println("Waiting for all goroutines to finish...")
	wg.Wait()
	log.Println("All goroutines completed.")
	fmt.Println("Test iteration complete.")
}

// metricas de rede (vazão instantanea + media)
// tamanho do buffer
// identificação do tile + segmento + prioridade (fov)
func AdaptationAlg() {

}
