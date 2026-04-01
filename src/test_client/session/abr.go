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
	AllTiles        []int
}

type SegmentConfig struct {
	ID            string
	FOVBitrate    model.Bitrate
	NonFOVBitrate model.Bitrate
}

type BitrateInfo struct {
	Bitrate   model.Bitrate
	Threshold float64
}

type bufferAwareABR struct {
	bitrates []BitrateInfo
}

type fixedABR struct {
	config SegmentConfig
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

func NewFixedABRController() ABRController {
	return NewFixedABRControllerWithID("fixed_all_low")
}

func NewFixedABRControllerWithID(id string) ABRController {
	return &fixedABR{
		config: SegmentConfig{
			ID:            id,
			FOVBitrate:    model.LOW_BITRATE,
			NonFOVBitrate: model.LOW_BITRATE,
		},
	}
}

func SelectABRController(env Environment) ABRController {
	mode := strings.ToLower(strings.TrimSpace(env.ABRMode))
	switch mode {
	case "fixed":
		return NewFixedABRController()
	case "article", "article50":
		return NewFixedABRControllerWithID("article50_all_low")
	case "article30":
		return NewFixedABRControllerWithID("article30_all_low")
	case "", "bola", "bola_finite", "bolafinite":
		return NewBOLAFiniteABR()
	case "default", "legacy", "threshold":
		return NewDefaultABRController()
	default:
		log.Printf("Unknown ABR_MODE=%q, defaulting to BOLA", env.ABRMode)
		return NewBOLAFiniteABR()
	}
}

func (c *fixedABR) SelectConfig(ctx SegmentContext) SegmentConfig {
	return c.config
}

func (c *bufferAwareABR) SelectConfig(ctx SegmentContext) SegmentConfig {
	fovBitrate := c.selectBitrate(ctx.AvgThroughput, ctx.BufferLevel, true)
	return SegmentConfig{
		ID:            "default",
		FOVBitrate:    fovBitrate,
		NonFOVBitrate: model.LOW_BITRATE,
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
