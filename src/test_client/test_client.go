// Basic client for testing the server functionality

package test_client

import (
	"fmt"
	"log"
	"main/src/model"
	"os"
	"sync"
	"time"
)

// If pipeline = true, use the same stream for all requests.
// If pipeline = false, use one stream for each request.
const pipeline = false

// Proportion of medium priority
const mediumPriorityRatio = 1.0 / 3.0

// Proportion of high priority
const highPriorityRatio = 1.0 / 3.0

const requestTimeout = 1 * time.Minute

func StartTestClient(serverURL string, serverPort int, parallelism int) {
	client := NewClient(ClientOptions{
		Pipeline:   pipeline,
		ServerURL:  serverURL,
		ServerPort: serverPort,
		Timeout:    requestTimeout,
	})

	err := client.Connect()
	if err != nil {
		log.Println("failed to connect")
		return
	}

	statisticsPath := fmt.Sprintf("statistics-%d.csv", os.Getpid())

	statisticsLogger := NewStatisticsLogger(statisticsPath)
	runTestIteration(client, statisticsLogger, parallelism)
	statisticsLogger.Close()
}

func runTestIteration(client *Client, statisticsLogger *StatisticsLogger, parallelism int) {
	startTime := time.Now()

	waitGroup := sync.WaitGroup{}

	bufferSemaphore := NewSemaphore(parallelism)

	counter := 0
	counterMediumPriority := 0
	counterHighPriority := 0

	for iSegment := 100; iSegment <= 177; iSegment++ {
		for iTile := 1; iTile <= 120; iTile++ {
			tile, segment := iTile, iSegment

			priority := model.LOW_PRIORITY
			if float64(counterHighPriority)/float64(counter) < highPriorityRatio {
				priority = model.HIGH_PRIORITY
				counterHighPriority++
			} else if float64(counterMediumPriority)/float64(counter) < mediumPriorityRatio {
				priority = model.MEDIUM_PRIORITY
				counterMediumPriority++
			}
			counter++

			bufferSemaphore.Acquire()
			waitGroup.Add(1)

			go func() {
				defer waitGroup.Done()
				defer bufferSemaphore.Release()

				log.Printf("Requesting (%d, %d)\n", segment, tile)

				request := model.VideoPacketRequest{
					Priority: priority,
					Bitrate:  model.HIGH_BITRATE,
					Segment:  segment,
					Tile:     tile,
				}

				requestTime := time.Now()
				response := client.Request(request)
				responseTime := time.Now()

				if response == nil {
					log.Panicf("did not receive response for (%d, %d)\n",
						segment, tile)
				}
				if len(response.Data) == 0 {
					log.Panicln("empty response")
				}

				log.Printf("Response received for (%d, %d)\n", segment, tile)
				if statisticsLogger != nil {
					statisticsLogger.Log(requestTime.Sub(startTime), request,
						responseTime.Sub(requestTime))
				}
			}()
		}
	}

	waitGroup.Wait()
}
