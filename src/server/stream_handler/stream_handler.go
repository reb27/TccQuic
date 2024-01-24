package stream_handler

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"main/src/model"
	"os"

	"github.com/lucas-clemente/quic-go"
)

// This class is responsible for handling incoming streams
type StreamHandler struct {
	taskScheduler TaskScheduler
}

func NewStreamHandler(policy QueuePolicy) *StreamHandler {
	return &StreamHandler{
		taskScheduler: NewTaskScheduler(policy),
	}
}

func (s *StreamHandler) Start() {
	go s.taskScheduler.Run()
}

func (s *StreamHandler) Stop() {
	s.taskScheduler.Stop()
}

func (s *StreamHandler) HandleStream(quicStream quic.Stream) {
	go (&stream{
		taskScheduler: s.taskScheduler,
		quicStream:    quicStream,
		reader:        bufio.NewReader(quicStream),
		writer:        bufio.NewWriter(quicStream),
		usageCount:    0,
	}).listen()
}

type stream struct {
	taskScheduler TaskScheduler
	quicStream    quic.Stream
	reader        *bufio.Reader
	writer        *bufio.Writer

	// Increased for each pending request
	usageCount int
}

func (s *stream) decreaseUsageCount() {
	s.usageCount--
	if s.usageCount == 0 {
		s.quicStream.Close()
	}
}

func (s *stream) listen() {
	s.usageCount++
	defer s.decreaseUsageCount()
	// until the stream is closed
	for {
		// receive file request
		req, err := model.ReadVideoPacketRequest(s.reader)
		if req == nil {
			if err != nil && err != io.EOF {
				log.Println(err)
			}
			return
		}

		// enqueue request processing
		s.usageCount++
		ok := s.taskScheduler.Enqueue(req.Priority, func() {
			defer s.decreaseUsageCount()
			s.handleRequest(req)
		})
		if !ok {
			log.Println("Task enqueue failed")
			return
		}
	}
}

func (s *stream) handleRequest(req *model.VideoPacketRequest) {
	log.Println("handleRequest segment =", req.Segment)
	// read file
	data := readFile(req)

	// send file response
	res := model.VideoPacketResponse{
		Priority: req.Priority,
		Bitrate:  req.Bitrate,
		Segment:  req.Segment,
		Tile:     req.Tile,
		Data:     data,
	}
	if err := res.Write(s.writer); err != nil {
		log.Println(err)
	} else {
		log.Println("Response sent")
	}
}

// Read file
func readFile(req *model.VideoPacketRequest) []byte {
	basePath, err := os.Getwd()
	if err != nil {
		log.Println(err)
	}
	filePath := fmt.Sprintf("/data/segments/video_tiled_10_dash_track%d_%d.m4s",
		req.Segment, req.Tile)
	data, err := os.ReadFile(basePath + filePath)
	if err != nil {
		log.Println(err)
	}
	return data
}
