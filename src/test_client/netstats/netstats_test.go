package netstats

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestAvgThroughputUsesAggregatedSegmentThroughput(t *testing.T) {
	collector := New(10)
	id1 := uuid.New()
	id2 := uuid.New()

	collector.RegisterSegment(1, 2)
	collector.RecordSend(id1, 1)
	collector.RecordSend(id2, 1)
	time.Sleep(10 * time.Millisecond)
	_, tileTP1 := collector.RecordRecv(id1, 100)
	_, tileTP2 := collector.RecordRecv(id2, 100)

	avg := collector.AvgThroughput()
	require.Greater(t, avg, tileTP1)
	require.Greater(t, avg, tileTP2)
	require.InDelta(t, 200.0, avg*0.010, 120.0)
}

func TestAvgThroughputBeforeValidSampleIsZero(t *testing.T) {
	collector := New(10)

	require.Zero(t, collector.AvgThroughput())
}

func TestIncompleteSegmentDoesNotUpdateThroughput(t *testing.T) {
	collector := New(10)
	id := uuid.New()

	collector.RegisterSegment(1, 2)
	collector.RecordSend(id, 1)
	time.Sleep(time.Millisecond)
	collector.RecordRecv(id, 100)

	require.Zero(t, collector.AvgThroughput())
	require.Zero(t, collector.Pending())
}

func TestAvgThroughputUpdatesWithSegmentEWMA(t *testing.T) {
	collector := New(10)

	first := completeSingleTileSegment(t, collector, 1, 100, 10*time.Millisecond)
	second := completeSingleTileSegment(t, collector, 2, 300, 10*time.Millisecond)

	want := ewmaAlpha*second + (1.0-ewmaAlpha)*first
	require.InDelta(t, want, collector.AvgThroughput(), want*0.35)
}

func TestFailedTilesCanCompleteSegmentWithoutUpdatingThroughput(t *testing.T) {
	collector := New(10)
	id := uuid.New()

	collector.RegisterSegment(1, 1)
	collector.RecordSend(id, 1)
	collector.RecordFailure(id)

	require.Zero(t, collector.Pending())
	require.Zero(t, collector.AvgThroughput())
}

func TestSkippedTilesCanCompleteSegmentWithoutAddingBytes(t *testing.T) {
	collector := New(10)

	collector.RegisterSegment(1, 1)
	collector.RecordSkipped(1)

	require.Zero(t, collector.AvgThroughput())
}

func TestSegmentClosesWithRecvFailureAndSkip(t *testing.T) {
	collector := New(10)
	recvID := uuid.New()
	failID := uuid.New()

	collector.RegisterSegment(1, 3)
	collector.RecordSend(recvID, 1)
	collector.RecordSend(failID, 1)
	time.Sleep(time.Millisecond)
	collector.RecordRecv(recvID, 100)
	collector.RecordFailure(failID)
	require.Zero(t, collector.AvgThroughput())

	collector.RecordSkipped(1)

	require.Zero(t, collector.Pending())
	require.Greater(t, collector.AvgThroughput(), 0.0)
}

func TestLateResponsesCountTowardAggregatedThroughput(t *testing.T) {
	collector := New(10)
	id := uuid.New()

	collector.RegisterSegment(1, 1)
	collector.RecordSend(id, 1)
	time.Sleep(time.Millisecond)
	// Netstats intentionally has no deadline/on-time input; late responses are
	// still received bytes and therefore count toward capacity estimation.
	collector.RecordRecv(id, 100)

	require.Greater(t, collector.AvgThroughput(), 0.0)
}

func completeSingleTileSegment(t *testing.T, collector *StatsCollector, segmentID int, bytes int, delay time.Duration) float64 {
	t.Helper()
	id := uuid.New()
	collector.RegisterSegment(segmentID, 1)
	collector.RecordSend(id, segmentID)
	time.Sleep(delay)
	_, tileTP := collector.RecordRecv(id, bytes)
	return tileTP
}
