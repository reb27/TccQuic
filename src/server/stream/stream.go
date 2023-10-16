package stream

import (
	"context"
	"time"

	"github.com/lucas-clemente/quic-go"
)

type Stream struct {
	inner    quic.Stream
	isClosed bool
	Priority float32
}

func newStream(stream quic.Stream, priority float32) *Stream {
	return &Stream{
		inner:    stream,
		isClosed: false,
		Priority: priority,
	}
}

func (s *Stream) Close() error {
	err := s.inner.Close()
	if err != nil {
		s.isClosed = true
	}
	return err
}

func (s *Stream) Read(p []byte) (int, error) {
	return s.inner.Read(p)
}
func (s *Stream) Write(p []byte) (int, error) {
	return s.inner.Write(p)
}
func (s *Stream) StreamID() quic.StreamID {
	return s.inner.StreamID()
}
func (s *Stream) CancelRead(code quic.StreamErrorCode) {
	s.inner.CancelRead(code)
}
func (s *Stream) SetReadDeadline(t time.Time) error {
	return s.inner.SetReadDeadline(t)
}
func (s *Stream) CancelWrite(code quic.StreamErrorCode) {
	s.inner.CancelWrite(code)
}
func (s *Stream) Context() context.Context {
	return s.inner.Context()
}
func (s *Stream) SetWriteDeadline(t time.Time) error {
	return s.inner.SetWriteDeadline(t)
}
func (s *Stream) SetDeadline(t time.Time) error {
	return s.inner.SetDeadline(t)
}
