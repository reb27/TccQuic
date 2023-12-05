// Basic client for testing the server functionality

package test_client

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"main/src/model"
	"sync"
	"time"

	"github.com/lucas-clemente/quic-go"
)

type Client struct {
	serverURL  string
	serverPort int
	waitGroup  sync.WaitGroup

	connection quic.Connection
}

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

	for i := 0; i < 3; i++ {
		priority := model.LOW_PRIORITY
		if i%6 == 0 {
			priority = model.HIGH_PRIORITY
		}

		c.spawnRequest(priority, i+1)
	}

	c.waitResponses()
}

func (c *Client) spawnRequest(priority model.Priority, segment int) {
	c.waitGroup.Add(1)

	go func() {
		defer c.waitGroup.Done()

		log.Println("Requesting segment=", segment)

		response, err := c.request(model.VideoPacketRequest{
			Priority: priority,
			Bitrate:  model.HIGH_BITRATE,
			Segment:  segment,
			Tile:     1,
		})
		if err != nil {
			log.Println(err)
			return
		}
		if len(response.Data) == 0 {
			log.Println("empty response")
			return
		}

		log.Println("Response received for segment=", segment)
	}()
}

func (c *Client) waitResponses() {
	c.waitGroup.Wait()
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
