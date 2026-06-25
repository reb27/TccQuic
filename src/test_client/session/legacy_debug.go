package session

import (
	"bufio"
	"fmt"
	"os"
	"sync"
	"time"

	"main/src/model"
)

type legacyDebugLogger struct {
	mu     sync.Mutex
	file   *os.File
	writer *bufio.Writer
	start  time.Time
}

func newLegacyDebugLogger(path string) (*legacyDebugLogger, error) {
	file, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	writer := bufio.NewWriter(file)
	if _, err := writer.WriteString("segment,time_ns,tier,fov_bitrate,near_fov_bitrate,background_bitrate,buffer_s,ready_through_segment,avg_throughput_bps,threshold_med_bps,threshold_high_bps\n"); err != nil {
		file.Close()
		return nil, err
	}
	if err := writer.Flush(); err != nil {
		file.Close()
		return nil, err
	}
	return &legacyDebugLogger{file: file, writer: writer, start: time.Now()}, nil
}

func (l *legacyDebugLogger) Log(ctx SegmentContext, cfg SegmentConfig, mediumThreshold, highThreshold float64) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = fmt.Fprintf(l.writer, "%d,%d,%s,%d,%d,%d,%.6f,%d,%.6f,%.6f,%.6f\n",
		ctx.SegmentID,
		time.Since(l.start).Nanoseconds(),
		legacyTierLabel(cfg.FOVBitrate),
		cfg.FOVBitrate,
		cfg.NearFOVBitrate,
		cfg.NonFOVBitrate,
		ctx.BufferLevel.Seconds(),
		ctx.ReadyThroughSegment,
		ctx.AvgThroughput,
		mediumThreshold,
		highThreshold,
	)
	_ = l.writer.Flush()
}

func (l *legacyDebugLogger) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.writer.Flush(); err != nil {
		_ = l.file.Close()
		return err
	}
	return l.file.Close()
}

func legacyTierLabel(bitrate model.Bitrate) string {
	switch bitrate {
	case model.HIGH_BITRATE:
		return "HIGH"
	case model.MEDIUM_BITRATE:
		return "MED"
	default:
		return "LOW"
	}
}
