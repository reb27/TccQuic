package client

//go run main.go client wfq
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
	buffer     Buffer
}

var (
	mu             sync.Mutex
	receiveLock    sync.Mutex
	responseBuffer = []model.VideoPacketResponse{}
	ints           = []model.VideoPacketRequest{{
		Priority: model.HIGH_PRIORITY,
		Bitrate:  model.LOW_BITRATE,
		Segment:  12,
		Tile:     0,
	}, {
		Priority: model.HIGH_PRIORITY,
		Bitrate:  model.LOW_BITRATE,
		Segment:  13,
		Tile:     0,
	}, {
		Priority: model.HIGH_PRIORITY,
		Bitrate:  model.LOW_BITRATE,
		Segment:  14,
		Tile:     0,
	}, {
		Priority: model.HIGH_PRIORITY,
		Bitrate:  model.LOW_BITRATE,
		Segment:  15,
		Tile:     0,
	}, {
		Priority: model.HIGH_PRIORITY,
		Bitrate:  model.LOW_BITRATE,
		Segment:  16,
		Tile:     0,
	}}
	inpt = model.VideoPacketRequest{
		Priority: model.HIGH_PRIORITY,
		Bitrate:  model.LOW_BITRATE,
		Segment:  17,
		Tile:     0,
	}
)

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

	// add
	wg.Add(1)
	go func() {
		c.addBuffer()
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

	// create stream

	var wg2 sync.WaitGroup
	var wg3 sync.WaitGroup

	semaphore := make(chan struct{}, 3)
	// send file request
	for {
		mu.Lock()
		if len(ints) == 0 {
			mu.Unlock()
			break
		}
		req := ints[0]
		if len(ints) != 1 {
			ints = ints[1:]
		} else {
			ints = make([]model.VideoPacketRequest, 5)
		}

		mu.Unlock()

		zaroreq := model.VideoPacketRequest{}
		if req == zaroreq {
			break
		}
		wg2.Add(1)
		wg3.Add(1)
		semaphore <- struct{}{}
		go func(req model.VideoPacketRequest) {
			defer func() {
				<-semaphore // Release semaphore
				wg2.Done()
			}()
			stream, err := connection.OpenStreamSync(context.Background())
			if err != nil {
				log.Fatal(err)
			}
			defer stream.Close()
			c.sendRequest(stream, req)
			str := fmt.Sprintf("%f", priority)
			fmt.Printf(str)
			log.Println("i:", req.Segment)
			res := c.receiveData(stream)
			receiveLock.Lock()
			responseBuffer = append(responseBuffer, res)
			receiveLock.Unlock()
			// write to file
			actualdate := time.Now().Format("2006-01-02_15-04-05.000000")

			file, err := os.Create("src/client/out/" + actualdate + str + "_" + fmt.Sprintf("%d", req.Segment) + ".m4s")
			if err != nil {
				///panic(err)
				wg3.Done()
				return
			}
			defer file.Close()

			content := res.Data
			_, err = file.Write(content)
			if err != nil {
				panic(err)
			}
			wg3.Done()
		}(req)
	}
	wg2.Wait()
	wg3.Wait()
	log.Println("recivebuffer:", len(responseBuffer))

}

// Send file request
func (c *Client) sendRequest(stream quic.Stream, req model.VideoPacketRequest) {
	// streamId := stream.StreamID()
	// fmt.Printf("Client stream %d: Sending '%+v'\n", streamId, req)

	if err := json.NewEncoder(stream).Encode(&req); err != nil {
		log.Fatal(err)
	}
}
func (c *Client) addBuffer() {
	couter := 3
	for {
		mu.Lock()
		if len(ints) <= 1 {
			ints = append(ints, inpt)
			couter--
			mu.Unlock()
		} else {
			mu.Unlock()
		}
		if couter == 0 {
			break
		}
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
