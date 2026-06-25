package session

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLegacySamplesBufferAfterPrefetchWait(t *testing.T) {
	playback := NewPlaybackSimulator(time.Second, 0, 1, 5)
	now := time.Now()
	for segment := 1; segment <= 5; segment++ {
		playback.segmentPlaybackTime[segment] = now.Add(time.Duration(segment-1) * time.Second)
	}
	tracker := newLegacyReadyTracker(1)
	for segment := 1; segment <= 3; segment++ {
		tracker.RegisterSegment(segment, 1)
		tracker.RecordTileComplete(segment)
	}
	s := &TestSession{playback: playback, legacyReady: tracker}

	got := make(chan time.Duration, 1)
	go func() {
		level, _ := s.bufferLevelForDecision(4)
		got <- level
	}()

	select {
	case <-got:
		t.Fatal("buffer sampling returned before the prefetch window opened")
	case <-time.After(20 * time.Millisecond):
	}

	playback.mutex.Lock()
	playback.currentPlaybackSegment = 1
	playback.mutex.Unlock()
	playback.cond.Broadcast()

	select {
	case level := <-got:
		require.Equal(t, 2*time.Second, level, "sample must reflect playback progress during the wait")
	case <-time.After(time.Second):
		t.Fatal("buffer sampling remained blocked after the prefetch window opened")
	}
}

func TestLegacyReadyTrackerRequiresEveryTileOutcome(t *testing.T) {
	tracker := newLegacyReadyTracker(1)
	tracker.RegisterSegment(1, 2)

	tracker.RecordTileComplete(1)
	require.Equal(t, 0, tracker.ReadyThrough(), "partial segment must not fill the buffer")

	tracker.RecordTileComplete(1)
	require.Equal(t, 1, tracker.ReadyThrough())
}

func TestLegacyReadyTrackerCompletedRoundSurvivesMissingTile(t *testing.T) {
	tracker := newLegacyReadyTracker(1)
	tracker.RegisterSegment(1, 2)

	// Both a received response and a terminal timeout complete work in the
	// request round. Delivery quality is tracked separately from buffer state.
	tracker.RecordTileComplete(1)
	tracker.RecordTileComplete(1)
	require.Equal(t, 1, tracker.ReadyThrough())
}

func TestLegacyReadyTrackerOnlyAdvancesContiguousSequence(t *testing.T) {
	tracker := newLegacyReadyTracker(1)
	tracker.RegisterSegment(1, 1)
	tracker.RegisterSegment(2, 1)

	tracker.RecordTileComplete(2)
	require.Equal(t, 0, tracker.ReadyThrough(), "future segment must not skip a gap")

	tracker.RecordTileComplete(1)
	require.Equal(t, 2, tracker.ReadyThrough(), "filling the gap releases contiguous ready segments")
}

func TestLegacyReadyTrackerRecoversAfterPlaybackPassesFailedGap(t *testing.T) {
	tracker := newLegacyReadyTracker(1)
	tracker.RegisterSegment(1, 2)
	tracker.RegisterSegment(2, 1)
	tracker.RecordTileComplete(1)
	tracker.RecordTileComplete(2)
	require.Equal(t, 0, tracker.ReadyThrough())

	require.Equal(t, 2, tracker.ReadyThroughForPlayback(1))
}
