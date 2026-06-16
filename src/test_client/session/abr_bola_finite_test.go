package session

import (
	"os"
	"path/filepath"
	"strconv"
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
	require.Equal(t, model.LOW_BITRATE, cfg.NearFOVBitrate)
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
	require.Equal(t, model.LOW_BITRATE, cfg.NearFOVBitrate)
	require.Equal(t, model.LOW_BITRATE, cfg.NonFOVBitrate)
}

func TestBolaQmaxChangesQualityDecision(t *testing.T) {
	ctx := SegmentContext{
		SegmentID:       10,
		FirstSegment:    1,
		LastSegment:     120,
		SegmentDuration: time.Second,
		TimeBudget:      20 * time.Second,
		AvgThroughput:   80 * 1024 * 1024,
		BufferLevel:     3 * time.Second,
		FOVTiles:        []int{1, 2},
		AllTiles:        []int{1, 2},
	}

	qmax3 := newBolaFiniteABRWithEstimatorAndQmax(fakeSizeProvider{size: 100}, 3)
	qmax5 := newBolaFiniteABRWithEstimatorAndQmax(fakeSizeProvider{size: 100}, 5)

	require.Equal(t, "A_all_low", qmax3.SelectConfig(ctx).ID)
	cfg := qmax5.SelectConfig(ctx)
	require.Equal(t, "C_fov_high", cfg.ID)
	require.Equal(t, model.LOW_BITRATE, cfg.NearFOVBitrate)
}

func TestBolaInvalidQmaxFallsBackToDefault(t *testing.T) {
	abr := newBolaFiniteABRWithEstimatorAndQmax(fakeSizeProvider{size: 100}, 0)
	bola, ok := abr.(*bolaFiniteABR)

	require.True(t, ok)
	require.Equal(t, defaultBOLAQmaxSegments, bola.qmaxSegments)
}

func TestBolaChoosesHighNearMedWhenNearFOVPresent(t *testing.T) {
	ctx := SegmentContext{
		SegmentID:       10,
		FirstSegment:    1,
		LastSegment:     120,
		SegmentDuration: time.Second,
		TimeBudget:      20 * time.Second,
		AvgThroughput:   80 * 1024 * 1024,
		BufferLevel:     3 * time.Second,
		FOVTiles:        []int{1, 2},
		NearFOVTiles:    []int{3, 4},
		AllTiles:        []int{1, 2, 3, 4},
	}

	abr := newBolaFiniteABRWithEstimatorAndQmax(fakeSizeProvider{size: 100}, 5)
	cfg := abr.SelectConfig(ctx)

	require.Equal(t, "D_fov_high_near_med", cfg.ID)
	require.Equal(t, model.HIGH_BITRATE, cfg.FOVBitrate)
	require.Equal(t, model.MEDIUM_BITRATE, cfg.NearFOVBitrate)
	require.Equal(t, model.LOW_BITRATE, cfg.NonFOVBitrate)
}

func TestBolaGuardrailCanDowngradeHighNearMed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bola-debug.csv")
	ctx := SegmentContext{
		SegmentID:       10,
		FirstSegment:    1,
		LastSegment:     120,
		SegmentDuration: time.Second,
		TimeBudget:      time.Second,
		AvgThroughput:   1_100,
		BufferLevel:     3 * time.Second,
		FOVTiles:        []int{1, 2},
		NearFOVTiles:    []int{3, 4},
		AllTiles:        []int{1, 2, 3, 4},
	}
	abr := newBolaFiniteABRWithEstimatorAndQmax(mapTileSizeProvider{
		byTile: map[int]map[model.Bitrate]int64{
			1: {
				model.LOW_BITRATE:    250,
				model.MEDIUM_BITRATE: 250,
				model.HIGH_BITRATE:   450,
			},
			2: {
				model.LOW_BITRATE:    250,
				model.MEDIUM_BITRATE: 250,
				model.HIGH_BITRATE:   450,
			},
			3: {
				model.LOW_BITRATE:    50,
				model.MEDIUM_BITRATE: 150,
			},
			4: {
				model.LOW_BITRATE:    50,
				model.MEDIUM_BITRATE: 150,
			},
		},
		fallback: 100,
	}, 5, path)

	cfg := abr.SelectConfig(ctx)
	row := readBolaDebugRow(t, path)

	require.Equal(t, "C_fov_high", cfg.ID)
	require.Equal(t, model.HIGH_BITRATE, cfg.FOVBitrate)
	require.Equal(t, model.LOW_BITRATE, cfg.NearFOVBitrate)
	require.Equal(t, model.LOW_BITRATE, cfg.NonFOVBitrate)
	require.Equal(t, "D_fov_high_near_med", row["cfg_before_guardrail"])
	require.Equal(t, "C_fov_high", row["cfg_after_guardrail"])
	require.Equal(t, "true", row["guardrail"])
	require.Equal(t, 1_100.0, parseDebugFloat(t, row["budget_bytes"]))
	require.Equal(t, 1_000.0, parseDebugFloat(t, row["size_high_bytes"]))
	require.Equal(t, 1_200.0, parseDebugFloat(t, row["size_fov_high_near_med_bytes"]))
	require.Greater(t,
		parseDebugFloat(t, row["score_fov_high_near_med"]),
		parseDebugFloat(t, row["score_high"]),
	)
}

func TestBolaConfigSizeUsesNearFOVBitrate(t *testing.T) {
	abr := newBolaFiniteABRWithEstimatorAndQmax(mapTileSizeProvider{
		byTile: map[int]map[model.Bitrate]int64{
			1: {model.HIGH_BITRATE: 1000},
			2: {model.MEDIUM_BITRATE: 500},
			3: {model.LOW_BITRATE: 100},
		},
		fallback: 1,
	}, 5)
	bola, ok := abr.(*bolaFiniteABR)
	require.True(t, ok)

	size := bola.configSize(
		[]int{1, 2, 3},
		map[int]struct{}{1: {}},
		map[int]struct{}{2: {}},
		SegmentConfig{
			ID:             "test",
			FOVBitrate:     model.HIGH_BITRATE,
			NearFOVBitrate: model.MEDIUM_BITRATE,
			NonFOVBitrate:  model.LOW_BITRATE,
		},
	)

	require.EqualValues(t, 1600, size)
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
	require.Equal(t, "segment,cfg_before_guardrail,cfg_after_guardrail,avg_tp_bps,avg_tp_kib_s,time_budget_s,budget_bytes,size_low_bytes,size_med_bytes,size_high_bytes,size_fov_high_near_med_bytes,guardrail,buffer_s,score_low,score_med,score_high,score_fov_high_near_med", lines[0])

	fields := strings.Split(lines[1], ",")
	require.Len(t, fields, 17)
	require.Equal(t, "7", fields[0])
	require.Equal(t, "1024.000000", fields[3])
	require.Equal(t, "1.000000", fields[4])
	require.Equal(t, "2.000000", fields[5])
	require.Equal(t, "2048.000000", fields[6])
}

func readBolaDebugRow(t *testing.T, path string) map[string]string {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	require.Len(t, lines, 2)
	header := strings.Split(lines[0], ",")
	values := strings.Split(lines[1], ",")
	require.Len(t, values, len(header))

	row := make(map[string]string, len(header))
	for i, key := range header {
		row[key] = values[i]
	}
	return row
}

func parseDebugFloat(t *testing.T, value string) float64 {
	t.Helper()
	parsed, err := strconv.ParseFloat(value, 64)
	require.NoError(t, err)
	return parsed
}
