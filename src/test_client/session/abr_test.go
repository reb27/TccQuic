package session

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"main/src/model"
)

func TestLegacyNearFOVBitrateFollowsFOVHighOnly(t *testing.T) {
	abr := NewDefaultABRController()

	tests := []struct {
		name        string
		throughput  float64
		buffer      time.Duration
		wantFOV     model.Bitrate
		wantNearFOV model.Bitrate
	}{
		{
			name:        "fov_high_promotes_near_fov_to_medium",
			throughput:  60_000,
			buffer:      2 * time.Second,
			wantFOV:     model.HIGH_BITRATE,
			wantNearFOV: model.MEDIUM_BITRATE,
		},
		{
			name:        "fov_medium_keeps_near_fov_low",
			throughput:  30_000,
			buffer:      2 * time.Second,
			wantFOV:     model.MEDIUM_BITRATE,
			wantNearFOV: model.LOW_BITRATE,
		},
		{
			name:        "low_buffer_keeps_near_fov_low",
			throughput:  60_000,
			buffer:      time.Second,
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
