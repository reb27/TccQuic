package session

import "strings"

func normalizedABRMode(mode string) string {
	return strings.ToLower(strings.TrimSpace(mode))
}

func highPriorityRatioForABRMode(mode string) (int, bool) {
	switch normalizedABRMode(mode) {
	case "article", "article50":
		return 50, true
	case "article30":
		return 30, true
	default:
		return 0, false
	}
}

func deterministicPriorityBucket(segmentID, tileID int) int {
	v := (segmentID * 73856093) ^ (tileID * 19349663)
	if v < 0 {
		v = -v
	}
	return v % 100
}

func isHighPriorityForABRMode(mode string, segmentID, tileID int) (bool, bool) {
	ratio, ok := highPriorityRatioForABRMode(mode)
	if !ok {
		return false, false
	}
	return deterministicPriorityBucket(segmentID, tileID) < ratio, true
}

func priorityTilesForABRMode(mode string, segmentID, firstTile, lastTile int) ([]int, bool) {
	ratio, ok := highPriorityRatioForABRMode(mode)
	if !ok {
		return nil, false
	}
	tiles := make([]int, 0, (lastTile-firstTile+1)*ratio/100)
	for tileID := firstTile; tileID <= lastTile; tileID++ {
		if deterministicPriorityBucket(segmentID, tileID) < ratio {
			tiles = append(tiles, tileID)
		}
	}
	return tiles, true
}
