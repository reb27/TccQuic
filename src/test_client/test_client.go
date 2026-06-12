package test_client

import (
	"log"
	"time"

	"main/src/test_client/session"
)

func StartTestClient(serverURL string, serverPort int, parallelism int, baseLatencyMs int) {
	envCfg := ResolveEnvironmentConfig()

	client := NewClient(ClientOptions{
		Pipeline:   envCfg.Pipeline,
		ServerURL:  serverURL,
		ServerPort: serverPort,
	})

	log.Printf("Base latency = %d ms", baseLatencyMs)

	if err := client.Connect(); err != nil {
		log.Printf("failed to connect: %v", err)
		return
	}

	env := session.Environment{
		FOVTracePath:                 envCfg.FOVTracePath,
		FOVTraceFPS:                  envCfg.FOVTraceFPS,
		StatisticsPath:               envCfg.StatisticsPath,
		SummaryPath:                  envCfg.SummaryPath,
		FOVDeliveryPath:              envCfg.FOVDeliveryPath,
		FOVGoodputPath:               envCfg.FOVGoodputPath,
		DeadlineLatenessPath:         envCfg.DeadlineLatenessPath,
		ClientQUICUplinkLossRatePath: envCfg.ClientQUICUplinkLossRatePath,
		ABRMode:                      envCfg.ABRMode,
		BOLADebugPath:                envCfg.BOLADebugPath,
		BOLAQmaxSegments:             envCfg.BOLAQmaxSegments,
	}

	opts := session.Options{
		Parallelism:     parallelism,
		BaseLatency:     time.Duration(baseLatencyMs) * time.Millisecond,
		SegmentDuration: time.Duration(defaultSegmentDurationSeconds) * time.Second,
		FirstSegment:    1,
		LastSegment:     120,
		FirstTile:       100,
		LastTile:        177,
	}
	if firstSeg, lastSeg, firstTile, lastTile, ok := detectMediaBounds(); ok {
		opts.FirstSegment = firstSeg
		opts.LastSegment = lastSeg
		opts.FirstTile = firstTile
		opts.LastTile = lastTile
	}
	if segs, ok := detectValidSegmentIDs(); ok && len(segs) > 0 {
		opts.ValidSegments = segs
		opts.FirstSegment = segs[0]
		opts.LastSegment = segs[len(segs)-1]
	}

	if limit := SegmentLimitFromEnv(); limit > 0 {
		if len(opts.ValidSegments) > limit {
			opts.ValidSegments = append([]int(nil), opts.ValidSegments[:limit]...)
			opts.FirstSegment = opts.ValidSegments[0]
			opts.LastSegment = opts.ValidSegments[len(opts.ValidSegments)-1]
			log.Printf("TEST_CLIENT_SEGMENT_LIMIT=%d: using %d segment(s) %d..%d", limit, len(opts.ValidSegments), opts.FirstSegment, opts.LastSegment)
		} else if len(opts.ValidSegments) == 0 {
			span := opts.LastSegment - opts.FirstSegment + 1
			if span > limit {
				opts.LastSegment = opts.FirstSegment + limit - 1
				log.Printf("TEST_CLIENT_SEGMENT_LIMIT=%d: using contiguous segments %d..%d", limit, opts.FirstSegment, opts.LastSegment)
			}
		}
	}

	testSession := session.NewTestSession(client, env, opts)

	if err := testSession.Run(); err != nil {
		log.Printf("test session failed: %v", err)
	}
}
