package session

import "sync"

// legacyReadyTracker tracks the highest contiguous segment whose complete tile
// request round has finished. A missed tile affects delivery quality, but does
// not stop playback of the remaining tiles or invalidate every later buffer
// sample. It is intentionally Legacy-only: BOLA keeps using the existing
// lastDownloadedSegment signal.
type legacyReadyTracker struct {
	mu             sync.Mutex
	readyThrough   int
	segmentResults map[int]*legacySegmentResult
}

type legacySegmentResult struct {
	expected  int
	completed int
	ready     bool
}

func newLegacyReadyTracker(firstSegment int) *legacyReadyTracker {
	return &legacyReadyTracker{
		readyThrough:   firstSegment - 1,
		segmentResults: make(map[int]*legacySegmentResult),
	}
}

func (t *legacyReadyTracker) RegisterSegment(segmentID int, expectedTiles int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	result := t.segment(segmentID)
	result.expected = expectedTiles
}

func (t *legacyReadyTracker) RecordTileComplete(segmentID int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	result := t.segment(segmentID)
	result.completed++
	if result.expected > 0 && result.completed == result.expected {
		result.ready = true
	}

	t.advanceContiguous()
}

func (t *legacyReadyTracker) ReadyThrough() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.readyThrough
}

// ReadyThroughForPlayback drops only gaps that playback has already passed,
// then releases any ready contiguous suffix. A failed future segment still
// blocks the buffer; a failed segment in the past cannot poison all later
// decisions forever.
func (t *legacyReadyTracker) ReadyThroughForPlayback(currentPlaybackSegment int) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	if currentPlaybackSegment > t.readyThrough {
		for segmentID := t.readyThrough + 1; segmentID <= currentPlaybackSegment; segmentID++ {
			delete(t.segmentResults, segmentID)
		}
		t.readyThrough = currentPlaybackSegment
	}
	t.advanceContiguous()
	return t.readyThrough
}

func (t *legacyReadyTracker) segment(segmentID int) *legacySegmentResult {
	result, ok := t.segmentResults[segmentID]
	if !ok {
		result = &legacySegmentResult{}
		t.segmentResults[segmentID] = result
	}
	return result
}

func (t *legacyReadyTracker) advanceContiguous() {
	for {
		next := t.segmentResults[t.readyThrough+1]
		if next == nil || !next.ready {
			return
		}
		delete(t.segmentResults, t.readyThrough+1)
		t.readyThrough++
	}
}
