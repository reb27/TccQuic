package model

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
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
	FoV      bool // Indica se o tile está no campo de visão
	// Prioridade semântica pi (0..1) para VoI/WFQ: FoV=1.0, Near-FoV=0.6, Background=0.2. Se 0, deriva-se de FoV.
	SemanticPriority float32
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
		"Priority: %d\nBitrate: %d\nSegment: %d\nTile: %d\nFoV: %t\nSemanticPriority: %g\nTimeout: %d\n\n",
		r.Priority, r.Bitrate, r.Segment, r.Tile, r.FoV, r.SemanticPriority, r.Timeout)
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
		case "FoV":
			request.FoV = (value == "true")
		case "SemanticPriority":
			var f float64
			if _, parseErr := fmt.Sscanf(value, "%f", &f); parseErr == nil {
				request.SemanticPriority = float32(f)
			}
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

// EstimateTileSize retorna o tamanho em bytes do arquivo do tile no disco (estimativa para VoI).
func EstimateTileSize(req *VideoPacketRequest) int64 {
	basePath, _ := os.Getwd()
	full := fmt.Sprintf("%s/data/segments/video_tiled_10_dash_track%d_%d.m4s",
		basePath, req.Tile, req.Segment)
	st, err := os.Stat(full)
	if err != nil {
		return 200000 // Fallback se arquivo não existir
	}
	return st.Size()
}
