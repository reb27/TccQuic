package client

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

var finishedVideo bool = false
var requestBuffer [5]model.VideoPacketRequest
var testArray [5]int

var (
	mu    sync.Mutex
	ints1 = []model.VideoPacketRequest{{
		Priority: model.HIGH_PRIORITY,
		Bitrate:  model.HIGH_BITRATE,
		Segment:  0,
		Tile:     0,
	}, {
		Priority: model.HIGH_PRIORITY,
		Bitrate:  model.HIGH_BITRATE,
		Segment:  1,
		Tile:     0,
	}, {
		Priority: model.HIGH_PRIORITY,
		Bitrate:  model.HIGH_BITRATE,
		Segment:  2,
		Tile:     0,
	}, {
		Priority: model.HIGH_PRIORITY,
		Bitrate:  model.HIGH_BITRATE,
		Segment:  3,
		Tile:     0,
	}, {
		Priority: model.HIGH_PRIORITY,
		Bitrate:  model.HIGH_BITRATE,
		Segment:  4,
		Tile:     0,
	}}
	ints2 = [4]model.VideoPacketRequest{}
)

type Client struct {
	serverURL  string
	serverPort int
	buffer     Buffer
}

func NewClient(serverURL string, serverPort int) *Client {
	return &Client{
		serverURL:  serverURL,
		serverPort: serverPort,
		buffer:     Buffer{},
	}
}

func (c *Client) Start() {
	url := fmt.Sprintf("%s:%d", c.serverURL, c.serverPort)

	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"quic-streaming"},
	}
	// Create new QUIC connection
	connection, err := quic.DialAddr(url, tlsConf, nil)
	if err != nil {
		log.Fatal(err)
	}

	var wg sync.WaitGroup

	// High priority stream
	wg.Add(1)
	go func() {
		c.handleStream(connection, model.HIGH_PRIORITY)
		wg.Done()
	}()

	// Low priority stream
	wg.Add(1)
	go func() {
		c.handleStream(connection, model.LOW_PRIORITY)
		wg.Done()
	}()

	// Consumer
	wg.Add(1)
	go func() {
		c.consumeBuffer()
		wg.Done()
	}()

	wg.Wait()
}

func (c *Client) handleStream(connection quic.Connection, priority model.Priority) {

	str := fmt.Sprintf("%f", priority)
	fmt.Printf(str)
	// create stream
	stream, err := connection.OpenStreamSync(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	// send file request
	for i := 0; i < 5; i++ {
		c.sendRequest(stream, priority, model.HIGH_BITRATE, i, 0)
		log.Println("i:", i)
		res := c.receiveData(stream)

		// write to file
		actualdate := time.Now().Format("2006-01-02_15-04-05")

		file, err := os.Create("src/client/out/" + actualdate + str + "_" + fmt.Sprintf("%d", i) + ".m4s")
		if err != nil {
			panic(err)
		}
		defer file.Close()

		content := res.Data
		_, err = file.Write(content)
		if err != nil {
			panic(err)
		}
	}

	// close stream
	stream.Close()
}

// Send file request
func (c *Client) sendRequest(stream quic.Stream, priority model.Priority, bitrate model.Bitrate, segment int, tile int) {
	// streamId := stream.StreamID()
	// fmt.Printf("Client stream %d: Sending '%+v'\n", streamId, req)
	req := model.VideoPacketRequest{
		Priority: priority,
		Bitrate:  model.HIGH_BITRATE,
		Segment:  segment,
		Tile:     tile,
	}
	if err := json.NewEncoder(stream).Encode(&req); err != nil {
		log.Fatal(err)
	}
}

// Receive file response
func (c *Client) receiveData(stream quic.Stream) (res model.VideoPacketResponse) {
	if err := json.NewDecoder(stream).Decode(&res); err != nil {
		log.Fatal(err)
	}
	// streamId := stream.StreamID()
	// fmt.Printf("Client stream %d: Got '%+v'\n", streamId, req)
	return
}

// Consume buffer
func (c *Client) consumeBuffer() {
	// TODO dequeue from buffer and simulate user watch behavior (1s sleep maybe?)
}
