package test_client

import (
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"

	"main/src/test_client/session"
)

var segmentFilePattern = regexp.MustCompile(`^video_tiled_(\d+)_dash_track(\d+)_(\d+)\.m4s$`)
var baseRepFilePattern = regexp.MustCompile(`^video_tiled_10_dash_track(\d+)_(\d+)\.m4s$`)

func segmentsDirectory() (string, bool) {
	basePath, err := os.Getwd()
	if err != nil {
		return "", false
	}
	return filepath.Join(basePath, "data", "segments"), true
}

// detectMediaBounds preserves the historical track<SEGMENT>_<TILE> interpretation
// used by BOLA.
func detectMediaBounds() (int, int, int, int, bool) {
	dir, ok := segmentsDirectory()
	if !ok {
		return 0, 0, 0, 0, false
	}
	return detectMediaBoundsAt(dir)
}

func detectMediaBoundsAt(dir string) (int, int, int, int, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0, 0, 0, false
	}
	firstSeg, lastSeg, firstTile, lastTile, found := 0, 0, 0, 0, false
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		m := segmentFilePattern.FindStringSubmatch(entry.Name())
		if m == nil {
			continue
		}
		seg, err1 := strconv.Atoi(m[2])
		tile, err2 := strconv.Atoi(m[3])
		if err1 != nil || err2 != nil {
			continue
		}
		if !found {
			firstSeg, lastSeg, firstTile, lastTile, found = seg, seg, tile, tile, true
			continue
		}
		firstSeg, lastSeg = minInt(firstSeg, seg), maxInt(lastSeg, seg)
		firstTile, lastTile = minInt(firstTile, tile), maxInt(lastTile, tile)
	}
	if found {
		log.Printf("Detected media bounds from dataset: segments=%d..%d tiles=%d..%d", firstSeg, lastSeg, firstTile, lastTile)
	}
	return firstSeg, lastSeg, firstTile, lastTile, found
}

// detectValidSegmentIDs preserves the historical first track component as the
// segment ID. Legacy uses detectLegacyMediaLayout instead.
func detectValidSegmentIDs() ([]int, bool) {
	dir, ok := segmentsDirectory()
	if !ok {
		return nil, false
	}
	return detectValidSegmentIDsAt(dir, 1)
}

// detectLegacyMediaLayout interprets the actual dataset layout
// track<TILE>_<SEGMENT> and returns its exact per-segment tile universe.
func detectLegacyMediaLayout() (int, int, int, int, *session.MediaLayout, bool) {
	dir, ok := segmentsDirectory()
	if !ok {
		return 0, 0, 0, 0, nil, false
	}
	return detectLegacyMediaLayoutAt(dir)
}

func detectLegacyMediaLayoutAt(dir string) (int, int, int, int, *session.MediaLayout, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0, 0, 0, nil, false
	}
	firstSeg, lastSeg, firstTile, lastTile, found := 0, 0, 0, 0, false
	segmentTiles := make(map[int][]int)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		m := baseRepFilePattern.FindStringSubmatch(entry.Name())
		if m == nil {
			continue
		}
		tile, err1 := strconv.Atoi(m[1])
		seg, err2 := strconv.Atoi(m[2])
		if err1 != nil || err2 != nil {
			continue
		}
		segmentTiles[seg] = append(segmentTiles[seg], tile)
		if !found {
			firstSeg, lastSeg, firstTile, lastTile, found = seg, seg, tile, tile, true
			continue
		}
		firstSeg, lastSeg = minInt(firstSeg, seg), maxInt(lastSeg, seg)
		firstTile, lastTile = minInt(firstTile, tile), maxInt(lastTile, tile)
	}
	if !found {
		return 0, 0, 0, 0, nil, false
	}
	for seg := range segmentTiles {
		sort.Ints(segmentTiles[seg])
	}
	log.Printf("Detected Legacy media layout (track<TILE>_<SEGMENT>): segments=%d..%d tiles=%d..%d", firstSeg, lastSeg, firstTile, lastTile)
	return firstSeg, lastSeg, firstTile, lastTile, &session.MediaLayout{SegmentTiles: segmentTiles}, true
}

func detectLegacyValidSegmentIDs() ([]int, bool) {
	dir, ok := segmentsDirectory()
	if !ok {
		return nil, false
	}
	return detectValidSegmentIDsAt(dir, 2)
}

func detectValidSegmentIDsAt(dir string, trackComponent int) ([]int, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, false
	}
	seen := make(map[int]struct{})
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		m := baseRepFilePattern.FindStringSubmatch(entry.Name())
		if m == nil {
			continue
		}
		id, parseErr := strconv.Atoi(m[trackComponent])
		if parseErr == nil {
			seen[id] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return nil, false
	}
	out := make([]int, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Ints(out)
	return out, true
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
