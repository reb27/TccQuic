package session

import (
	"bufio"
	"fmt"
	"io/fs"
	"log"
	"os"
	"sync"
)

type ABRDecisionLogger struct {
	fileWriter *bufio.Writer
	mutex      sync.Mutex
	file       fs.File
}

func NewABRDecisionLogger(path string) *ABRDecisionLogger {
	if path == "" {
		return nil
	}
	const header = "segment,cfg_id,fov_bitrate,nonfov_bitrate,avg_throughput_Bps,buffer_level_s,time_budget_s,fov_tile_count,total_tile_count\n"
	file, err := os.Create(path)
	if err != nil {
		log.Panicf("Failed to open %s: %s\n", path, err)
	}
	fileWriter := bufio.NewWriter(file)
	if _, err := fileWriter.WriteString(header); err != nil {
		log.Panicf("Failed to write to %s: %s\n", path, err)
	}
	return &ABRDecisionLogger{fileWriter: fileWriter, file: file}
}

func (s *ABRDecisionLogger) Log(segment int, cfg SegmentConfig, ctx SegmentContext) {
	if s == nil {
		return
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	row := fmt.Sprintf("%d,%s,%d,%d,%.2f,%.3f,%.3f,%d,%d\n",
		segment,
		cfg.ID,
		cfg.FOVBitrate,
		cfg.NonFOVBitrate,
		ctx.AvgThroughput,
		ctx.BufferLevel.Seconds(),
		ctx.TimeBudget.Seconds(),
		len(ctx.FOVTiles),
		len(ctx.AllTiles),
	)
	if _, err := s.fileWriter.WriteString(row); err != nil {
		log.Panicf("Failed to write: %s\n", err)
	}
}

func (s *ABRDecisionLogger) Close() {
	if s == nil {
		return
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.fileWriter.Flush()
	s.file.Close()
}
