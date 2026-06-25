package test_client

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestMediaDetectionScopesTileFirstLayoutToLegacy(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{
		"video_tiled_5_dash_track10_1.m4s",
		"video_tiled_10_dash_track10_1.m4s",
		"video_tiled_10_dash_track12_1.m4s",
		"video_tiled_10_dash_track10_3.m4s",
		"video_tiled_10_dash_track12_3.m4s",
		"video_tiled_15_dash_track12_3.m4s",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	firstSeg, lastSeg, firstTile, lastTile, ok := detectMediaBoundsAt(dir)
	if !ok || firstSeg != 10 || lastSeg != 12 || firstTile != 1 || lastTile != 3 {
		t.Fatalf("historical BOLA bounds changed: %d %d %d %d %v", firstSeg, lastSeg, firstTile, lastTile, ok)
	}
	bolaIDs, ok := detectValidSegmentIDsAt(dir, 1)
	if !ok || !reflect.DeepEqual([]int{10, 12}, bolaIDs) {
		t.Fatalf("historical BOLA IDs changed: %v %v", bolaIDs, ok)
	}

	firstSeg, lastSeg, firstTile, lastTile, layout, ok := detectLegacyMediaLayoutAt(dir)
	if !ok || firstSeg != 1 || lastSeg != 3 || firstTile != 10 || lastTile != 12 {
		t.Fatalf("Legacy bounds incorrect: %d %d %d %d %v", firstSeg, lastSeg, firstTile, lastTile, ok)
	}
	if !reflect.DeepEqual([]int{10, 12}, layout.SegmentTiles[1]) || !reflect.DeepEqual([]int{10, 12}, layout.SegmentTiles[3]) {
		t.Fatalf("Legacy tile universe incorrect: %+v", layout.SegmentTiles)
	}
	legacyIDs, ok := detectValidSegmentIDsAt(dir, 2)
	if !ok || !reflect.DeepEqual([]int{1, 3}, legacyIDs) {
		t.Fatalf("Legacy IDs incorrect: %v %v", legacyIDs, ok)
	}
}

func TestIsLegacyABRModeIsDiscriminant(t *testing.T) {
	for _, mode := range []string{"legacy", "default", "threshold", " LEGACY "} {
		if !isLegacyABRMode(mode) {
			t.Fatalf("expected Legacy mode for %q", mode)
		}
	}
	for _, mode := range []string{"", "bola", "bola_finite", "bolafinite"} {
		if isLegacyABRMode(mode) {
			t.Fatalf("expected BOLA mode for %q", mode)
		}
	}
}
