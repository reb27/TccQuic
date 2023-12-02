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
	connection, err := quic.DialAddr(url, tlsConf, config)
	if err != nil {
		log.Println(err)
		return
	}

	log.Println("Connected")
	c.connection = connection

	c.spawnRequest(model.VideoPacketRequest{
		Priority: model.LOW_PRIORITY,
		Bitrate:  model.HIGH_BITRATE,
		Segment:  1,
		Tile:     1,
	})

	c.waitResponses()
}

func (c *Client) spawnRequest(r model.VideoPacketRequest) {
	c.waitGroup.Add(1)

	go func() {
		defer c.waitGroup.Done()

		response, err := c.request(r)
		if err != nil {
			log.Println(err)
			return
		}

		log.Println(response)
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
