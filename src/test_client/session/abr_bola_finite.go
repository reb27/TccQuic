package session

import (
	"bufio"
	"fmt"
	"log"
	"math"
	"os"
	"sync"

	"main/src/model"
)

type bolaFiniteABR struct {
	sizeProvider TileSizeProvider
	qmaxSegments float64
	gamma        float64
	wFOV         float64
	wNonFOV      float64
	configs      []SegmentConfig
	debug        *bolaDebugLogger
}

type bolaConfigOption struct {
	cfg     SegmentConfig
	size    int64
	utility float64
	score   float64
}

type TileSizeProvider interface {
	AvgSize(tileID int, bitrate model.Bitrate) int64
}

func NewBOLAFiniteABR(debugPath string) ABRController {
	estimator, err := NewTileSizeEstimator("data/segments")
	if err != nil {
		log.Printf("BOLA ABR: failed to build tile size estimator: %v (using fallback)", err)
		estimator = NewFallbackTileSizeEstimator()
	}
	return newBolaFiniteABRWithEstimator(estimator, debugPath)
}

func newBolaFiniteABRWithEstimator(estimator TileSizeProvider, debugPath ...string) ABRController {
	var logger *bolaDebugLogger
	if len(debugPath) > 0 && debugPath[0] != "" {
		var err error
		logger, err = newBolaDebugLogger(debugPath[0])
		if err != nil {
			log.Printf("BOLA ABR: failed to open debug CSV %s: %v", debugPath[0], err)
		}
	}
	return &bolaFiniteABR{
		sizeProvider: estimator,
		qmaxSegments: 3,
		gamma:        1.0,
		wFOV:         1.0,
		wNonFOV:      0.2,
		configs: []SegmentConfig{
			{ID: "A_all_low", FOVBitrate: model.LOW_BITRATE, NonFOVBitrate: model.LOW_BITRATE},
			{ID: "B_fov_med", FOVBitrate: model.MEDIUM_BITRATE, NonFOVBitrate: model.LOW_BITRATE},
			{ID: "C_fov_high", FOVBitrate: model.HIGH_BITRATE, NonFOVBitrate: model.LOW_BITRATE},
		},
		debug: logger,
	}
}

func (b *bolaFiniteABR) SelectConfig(ctx SegmentContext) SegmentConfig {
	if len(b.configs) == 0 {
		return SegmentConfig{ID: "A_all_low", FOVBitrate: model.LOW_BITRATE, NonFOVBitrate: model.LOW_BITRATE}
	}
	p := ctx.SegmentDuration.Seconds()
	if p <= 0 {
		return b.configs[0]
	}

	fovSet := make(map[int]struct{}, len(ctx.FOVTiles))
	for _, t := range ctx.FOVTiles {
		fovSet[t] = struct{}{}
	}
	fovCount := 0
	nonFOVCount := 0
	for _, tileID := range ctx.AllTiles {
		if _, ok := fovSet[tileID]; ok {
			fovCount++
		} else {
			nonFOVCount++
		}
	}
	if fovCount+nonFOVCount == 0 {
		return b.configs[0]
	}

	options := make([]bolaConfigOption, 0, len(b.configs))
	for _, cfg := range b.configs {
		size := b.configSize(ctx.AllTiles, fovSet, cfg)
		utility := b.wFOV*float64(fovCount)*utilityFor(cfg.FOVBitrate) + b.wNonFOV*float64(nonFOVCount)*utilityFor(cfg.NonFOVBitrate)
		options = append(options, bolaConfigOption{cfg: cfg, size: size, utility: utility})
	}

	uMax := options[0].utility
	for i := 1; i < len(options); i++ {
		if options[i].utility > uMax {
			uMax = options[i].utility
		}
	}
	if uMax <= 0 {
		return b.configs[0]
	}

	Q := ctx.BufferLevel.Seconds() / p
	remainingSeg := ctx.LastSegment - ctx.SegmentID + 1
	if remainingSeg < 1 {
		remainingSeg = 1
	}
	remainingSec := float64(remainingSeg) * p
	playbackPos := float64(ctx.SegmentID-ctx.FirstSegment) * p
	if playbackPos < 0 {
		playbackPos = 0
	}
	t := math.Min(playbackPos, remainingSec)
	tPrime := math.Max(t/2.0, 3.0*p)
	QDmax := math.Min(b.qmaxSegments, tPrime/p)
	denom := uMax + b.gamma*p
	if denom <= 0 {
		denom = 1e-9
	}
	V := (QDmax - 1.0) / denom

	bestIdx := 0
	bestScore := math.Inf(-1)
	for i, opt := range options {
		if opt.size <= 0 {
			continue
		}
		score := (V*(opt.utility+b.gamma*p) - Q) / float64(opt.size)
		options[i].score = score
		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}
	if bestScore <= 0 {
		bestIdx = 0
	}

	chosenBeforeGuardrail := options[bestIdx]
	chosen := chosenBeforeGuardrail
	chosenScore := chosenBeforeGuardrail.score
	guardrail := false
	budgetBytes := 0.0
	if ctx.AvgThroughput > 0 && ctx.TimeBudget > 0 {
		budgetBytes = ctx.AvgThroughput * ctx.TimeBudget.Seconds()
		if budgetBytes > 0 && float64(chosen.size) > budgetBytes {
			for i := bestIdx; i >= 0; i-- {
				if options[i].size > 0 && float64(options[i].size) <= budgetBytes {
					chosen = options[i]
					guardrail = true
					break
				}
			}
			if float64(options[0].size) > budgetBytes {
				chosen = options[0]
				guardrail = true
			}
		}
	}

	throughputKiBps := ctx.AvgThroughput / 1024.0
	budgetKB := budgetBytes / 1024.0
	chosenKB := float64(chosen.size) / 1024.0
	log.Printf("ABR (BOLA): seg=%d cfg=%s fov=%d nonfov=%d buf=%.2fs thr=%.1f KiB/s budget=%.1f KiB size=%.1f KiB score=%.4f guardrail=%t uMax=%.2f V=%.4f Q=%.2f",
		ctx.SegmentID, chosen.cfg.ID, chosen.cfg.FOVBitrate, chosen.cfg.NonFOVBitrate, Q*p, throughputKiBps, budgetKB, chosenKB, chosenScore, guardrail, uMax, V, Q)
	b.writeDebug(ctx, options, chosenBeforeGuardrail, chosen, budgetBytes, guardrail)

	return chosen.cfg
}

func utilityFor(bitrate model.Bitrate) float64 {
	return math.Log(float64(bitrate))
}

func (b *bolaFiniteABR) configSize(allTiles []int, fovSet map[int]struct{}, cfg SegmentConfig) int64 {
	var total int64
	for _, tileID := range allTiles {
		bitrate := cfg.NonFOVBitrate
		if _, ok := fovSet[tileID]; ok {
			bitrate = cfg.FOVBitrate
		}
		size := b.sizeProvider.AvgSize(tileID, bitrate)
		if size <= 0 {
			size = 1
		}
		total += size
	}
	return total
}

func (b *bolaFiniteABR) writeDebug(ctx SegmentContext, options []bolaConfigOption, before bolaConfigOption, after bolaConfigOption, budgetBytes float64, guardrail bool) {
	if b.debug == nil {
		return
	}
	b.debug.Log(bolaDebugRow{
		segment:            ctx.SegmentID,
		cfgBeforeGuardrail: before.cfg.ID,
		cfgAfterGuardrail:  after.cfg.ID,
		avgTPBps:           ctx.AvgThroughput,
		avgTPKiBps:         ctx.AvgThroughput / 1024.0,
		timeBudgetS:        ctx.TimeBudget.Seconds(),
		budgetBytes:        budgetBytes,
		sizeLowBytes:       sizeByConfigID(options, "A_all_low"),
		sizeMedBytes:       sizeByConfigID(options, "B_fov_med"),
		sizeHighBytes:      sizeByConfigID(options, "C_fov_high"),
		guardrail:          guardrail,
		bufferS:            ctx.BufferLevel.Seconds(),
		scoreLow:           scoreByConfigID(options, "A_all_low"),
		scoreMed:           scoreByConfigID(options, "B_fov_med"),
		scoreHigh:          scoreByConfigID(options, "C_fov_high"),
	})
}

func sizeByConfigID(options []bolaConfigOption, id string) int64 {
	for _, opt := range options {
		if opt.cfg.ID == id {
			return opt.size
		}
	}
	return 0
}

func scoreByConfigID(options []bolaConfigOption, id string) float64 {
	for _, opt := range options {
		if opt.cfg.ID == id {
			return opt.score
		}
	}
	return 0
}

type bolaDebugRow struct {
	segment            int
	cfgBeforeGuardrail string
	cfgAfterGuardrail  string
	avgTPBps           float64
	avgTPKiBps         float64
	timeBudgetS        float64
	budgetBytes        float64
	sizeLowBytes       int64
	sizeMedBytes       int64
	sizeHighBytes      int64
	guardrail          bool
	bufferS            float64
	scoreLow           float64
	scoreMed           float64
	scoreHigh          float64
}

type bolaDebugLogger struct {
	mu     sync.Mutex
	file   *os.File
	writer *bufio.Writer
}

func newBolaDebugLogger(path string) (*bolaDebugLogger, error) {
	file, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	writer := bufio.NewWriter(file)
	logger := &bolaDebugLogger{file: file, writer: writer}
	if _, err := writer.WriteString("segment,cfg_before_guardrail,cfg_after_guardrail,avg_tp_bps,avg_tp_kib_s,time_budget_s,budget_bytes,size_low_bytes,size_med_bytes,size_high_bytes,guardrail,buffer_s,score_low,score_med,score_high\n"); err != nil {
		file.Close()
		return nil, err
	}
	if err := writer.Flush(); err != nil {
		file.Close()
		return nil, err
	}
	return logger, nil
}

func (l *bolaDebugLogger) Log(row bolaDebugRow) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if _, err := fmt.Fprintf(l.writer, "%d,%s,%s,%.6f,%.6f,%.6f,%.6f,%d,%d,%d,%t,%.6f,%.12f,%.12f,%.12f\n",
		row.segment,
		row.cfgBeforeGuardrail,
		row.cfgAfterGuardrail,
		row.avgTPBps,
		row.avgTPKiBps,
		row.timeBudgetS,
		row.budgetBytes,
		row.sizeLowBytes,
		row.sizeMedBytes,
		row.sizeHighBytes,
		row.guardrail,
		row.bufferS,
		row.scoreLow,
		row.scoreMed,
		row.scoreHigh,
	); err != nil {
		log.Printf("BOLA ABR: failed to write debug CSV row: %v", err)
		return
	}
	if err := l.writer.Flush(); err != nil {
		log.Printf("BOLA ABR: failed to flush debug CSV row: %v", err)
	}
}
