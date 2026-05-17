package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"main/src/model"
)

func TestTileSizeEstimatorUsesRepresentationForRequestedBitrate(t *testing.T) {
	dir := t.TempDir()
	writeSegmentFile(t, dir, "video_tiled_5_dash_track1_1.m4s", 10)
	writeSegmentFile(t, dir, "video_tiled_10_dash_track1_1.m4s", 20)
	writeSegmentFile(t, dir, "video_tiled_15_dash_track1_1.m4s", 30)

	estimator, err := NewTileSizeEstimator(dir)
	require.NoError(t, err)

	require.EqualValues(t, 10, estimator.AvgSize(1, model.LOW_BITRATE))
	require.EqualValues(t, 20, estimator.AvgSize(1, model.MEDIUM_BITRATE))
	require.EqualValues(t, 30, estimator.AvgSize(1, model.HIGH_BITRATE))
}

func TestTileSizeEstimatorFallsBackByBitrateThenGlobal(t *testing.T) {
	dir := t.TempDir()
	writeSegmentFile(t, dir, "video_tiled_5_dash_track1_1.m4s", 10)
	writeSegmentFile(t, dir, "video_tiled_10_dash_track1_1.m4s", 20)
	writeSegmentFile(t, dir, "video_tiled_15_dash_track1_1.m4s", 30)

	estimator, err := NewTileSizeEstimator(dir)
	require.NoError(t, err)

	require.EqualValues(t, 10, estimator.AvgSize(99, model.LOW_BITRATE))
	require.EqualValues(t, 20, estimator.AvgSize(99, model.MEDIUM_BITRATE))
	require.EqualValues(t, 30, estimator.AvgSize(99, model.HIGH_BITRATE))
	require.EqualValues(t, 20, estimator.AvgSize(99, model.Bitrate(123)))
}

func writeSegmentFile(t *testing.T, dir string, name string, size int) {
	t.Helper()
	err := os.WriteFile(filepath.Join(dir, name), []byte(strings.Repeat("x", size)), 0o644)
	require.NoError(t, err)
}
