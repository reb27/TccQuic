package client

//sudo sysctl -w net.core.rmem_max=2500000
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
type ResBufferElement struct {
	res     model.VideoPacketResponse
	ReqTime time.Time
	ResTime time.Time
}

var (
	bufferFinished bool = false

	addFinish       bool = false
	adbufferLock    sync.Mutex
	mu              sync.Mutex
	receiveLock     sync.Mutex
	responseBuffer  = []model.VideoPacketResponse{}
	responseBuffer2 = []ResBufferElement{}

	ints = []model.VideoPacketRequest{{
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
	config := &quic.Config{
		MaxIdleTimeout:       5 * time.Minute,  // Set a longer maximum idle timeout
		HandshakeIdleTimeout: 10 * time.Second, // Set the receive connection flow control window size to 20 MB
	}
	// Create new QUIC connection
	connection, err := quic.DialAddr(url, tlsConf, config)
	if err != nil {
		log.Fatal(err)
	}

	var wg sync.WaitGroup

	// add
	wg.Add(1)

	adbufferLock.Lock()
	go func() {
		c.addBuffer()
		wg.Done()
	}()
	adbufferLock.Lock()
	// Consumer
	wg.Add(1)
	go func() {
		c.consumeBuffer()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		c.handleStream(connection, model.HIGH_PRIORITY)
		wg.Done()
	}()
	// High priority stream
	wg.Add(1)
	go func() {
		c.handleStream(connection, model.LOW_PRIORITY)
		wg.Done()
	}()
	wg.Wait()
}

func (c *Client) handleStream(connection quic.Connection, priority model.Priority) {

	// create stream
	var wg2 sync.WaitGroup

	// send file request
	semaphore := make(chan struct{}, 1)
	for {

		zaroreq := model.VideoPacketRequest{}

		if bufferFinished == true && ints[0] == zaroreq {

			log.Println("bufferFinished", len(ints))
			break
		}
		mu.Lock()
		req := ints[0]
		if len(ints) != 1 {
			log.Println("before reduction", len(ints))
			ints = ints[1:]
			log.Println("after reduction", len(ints))

		} else {
			ints = make([]model.VideoPacketRequest, 1)
		}

		mu.Unlock()

		if req == zaroreq {

		} else {
			wg2.Add(1)
			semaphore <- struct{}{}
			go func(req model.VideoPacketRequest) {
				defer func() {
					<-semaphore
					wg2.Done()
				}()
				stream, err := connection.OpenStreamSync(context.Background())
				if err != nil {
					log.Fatal(err)
				}
				defer stream.Close()
				c.sendRequest(stream, req)
				tempReqTime := time.Now()

				str := fmt.Sprintf("%f", priority)
				fmt.Printf(str)
				log.Println("i:", req.Segment)

				res := c.receiveData(stream)
				receiveLock.Lock()

				r := ResBufferElement{
					res:     res,        // preencher com os valores desejados para o campo res
					ResTime: time.Now(), // usar a função time.Now() para obter a hora atual para o campo time
					ReqTime: tempReqTime,
				}

				responseBuffer = append(responseBuffer, r.res)
				responseBuffer2 = append(responseBuffer2, r)

				receiveLock.Unlock()
				// write to file
				actualdate := time.Now().Format("2006-01-02_15-04-05.000000")

				file, err := os.Create("src/client/out/" + actualdate + str + "_" + fmt.Sprintf("%d", req.Segment) + ".m4s")
				if err != nil {
					///panic(err)
					return
				}
				defer file.Close()

				content := res.Data
				_, err = file.Write(content)
				if err != nil {
					panic(err)
				}
			}(req)
		}
	}
	log.Println("TesteA:", len(responseBuffer))
	wg2.Wait()
	log.Println("TesteB:", len(responseBuffer))
	log.Println("receivebuffer:", len(responseBuffer))

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
	counter := 6
	log.Println("addbuffermulock", len(ints))
	adbufferLock.Unlock()
	for {

		mu.Lock()
		if len(ints) <= 1 && counter > 0 {
			ints = append(ints, inpt)
			counter = counter - 1
			log.Println("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", counter)

		}
		mu.Unlock()
		if counter == 0 {
			break
		}
	}
	zaroreq := model.VideoPacketRequest{}
	for {
		if zaroreq == ints[0] && len(ints) == 1 {
			bufferFinished = true
			break
		}
	}
	addFinish = true

}

// Receive file response
func (c *Client) receiveData(stream quic.Stream) (res model.VideoPacketResponse) {
	if err := json.NewDecoder(stream).Decode(&res); err != nil {
		/* log.Fatal(err) */
	}
	// streamId := stream.StreamID()
	// fmt.Printf("Client stream %d: Got '%+v'\n", streamId, req)
	return
}

// Consume buffer
func (c *Client) consumeBuffer() {

	log.Println("consumeBuffer:", len(responseBuffer))
	flag1 := true
	counter := 0
	ticker := time.NewTicker(10 * time.Millisecond)
	for range ticker.C {
		if addFinish && len(responseBuffer) == 0 {
			break
		}
		for flag1 && len(responseBuffer) <= 4 {
			if len(responseBuffer) == 4 {
				flag1 = false
			}
			if addFinish && len(responseBuffer) == 0 {
				break
			}
		}
		if addFinish && len(responseBuffer) == 0 {
			break
		}
		receiveLock.Lock()
		if len(responseBuffer) != 1 {
			//log.Println("Testec:", responseBuffer[0].Segment)
			log.Println("Testec:", responseBuffer2[len(responseBuffer)].res.Segment)
			log.Println("Testec:", responseBuffer2[len(responseBuffer)].ReqTime)
			log.Println("Testec:", responseBuffer2[len(responseBuffer)].ResTime)
			responseBuffer = responseBuffer[1:]
			counter++
		} else {

			responseBuffer = make([]model.VideoPacketResponse, 0)
			counter++
			flag1 = true
		}

		receiveLock.Unlock()

		log.Println("Testec:", len(responseBuffer))

	}
	log.Println("Consumed:", counter)
	// TODO dequeue from buffer and simulate user watch behavior (1s sleep maybe?)
}
