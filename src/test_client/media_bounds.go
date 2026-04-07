package test_client

import (
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

var segmentFilePattern = regexp.MustCompile(`^video_tiled_10_dash_track(\d+)_(\d+)\.m4s$`)

// detectMediaBounds scans data/segments and returns (firstSegment, lastSegment, firstTile, lastTile, ok).
// It auto-adapts test ranges to the dataset layout available on disk.
func detectMediaBounds() (int, int, int, int, bool) {
	basePath, err := os.Getwd()
	if err != nil {
		return 0, 0, 0, 0, false
	}
	segmentsDir := filepath.Join(basePath, "data", "segments")
	entries, err := os.ReadDir(segmentsDir)
	if err != nil {
		return 0, 0, 0, 0, false
	}

	firstSeg, lastSeg := 0, 0
	firstTile, lastTile := 0, 0
	found := false
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := segmentFilePattern.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		seg, err1 := strconv.Atoi(m[1])
		tile, err2 := strconv.Atoi(m[2])
		if err1 != nil || err2 != nil {
			continue
		}
		if !found {
			firstSeg, lastSeg = seg, seg
			firstTile, lastTile = tile, tile
			found = true
			continue
		}
		if seg < firstSeg {
			firstSeg = seg
		}
		if seg > lastSeg {
			lastSeg = seg
		}
		if tile < firstTile {
			firstTile = tile
		}
		if tile > lastTile {
			lastTile = tile
		}
	}
	if !found {
		return 0, 0, 0, 0, false
	}
	log.Printf("Detected media bounds from dataset: segments=%d..%d tiles=%d..%d", firstSeg, lastSeg, firstTile, lastTile)
	return firstSeg, lastSeg, firstTile, lastTile, true
}

