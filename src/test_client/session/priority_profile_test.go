package session

import "testing"

func TestHighPriorityRatioForABRMode(t *testing.T) {
	tests := []struct {
		mode  string
		ratio int
		ok    bool
	}{
		{mode: "article", ratio: 50, ok: true},
		{mode: "article50", ratio: 50, ok: true},
		{mode: "article30", ratio: 30, ok: true},
		{mode: "Article30", ratio: 30, ok: true},
	}

	for _, tt := range tests {
		gotRatio, gotOK := highPriorityRatioForABRMode(tt.mode)
		if gotRatio != tt.ratio || gotOK != tt.ok {
			t.Fatalf("mode=%q got=(%d,%t) want=(%d,%t)", tt.mode, gotRatio, gotOK, tt.ratio, tt.ok)
		}
	}
}

func TestPriorityTilesForABRModeDeterministicAndBounded(t *testing.T) {
	tilesA, okA := priorityTilesForABRMode("article50", 120, 100, 199)
	tilesB, okB := priorityTilesForABRMode("article50", 120, 100, 199)
	if !okA || !okB {
		t.Fatalf("expected article50 to enable deterministic priority profile")
	}
	if len(tilesA) != len(tilesB) {
		t.Fatalf("deterministic profile returned different sizes: %d vs %d", len(tilesA), len(tilesB))
	}
	for i := range tilesA {
		if tilesA[i] != tilesB[i] {
			t.Fatalf("deterministic profile mismatch at index %d: %d vs %d", i, tilesA[i], tilesB[i])
		}
	}
	for _, tileID := range tilesA {
		if tileID < 100 || tileID > 199 {
			t.Fatalf("tile out of requested range: %d", tileID)
		}
	}
}

func TestPriorityTilesForABRModeDisabledForLegacyModes(t *testing.T) {
	if _, ok := priorityTilesForABRMode("bola", 120, 100, 199); ok {
		t.Fatalf("expected no article profile for bola mode")
	}
	if _, ok := priorityTilesForABRMode("legacy", 120, 100, 199); ok {
		t.Fatalf("expected no article profile for legacy mode")
	}
}
