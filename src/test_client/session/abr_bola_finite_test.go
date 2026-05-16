package session

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"main/src/model"
)

type fakeSizeProvider struct {
	size int64
}

func (f fakeSizeProvider) AvgSize(tileID int) int64 {
	return f.size
}

// mapTileSizeProvider simula tamanhos médios por tile (ex.: rep. base 10 no
// dataset real), útil para cenários com massa variável entre tiles / FoV.
type mapTileSizeProvider struct {
	byTile   map[int]int64
	fallback int64
}

func (m mapTileSizeProvider) AvgSize(tileID int) int64 {
	if v, ok := m.byTile[tileID]; ok && v > 0 {
		return v
	}
	if m.fallback > 0 {
		return m.fallback
	}
	return 1
}

func TestBolaGuardrailFallsBack(t *testing.T) {
	abr := newBolaFiniteABRWithEstimator(fakeSizeProvider{size: 100})
	ctx := SegmentContext{
		SegmentID:       1,
		FirstSegment:    1,
		LastSegment:     3,
		SegmentDuration: time.Second,
		TimeBudget:      200 * time.Millisecond,
		AvgThroughput:   50,
		BufferLevel:     3 * time.Second,
		FOVTiles:        []int{1, 2},
		AllTiles:        []int{1, 2, 3, 4},
	}

	cfg := abr.SelectConfig(ctx)
	require.Equal(t, "A_all_low", cfg.ID)
	require.Equal(t, model.LOW_BITRATE, cfg.FOVBitrate)
	require.Equal(t, model.LOW_BITRATE, cfg.NonFOVBitrate)
}

func TestBolaNoFOVDefaultsToLow(t *testing.T) {
	abr := newBolaFiniteABRWithEstimator(fakeSizeProvider{size: 100})
	ctx := SegmentContext{
		SegmentID:       1,
		FirstSegment:    1,
		LastSegment:     3,
		SegmentDuration: time.Second,
		TimeBudget:      time.Second,
		AvgThroughput:   1_000_000,
		BufferLevel:     3 * time.Second,
		FOVTiles:        nil,
		AllTiles:        []int{1, 2, 3},
	}

	cfg := abr.SelectConfig(ctx)
	require.Equal(t, "A_all_low", cfg.ID)
	require.Equal(t, model.LOW_BITRATE, cfg.FOVBitrate)
	require.Equal(t, model.LOW_BITRATE, cfg.NonFOVBitrate)
}

// TestBolaWithVariableTileMassesTable cobre cenários “dataset-like”: tiles com
// massas distintas (como após variantes de bitrate no disco), buffer e
// orçamento variando. Rápido (sem rede) e estável para CI.
func TestBolaWithVariableTileMassesTable(t *testing.T) {
	baseCtx := func(avgThroughput float64) SegmentContext {
		return SegmentContext{
			SegmentID:       3,
			FirstSegment:    1,
			LastSegment:     12,
			SegmentDuration: time.Second,
			AvgThroughput:   avgThroughput,
		}
	}

	tests := []struct {
		name          string
		tileSizes     map[int]int64
		fov           []int
		all           []int
		buffer        time.Duration
		timeBudget    time.Duration
		avgThroughput float64
		wantID        string
	}{
		{
			name: "fov_pesado_buffer_curto_ainda_sobe_qualidade",
			tileSizes: map[int]int64{
				1: 12000, 2: 8000, 3: 400, 4: 200,
			},
			fov:           []int{1, 2},
			all:           []int{1, 2, 3, 4},
			buffer:        1200 * time.Millisecond,
			timeBudget:    20 * time.Second,
			avgThroughput: 80 * 1024 * 1024,
			wantID:        "C_fov_high",
		},
		{
			name: "mesma_heterogeneidade_orcamento_apertado_fica_em_low",
			tileSizes: map[int]int64{
				1: 12000, 2: 8000, 3: 400, 4: 200,
			},
			fov:           []int{1, 2},
			all:           []int{1, 2, 3, 4},
			buffer:        1200 * time.Millisecond,
			timeBudget:    5 * time.Millisecond,
			avgThroughput: 8000,
			wantID:        "A_all_low",
		},
		{
			name: "tiles_leves_guardrail_escolhe_med_high_nao_cabe",
			tileSizes: map[int]int64{
				1: 400, 2: 300, 3: 500, 4: 500,
			},
			fov:           []int{1, 2},
			all:           []int{1, 2, 3, 4},
			buffer:        900 * time.Millisecond,
			timeBudget:    1 * time.Millisecond,
			avgThroughput: 2_800_000,
			wantID:        "B_fov_med",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			abr := newBolaFiniteABRWithEstimator(mapTileSizeProvider{byTile: tt.tileSizes, fallback: 500})
			ctx := baseCtx(tt.avgThroughput)
			ctx.FOVTiles = tt.fov
			ctx.AllTiles = tt.all
			ctx.BufferLevel = tt.buffer
			ctx.TimeBudget = tt.timeBudget
			cfg := abr.SelectConfig(ctx)
			require.Equal(t, tt.wantID, cfg.ID, "FoV=%v tiles=%v buf=%s budget=%s",
				tt.fov, tt.all, tt.buffer, tt.timeBudget)
		})
	}
}
