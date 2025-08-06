package test_client

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"main/src/model"
	"main/src/netstats"
	"sync"
	"time"

	"github.com/lucas-clemente/quic-go"
)

type ClientOptions struct {
	// If Pipeline = true, use the same stream for all requests.
	// If Pipeline = false, use one stream for each request.
	Pipeline bool

	// URL of the server
	ServerURL string

	// Port of the server
	ServerPort int
}

type requestId struct {
	segment int
	tile    int
}

type pendingResponse struct {
	numericID       int
	responseChannel chan *model.VideoPacketResponse
}

type Client struct {
	Options        ClientOptions
	connection     quic.Connection
	pipelineStream quic.Stream

	statsCollector *netstats.StatsCollector
	nextRequestID  int

	waitingResponses      map[requestId]pendingResponse
	waitingResponsesMutex sync.Mutex
}

func NewClient(options ClientOptions) *Client {
	return &Client{
		Options:          options,
		statsCollector:   netstats.New(20), // Usando uma janela de 20 medições
		nextRequestID:    0,
		waitingResponses: make(map[requestId]pendingResponse),
	}
}

// Connect the client
func (c *Client) Connect() (err error) {
	url := fmt.Sprintf("%s:%d", c.Options.ServerURL, c.Options.ServerPort)
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
	c.connection, err = quic.DialAddr(url, tlsConf, config)
	if err != nil {
		log.Println(err)
		return
	}

	log.Println("Connected")

	if c.Options.Pipeline {
		c.pipelineStream, err = c.openStream()
		if err != nil {
			return
		}
	}

	return
}

// Send a request
func (c *Client) Request(
	r model.VideoPacketRequest, timeout time.Duration) *model.VideoPacketResponse {

	if c.pipelineStream != nil {
		return c.requestWithStream(c.pipelineStream, r, timeout)

	} else {
		stream, err := c.openStream()
		if err != nil {
			log.Println("Open stream failed: ", err)
			return nil
		}
		defer stream.Close()

		return c.requestWithStream(stream, r, timeout)
	}
}

// Send a request with a stream
func (c *Client) requestWithStream(stream quic.Stream,
	r model.VideoPacketRequest,
	timeout time.Duration) *model.VideoPacketResponse {
	// Register request id
	c.waitingResponsesMutex.Lock()
	numericID := c.nextRequestID
	c.nextRequestID++

	id := requestId{
		segment: r.Segment,
		tile:    r.Tile,
	}
	responseChannel := make(chan *model.VideoPacketResponse)
	c.waitingResponses[id] = pendingResponse{
		numericID:       numericID,
		responseChannel: responseChannel,
	}
c.waitingResponsesMutex.Unlock()

	// Record send time
	c.statsCollector.RecordSend(numericID)

	// Request
	if err := r.Write(stream); err != nil {
		c.waitingResponsesMutex.Lock()
		delete(c.waitingResponses, id)
		c.waitingResponsesMutex.Unlock()
		log.Println("Write failed: ", err)
		return nil
	}

	// Response
	select {
	case res := <-responseChannel:
		return res
	case <-time.After(time.Duration(timeout)):
		return nil
	}
}

// Opens an stream and handles responses.
func (c *Client) openStream() (stream quic.Stream, err error) {
	stream, err = c.connection.OpenStreamSync(context.Background())
	if err != nil {
		return
	}

	go func() {
		reader := bufio.NewReader(stream)
		for {
			res, err := model.ReadVideoPacketResponse(reader)
			if res == nil {
				if err != nil && err != io.EOF {
					log.Println("Read failed: ", err)
				}
				return
			}

			id := requestId{
				segment: res.Segment,
				tile:    res.Tile,
			}

			c.waitingResponsesMutex.Lock()
			pending, ok := c.waitingResponses[id]
			if ok {
				delete(c.waitingResponses, id)
			}
			c.waitingResponsesMutex.Unlock()

			if ok {
				_, tp := c.statsCollector.RecordRecv(pending.numericID, len(res.Data))
				avgTp := c.statsCollector.AvgThroughput()
				log.Printf("Throughput: %.2f MB/s, Avg Throughput: %.2f MB/s", tp/1024/1024, avgTp/1024/1024)
				pending.responseChannel <- res
			}
		}
	}()

	return
}
