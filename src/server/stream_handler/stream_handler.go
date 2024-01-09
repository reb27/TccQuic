package stream_handler

import (
	"bufio"
	"fmt"
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

func (s *StreamHandler) HandleStream(stream quic.Stream) {
	// receive file request
	req, err := model.ReadVideoPacketRequest(bufio.NewReader(stream))
	if err != nil {
		log.Println(err)
		stream.Close()
		return
	}

	// enqueue request processing
	ok := s.taskScheduler.Enqueue(req.Priority, func() {
		s.handleRequest(stream, req)
	})
	if !ok {
		log.Println("Task enqueue failed")
		stream.Close()
	}
}

func (s *StreamHandler) handleRequest(stream quic.Stream, req *model.VideoPacketRequest) {
	defer stream.Close()

	log.Println("handleRequest segment =", req.Segment)
	// read file
	data := s.readFile(req)

	// send file response
	res := model.VideoPacketResponse{
		Priority: req.Priority,
		Bitrate:  req.Bitrate,
		Segment:  req.Segment,
		Tile:     req.Tile,
		Data:     data,
	}
	if err := res.Write(stream); err != nil {
		log.Println(err)
	} else {
		log.Println("Response sent")
	}
}

// Read file
func (s *StreamHandler) readFile(req *model.VideoPacketRequest) []byte {
	basePath, err := os.Getwd()
	if err != nil {
		log.Println(err)
	}
	// TODO check the file name logic
	//data, err := os.ReadFile(basePath + fmt.Sprintf("/data/segments/video_tiled_%d_dash_track%d_%d.m4s", bitrate, segment, tile))
	filePath := fmt.Sprintf("/data/segments/video_tiled_10_dash_track%d_%d.m4s",
		req.Segment, req.Tile)
	data, err := os.ReadFile(basePath + filePath)
	if err != nil {
		log.Println(err)
	}
	return data
}
