package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLegacyDebugWritesOneCompleteDecisionRow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-decisions.csv")
	logger, err := newLegacyDebugLogger(path)
	require.NoError(t, err)
	abr := newLegacyABRWithEstimatorAndDebug(nil, logger)

	abr.SelectConfig(SegmentContext{
		SegmentID:           7,
		BufferLevel:         legacyHighBufferLevel,
		ReadyThroughSegment: 6,
		AvgThroughput:       legacyFallbackHighThreshold,
	})
	require.NoError(t, abr.Close())

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	require.Len(t, lines, 2)
	require.Equal(t, "segment,time_ns,tier,fov_bitrate,near_fov_bitrate,background_bitrate,buffer_s,ready_through_segment,avg_throughput_bps,threshold_med_bps,threshold_high_bps", lines[0])
	fields := strings.Split(lines[1], ",")
	require.Equal(t, "7", fields[0])
	require.Equal(t, "HIGH", fields[2])
	require.Equal(t, "6", fields[7])
}
