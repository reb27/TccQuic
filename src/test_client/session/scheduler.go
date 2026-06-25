package session

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"main/src/model"
	"main/src/test_client/fov"
	"main/src/test_client/metrics"
	"main/src/test_client/netstats"
)

const staleMeasurementGrace = 200 * time.Millisecond

type TileScheduler struct {
	client                RequestSender
	playback              *PlaybackSimulator
	collector             *netstats.StatsCollector
	metrics               *metrics.Session
	statsLogger           *metrics.StatisticsLogger
	startTime             time.Time
	firstRequestOnce      sync.Once
	firstRequestTime      time.Time
	lastDownloadedSegment *atomic.Int32
	legacyReady           *legacyReadyTracker
	sem                   Semaphore
	wg                    sync.WaitGroup
}

func NewTileScheduler(client RequestSender, playback *PlaybackSimulator, collector *netstats.StatsCollector, metrics *metrics.Session, statsLogger *metrics.StatisticsLogger, sem Semaphore, startTime time.Time, lastDownloadedSegment *atomic.Int32, legacyReady *legacyReadyTracker) *TileScheduler {
	return &TileScheduler{
		client:                client,
		playback:              playback,
		collector:             collector,
		metrics:               metrics,
		statsLogger:           statsLogger,
		startTime:             startTime,
		lastDownloadedSegment: lastDownloadedSegment,
		legacyReady:           legacyReady,
		sem:                   sem,
	}
}

const nearFoVMargin = 2

type TileRequestPlanItem struct {
	TileID           int
	Bitrate          model.Bitrate
	Priority         model.Priority
	InFOV            bool
	NearFOV          bool
	SemanticPriority float32
	RequestOrder     int
}

func BuildTileRequestPlan(segmentID int, cfg SegmentConfig, tiles []int, fovTrace *fov.FOVTrace) []TileRequestPlanItem {
	plan := make([]TileRequestPlanItem, 0, len(tiles))
	for _, tileID := range tiles {
		inFOV := fovTrace != nil && fovTrace.Contains(segmentID, tileID)
		nearFOV := !inFOV && fovTrace != nil && fovTrace.NearFoV(segmentID, tileID, nearFoVMargin)

		priority := model.LOW_PRIORITY
		semanticPriority := float32(model.SemanticPiBackground)
		if inFOV {
			priority = model.HIGH_PRIORITY
			semanticPriority = float32(model.SemanticPiFoV)
		} else if nearFOV {
			priority = model.MEDIUM_PRIORITY
			semanticPriority = float32(model.SemanticPiNearFoV)
		}

		requestBitrate := cfg.NonFOVBitrate
		if inFOV {
			requestBitrate = cfg.FOVBitrate
		} else if nearFOV {
			requestBitrate = cfg.NearFOVBitrate
		}

		plan = append(plan, TileRequestPlanItem{
			TileID:           tileID,
			Bitrate:          requestBitrate,
			Priority:         priority,
			InFOV:            inFOV,
			NearFOV:          nearFOV,
			SemanticPriority: semanticPriority,
		})
	}

	legacyConfig := strings.HasPrefix(cfg.ID, "legacy_")
	sort.SliceStable(plan, func(i, j int) bool {
		// Preserve the historical BOLA ordering exactly: FoV first, then
		// bitrate, then tile ID. Legacy additionally resolves equal-quality
		// non-FoV requests as Near-FoV before background.
		if plan[i].InFOV != plan[j].InFOV {
			return plan[i].InFOV
		}
		if legacyConfig && plan[i].NearFOV != plan[j].NearFOV {
			return plan[i].NearFOV
		}
		if plan[i].Bitrate != plan[j].Bitrate {
			return plan[i].Bitrate > plan[j].Bitrate
		}
		return plan[i].TileID < plan[j].TileID
	})

	for i := range plan {
		plan[i].RequestOrder = i + 1
	}
	return plan
}

func spatialRequestRank(item TileRequestPlanItem) int {
	if item.InFOV {
		return 0
	}
	if item.NearFOV {
		return 1
	}
	return 2
}

func (s *TileScheduler) ScheduleSegment(segmentID int, deadline time.Time, cfg SegmentConfig, tiles []int, fovTrace *fov.FOVTrace) {
	plan := BuildTileRequestPlan(segmentID, cfg, tiles, fovTrace)
	s.collector.RegisterSegment(segmentID, len(plan))
	if s.legacyReady != nil {
		s.legacyReady.RegisterSegment(segmentID, len(plan))
	}
	for _, item := range plan {
		s.sem.Acquire()
		s.wg.Add(1)
		go s.handleTile(segmentID, deadline, item)
	}
}

func (s *TileScheduler) handleTile(segmentID int, deadline time.Time, item TileRequestPlanItem) {
	defer func() {
		s.sem.Release()
		s.wg.Done()
	}()

	remaining := time.Until(deadline)
	if remaining <= 0 {
		s.registerTimeout(segmentID, item, deadline)
		return
	}

	timeoutMs := int(remaining / time.Millisecond)
	if timeoutMs <= 0 {
		timeoutMs = 1
	}

	request := model.VideoPacketRequest{
		ID:               uuid.New(),
		Priority:         item.Priority,
		Bitrate:          item.Bitrate,
		Segment:          segmentID,
		Tile:             item.TileID,
		FoV:              item.InFOV,
		TileFirstLayout:  s.legacyReady != nil,
		SemanticPriority: item.SemanticPriority,
		Timeout:          timeoutMs,
	}

	fmt.Printf("Sending request for segment %d, tile %d (priority=%d, FOV=%t, order=%d)\n", segmentID, item.TileID, item.Priority, item.InFOV, item.RequestOrder)
	s.firstRequestOnce.Do(func() { s.firstRequestTime = time.Now() })
	sendBufferSec := s.playback.GetBufferLevel(int(s.lastDownloadedSegment.Load())).Seconds()
	s.collector.RecordSend(request.ID, segmentID)

	requestTime := time.Since(s.startTime)
	requestTimeout := remaining + staleMeasurementGrace
	if requestTimeout <= 0 {
		requestTimeout = time.Millisecond
	}
	response := s.client.Request(request, requestTimeout)
	arrivalAt := time.Now()
	responseTime := time.Since(s.startTime)
	lateness := arrivalAt.Sub(deadline)
	if lateness < 0 {
		lateness = 0
	}

	bytesReceived := 0
	instaThroughput := 0.0
	timedOut := false
	if response == nil {
		fmt.Printf("Timeout: no response for segment %d, tile %d\n", segmentID, item.TileID)
		timedOut = true
		s.collector.RecordFailure(request.ID)
	} else {
		if len(response.Data) == 0 {
			log.Panicf("Empty response for (%d, %d)", segmentID, item.TileID)
		}
		bytesReceived = len(response.Data)
		_, instaThroughput = s.collector.RecordRecv(request.ID, bytesReceived)
		late := time.Now().After(deadline)
		s.metrics.Stale.Add(bytesReceived, late)
		if late {
			fmt.Printf("Late response for segment %d, tile %d\n", segmentID, item.TileID)
			timedOut = true
		} else {
			fmt.Printf("Received response for segment %d, tile %d\n", segmentID, item.TileID)
		}
	}

	s.metrics.DeadlineLateness.Record(segmentID, item.TileID, lateness)
	onTime := response != nil && !timedOut
	if s.legacyReady != nil {
		s.legacyReady.RecordTileComplete(segmentID)
	}
	s.metrics.FOVHit.Add(segmentID, item.InFOV, onTime)
	s.metrics.FOVGoodput.Add(responseTime, bytesReceived, item.InFOV, onTime)
	s.metrics.Deadlines.Add(item.InFOV, !onTime)
	ratio, complete := s.metrics.AllTiles.Record(segmentID, item.TileID, onTime)
	tmrValue := -1.0
	if complete {
		tmrValue = ratio
	}
	if item.InFOV {
		s.metrics.FOVTiles.Record(segmentID, item.TileID, onTime)
	}

	if response != nil {
		for {
			oldValue := s.lastDownloadedSegment.Load()
			if int32(segmentID) <= oldValue {
				break
			}
			if s.lastDownloadedSegment.CompareAndSwap(oldValue, int32(segmentID)) {
				break
			}
		}
	}

	if s.statsLogger != nil {
		completed := !timedOut
		if s.legacyReady != nil {
			// Legacy validation distinguishes a response that completed after its
			// deadline from a request that never produced a response. Preserve the
			// historical BOLA statistics semantics by scoping this to Legacy.
			completed = response != nil
		}
		s.statsLogger.Log(requestTime, request, responseTime-requestTime, timedOut, false, completed, instaThroughput, sendBufferSec, tmrValue, item.InFOV, onTime, item.RequestOrder)
	}
}

func (s *TileScheduler) registerTimeout(segmentID int, item TileRequestPlanItem, deadline time.Time) {
	s.collector.RecordSkipped(segmentID)
	if s.legacyReady != nil {
		s.legacyReady.RecordTileComplete(segmentID)
	}
	lateness := time.Since(deadline)
	if lateness < 0 {
		lateness = 0
	}
	s.metrics.DeadlineLateness.Record(segmentID, item.TileID, lateness)
	ratio, complete := s.metrics.AllTiles.Record(segmentID, item.TileID, false)
	tmrValue := -1.0
	if complete {
		tmrValue = ratio
	}
	if item.InFOV {
		s.metrics.FOVTiles.Record(segmentID, item.TileID, false)
	}
	s.metrics.Deadlines.Add(item.InFOV, true)
	s.metrics.FOVHit.Add(segmentID, item.InFOV, false)
	if s.statsLogger != nil {
		lastReady := int(s.lastDownloadedSegment.Load())
		if s.legacyReady != nil {
			lastReady = s.legacyReady.ReadyThrough()
		}
		bufferSec := s.playback.GetBufferLevel(lastReady).Seconds()
		s.statsLogger.Log(time.Since(s.startTime), model.VideoPacketRequest{
			ID:               uuid.Nil,
			Priority:         item.Priority,
			Bitrate:          item.Bitrate,
			Segment:          segmentID,
			Tile:             item.TileID,
			FoV:              item.InFOV,
			SemanticPriority: item.SemanticPriority,
			Timeout:          0,
		}, 0, true, true, false, 0.0, bufferSec, tmrValue, item.InFOV, false, item.RequestOrder)
	}
}

func (s *TileScheduler) Wait() {
	s.wg.Wait()
}

func (s *TileScheduler) FirstRequestTime() time.Time {
	return s.firstRequestTime
}
