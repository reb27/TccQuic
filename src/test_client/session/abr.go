package session

import (
	"log"
	"strings"
	"time"

	"main/src/model"
)

type ABRController interface {
	SelectConfig(ctx SegmentContext) SegmentConfig
}

type SegmentContext struct {
	SegmentID       int
	FirstSegment    int
	LastSegment     int
	SegmentDuration time.Duration
	TimeBudget      time.Duration
	AvgThroughput   float64
	BufferLevel     time.Duration
	FOVTiles        []int
	NearFOVTiles    []int
	AllTiles        []int
}

type SegmentConfig struct {
	ID             string
	FOVBitrate     model.Bitrate
	NearFOVBitrate model.Bitrate
	NonFOVBitrate  model.Bitrate
}

type BitrateInfo struct {
	Bitrate   model.Bitrate
	Threshold float64
}

type bufferAwareABR struct {
	bitrates []BitrateInfo
}

const (
	minBufferLevel = 2 * time.Second
	maxBufferLevel = 10 * time.Second
)

func NewDefaultABRController() ABRController {
	return &bufferAwareABR{
		bitrates: []BitrateInfo{
			{Bitrate: model.HIGH_BITRATE, Threshold: 60000.0},
			{Bitrate: model.MEDIUM_BITRATE, Threshold: 30000.0},
			{Bitrate: model.LOW_BITRATE, Threshold: 0.0},
		},
	}
}

func SelectABRController(env Environment) ABRController {
	mode := strings.ToLower(strings.TrimSpace(env.ABRMode))
	switch mode {
	case "", "bola", "bola_finite", "bolafinite":
		return NewBOLAFiniteABR(env.BOLADebugPath, env.BOLAQmaxSegments)
	case "default", "legacy", "threshold":
		return NewDefaultABRController()
	default:
		log.Printf("Unknown ABR_MODE=%q, defaulting to BOLA", env.ABRMode)
		return NewBOLAFiniteABR(env.BOLADebugPath, env.BOLAQmaxSegments)
	}
}

func (c *bufferAwareABR) SelectConfig(ctx SegmentContext) SegmentConfig {
	fovBitrate := c.selectBitrate(ctx.AvgThroughput, ctx.BufferLevel, true)
	nearFOVBitrate := model.LOW_BITRATE
	if fovBitrate == model.HIGH_BITRATE {
		nearFOVBitrate = model.MEDIUM_BITRATE
	}
	return SegmentConfig{
		ID:             "default",
		FOVBitrate:     fovBitrate,
		NearFOVBitrate: nearFOVBitrate,
		NonFOVBitrate:  model.LOW_BITRATE,
	}
}

func (c *bufferAwareABR) selectBitrate(avgThroughput float64, bufferLevel time.Duration, inFOV bool) model.Bitrate {
	if !inFOV {
		return model.LOW_BITRATE
	}
	if bufferLevel < minBufferLevel {
		log.Printf("ABR (Buffer): Buffer level (%v) is below minimum (%v). Forcing LOW_BITRATE.", bufferLevel, minBufferLevel)
		return model.LOW_BITRATE
	}
	if bufferLevel > maxBufferLevel {
	}
	for _, brInfo := range c.bitrates {
		if avgThroughput >= brInfo.Threshold {
			return brInfo.Bitrate
		}
	}
	return model.LOW_BITRATE
}
