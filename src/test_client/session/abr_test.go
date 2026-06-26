package session

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"main/src/model"
)

func TestLegacyNearFOVBitrateFollowsFOVHighOnly(t *testing.T) {
	abr := newLegacyABRWithEstimator(nil)

	tests := []struct {
		name        string
		throughput  float64
		buffer      time.Duration
		wantFOV     model.Bitrate
		wantNearFOV model.Bitrate
	}{
		{
			name:        "fov_high_promotes_near_fov_to_medium",
			throughput:  legacyRequiredHighThroughput(legacyFallbackHighThreshold),
			buffer:      0,
			wantFOV:     model.HIGH_BITRATE,
			wantNearFOV: model.MEDIUM_BITRATE,
		},
		{
			name:        "fov_medium_keeps_near_fov_low",
			throughput:  legacyRequiredMediumThroughput(legacyFallbackMediumThreshold),
			buffer:      2 * time.Second,
			wantFOV:     model.MEDIUM_BITRATE,
			wantNearFOV: model.LOW_BITRATE,
		},
		{
			name:        "low_fov_keeps_near_fov_low",
			throughput:  legacyRequiredMediumThroughput(legacyFallbackMediumThreshold) - 1,
			buffer:      10 * time.Second,
			wantFOV:     model.LOW_BITRATE,
			wantNearFOV: model.LOW_BITRATE,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := abr.SelectConfig(SegmentContext{
				AvgThroughput: tt.throughput,
				BufferLevel:   tt.buffer,
			})

			require.Equal(t, tt.wantFOV, cfg.FOVBitrate)
			require.Equal(t, tt.wantNearFOV, cfg.NearFOVBitrate)
			require.Equal(t, model.LOW_BITRATE, cfg.NonFOVBitrate)
		})
	}
}

func TestLegacyThresholdBoundariesAndSpatialConfiguration(t *testing.T) {
	abr := newLegacyABRWithEstimator(nil)
	tests := []struct {
		name       string
		throughput float64
		buffer     time.Duration
		want       model.Bitrate
	}{
		{"below_medium_margin", legacyRequiredMediumThroughput(legacyFallbackMediumThreshold) - 1, 0, model.LOW_BITRATE},
		{"at_medium_margin", legacyRequiredMediumThroughput(legacyFallbackMediumThreshold), 0, model.MEDIUM_BITRATE},
		{"below_high_margin", legacyRequiredHighThroughput(legacyFallbackHighThreshold) - 1, 0, model.MEDIUM_BITRATE},
		{"at_high_margin", legacyRequiredHighThroughput(legacyFallbackHighThreshold), 0, model.HIGH_BITRATE},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := abr.SelectConfig(SegmentContext{AvgThroughput: tt.throughput, BufferLevel: tt.buffer})
			require.Equal(t, tt.want, cfg.FOVBitrate)
			require.Equal(t, model.LOW_BITRATE, cfg.NonFOVBitrate)
			if tt.want == model.HIGH_BITRATE {
				require.Equal(t, model.MEDIUM_BITRATE, cfg.NearFOVBitrate)
			} else {
				require.Equal(t, model.LOW_BITRATE, cfg.NearFOVBitrate)
			}
		})
	}
}

func TestLegacyBufferDoesNotInfluenceThroughputDecision(t *testing.T) {
	abr := newLegacyABRWithEstimator(nil)

	tests := []struct {
		name       string
		throughput float64
		want       model.Bitrate
	}{
		{"low_throughput", legacyRequiredMediumThroughput(legacyFallbackMediumThreshold) - 1, model.LOW_BITRATE},
		{"medium_throughput", legacyRequiredMediumThroughput(legacyFallbackMediumThreshold), model.MEDIUM_BITRATE},
		{"high_throughput", legacyRequiredHighThroughput(legacyFallbackHighThreshold), model.HIGH_BITRATE},
	}

	buffers := []time.Duration{0, time.Nanosecond, time.Second, 10 * time.Second}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, buffer := range buffers {
				cfg := abr.SelectConfig(SegmentContext{
					AvgThroughput: tt.throughput,
					BufferLevel:   buffer,
				})
				require.Equal(t, tt.want, cfg.FOVBitrate, "buffer=%v", buffer)
			}
		})
	}
}

func TestLegacyInvalidThroughputOrThresholdsFallBackToLow(t *testing.T) {
	abr := newLegacyABRWithEstimator(nil)

	tests := []struct {
		name            string
		avgThroughput   float64
		mediumThreshold float64
		highThreshold   float64
	}{
		{"zero_throughput", 0, legacyFallbackMediumThreshold, legacyFallbackHighThreshold},
		{"negative_throughput", -1, legacyFallbackMediumThreshold, legacyFallbackHighThreshold},
		{"nan_throughput", math.NaN(), legacyFallbackMediumThreshold, legacyFallbackHighThreshold},
		{"infinite_throughput", math.Inf(1), legacyFallbackMediumThreshold, legacyFallbackHighThreshold},
		{"zero_medium_threshold", legacyFallbackHighThreshold, 0, legacyFallbackHighThreshold},
		{"nan_medium_threshold", legacyFallbackHighThreshold, math.NaN(), legacyFallbackHighThreshold},
		{"zero_high_threshold", legacyFallbackHighThreshold, legacyFallbackMediumThreshold, 0},
		{"high_below_medium_threshold", legacyFallbackHighThreshold, legacyFallbackHighThreshold, legacyFallbackMediumThreshold},
		{"infinite_high_threshold", legacyFallbackHighThreshold, legacyFallbackMediumThreshold, math.Inf(1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, model.LOW_BITRATE, abr.selectBitrate(tt.avgThroughput, tt.mediumThreshold, tt.highThreshold))
		})
	}
}

func TestLegacyTransitionsLowMediumHighMediumLow(t *testing.T) {
	abr := newLegacyABRWithEstimator(nil)
	contexts := []SegmentContext{
		{AvgThroughput: legacyRequiredMediumThroughput(legacyFallbackMediumThreshold) - 1, BufferLevel: 10 * time.Second},
		{AvgThroughput: legacyRequiredMediumThroughput(legacyFallbackMediumThreshold), BufferLevel: 0},
		{AvgThroughput: legacyRequiredHighThroughput(legacyFallbackHighThreshold), BufferLevel: 0},
		{AvgThroughput: legacyRequiredHighThroughput(legacyFallbackHighThreshold) - 1, BufferLevel: 10 * time.Second},
		{AvgThroughput: legacyRequiredMediumThroughput(legacyFallbackMediumThreshold) - 1, BufferLevel: 0},
	}
	want := []model.Bitrate{model.LOW_BITRATE, model.MEDIUM_BITRATE, model.HIGH_BITRATE, model.MEDIUM_BITRATE, model.LOW_BITRATE}

	for i, ctx := range contexts {
		require.Equal(t, want[i], abr.SelectConfig(ctx).FOVBitrate, "transition %d", i)
	}
}

func TestLegacyPlaybackBufferSampleDoesNotGateHighDecision(t *testing.T) {
	playback := NewPlaybackSimulator(time.Second, 0, 1, 5)
	playback.currentPlaybackSegment = 1
	now := time.Now()
	for segment := 1; segment <= 5; segment++ {
		playback.segmentPlaybackTime[segment] = now.Add(time.Duration(segment-1) * time.Second)
	}

	// Before segment 4 is scheduled, segments 2 and 3 are the maximum complete
	// future prefix allowed by the three-segment prefetch window.
	buffer := playback.GetBufferLevel(3)
	require.Equal(t, legacyHighBufferLevel, buffer)
	abr := newLegacyABRWithEstimator(nil)
	require.Equal(t, model.HIGH_BITRATE, abr.SelectConfig(SegmentContext{
		AvgThroughput: legacyRequiredHighThroughput(legacyFallbackHighThreshold),
		BufferLevel:   0,
	}).FOVBitrate)
	require.Equal(t, model.HIGH_BITRATE, abr.SelectConfig(SegmentContext{
		AvgThroughput: legacyRequiredHighThroughput(legacyFallbackHighThreshold),
		BufferLevel:   buffer,
	}).FOVBitrate)
}

func TestLegacyThresholdsUseRealSpatialConfigurationCost(t *testing.T) {
	estimator := &mapTileSizeProvider{byTile: map[int]map[model.Bitrate]int64{
		1: {model.LOW_BITRATE: 100, model.MEDIUM_BITRATE: 200, model.HIGH_BITRATE: 300},
		2: {model.LOW_BITRATE: 100, model.MEDIUM_BITRATE: 200, model.HIGH_BITRATE: 300},
		3: {model.LOW_BITRATE: 100, model.MEDIUM_BITRATE: 200, model.HIGH_BITRATE: 300},
	}}
	abr := newLegacyABRWithEstimator(estimator)
	ctx := SegmentContext{
		SegmentDuration: 2 * time.Second,
		FOVTiles:        []int{1},
		NearFOVTiles:    []int{2},
		AllTiles:        []int{1, 2, 3},
	}

	medium, high := abr.thresholds(ctx)
	require.Equal(t, 200.0, medium, "MED is FoV-MED plus all other tiles LOW")
	require.Equal(t, 300.0, high, "HIGH is FoV-HIGH, Near-MED and background LOW")
}

func TestLegacyThresholdsUseTileFieldFromDatasetFilename(t *testing.T) {
	dir := t.TempDir()
	for _, item := range []struct {
		name string
		size int
	}{
		{"video_tiled_5_dash_track42_7.m4s", 100},
		{"video_tiled_10_dash_track42_7.m4s", 200},
		{"video_tiled_15_dash_track42_7.m4s", 300},
		{"video_tiled_5_dash_track7_42.m4s", 900},
		{"video_tiled_10_dash_track7_42.m4s", 900},
		{"video_tiled_15_dash_track7_42.m4s", 900},
	} {
		writeSegmentFile(t, dir, item.name, item.size)
	}
	estimator, err := NewTileSizeEstimator(dir)
	require.NoError(t, err)
	abr := newLegacyABRWithEstimator(estimator)
	medium, high := abr.thresholds(SegmentContext{
		SegmentDuration: time.Second,
		FOVTiles:        []int{42},
		AllTiles:        []int{42},
	})
	require.Equal(t, 200.0, medium)
	require.Equal(t, 300.0, high)
}
