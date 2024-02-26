// Basic client for testing the server functionality

package test_client

import (
	"fmt"
	"log"
	"main/src/model"
	"os"
	"time"
)

// If pipeline = true, use the same stream for all requests.
// If pipeline = false, use one stream for each request.
const pipeline = false

// Proportion of medium priority
const mediumPriorityRatio = 0.0

// Proportion of high priority
const highPriorityRatio = 1.0 / 2.0

func StartTestClient(serverURL string, serverPort int, parallelism int) {
	client := NewClient(ClientOptions{
		Pipeline:   pipeline,
		ServerURL:  serverURL,
		ServerPort: serverPort,
	})

	err := client.Connect()
	if err != nil {
		log.Println("failed to connect")
		return
	}

	statisticsPath := fmt.Sprintf("statistics-%d.csv", os.Getpid())

	statisticsLogger := NewStatisticsLogger(statisticsPath)
	runTestIteration(client, parallelism, statisticsLogger)
	statisticsLogger.Close()
}

func runTestIteration(client *Client, parallelism int, statisticsLogger *StatisticsLogger) {
	startTime := time.Now()

	segmentDuration := 1 * time.Second
	baseLatency := 250 * time.Millisecond
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

	playbackSimulator.Start()

	for iSegment := firstSegment; iSegment <= lastSegment; iSegment++ {
		// Request tiles
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

			request := model.VideoPacketRequest{
				Priority: priority,
				Bitrate:  model.HIGH_BITRATE,
				Segment:  segment,
				Tile:     tile,
			}

			// Segmento ja foi reproduzido! Nao fazer a requisicao
			if playbackSimulator.CurrentBufferSegment() != segment {
				log.Printf("Skipped (%d, %d)\n", segment, tile)

				if statisticsLogger != nil {
					statisticsLogger.Log(time.Since(startTime), request,
						baseLatency+segmentDuration, true, true, false)
				}

				continue
			}

			parallelismSemaphore.Acquire()

			go func() {
				defer parallelismSemaphore.Release()

				log.Printf("Requesting (%d, %d)\n", segment, tile)

				requestTime := time.Since(startTime)
				response := client.Request(request, baseLatency+segmentDuration)
				responseTime := time.Since(startTime)

				var timedOut bool
				if response == nil {
					log.Printf("did not receive response for (%d, %d)\n",
						segment, tile)
					timedOut = true
				} else {
					if len(response.Data) == 0 {
						log.Panicln("empty response")
					}

					if playbackSimulator.CurrentBufferSegment() != segment {
						log.Printf("Response received for (%d, %d), but timed out\n", segment, tile)
						timedOut = true
					} else {
						log.Printf("Response received for (%d, %d)\n", segment, tile)
						timedOut = false
					}
				}

				if statisticsLogger != nil {
					statisticsLogger.Log(requestTime, request,
						responseTime-requestTime, timedOut, false, !timedOut)
				}
			}()
		}

		// Wait for playback
		playbackSimulator.WaitForPlaybackStart(iSegment)
	}
}
