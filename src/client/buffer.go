package client

import (
	"bufio"
	"fmt"
	"log"
	"main/src/model"
	"os"
	"strconv"
	"strings"
	"sync"
)

// TODO implement Buffer logic, attributes and functions (simple queue and dequeue)
type Buffer struct {
}

// Implementação da interface Configurable
type ConfigurableImpl struct {
	semaphoreCount int
	tiles          int
	segments       int
	segmentTime    int
	adaptRate      []int
	requestSize    int
	responseSize   int
	initialSize    int
}

func NewConfigurable() *ConfigurableImpl {
	return &ConfigurableImpl{}
}

func (c *ConfigurableImpl) SetSemaphoreCount(count int) {
	c.semaphoreCount = count
}

func (c *ConfigurableImpl) SetTiles(size int) {
	c.tiles = size
}

func (c *ConfigurableImpl) SetSegments(size int) {
	c.segments = size
}

func (c *ConfigurableImpl) SetSegmentTime(size int) {
	c.segmentTime = size
}

func (c *ConfigurableImpl) SetAdaptRate(array []int) {
	c.adaptRate = array
}

func (c *ConfigurableImpl) SetRequestBufferSize(size int) {
	c.requestSize = size
}

func (c *ConfigurableImpl) SetResponseBufferSize(size int) {
	c.responseSize = size
}

func (c *ConfigurableImpl) SetInitialResponseBufferSize(size int) {
	c.initialSize = size
}
func (c *ConfigurableImpl) GetSemaphoreCount() int {
	return c.semaphoreCount
}

func (c *ConfigurableImpl) GetTiles() int {
	return c.tiles
}

func (c *ConfigurableImpl) GetSegments() int {
	return c.segments
}

func (c *ConfigurableImpl) GetSegmentTime() int {
	return c.segmentTime
}

func (c *ConfigurableImpl) GetAdaptRate() []int {
	return c.adaptRate
}

func (c *ConfigurableImpl) GetRequestBufferSize() int {
	return c.requestSize
}

func (c *ConfigurableImpl) GetResponseBufferSize() int {
	return c.responseSize
}

func (c *ConfigurableImpl) GetInitialResponseBufferSize() int {
	return c.initialSize
}
func (c *ConfigurableImpl) ReadFile(filename string) error {
	// Open the file for reading
	log.Println("scanner")

	file, err := os.Open("configFile.txt")
	if err != nil {
		return err
	}
	defer file.Close()

	// Create a scanner to read the file line by line
	scanner := bufio.NewScanner(file)

	// Loop over the file lines
	for scanner.Scan() {
		// Get the line
		line := scanner.Text()

		// Split the line by the first whitespace
		parts := strings.SplitN(line, " ", 2)

		// Check if the line has at least two parts (a variable name and a value)
		if len(parts) < 2 {
			continue // Skip invalid lines
		}

		// Get the variable name and value from the line parts
		name := parts[0]
		value := parts[1]

		// Use a switch statement to modify the corresponding field of c that matches the name
		switch name {
		case "semaphoreCount":
			c.semaphoreCount, err = strconv.Atoi(value)
			if err != nil {
				return err
			}
		case "tiles":
			c.tiles, err = strconv.Atoi(value)
			if err != nil {
				return err
			}
		case "segments":
			c.segments, err = strconv.Atoi(value)
			if err != nil {
				return err
			}
		case "segmentTime":
			c.segmentTime, err = strconv.Atoi(value)
			if err != nil {
				return err
			}
		case "adaptRate":
			c.adaptRate, err = ParseArray(value)
			log.Println("aaaaui", value)
			if err != nil {
				return err
			}
		case "requestSize":
			c.requestSize, err = strconv.Atoi(value)
			if err != nil {
				return err
			}
		case "responseSize":
			c.responseSize, err = strconv.Atoi(value)
			if err != nil {
				return err
			}
		case "initialSize":
			c.initialSize, err = strconv.Atoi(value)
			if err != nil {
				return err
			}
		default:
			fmt.Println("Unknown variable:", name)
			continue // Skip unknown variables
		}
	}
	return err
}

// Implementação da interface RequestBuffer
type RequestBufferImpl struct {
	buffer []model.VideoPacketRequest
	lock   sync.Mutex
}

func NewRequestBuffer() *RequestBufferImpl {
	return &RequestBufferImpl{
		buffer: make([]model.VideoPacketRequest, 0),
	}
}

func (rb *RequestBufferImpl) AddRequest(request model.VideoPacketRequest, position int) {
	rb.lock.Lock()
	defer rb.lock.Unlock()

	if position >= len(rb.buffer) {
		rb.buffer = append(rb.buffer, request)
	} else {
		rb.buffer = append(rb.buffer[:position+1], rb.buffer[position:]...)
		rb.buffer[position] = request
	}
}

func (rb *RequestBufferImpl) GetBuffer() []model.VideoPacketRequest {
	rb.lock.Lock()
	defer rb.lock.Unlock()

	return rb.buffer
}

func (rb *RequestBufferImpl) DeleteTilesBySegment(segment int) {
	rb.lock.Lock()
	defer rb.lock.Unlock()

	filtered := make([]model.VideoPacketRequest, 0)
	for _, request := range rb.buffer {
		if request.Segment != segment {
			filtered = append(filtered, request)
		}
	}
	rb.buffer = filtered
}

func (rb *RequestBufferImpl) Lock() {
	rb.lock.Lock()
}

func (rb *RequestBufferImpl) Unlock() {
	rb.lock.Unlock()
}

func (rb *RequestBufferImpl) Size() int {
	rb.lock.Lock()
	defer rb.lock.Unlock()

	return len(rb.buffer)
}

func (rb *RequestBufferImpl) SetInitialSize(size int) {
	rb.buffer = make([]model.VideoPacketRequest, size)
}

// Implementação da interface ResponseBuffer
type ResponseBufferImpl struct {
	buffer []model.VideoPacketResponse
	lock   sync.Mutex
}

func NewResponseBuffer() *ResponseBufferImpl {
	return &ResponseBufferImpl{
		buffer: make([]model.VideoPacketResponse, 0),
	}
}

func (rb *ResponseBufferImpl) AddResponse(response model.VideoPacketResponse, position int) {
	rb.lock.Lock()
	defer rb.lock.Unlock()

	if position >= len(rb.buffer) {
		rb.buffer = append(rb.buffer, response)
	} else {
		rb.buffer = append(rb.buffer[:position+1], rb.buffer[position:]...)
		rb.buffer[position] = response
	}
}

func (rb *ResponseBufferImpl) GetBuffer() []model.VideoPacketResponse {
	rb.lock.Lock()
	defer rb.lock.Unlock()

	return rb.buffer
}

func (rb *ResponseBufferImpl) DeleteTilesBySegment(segment int) {
	rb.lock.Lock()
	defer rb.lock.Unlock()

	filtered := make([]model.VideoPacketResponse, 0)
	for _, response := range rb.buffer {
		if response.Segment != segment {
			filtered = append(filtered, response)
		}
	}
	rb.buffer = filtered
}

func (rb *ResponseBufferImpl) Lock() {
	rb.lock.Lock()
}

func (rb *ResponseBufferImpl) Unlock() {
	rb.lock.Unlock()
}

func (rb *ResponseBufferImpl) Size() int {
	rb.lock.Lock()
	defer rb.lock.Unlock()

	return len(rb.buffer)
}

func (rb *ResponseBufferImpl) SetInitialSize(size int) {
	rb.buffer = make([]model.VideoPacketResponse, size)
}

func (rb *ResponseBufferImpl) GetInstantBitrate() float64 {
	rb.lock.Lock()
	defer rb.lock.Unlock()

	if len(rb.buffer) > 0 {
		return float64(rb.buffer[len(rb.buffer)-1].Bitrate)
	}
	return 0.0
}
