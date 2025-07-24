package model

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

type VideoPacketRequest struct {
	ID       uuid.UUID
	Priority Priority
	Bitrate  Bitrate
	Segment  int
	Tile     int
	// [milliseconds] If this timeout elapses, do not send a response.
	Timeout int
}

type VideoPacketResponse struct {
	Priority Priority
	Bitrate  Bitrate
	Segment  int
	Tile     int
	Data     []byte
}

// Write a VideoPacketRequest.
func (r *VideoPacketRequest) Write(writer io.Writer) (err error) {
	// Format mimics HTTP:
	// Headers - "Key: Value" separated by \n
	// Followed by empty line
	// Followed by optional data
	_, err = fmt.Fprintf(writer,
		"Priority: %d\nBitrate: %d\nSegment: %d\nTile: %d\nTimeout: %d\n\n",
		r.Priority, r.Bitrate, r.Segment, r.Tile, r.Timeout)
	return
}

// Read a VideoPacketRequest.
func ReadVideoPacketRequest(reader *bufio.Reader) (req *VideoPacketRequest, err error) {
	request := &VideoPacketRequest{}

	for {
		var line string
		if line, err = reader.ReadString('\n'); err != nil {
			return
		}

		line = line[:len(line)-1] // Removes the \n
		if len(line) == 0 {
			req = request
			return
		}

		kv := strings.SplitN(line, ":", 2)
		if len(kv) != 2 {
			err = errors.New("not a key value pair")
			return
		}

		key := strings.TrimSpace(kv[0])
		value := strings.TrimSpace(kv[1])

		switch key {
		case "Priority":
			var intValue int
			if intValue, err = strconv.Atoi(value); err != nil {
				return
			}
			request.Priority = Priority(intValue)
		case "Bitrate":
			var intValue int
			if intValue, err = strconv.Atoi(value); err != nil {
				return
			}
			request.Bitrate = Bitrate(intValue)
		case "Segment":
			var intValue int
			if intValue, err = strconv.Atoi(value); err != nil {
				return
			}
			request.Segment = intValue
		case "Tile":
			var intValue int
			if intValue, err = strconv.Atoi(value); err != nil {
				return
			}
			request.Tile = intValue
		case "Timeout":
			var intValue int
			if intValue, err = strconv.Atoi(value); err != nil {
				return
			}
			request.Timeout = intValue
		}
	}
}

// Write a VideoPacketResponse.
func (r *VideoPacketResponse) Write(writer *bufio.Writer) (err error) {
	// Format mimics HTTP:
	// Headers - "Key: Value" separated by \n
	// Followed by empty line
	// Followed by optional data
	_, err = fmt.Fprintf(writer,
		"Priority: %d\nBitrate: %d\nSegment: %d\nTile: %d\n"+
			"Content-Length: %d\n\n",
		r.Priority, r.Bitrate, r.Segment, r.Tile, len(r.Data))
	if err != nil {
		return err
	}

	_, err = writer.Write(r.Data)
	if err != nil {
		return err
	}

	err = writer.Flush()
	return
}

// Read a VideoPacketResponse.
func ReadVideoPacketResponse(reader *bufio.Reader) (res *VideoPacketResponse, err error) {
	response := &VideoPacketResponse{}

	contentLength := 0

	for {
		var line string
		if line, err = reader.ReadString('\n'); err != nil {
			return
		}

		line = line[:len(line)-1] // Removes the \n
		if len(line) == 0 {
			response.Data = make([]byte, contentLength)
			if _, err = io.ReadFull(reader, response.Data); err != nil {
				return
			}
			res = response
			return
		}

		kv := strings.SplitN(line, ":", 2)
		if len(kv) != 2 {
			err = errors.New("not a key value pair")
			return
		}

		key := strings.TrimSpace(kv[0])
		value := strings.TrimSpace(kv[1])

		switch key {
		case "Priority":
			var intValue int
			if intValue, err = strconv.Atoi(value); err != nil {
				return
			}
			response.Priority = Priority(intValue)
		case "Bitrate":
			var intValue int
			if intValue, err = strconv.Atoi(value); err != nil {
				return
			}
			response.Bitrate = Bitrate(intValue)
		case "Segment":
			var intValue int
			if intValue, err = strconv.Atoi(value); err != nil {
				return
			}
			response.Segment = intValue
		case "Tile":
			var intValue int
			if intValue, err = strconv.Atoi(value); err != nil {
				return
			}
			response.Tile = intValue
		case "Content-Length":
			if contentLength, err = strconv.Atoi(value); err != nil {
				return
			}
		}
	}
}
