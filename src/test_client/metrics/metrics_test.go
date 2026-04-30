package metrics

import (
	"main/src/model"
	"os"
	"strings"
	"testing"
	"time"
)

func TestClientQUICUplinkLossRateAggOverallPercent(t *testing.T) {
	agg := NewClientQUICUplinkLossRateAgg(time.Second)
	base := time.Unix(100, 0)
	agg.StartDataPhase(base)

	if got := agg.OverallPercent(); got != 0 {
		t.Fatalf("expected 0%% for empty agg, got %.2f", got)
	}

	agg.AddLost(base.Add(100 * time.Millisecond))
	for i := 0; i < 9; i++ {
		agg.AddAcked(base.Add(200*time.Millisecond + time.Duration(i)*time.Millisecond))
	}

	if got := agg.OverallPercent(); got != 10 {
		t.Fatalf("expected 10%% overall loss rate, got %.2f", got)
	}

	lost, acked := agg.Totals()
	if lost != 1 || acked != 9 {
		t.Fatalf("expected totals 1/9, got %d/%d", lost, acked)
	}
}

func TestClientQUICUplinkLossRateAggSeriesAndHandshakeExclusion(t *testing.T) {
	agg := NewClientQUICUplinkLossRateAgg(time.Second)
	base := time.Unix(200, 0)

	agg.AddLost(base.Add(-500 * time.Millisecond))
	agg.AddAcked(base.Add(-250 * time.Millisecond))

	agg.StartDataPhase(base)
	agg.AddLost(base.Add(100 * time.Millisecond))
	agg.AddAcked(base.Add(200 * time.Millisecond))
	agg.AddAcked(base.Add(1300 * time.Millisecond))

	series := agg.Series()
	if len(series) != 2 {
		t.Fatalf("expected 2 buckets, got %d", len(series))
	}

	if series[0].WindowStart != 0 || series[0].WindowEnd != time.Second {
		t.Fatalf("unexpected first bucket window: %+v", series[0])
	}
	if series[0].LostPackets != 1 || series[0].AckedPackets != 1 || series[0].Percent != 50 {
		t.Fatalf("unexpected first bucket counts: %+v", series[0])
	}

	if series[1].WindowStart != time.Second || series[1].WindowEnd != 2*time.Second {
		t.Fatalf("unexpected second bucket window: %+v", series[1])
	}
	if series[1].LostPackets != 0 || series[1].AckedPackets != 1 || series[1].Percent != 0 {
		t.Fatalf("unexpected second bucket counts: %+v", series[1])
	}
}

func TestStatisticsLoggerIncludesBitrateColumn(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/stats.csv"
	logger := NewStatisticsLogger(path)
	logger.Log(
		time.Second,
		model.VideoPacketRequest{Segment: 1, Tile: 2, Priority: 0, Bitrate: model.HIGH_BITRATE},
		50*time.Millisecond,
		false,
		false,
		true,
		1.0,
		1.5,
		0.0,
		true,
		true,
	)
	logger.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read stats: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, ",on_time,bitrate\n") {
		t.Fatalf("expected bitrate in header: %s", content)
	}
	if !strings.Contains(content, ",true,true,10\n") {
		t.Fatalf("expected HIGH_BITRATE (10) in row: %s", content)
	}
}

func TestSummaryLoggerWritesClientQUICLossColumns(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/summary.csv"

	logger := NewSummaryLogger(path)
	logger.LogSession(150*time.Millisecond, 90, 80, 5, 10, 20, 95, 1234, 88, 12.5, 3, 21)
	logger.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read summary file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "client_quic_uplink_loss_rate_percent,client_quic_uplink_lost_packets,client_quic_uplink_acked_packets") {
		t.Fatalf("missing client QUIC loss columns in header: %s", content)
	}
	if !strings.Contains(content, ",12.50,3,21\n") {
		t.Fatalf("missing expected client QUIC loss row values: %s", content)
	}
}

func TestWriteClientQUICUplinkLossRateSeries(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/loss.csv"

	WriteClientQUICUplinkLossRateSeries(path, []ClientQUICUplinkLossRateSample{
		{
			WindowStart:  0,
			WindowEnd:    time.Second,
			LostPackets:  1,
			AckedPackets: 9,
			Percent:      10,
		},
	})

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read loss series file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "window_start_s,window_end_s,lost_packets,acked_packets,client_quic_uplink_loss_rate_percent") {
		t.Fatalf("missing loss series header: %s", content)
	}
	if !strings.Contains(content, "0.000,1.000,1,9,10.00\n") {
		t.Fatalf("missing expected loss series row: %s", content)
	}
}
