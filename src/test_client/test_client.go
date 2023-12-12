// Basic client for testing the server functionality

package test_client

import (
	"context"
	"crypto/tls"
	"encoding/json"
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

const maxConcurrentRequests = 10

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

	waitGroup := sync.WaitGroup{}

	bufferSemaphore := NewSemaphore(maxConcurrentRequests)
	statisticsPath := fmt.Sprintf("statistics-%d.csv", os.Getpid())
	statisticsLogger := NewStatisticsLogger(statisticsPath)

	startTime := time.Now()

	for i := 0; i < 120; i++ {
		priority := model.LOW_PRIORITY
		if i%6 == 0 {
			priority = model.HIGH_PRIORITY
		}

		segment := i + 1

		bufferSemaphore.Acquire()
		waitGroup.Add(1)

		go func() {
			defer waitGroup.Done()
			defer bufferSemaphore.Release()

			log.Println("Requesting segment=", segment)

			request := model.VideoPacketRequest{
				Priority: priority,
				Bitrate:  model.HIGH_BITRATE,
				Segment:  segment,
				Tile:     1,
			}

			requestTime := time.Now()
			response, err := c.request(request)
			responseTime := time.Now()

			if err != nil {
				log.Println(err)
				return
			}
			if len(response.Data) == 0 {
				log.Panicln("empty response")
			}

			log.Println("Response received for segment=", segment)
			statisticsLogger.Log(requestTime.Sub(startTime), request,
				responseTime.Sub(requestTime))
		}()
	}

	statisticsLogger.Close()

	waitGroup.Wait()
}

func (c *Client) request(r model.VideoPacketRequest) (response model.VideoPacketResponse, err error) {
	stream, err := c.connection.OpenStreamSync(context.Background())
	if err != nil {
		return
	}
	defer stream.Close()

	if err = json.NewEncoder(stream).Encode(&r); err != nil {
		log.Println("Request encode failed: ", err)
		return
	}
	if err = json.NewDecoder(stream).Decode(&response); err != nil {
		log.Println("Response decode failed: ", err)
		return
	}
	return
}
