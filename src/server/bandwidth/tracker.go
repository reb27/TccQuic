package bandwidth

import (
	"sync"
	"time"
)

// Tracker estima a vazão (Throughput) em bytes por segundo
type Tracker struct {
	mu          sync.Mutex
	windowSize  time.Duration
	history     []sample
	currentRate float64 // bytes/sec
}

type sample struct {
	ts    time.Time
	bytes int
}

func NewTracker(window time.Duration) *Tracker {
	return &Tracker{
		windowSize: window,
		history:    []sample{},
	}
}

// Add registra uma transmissão de n bytes
func (t *Tracker) Add(n int) {
	if n <= 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	t.history = append(t.history, sample{ts: now, bytes: n})
	t.cleanup(now)
}

func (t *Tracker) cleanup(now time.Time) {
	cutoff := now.Add(-t.windowSize)
	idx := 0
	for i, s := range t.history {
		if s.ts.After(cutoff) {
			idx = i
			break
		}
		// Se chegarmos aqui, a amostra está fora da janela
		if i == len(t.history)-1 {
			idx = len(t.history)
		}
	}

	if idx > 0 {
		t.history = t.history[idx:]
	}

	sum := 0
	for _, s := range t.history {
		sum += s.bytes
	}

	if len(t.history) < 2 {
		if len(t.history) == 1 {
			t.currentRate = float64(t.history[0].bytes) / 0.1 // fallback
		} else {
			t.currentRate = 0
		}
		return
	}

	first := t.history[0].ts
	duration := now.Sub(first).Seconds()
	if duration > 0 {
		t.currentRate = float64(sum) / duration
	}
}

// GetThroughput retorna a estimativa atual em bytes/seg
func (t *Tracker) GetThroughput() float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cleanup(time.Now())
	return t.currentRate
}
