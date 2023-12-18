package stream_handler

import (
	"encoding/json"
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
	req := s.receiveData(stream)
	if req == (model.VideoPacketRequest{}) {
		log.Println("Invalid request")
		stream.Close()
		return
	}

	// enqueue request processing
	ok := s.taskScheduler.Enqueue(req.Priority, func() {
		s.handleRequest(stream, req)
	})
	if !ok {
		log.Println("Task enqueue failed")
	}
}

func (s *StreamHandler) handleRequest(stream quic.Stream, req model.VideoPacketRequest) {
	defer stream.Close()

	log.Println("handleRequest segment =", req.Segment)
	// read file
	data := s.readFile(req.Bitrate, req.Segment, req.Tile)

	// send file response
	s.sendData(stream, req.Priority, req.Bitrate, req.Segment, req.Tile, data)
	log.Println("Response sent")
}

// Read file
func (s *StreamHandler) readFile(bitrate model.Bitrate, segment int, tile int) []byte {
	basePath, err := os.Getwd()
	if err != nil {
		log.Println(err)
	}
	// TODO check the file name logic
	//data, err := os.ReadFile(basePath + fmt.Sprintf("/data/segments/video_tiled_%d_dash_track%d_%d.m4s", bitrate, segment, tile))
	data, err := os.ReadFile(basePath + fmt.Sprintf("/data/segments/video_tiled_10_dash_track10_%d.m4s", segment))
	if err != nil {
		log.Println(err)
	}
	return data
}

// Receive file request
func (s *StreamHandler) receiveData(stream quic.Stream) (req model.VideoPacketRequest) {
	if err := json.NewDecoder(stream).Decode(&req); err != nil {
		log.Println(err)
	}
	// streamId := stream.StreamID()
	// fmt.Printf("Server stream %d: Got '%+v'\n", streamId, req)
	return
}

// Send file response
func (s *StreamHandler) sendData(stream quic.Stream, priority model.Priority, bitrate model.Bitrate, segment int, tile int, data []byte) {
	// streamId := stream.StreamID()
	// fmt.Printf("Server stream %d: Sending '%+v'\n", streamId, res)
	res := model.VideoPacketResponse{
		Priority: priority,
		Bitrate:  bitrate,
		Segment:  segment,
		Tile:     tile,
		Data:     data,
	}
	if err := json.NewEncoder(stream).Encode(&res); err != nil {
		log.Println(err)
	}
}
