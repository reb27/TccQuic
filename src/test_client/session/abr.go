package session

import (
	"log"
	"math"
	"strings"
	"time"

	"main/src/model"
)

type ABRController interface {
	SelectConfig(ctx SegmentContext) SegmentConfig
}

type SegmentContext struct {
	SegmentID           int
	FirstSegment        int
	LastSegment         int
	SegmentDuration     time.Duration
	TimeBudget          time.Duration
	AvgThroughput       float64
	BufferLevel         time.Duration
	FOVTiles            []int
	NearFOVTiles        []int
	AllTiles            []int
	ReadyThroughSegment int
}

type SegmentConfig struct {
	ID             string
	FOVBitrate     model.Bitrate
	NearFOVBitrate model.Bitrate
	NonFOVBitrate  model.Bitrate
}

type bufferAwareABR struct {
	sizeProvider TileSizeProvider
	debug        *legacyDebugLogger
}

const (
	// Kept for validation of the contiguous Legacy buffer metric. Throughput
	// selection no longer gates quality on these levels.
	legacyMediumBufferLevel = 1 * time.Second
	legacyHighBufferLevel   = 2 * time.Second
	// Conservative margins over the measured EWMA. This keeps Legacy
	// throughput-based while avoiding upgrades on barely sufficient samples.
	legacyMediumThroughputMargin = 1.05
	legacyHighThroughputMargin   = 1.10
	// Measured over segments 1..60 with the normal FoV trace. These values are
	// only used if the on-disk media estimator is unavailable.
	legacyFallbackMediumThreshold = 381_551.0
	legacyFallbackHighThreshold   = 431_738.0
)

func NewDefaultABRController(debugPath ...string) ABRController {
	var logger *legacyDebugLogger
	if len(debugPath) > 0 && debugPath[0] != "" {
		var err error
		logger, err = newLegacyDebugLogger(debugPath[0])
		if err != nil {
			log.Printf("Legacy ABR: failed to open debug CSV %s: %v", debugPath[0], err)
		}
	}
	estimator, err := NewTileSizeEstimator("data/segments")
	if err != nil {
		log.Printf("Legacy ABR: failed to build tile size estimator: %v (using measured fallback thresholds)", err)
		return newLegacyABRWithEstimatorAndDebug(nil, logger)
	}
	return newLegacyABRWithEstimatorAndDebug(estimator, logger)
}

func newLegacyABRWithEstimator(estimator TileSizeProvider) *bufferAwareABR {
	return newLegacyABRWithEstimatorAndDebug(estimator, nil)

}

func newLegacyABRWithEstimatorAndDebug(estimator TileSizeProvider, debug *legacyDebugLogger) *bufferAwareABR {
	return &bufferAwareABR{sizeProvider: estimator, debug: debug}
}

func SelectABRController(env Environment) ABRController {
	mode := strings.ToLower(strings.TrimSpace(env.ABRMode))
	switch mode {
	case "", "bola", "bola_finite", "bolafinite":
		return NewBOLAFiniteABR(env.BOLADebugPath, env.BOLAQmaxSegments)
	case "default", "legacy", "threshold":
		return NewDefaultABRController(env.LegacyDebugPath)
	default:
		log.Printf("Unknown ABR_MODE=%q, defaulting to BOLA", env.ABRMode)
		return NewBOLAFiniteABR(env.BOLADebugPath, env.BOLAQmaxSegments)
	}
}

func (c *bufferAwareABR) SelectConfig(ctx SegmentContext) SegmentConfig {
	mediumThreshold, highThreshold := c.thresholds(ctx)
	fovBitrate := c.selectBitrate(ctx.AvgThroughput, mediumThreshold, highThreshold)
	nearFOVBitrate := model.LOW_BITRATE
	if fovBitrate == model.HIGH_BITRATE {
		nearFOVBitrate = model.MEDIUM_BITRATE
	}
	cfg := SegmentConfig{
		ID:             legacyTierName(fovBitrate),
		FOVBitrate:     fovBitrate,
		NearFOVBitrate: nearFOVBitrate,
		NonFOVBitrate:  model.LOW_BITRATE,
	}
	c.debug.Log(ctx, cfg, mediumThreshold, highThreshold)
	return cfg
}

func (c *bufferAwareABR) Close() error {
	return c.debug.Close()
}

func (c *bufferAwareABR) selectBitrate(avgThroughput float64, mediumThreshold, highThreshold float64) model.Bitrate {
	if !validLegacyThroughputInput(avgThroughput, mediumThreshold, highThreshold) {
		log.Printf("ABR (Legacy): LOW invalid throughput input throughput=%.0f medium_threshold=%.0f high_threshold=%.0f", avgThroughput, mediumThreshold, highThreshold)
		return model.LOW_BITRATE
	}
	mediumRequired := legacyRequiredMediumThroughput(mediumThreshold)
	if avgThroughput < mediumRequired {
		log.Printf("ABR (Legacy): LOW throughput=%.0f medium_required=%.0f medium_threshold=%.0f", avgThroughput, mediumRequired, mediumThreshold)
		return model.LOW_BITRATE
	}
	if avgThroughput >= legacyRequiredHighThroughput(highThreshold) {
		return model.HIGH_BITRATE
	}
	return model.MEDIUM_BITRATE
}

func legacyRequiredMediumThroughput(mediumThreshold float64) float64 {
	return mediumThreshold * legacyMediumThroughputMargin
}

func legacyRequiredHighThroughput(highThreshold float64) float64 {
	return highThreshold * legacyHighThroughputMargin
}

func validLegacyThroughputInput(avgThroughput, mediumThreshold, highThreshold float64) bool {
	return avgThroughput > 0 &&
		mediumThreshold > 0 &&
		highThreshold >= mediumThreshold &&
		isFinite(avgThroughput) &&
		isFinite(mediumThreshold) &&
		isFinite(highThreshold)
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func (c *bufferAwareABR) thresholds(ctx SegmentContext) (medium, high float64) {
	if c.sizeProvider == nil || ctx.SegmentDuration <= 0 || len(ctx.AllTiles) == 0 {
		return legacyFallbackMediumThreshold, legacyFallbackHighThreshold
	}

	fov := make(map[int]struct{}, len(ctx.FOVTiles))
	for _, tileID := range ctx.FOVTiles {
		fov[tileID] = struct{}{}
	}
	near := make(map[int]struct{}, len(ctx.NearFOVTiles))
	for _, tileID := range ctx.NearFOVTiles {
		if _, inFOV := fov[tileID]; !inFOV {
			near[tileID] = struct{}{}
		}
	}

	mediumBytes := int64(0)
	highBytes := int64(0)
	for _, tileID := range ctx.AllTiles {
		mediumBitrate := model.LOW_BITRATE
		highBitrate := model.LOW_BITRATE
		if _, inFOV := fov[tileID]; inFOV {
			mediumBitrate = model.MEDIUM_BITRATE
			highBitrate = model.HIGH_BITRATE
		} else if _, nearFOV := near[tileID]; nearFOV {
			highBitrate = model.MEDIUM_BITRATE
		}
		mediumBytes += c.sizeProvider.AvgSize(tileID, mediumBitrate)
		highBytes += c.sizeProvider.AvgSize(tileID, highBitrate)
	}

	seconds := ctx.SegmentDuration.Seconds()
	medium = float64(mediumBytes) / seconds
	high = float64(highBytes) / seconds
	if medium <= 0 || high < medium {
		return legacyFallbackMediumThreshold, legacyFallbackHighThreshold
	}
	return medium, high
}

func legacyTierName(bitrate model.Bitrate) string {
	switch bitrate {
	case model.HIGH_BITRATE:
		return "legacy_high"
	case model.MEDIUM_BITRATE:
		return "legacy_med"
	default:
		return "legacy_low"
	}
}
