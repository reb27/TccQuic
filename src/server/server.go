package server

//go run main.go server wfq
import (
	"context"
	"fmt"
	"log"
	"main/src/server/stream_handler"
	"time"

	"github.com/lucas-clemente/quic-go"
)

type Server struct {
	serverURL   string
	serverPort  int
	queuePolicy stream_handler.QueuePolicy
}

func NewServer(serverURL string, serverPort int, queuePolicy string) *Server {
	return &Server{
		serverURL:   serverURL,
		serverPort:  serverPort,
		queuePolicy: stream_handler.QueuePolicy(queuePolicy),
	}
}

func (s *Server) Start() {

	url := fmt.Sprintf("%s:%d", s.serverURL, s.serverPort)
	config := &quic.Config{
		MaxIdleTimeout:        500 * time.Minute, // Set a longer maximum idle timeout
		HandshakeIdleTimeout:  100 * time.Second, // Set the receive connection flow control window size to 20 MB
		MaxIncomingStreams:    20000,             // Set the maximum number of incoming streams
		MaxIncomingUniStreams: 20000,             // Set the maximum number of incoming unidirectional streams
	}
	listener, err := quic.ListenAddr(url, generateTLSConfig(), config)
	if err != nil {
		log.Println(err)
	}

	log.Println("Server listening on", url)

	for {

		connection, err := listener.Accept(context.Background())
		if err != nil {
			log.Println(err)
		}
		if err == nil {
			s.onConnectionAccepted(connection)
		}

	}
}

func (s *Server) onConnectionAccepted(connection quic.Connection) {
	streamHandler := stream_handler.NewStreamHandler(s.queuePolicy)

	// accept streams in background
	go func() {
		for {
			stream, err := connection.AcceptStream(context.Background())
			if err != nil {
				log.Println(err)
			}
			if streamFinished := connection.Context().Err(); streamFinished != nil {
				streamHandler.Stop()
				return
			}
			if err == nil {
				streamHandler.HandleStream(stream)
			}
		}
	}()

	// handle streams, non blocking
	streamHandler.Start()
}
