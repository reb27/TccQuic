package test_client

import (
	"bufio"
	"fmt"
	"io/fs"
	"log"
	"main/src/model"
	"os"
	"time"
)

type StatisticsLogger struct {
	fileWriter *bufio.Writer
	file       fs.File
}

func NewStatisticsLogger(path string) *StatisticsLogger {
	const header string = "time_ns,segment,priority,latency_ns\n"

	file, err := os.Create(path)
	if err != nil {
		log.Panicf("Failed to open %s: %s\n", path, err)
	}
	fileWriter := bufio.NewWriter(file)

	if _, err := fileWriter.WriteString(header); err != nil {
		log.Panicf("Failed to write to %s: %s\n", path, err)
	}

	return &StatisticsLogger{
		fileWriter,
		file,
	}
}

func (s *StatisticsLogger) Log(timeFromStart time.Duration,
	r model.VideoPacketRequest, latency time.Duration) {
	row := fmt.Sprintf("%d,%d,%d,%d\n", timeFromStart.Nanoseconds(),
		r.Segment, r.Priority, latency.Nanoseconds())

	if _, err := s.fileWriter.WriteString(row); err != nil {
		log.Panicf("Failed to write: %s\n", err)
	}
}

func (s *StatisticsLogger) Close() {
	s.fileWriter.Flush()
	s.file.Close()
}
