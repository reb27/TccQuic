// Package netstats implementa um módulo independente para medir
// atraso (latência) e vazão (throughput) das transferências de tiles.
// Ele não conhece detalhes de QUIC, buffer ou ABR: apenas registra quando
// cada requisição saiu e quando a resposta chegou.
// Se outros módulos mudarem, desde que ainda chamem RecordSend/RecordRecv
// com o mesmo ID, este código continua funcionando.
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
}

// StatsCollector agrega várias StatEntry e mantém uma janela circular
// com as últimas "window" medições de vazão para calcular a média.
// zero dependencies externas além da biblioteca padrão.
type StatsCollector struct {
	mu      sync.Mutex
	pending map[uuid.UUID]*StatEntry // requisições pendentes (ainda sem resposta)
	window  []float64                // vazões recentes
	idx     int                      // ponteiro de inserção (cresce para sempre)
}

// New cria um coletor com janela "window" (>=1) medições.
func New(window int) *StatsCollector {
	if window < 1 {
		window = 1
	}
	return &StatsCollector{
		pending: make(map[uuid.UUID]*StatEntry),
		window:  make([]float64, window),
	}
}

// RecordSend deve ser chamado assim que a requisição ID sair.
func (sc *StatsCollector) RecordSend(id uuid.UUID) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.pending[id] = &StatEntry{SentAt: time.Now()}
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

	entry.RecvAt = time.Now()
	entry.Bytes = bytes
	entry.Delay = entry.RecvAt.Sub(entry.SentAt)
	if entry.Delay > 0 {
		entry.TP = float64(bytes) / entry.Delay.Seconds()
	}

	// grava vazão na janela circular
	sc.window[sc.idx%len(sc.window)] = entry.TP
	sc.idx++

	// remove do mapa para não crescer sem limite
	delete(sc.pending, id)

	return entry.Delay, entry.TP
}

// AvgThroughput devolve a média simples das vazões na janela.
func (sc *StatsCollector) AvgThroughput() float64 {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	sum := 0.0
	for _, v := range sc.window {
		sum += v
	}
	return sum / float64(len(sc.window))
}

// Pending retorna quantas requisições ainda não receberam resposta.
// Útil para depuração do cliente.
func (sc *StatsCollector) Pending() int {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return len(sc.pending)
}
