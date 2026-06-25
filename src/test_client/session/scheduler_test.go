package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"main/src/model"
	"main/src/test_client/fov"
)

func TestBuildTileRequestPlanPrioritizesFOV(t *testing.T) {
	trace := loadTestFOVTrace(t, "frames,tile\n1,5\n")
	cfg := SegmentConfig{
		ID:             "test",
		FOVBitrate:     model.HIGH_BITRATE,
		NearFOVBitrate: model.MEDIUM_BITRATE,
		NonFOVBitrate:  model.LOW_BITRATE,
	}

	plan := BuildTileRequestPlan(1, cfg, []int{8, 5, 3}, trace)

	assertPlanTiles(t, plan, []int{5, 3, 8})
	if !plan[0].InFOV || plan[0].Bitrate != model.HIGH_BITRATE || plan[0].Priority != model.HIGH_PRIORITY {
		t.Fatalf("expected first request to be HIGH/FOV, got %+v", plan[0])
	}
	if plan[1].Bitrate != model.MEDIUM_BITRATE || plan[1].Priority != model.MEDIUM_PRIORITY || plan[1].SemanticPriority != float32(model.SemanticPiNearFoV) {
		t.Fatalf("expected near-FOV request to use MEDIUM bitrate and semantic priority on tile 3, got %+v", plan[1])
	}
	if plan[2].Bitrate != model.LOW_BITRATE || plan[2].Priority != model.LOW_PRIORITY || plan[2].SemanticPriority != float32(model.SemanticPiBackground) {
		t.Fatalf("expected background request to use LOW bitrate and semantic priority on tile 8, got %+v", plan[2])
	}
	for i, item := range plan {
		if item.RequestOrder != i+1 {
			t.Fatalf("expected request order %d, got %+v", i+1, item)
		}
	}
}

func TestBuildTileRequestPlanDeterministicWithoutFOVTrace(t *testing.T) {
	cfg := SegmentConfig{
		ID:             "test",
		FOVBitrate:     model.HIGH_BITRATE,
		NearFOVBitrate: model.MEDIUM_BITRATE,
		NonFOVBitrate:  model.LOW_BITRATE,
	}

	plan := BuildTileRequestPlan(1, cfg, []int{9, 2, 5}, nil)

	assertPlanTiles(t, plan, []int{2, 5, 9})
	for _, item := range plan {
		if item.InFOV || item.NearFOV {
			t.Fatalf("expected all requests to be background without FOV trace, got %+v", item)
		}
		if item.Bitrate != model.LOW_BITRATE {
			t.Fatalf("expected non-FOV bitrate, got %+v", item)
		}
	}
}

func TestBuildTileRequestPlanAllLowStillPrioritizesFOVNearAndBackground(t *testing.T) {
	trace := loadTestFOVTrace(t, "frames,tile\n1,5\n")
	cfg := SegmentConfig{
		ID:             "legacy_low",
		FOVBitrate:     model.LOW_BITRATE,
		NearFOVBitrate: model.LOW_BITRATE,
		NonFOVBitrate:  model.LOW_BITRATE,
	}

	for _, tiles := range [][]int{{8, 5, 3, 7, 4}, {4, 7, 3, 5, 8}} {
		plan := BuildTileRequestPlan(1, cfg, tiles, trace)
		assertPlanTiles(t, plan, []int{5, 3, 4, 7, 8})
		requireSpatialOrder(t, plan)
	}
}

func TestBuildTileRequestPlanScopesNearFOVTieBreakToLegacy(t *testing.T) {
	trace := loadTestFOVTrace(t, "frames,tile\n1,5\n")
	allLow := func(id string) SegmentConfig {
		return SegmentConfig{ID: id, FOVBitrate: model.LOW_BITRATE, NearFOVBitrate: model.LOW_BITRATE, NonFOVBitrate: model.LOW_BITRATE}
	}

	bola := BuildTileRequestPlan(1, allLow("A_all_low"), []int{1, 3, 5}, trace)
	assertPlanTiles(t, bola, []int{5, 1, 3})

	legacy := BuildTileRequestPlan(1, allLow("legacy_low"), []int{1, 3, 5}, trace)
	assertPlanTiles(t, legacy, []int{5, 3, 1})
	requireSpatialOrder(t, legacy)
}

func TestBuildTileRequestPlanPreservesBOLAQualityOrdering(t *testing.T) {
	trace := loadTestFOVTrace(t, "frames,tile\n1,5\n")
	cfg := SegmentConfig{ID: "D_fov_high_near_med", FOVBitrate: model.HIGH_BITRATE, NearFOVBitrate: model.MEDIUM_BITRATE, NonFOVBitrate: model.LOW_BITRATE}
	plan := BuildTileRequestPlan(1, cfg, []int{1, 3, 5}, trace)
	assertPlanTiles(t, plan, []int{5, 3, 1})
}

func TestNearFOVTilesForSegmentExcludesFOVAndBackground(t *testing.T) {
	trace := loadTestFOVTrace(t, "frames,tile\n1,5\n")

	tiles := nearFOVTilesForSegment(trace, 1, []int{3, 5, 8})

	assertTiles(t, tiles, []int{3})
}

func loadTestFOVTrace(t *testing.T, content string) *fov.FOVTrace {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fov.csv")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fov trace: %v", err)
	}
	trace, err := fov.LoadFOVTrace(path, 1, time.Second)
	if err != nil {
		t.Fatalf("load fov trace: %v", err)
	}
	return trace
}

func assertPlanTiles(t *testing.T, plan []TileRequestPlanItem, want []int) {
	t.Helper()
	if len(plan) != len(want) {
		t.Fatalf("expected %d planned requests, got %d: %+v", len(want), len(plan), plan)
	}
	for i, tileID := range want {
		if plan[i].TileID != tileID {
			t.Fatalf("expected tile order %v, got %+v", want, plan)
		}
	}
}

func assertTiles(t *testing.T, got []int, want []int) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("expected tiles %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected tiles %v, got %v", want, got)
		}
	}
}

func requireSpatialOrder(t *testing.T, plan []TileRequestPlanItem) {
	t.Helper()
	previous := -1
	for _, item := range plan {
		rank := spatialRequestRank(item)
		if rank < previous {
			t.Fatalf("spatial priority inversion in plan: %+v", plan)
		}
		previous = rank
	}
}
