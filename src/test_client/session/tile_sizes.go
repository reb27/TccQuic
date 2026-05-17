package session

import (
	"fmt"
	"os"
	"regexp"

	"main/src/model"
)

type tileSizeKey struct {
	tileID  int
	bitrate model.Bitrate
}

type TileSizeEstimator struct {
	byTileBitrate map[tileSizeKey]int64
	byBitrateAvg  map[model.Bitrate]int64
	globalAvg     int64
}

var tileSizeFilePattern = regexp.MustCompile(`^video_tiled_(\d+)_dash_track(\d+)_(\d+)\.m4s$`)

func NewTileSizeEstimator(dir string) (*TileSizeEstimator, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	sums := make(map[tileSizeKey]int64)
	counts := make(map[tileSizeKey]int64)
	bitrateSums := make(map[model.Bitrate]int64)
	bitrateCounts := make(map[model.Bitrate]int64)
	var totalSize int64
	var totalCount int64

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		m := tileSizeFilePattern.FindStringSubmatch(name)
		if m == nil {
			continue
		}
		// m[1] = representation, m[2] = segment, m[3] = tile
		var rep int
		if _, err := fmt.Sscanf(m[1], "%d", &rep); err != nil {
			continue
		}
		bitrate, ok := bitrateForRepresentation(rep)
		if !ok {
			continue
		}
		var tileID int
		if _, err := fmt.Sscanf(m[3], "%d", &tileID); err != nil {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		size := info.Size()
		key := tileSizeKey{tileID: tileID, bitrate: bitrate}
		sums[key] += size
		counts[key]++
		bitrateSums[bitrate] += size
		bitrateCounts[bitrate]++
		totalSize += size
		totalCount++
	}

	byTileBitrate := make(map[tileSizeKey]int64, len(sums))
	for key, sum := range sums {
		count := counts[key]
		if count > 0 {
			byTileBitrate[key] = sum / count
		}
	}

	byBitrateAvg := make(map[model.Bitrate]int64, len(bitrateSums))
	for bitrate, sum := range bitrateSums {
		count := bitrateCounts[bitrate]
		if count > 0 {
			byBitrateAvg[bitrate] = sum / count
		}
	}

	globalAvg := int64(0)
	if totalCount > 0 {
		globalAvg = totalSize / totalCount
	}
	if globalAvg <= 0 {
		globalAvg = 1
	}

	return &TileSizeEstimator{
		byTileBitrate: byTileBitrate,
		byBitrateAvg:  byBitrateAvg,
		globalAvg:     globalAvg,
	}, nil
}

func NewFallbackTileSizeEstimator() *TileSizeEstimator {
	return &TileSizeEstimator{
		byTileBitrate: map[tileSizeKey]int64{},
		byBitrateAvg:  map[model.Bitrate]int64{},
		globalAvg:     1,
	}
}

func (t *TileSizeEstimator) AvgSize(tileID int, bitrate model.Bitrate) int64 {
	if t == nil {
		return 0
	}
	if v, ok := t.byTileBitrate[tileSizeKey{tileID: tileID, bitrate: bitrate}]; ok && v > 0 {
		return v
	}
	if v, ok := t.byBitrateAvg[bitrate]; ok && v > 0 {
		return v
	}
	return t.globalAvg
}

func bitrateForRepresentation(rep int) (model.Bitrate, bool) {
	switch rep {
	case 5:
		return model.LOW_BITRATE, true
	case 10:
		return model.MEDIUM_BITRATE, true
	case 15:
		return model.HIGH_BITRATE, true
	default:
		return 0, false
	}
}
