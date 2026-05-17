// Package netstats implementa um módulo independente para medir
// atraso (latência) de tiles e vazão agregada por segmento.
// Ele não conhece detalhes de QUIC, buffer ou ABR: apenas registra quando
// cada requisição saiu e quando a resposta chegou.
package netstats

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// StatEntry guarda as marcas de tempo e resultados de UMA requisição.
// Comentários em linguagem simples para facilitar leitura futura.
// SentAt  → hora em que o pedido saiu
// RecvAt  → hora em que a resposta chegou
// Bytes   → tamanho recebido (para calcular vazão)
// Delay   → RecvAt - SentAt
// TP      → Vazão instantânea em bytes/segundo
type StatEntry struct {
	SentAt time.Time
	RecvAt time.Time
	Bytes  int
	Delay  time.Duration
	TP     float64

	Segment int
}

type segmentEntry struct {
	expected    int
	completed   int
	totalBytes  int
	firstSentAt time.Time
	lastDoneAt  time.Time
	finalized   bool
}

const ewmaAlpha = 0.35

// StatsCollector agrega várias StatEntry e mantém uma estimativa EWMA da vazão
// agregada por segmento completo.
type StatsCollector struct {
	mu       sync.Mutex
	pending  map[uuid.UUID]*StatEntry
	segments map[int]*segmentEntry
	ewma     float64
	hasEWMA  bool
}

// New cria um coletor de métricas. O parâmetro window é mantido para
// compatibilidade com os chamadores existentes; a média do ABR agora é EWMA.
func New(window int) *StatsCollector {
	return &StatsCollector{
		pending:  make(map[uuid.UUID]*StatEntry),
		segments: make(map[int]*segmentEntry),
	}
}

// RegisterSegment informa quantos tiles fazem parte da rodada de download de
// um segmento.
func (sc *StatsCollector) RegisterSegment(segmentID int, expectedTiles int) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	seg := sc.segment(segmentID)
	if expectedTiles > seg.expected {
		seg.expected = expectedTiles
	}
}

// RecordSend deve ser chamado assim que a requisição ID sair.
func (sc *StatsCollector) RecordSend(id uuid.UUID, segmentID int) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	sentAt := time.Now()
	sc.pending[id] = &StatEntry{SentAt: sentAt, Segment: segmentID}
	seg := sc.segment(segmentID)
	if seg.firstSentAt.IsZero() || sentAt.Before(seg.firstSentAt) {
		seg.firstSentAt = sentAt
	}
}

// RecordRecv deve ser chamado quando a resposta do mesmo ID chegar.
// bytes é o tamanho efetivamente recebido.
// Ela devolve atraso (delay) e vazão instantânea (tp) já calculados.
func (sc *StatsCollector) RecordRecv(id uuid.UUID, bytes int) (delay time.Duration, tp float64) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	entry, ok := sc.pending[id]
	if !ok {
		// resposta desconhecida (ex.: timeout já removido) – ignora
		return 0, 0
	}

	recvAt := time.Now()
	entry.RecvAt = recvAt
	entry.Bytes = bytes
	entry.Delay = entry.RecvAt.Sub(entry.SentAt)
	if entry.Delay > 0 {
		entry.TP = float64(bytes) / entry.Delay.Seconds()
	}

	delete(sc.pending, id)
	sc.completeTile(entry.Segment, bytes, recvAt)

	return entry.Delay, entry.TP
}

// RecordFailure marca uma requisição enviada como concluída sem bytes.
func (sc *StatsCollector) RecordFailure(id uuid.UUID) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	entry, ok := sc.pending[id]
	if !ok {
		return
	}
	delete(sc.pending, id)
	sc.completeTile(entry.Segment, 0, time.Now())
}

// RecordSkipped marca um tile que não chegou a ser enviado.
func (sc *StatsCollector) RecordSkipped(segmentID int) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	sc.completeTile(segmentID, 0, time.Now())
}

// AvgThroughput devolve a estimativa EWMA da vazão agregada por segmento, em
// bytes por segundo.
func (sc *StatsCollector) AvgThroughput() float64 {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	return sc.ewma
}

// Pending retorna quantas requisições ainda não receberam resposta.
// Útil para depuração do cliente.
func (sc *StatsCollector) Pending() int {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return len(sc.pending)
}

func (sc *StatsCollector) segment(segmentID int) *segmentEntry {
	seg, ok := sc.segments[segmentID]
	if !ok {
		seg = &segmentEntry{}
		sc.segments[segmentID] = seg
	}
	return seg
}

func (sc *StatsCollector) completeTile(segmentID int, bytes int, doneAt time.Time) {
	seg := sc.segment(segmentID)
	seg.completed++
	if bytes > 0 {
		seg.totalBytes += bytes
	}
	if seg.lastDoneAt.IsZero() || doneAt.After(seg.lastDoneAt) {
		seg.lastDoneAt = doneAt
	}
	sc.finalizeSegmentIfComplete(seg)
}

func (sc *StatsCollector) finalizeSegmentIfComplete(seg *segmentEntry) {
	if seg.finalized || seg.expected <= 0 || seg.completed < seg.expected {
		return
	}
	seg.finalized = true
	elapsed := seg.lastDoneAt.Sub(seg.firstSentAt)
	if seg.totalBytes <= 0 || elapsed <= 0 {
		return
	}
	throughput := float64(seg.totalBytes) / elapsed.Seconds()
	if !sc.hasEWMA {
		sc.ewma = throughput
		sc.hasEWMA = true
		return
	}
	sc.ewma = ewmaAlpha*throughput + (1.0-ewmaAlpha)*sc.ewma
}
