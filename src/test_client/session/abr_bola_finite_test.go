package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"main/src/model"
)

type fakeSizeProvider struct {
	size int64
}

func (f fakeSizeProvider) AvgSize(tileID int, bitrate model.Bitrate) int64 {
	return f.size
}

// mapTileSizeProvider simula tamanhos medios por tile e bitrate, como o
// dataset real apos o mapeamento LOW->rep5, MEDIUM->rep10, HIGH->rep15.
type mapTileSizeProvider struct {
	byTile   map[int]map[model.Bitrate]int64
	fallback int64
}

func (m mapTileSizeProvider) AvgSize(tileID int, bitrate model.Bitrate) int64 {
	if byBitrate, ok := m.byTile[tileID]; ok {
		if v, ok := byBitrate[bitrate]; ok && v > 0 {
			return v
		}
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
		tileSizes     map[int]map[model.Bitrate]int64
		fov           []int
		all           []int
		buffer        time.Duration
		timeBudget    time.Duration
		avgThroughput float64
		wantID        string
	}{
		{
			name: "fov_pesado_buffer_curto_ainda_sobe_qualidade",
			tileSizes: map[int]map[model.Bitrate]int64{
				1: {
					model.LOW_BITRATE:    7000,
					model.MEDIUM_BITRATE: 12000,
					model.HIGH_BITRATE:   16000,
				},
				2: {
					model.LOW_BITRATE:    5000,
					model.MEDIUM_BITRATE: 8000,
					model.HIGH_BITRATE:   11000,
				},
				3: {
					model.LOW_BITRATE: 400,
				},
				4: {
					model.LOW_BITRATE: 200,
				},
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
			tileSizes: map[int]map[model.Bitrate]int64{
				1: {
					model.LOW_BITRATE:    7000,
					model.MEDIUM_BITRATE: 12000,
					model.HIGH_BITRATE:   16000,
				},
				2: {
					model.LOW_BITRATE:    5000,
					model.MEDIUM_BITRATE: 8000,
					model.HIGH_BITRATE:   11000,
				},
				3: {
					model.LOW_BITRATE: 400,
				},
				4: {
					model.LOW_BITRATE: 200,
				},
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
			tileSizes: map[int]map[model.Bitrate]int64{
				1: {
					model.LOW_BITRATE:    100,
					model.MEDIUM_BITRATE: 150,
					model.HIGH_BITRATE:   250,
				},
				2: {
					model.LOW_BITRATE:    100,
					model.MEDIUM_BITRATE: 150,
					model.HIGH_BITRATE:   250,
				},
				3: {
					model.LOW_BITRATE: 100,
				},
				4: {
					model.LOW_BITRATE: 100,
				},
			},
			fov:           []int{1, 2},
			all:           []int{1, 2, 3, 4},
			buffer:        900 * time.Millisecond,
			timeBudget:    1 * time.Millisecond,
			avgThroughput: 600_000,
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

func TestBolaDebugCSVWritesHeaderAndRow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bola-debug.csv")
	abr := newBolaFiniteABRWithEstimator(fakeSizeProvider{size: 100}, path)
	ctx := SegmentContext{
		SegmentID:       7,
		FirstSegment:    1,
		LastSegment:     10,
		SegmentDuration: time.Second,
		TimeBudget:      2 * time.Second,
		AvgThroughput:   1024,
		BufferLevel:     time.Second,
		FOVTiles:        []int{1},
		AllTiles:        []int{1, 2},
	}

	abr.SelectConfig(ctx)

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	require.Len(t, lines, 2)
	require.Equal(t, "segment,cfg_before_guardrail,cfg_after_guardrail,avg_tp_bps,avg_tp_kib_s,time_budget_s,budget_bytes,size_low_bytes,size_med_bytes,size_high_bytes,guardrail,buffer_s,score_low,score_med,score_high", lines[0])

	fields := strings.Split(lines[1], ",")
	require.Len(t, fields, 15)
	require.Equal(t, "7", fields[0])
	require.Equal(t, "1024.000000", fields[3])
	require.Equal(t, "1.000000", fields[4])
	require.Equal(t, "2.000000", fields[5])
	require.Equal(t, "2048.000000", fields[6])
}
