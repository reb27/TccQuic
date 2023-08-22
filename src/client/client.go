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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lucas-clemente/quic-go"
)

// A function that converts a string like "[1, 2, 3, 4]" to a slice of int
func ParseArray(input string) ([]int, error) {
	// Remove square brackets and whitespace
	input = strings.TrimPrefix(input, "[")
	input = strings.TrimSuffix(input, "]")
	input = strings.TrimSpace(input)

	// Split the input into individual elements
	elements := strings.Split(input, ",")

	// Parse each element into an integer and store in the result array
	var result []int
	for _, element := range elements {
		value, err := strconv.Atoi(strings.TrimSpace(element))
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}

	return result, nil
}

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

	addFinish      bool = false
	streamFinished bool = false

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

	var configInfo = NewConfigurable()
	var marks = [5]int{50, 45, 30, 49, 38}
	log.Println("marks:", marks)

	configInfo.ReadFile("configFile")

	log.Println("configInfo.adaptRate:", configInfo.adaptRate)
	log.Println("configInfo.segments:", configInfo.segments)

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
		//log.Fatal(err)
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
		log.Println("consumeBuffer finished")
	}()
	wg.Add(1)
	go func() {
		c.handleStream(connection, model.HIGH_PRIORITY, configInfo)
		wg.Done()
	}()

	wg.Wait()
}

func (c *Client) handleStream(connection quic.Connection, priority model.Priority, configInfo *ConfigurableImpl) {

	// create stream
	var wg2 sync.WaitGroup
	// send file request
	semaphore := make(chan struct{}, configInfo.semaphoreCount)
	for {

		zaroreq := model.VideoPacketRequest{}

		if bufferFinished == true && ints[0] == zaroreq {

			break
		}
		mu.Lock()
		req := ints[0]
		if len(ints) != 1 {
			ints = ints[1:]

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
					//log.Fatal(err)
				}
				defer stream.Context().Done()
				c.sendRequest(stream, req)
				tempReqTime := time.Now()

				str := fmt.Sprintf("%f", priority)
				//fmt.Printf(str)
				//stream.CancelRead(404)

				res := c.receiveData(stream)
				receiveLock.Lock()

				r := ResBufferElement{
					res:     res,        // preencher com os valores desejados para o campo res
					ResTime: time.Now(), // usar a função time.Now() para obter a hora atual para o campo time
					ReqTime: tempReqTime,
				}

				responseBuffer2 = append(responseBuffer2, r)
				log.Println("---------------elementToBuffer:----------", r.res.Segment)

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
				resBytes, err := json.Marshal(res)
				if err != nil {
					//fmt.Println("Error:", err)
					return
				}

				sizeInBytes := len(resBytes)
				fmt.Printf("Size of res: %d bytes\n", sizeInBytes)
				_, err = file.Write(content)
				if err != nil {
					//panic(err)
				}
			}(req)
		}
	}
	wg2.Wait()
	streamFinished = true

}

// Send file request
func (c *Client) sendRequest(stream quic.Stream, req model.VideoPacketRequest) {
	// streamId := stream.StreamID()
	// fmt.Printf("Client stream %d: Sending '%+v'\n", streamId, req)

	if err := json.NewEncoder(stream).Encode(&req); err != nil {
		//log.Fatal(err)
	}
}

func (c *Client) addBuffer() {
	counter := 12
	counter2 := 1

	adbufferLock.Unlock()
	for {

		mu.Lock()
		if len(ints) >= 1 && counter > 0 {
			inpt.Segment = counter
			ints = append(ints, inpt)
			counter = counter - 1

		}
		mu.Unlock()
		if counter == 0 {
			if counter2 == 0 {
				break
			} else {
				counter2 = counter2 - 1
				counter = 120
			}
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

	flag1 := true
	counter := 0
	ticker := time.NewTicker(1 * time.Millisecond)
	for range ticker.C {
		if addFinish && len(responseBuffer2) == 0 && streamFinished {
			break
		}
		for flag1 && len(responseBuffer2) <= 4 && streamFinished {
			if len(responseBuffer2) == 4 {
				flag1 = false
			}
			if addFinish && streamFinished {
				break
			}
		}
		if addFinish && len(responseBuffer2) == 0 && streamFinished {
			break
		}
		receiveLock.Lock()
		if len(responseBuffer2) != 0 {
			//log.Println("Testec:", responseBuffer[0].Segment)
			//log.Println("Testec1:", responseBuffer2[0].res.Segment)
			//log.Println("Testec2:", int64(responseBuffer2[0].ResTime.Nanosecond())-int64(responseBuffer2[0].ReqTime.Nanosecond()))
			//responseBuffer2 = responseBuffer2[1:]
			//counter++
			//log.Println("Testec:", responseBuffer[0].Segment)

			timeReQ := int64(responseBuffer2[0].ReqTime.Nanosecond())
			timeReS := int64(responseBuffer2[0].ResTime.Nanosecond())
			secReQ := (int64(responseBuffer2[0].ReqTime.Second()) * 1000000000)
			secReS := (int64(responseBuffer2[0].ResTime.Second()) * 1000000000)
			Testec2 := int64((secReS + timeReS) - (secReQ + timeReQ))

			log.Println("Testec1:", responseBuffer2[0].res.Segment, "Testec2:", Testec2, "timeReQ:", timeReQ, "timeReS:", timeReS, "secReQ:", (secReQ + timeReQ), "secReS:", (secReS + timeReS), "responseBuffer2", len(responseBuffer2))

			responseBuffer2 = responseBuffer2[1:]
			log.Println("responseBuffer2", len(responseBuffer2))

			counter++
		} else {

			counter++
			flag1 = true
		}

		receiveLock.Unlock()

	}
	// TODO dequeue from buffer and simulate user watch behavior (1s sleep maybe?)
}
