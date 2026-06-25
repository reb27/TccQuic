package session

import (
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"main/src/model"
	"main/src/test_client/fov"
	"main/src/test_client/metrics"
	"main/src/test_client/netstats"
)

type RequestSender interface {
	Request(model.VideoPacketRequest, time.Duration) *model.VideoPacketResponse
	ClientQUICUplinkLossSnapshot() (float64, uint64, uint64, []metrics.ClientQUICUplinkLossRateSample)
}

type Options struct {
	Parallelism     int
	BaseLatency     time.Duration
	SegmentDuration time.Duration
	FirstSegment    int
	LastSegment     int
	FirstTile       int
	LastTile        int
	MediaLayout     *MediaLayout
	// ValidSegments, if non-empty, restricts iteration to these segment IDs (sorted).
	// FirstSegment/LastSegment should still be min/max of this list for playback timing and metrics spans.
	ValidSegments []int
}

type MediaLayout struct {
	SegmentTiles map[int][]int
}

type Environment struct {
	FOVTracePath                 string
	FOVTraceFPS                  int
	StatisticsPath               string
	SummaryPath                  string
	FOVDeliveryPath              string
	FOVGoodputPath               string
	DeadlineLatenessPath         string
	ClientQUICUplinkLossRatePath string
	ABRMode                      string
	BOLADebugPath                string
	BOLAQmaxSegments             int
	LegacyDebugPath              string
}

type TestSession struct {
	client        RequestSender
	env           Environment
	opts          Options
	statsLogger   *metrics.StatisticsLogger
	summaryLogger *metrics.SummaryLogger
	playback      *PlaybackSimulator
	metrics       *metrics.Session
	collector     *netstats.StatsCollector
	abr           ABRController
	semaphore     Semaphore
	fovTrace      *fov.FOVTrace

	lastDownloadedSegment atomic.Int32
	legacyReady           *legacyReadyTracker
	fallbackTileUniverse  []int
}

func NewTestSession(client RequestSender, env Environment, opts Options) *TestSession {
	statsLogger := metrics.NewStatisticsLogger(env.StatisticsPath)
	summaryLogger := metrics.NewSummaryLogger(env.SummaryPath)
	playback := NewPlaybackSimulator(opts.SegmentDuration, opts.BaseLatency, opts.FirstSegment, opts.LastSegment)
	metricSet := metrics.NewSession(opts.SegmentDuration)
	collector := netstats.New(opts.LastSegment - opts.FirstSegment + 1)
	semaphore := NewSemaphore(opts.Parallelism)
	abr := SelectABRController(env)

	s := &TestSession{
		client:               client,
		env:                  env,
		opts:                 opts,
		statsLogger:          statsLogger,
		summaryLogger:        summaryLogger,
		playback:             playback,
		metrics:              metricSet,
		collector:            collector,
		abr:                  abr,
		semaphore:            semaphore,
		fallbackTileUniverse: buildTileUniverse(opts.FirstTile, opts.LastTile),
	}
	s.lastDownloadedSegment.Store(int32(opts.FirstSegment - 1))
	if _, legacy := abr.(*bufferAwareABR); legacy {
		s.legacyReady = newLegacyReadyTracker(opts.FirstSegment)
	}
	return s
}

func (s *TestSession) Run() error {
	defer s.statsLogger.Close()
	defer s.summaryLogger.Close()
	if legacy, ok := s.abr.(*bufferAwareABR); ok {
		defer legacy.Close()
	}
	s.playback.Start()
	if err := s.loadFOVTrace(); err != nil {
		log.Printf("Failed to load FOV trace from %s: %v (continuing without FOV prioritisation)", s.env.FOVTracePath, err)
	}

	startTime := time.Now()
	scheduler := NewTileScheduler(s.client, s.playback, s.collector, s.metrics, s.statsLogger, s.semaphore, startTime, &s.lastDownloadedSegment, s.legacyReady)
	if len(s.opts.ValidSegments) > 0 {
		log.Printf("Starting test iteration for %d segment(s) present on disk (tiles %d to %d)", len(s.opts.ValidSegments), s.opts.FirstTile, s.opts.LastTile)
	} else {
		log.Printf("Starting test iteration for segments %d to %d (tiles %d to %d)", s.opts.FirstSegment, s.opts.LastSegment, s.opts.FirstTile, s.opts.LastTile)
	}
	fmt.Printf("Test started with parallelism = %d\n", s.opts.Parallelism)

	for _, segmentID := range s.segmentIDsInOrder() {
		s.processSegment(segmentID, scheduler)
	}

	log.Println("Waiting for all goroutines to finish...")
	scheduler.Wait()
	log.Println("All goroutines completed.")
	fmt.Println("Test iteration complete.")
	s.finalize(startTime, scheduler.FirstRequestTime())
	return nil
}

func (s *TestSession) segmentIDsInOrder() []int {
	if len(s.opts.ValidSegments) > 0 {
		return s.opts.ValidSegments
	}
	n := s.opts.LastSegment - s.opts.FirstSegment + 1
	if n <= 0 {
		return nil
	}
	out := make([]int, 0, n)
	for i := s.opts.FirstSegment; i <= s.opts.LastSegment; i++ {
		out = append(out, i)
	}
	return out
}

func (s *TestSession) processSegment(segmentID int, scheduler *TileScheduler) {
	bufferLevel, readyThrough := s.bufferLevelForDecision(segmentID)
	avgThroughput := s.collector.AvgThroughput()
	timeBudget := s.playback.GetTimeToReceive(segmentID)
	if timeBudget <= 0 {
		timeBudget = s.opts.SegmentDuration
	}
	maxAhead := 3 * s.opts.SegmentDuration
	if timeBudget > maxAhead {
		timeBudget = maxAhead
	}
	timeBudget += s.opts.SegmentDuration
	segmentDeadline := time.Now().Add(timeBudget)

	segmentTiles := s.tilesForSegment(segmentID)
	if len(segmentTiles) == 0 {
		log.Printf("Skipping segment %d: no media tiles available", segmentID)
		return
	}

	s.metrics.AllTiles.SetRequired(segmentID, segmentTiles)
	s.metrics.DeadlineLateness.SetRequired(segmentID, segmentTiles)

	var fovTiles []int
	var nearFOVTiles []int
	if s.fovTrace != nil {
		fovTiles = filterTilesAvailable(s.fovTrace.TilesForSegment(segmentID), segmentTiles)
		nearFOVTiles = nearFOVTilesForSegment(s.fovTrace, segmentID, segmentTiles)
	}
	s.metrics.FOVTiles.SetRequired(segmentID, fovTiles)

	ctx := SegmentContext{
		SegmentID:       segmentID,
		FirstSegment:    s.opts.FirstSegment,
		LastSegment:     s.opts.LastSegment,
		SegmentDuration: s.opts.SegmentDuration,
		TimeBudget:      timeBudget,
		AvgThroughput:   avgThroughput,
		BufferLevel:     bufferLevel,
		FOVTiles:        fovTiles,
		NearFOVTiles:    nearFOVTiles,
		AllTiles:        segmentTiles,
	}
	if s.legacyReady != nil {
		ctx.ReadyThroughSegment = readyThrough
	}
	cfg := s.abr.SelectConfig(ctx)
	log.Printf("ABR: cfg=%s fov_bitrate=%d near_fov_bitrate=%d nonfov_bitrate=%d avg_tp=%.2f buffer=%.2f s", cfg.ID, cfg.FOVBitrate, cfg.NearFOVBitrate, cfg.NonFOVBitrate, avgThroughput, bufferLevel.Seconds())
	scheduler.ScheduleSegment(segmentID, segmentDeadline, cfg, segmentTiles, s.fovTrace)
}

func (s *TestSession) bufferLevelForDecision(segmentID int) (time.Duration, int) {
	if s.legacyReady != nil {
		// Legacy must sample after a potentially long prefetch wait. Sampling
		// before it made the ABR decision use stale buffer state.
		s.playback.WaitUntilWithinPrefetchWindow(segmentID)
		readyThrough := s.legacyReady.ReadyThroughForPlayback(s.playback.CurrentPlaybackSegment())
		return s.playback.GetBufferLevel(readyThrough), readyThrough
	}

	// Preserve the existing BOLA sampling behaviour.
	bufferLevel := s.playback.GetBufferLevel(int(s.lastDownloadedSegment.Load()))
	s.playback.WaitUntilWithinPrefetchWindow(segmentID)
	return bufferLevel, int(s.lastDownloadedSegment.Load())
}

func (s *TestSession) finalize(startTime time.Time, firstRequestTime time.Time) {
	elapsed := time.Since(startTime)
	joinLatency := time.Duration(0)
	if !firstRequestTime.IsZero() {
		playbackStart := s.playback.GetPlaybackStartTime()
		joinLatency = playbackStart.Sub(firstRequestTime)
		if joinLatency < 0 {
			joinLatency = 0
		}
	}

	completionRate := s.metrics.AllTiles.Rate(s.opts.FirstSegment, s.opts.LastSegment)
	lastFOVSegment := s.lastFOVSegment()
	fovCompletionRate := -1.0
	if lastFOVSegment > 0 {
		fovCompletionRate = s.metrics.FOVTiles.Rate(s.opts.FirstSegment, lastFOVSegment)
	}

	staleRatio := s.metrics.Stale.RatioPercent()
	timelyRatio := s.metrics.Stale.TimelyPercent()
	fovMissRate, nonFOVMissRate := s.metrics.Deadlines.Rates()
	fovHitRate := s.metrics.FOVHit.RateOverall()
	fovGoodputRate := s.metrics.FOVGoodput.OverallKbps(elapsed)
	clientQUICUplinkLossRate, clientQUICUplinkLostPackets, clientQUICUplinkAckedPackets, clientQUICUplinkLossSeries := s.client.ClientQUICUplinkLossSnapshot()

	if s.summaryLogger != nil {
		s.summaryLogger.LogSession(joinLatency, completionRate, fovCompletionRate, staleRatio, fovMissRate, nonFOVMissRate, fovHitRate, fovGoodputRate, timelyRatio, clientQUICUplinkLossRate, clientQUICUplinkLostPackets, clientQUICUplinkAckedPackets)
	}
	if s.env.FOVDeliveryPath != "" {
		metrics.WriteFOVDeliverySeries(s.env.FOVDeliveryPath, s.metrics.FOVHit.Series(s.opts.FirstSegment, s.opts.LastSegment))
	}
	if s.env.FOVGoodputPath != "" {
		metrics.WriteFOVGoodputSeries(s.env.FOVGoodputPath, s.metrics.FOVGoodput.Series())
	}
	if s.env.DeadlineLatenessPath != "" {
		metrics.WriteDeadlineLatenessSeries(s.env.DeadlineLatenessPath, s.metrics.DeadlineLateness.Series(s.opts.FirstSegment, s.opts.LastSegment))
	}
	if s.env.ClientQUICUplinkLossRatePath != "" {
		metrics.WriteClientQUICUplinkLossRateSeries(s.env.ClientQUICUplinkLossRatePath, clientQUICUplinkLossSeries)
	}
}

func (s *TestSession) lastFOVSegment() int {
	if s.fovTrace == nil {
		return 0
	}
	last := s.fovTrace.MaxSegment()
	if last > s.opts.LastSegment {
		last = s.opts.LastSegment
	}
	if last < s.opts.FirstSegment {
		return 0
	}
	return last
}

func (s *TestSession) loadFOVTrace() error {
	trace, err := fov.LoadFOVTrace(s.env.FOVTracePath, s.env.FOVTraceFPS, s.opts.SegmentDuration)
	if err != nil {
		s.fovTrace = nil
		return err
	}
	s.fovTrace = trace
	log.Printf("Loaded FOV trace: fps=%d, segments=%d", s.env.FOVTraceFPS, s.fovTrace.MaxSegment())
	return nil
}

func buildTileUniverse(firstTile, lastTile int) []int {
	size := lastTile - firstTile + 1
	if size <= 0 {
		return nil
	}
	tiles := make([]int, 0, size)
	for tileID := firstTile; tileID <= lastTile; tileID++ {
		tiles = append(tiles, tileID)
	}
	return tiles
}

func (s *TestSession) tilesForSegment(segmentID int) []int {
	if s.opts.MediaLayout != nil {
		if tiles := s.opts.MediaLayout.SegmentTiles[segmentID]; len(tiles) > 0 {
			return tiles
		}
	}
	return s.fallbackTileUniverse
}

func filterTilesAvailable(tiles []int, available []int) []int {
	if len(tiles) == 0 {
		return nil
	}
	availableSet := make(map[int]struct{}, len(available))
	for _, tile := range available {
		availableSet[tile] = struct{}{}
	}
	filtered := make([]int, 0, len(tiles))
	for _, tile := range tiles {
		if _, ok := availableSet[tile]; ok {
			filtered = append(filtered, tile)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func nearFOVTilesForSegment(trace *fov.FOVTrace, segmentID int, segmentTiles []int) []int {
	if trace == nil || len(segmentTiles) == 0 {
		return nil
	}
	tiles := make([]int, 0)
	for _, tileID := range segmentTiles {
		if trace.NearFoV(segmentID, tileID, nearFoVMargin) {
			tiles = append(tiles, tileID)
		}
	}
	if len(tiles) == 0 {
		return nil
	}
	return tiles
}
