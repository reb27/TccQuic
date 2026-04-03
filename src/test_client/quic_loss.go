package test_client

import (
	"context"
	"sync"
	"time"

	"main/src/test_client/metrics"

	"github.com/lucas-clemente/quic-go/logging"
)

type clientQUICUplinkLossCollector struct {
	logging.NullTracer

	agg *metrics.ClientQUICUplinkLossRateAgg
}

func newClientQUICUplinkLossCollector(agg *metrics.ClientQUICUplinkLossRateAgg) *clientQUICUplinkLossCollector {
	return &clientQUICUplinkLossCollector{agg: agg}
}

func (c *clientQUICUplinkLossCollector) TracerForConnection(context.Context, logging.Perspective, logging.ConnectionID) logging.ConnectionTracer {
	return &clientQUICUplinkConnectionTracer{agg: c.agg}
}

func (c *clientQUICUplinkLossCollector) StartDataPhase() {
	if c == nil || c.agg == nil {
		return
	}
	c.agg.StartDataPhase(time.Now())
}

type clientQUICUplinkConnectionTracer struct {
	logging.NullConnectionTracer

	agg  *metrics.ClientQUICUplinkLossRateAgg
	once sync.Once
}

func (t *clientQUICUplinkConnectionTracer) AcknowledgedPacket(logging.EncryptionLevel, logging.PacketNumber) {
	if t == nil || t.agg == nil {
		return
	}
	t.agg.AddAcked(time.Now())
}

func (t *clientQUICUplinkConnectionTracer) LostPacket(logging.EncryptionLevel, logging.PacketNumber, logging.PacketLossReason) {
	if t == nil || t.agg == nil {
		return
	}
	t.agg.AddLost(time.Now())
}

func (t *clientQUICUplinkConnectionTracer) Close() {
	t.once.Do(func() {})
}
