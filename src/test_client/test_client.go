// Basic client for testing the server functionality

package test_client

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"main/src/model"
	"os"
	"sync"
	"time"

	"github.com/lucas-clemente/quic-go"
)

type Client struct {
	serverURL  string
	serverPort int

	connection quic.Connection
}

const maxConcurrentRequests = 128

// Proportion of each priority
const mediumPriorityRatio = 1.0 / 3.0
const highPriorityRatio = 1.0 / 3.0

func NewClient(serverURL string, serverPort int) *Client {
	return &Client{
		serverURL:  serverURL,
		serverPort: serverPort,
	}
}

func (c *Client) Start() {
	url := fmt.Sprintf("%s:%d", c.serverURL, c.serverPort)
	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"quic-streaming"},
	}
	config := &quic.Config{
		MaxIdleTimeout:        500 * time.Minute, // Set a longer maximum idle timeout
		HandshakeIdleTimeout:  100 * time.Second, // Set the receive connection flow control window size to 20 MB
		MaxIncomingStreams:    20000,             // Set the maximum number of incoming streams
		MaxIncomingUniStreams: 20000,             // Set the maximum number of incoming unidirectional streams
	}

	// Create new QUIC connection
	log.Println("Connecting...")
	connection, err := quic.DialAddr(url, tlsConf, config)
	if err != nil {
		log.Println(err)
		return
	}

	log.Println("Connected")
	c.connection = connection

	statisticsPath := fmt.Sprintf("statistics-%d.csv", os.Getpid())

	statisticsLogger := NewStatisticsLogger(statisticsPath)
	c.runTestIteration(statisticsLogger)
	statisticsLogger.Close()
}

func (c *Client) runTestIteration(statisticsLogger *StatisticsLogger) {
	startTime := time.Now()

	waitGroup := sync.WaitGroup{}

	bufferSemaphore := NewSemaphore(maxConcurrentRequests)

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

				log.Printf("Requesting (%d, %d)\n", tile, segment)

				request := model.VideoPacketRequest{
					Priority: priority,
					Bitrate:  model.HIGH_BITRATE,
					Segment:  segment,
					Tile:     tile,
				}

				requestTime := time.Now()
				response, err := c.request(request)
				responseTime := time.Now()

				if err != nil {
					return
				}
				if len(response.Data) == 0 {
					log.Panicln("empty response")
				}

				log.Printf("Response received for (%d, %d)\n", tile, segment)
				if statisticsLogger != nil {
					statisticsLogger.Log(requestTime.Sub(startTime), request,
						responseTime.Sub(requestTime))
				}
			}()
		}
	}

	waitGroup.Wait()
}

func (c *Client) request(r model.VideoPacketRequest) (res *model.VideoPacketResponse, err error) {
	stream, err := c.connection.OpenStreamSync(context.Background())
	if err != nil {
		log.Println("Open stream failed: ", err)
		return
	}
	defer stream.Close()

	// Request

	if err = r.Write(stream); err != nil {
		log.Println("Write failed: ", err)
		return
	}

	// Response

	reader := bufio.NewReader(stream)
	if res, err = model.ReadVideoPacketResponse(reader); err != nil {
		log.Println("Read failed: ", err)
		return
	}

	return
}
