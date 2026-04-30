package test_client

import (
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
)

var segmentFilePattern = regexp.MustCompile(`^video_tiled_(\d+)_dash_track(\d+)_(\d+)\.m4s$`)

// baseRepFilePattern matches only the canonical representation (default 10) to avoid
// counting the same segment three times when multiple bitrates exist on disk.
var baseRepFilePattern = regexp.MustCompile(`^video_tiled_10_dash_track(\d+)_(\d+)\.m4s$`)

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
		// m[1] = representation, m[2] = segment, m[3] = tile
		seg, err1 := strconv.Atoi(m[2])
		tile, err2 := strconv.Atoi(m[3])
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

// detectValidSegmentIDs returns sorted unique segment IDs that have at least one
// base-representation (rep 10) tile file. Datasets may omit some segment numbers
// between min and max; iterating only these IDs avoids mass "file missing" noise.
func detectValidSegmentIDs() ([]int, bool) {
	basePath, err := os.Getwd()
	if err != nil {
		return nil, false
	}
	segmentsDir := filepath.Join(basePath, "data", "segments")
	entries, err := os.ReadDir(segmentsDir)
	if err != nil {
		return nil, false
	}
	seen := make(map[int]struct{})
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := baseRepFilePattern.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		seg, err1 := strconv.Atoi(m[1])
		if err1 != nil {
			continue
		}
		seen[seg] = struct{}{}
	}
	if len(seen) == 0 {
		return nil, false
	}
	out := make([]int, 0, len(seen))
	for seg := range seen {
		out = append(out, seg)
	}
	sort.Ints(out)
	log.Printf("Detected %d segment IDs present on disk (rep=10 index): first=%d last=%d", len(out), out[0], out[len(out)-1])
	return out, true
}
