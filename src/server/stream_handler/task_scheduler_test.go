package stream_handler

import (
	"sync"
	"testing"
	"time"

	"main/src/model"
)

// Regressão dos bugs de fiação do WFQ:
//  1. wfqPhi/phiBase eram indexados como se LOW=0, mas model.Priority tem
//     HIGH=0 — o FoV recebia peso 1 e o fundo peso 3 (invertido).
//  2. Cada request criava uma entry NOVA no wfqScheduler, zerando o virtual
//     finish acumulado — o WFQ degenerava em FIFO por ordem de chegada.
//
// Com backlog igual nas três classes e pesos base 3/2/1 (FoV/vizinhança/fundo),
// o WFQ deve servir HIGH com mais frequência que LOW no início do escoamento.
func TestWFQTaskSchedulerServesHighProportionallyMore(t *testing.T) {
	s := NewTaskScheduler(PolicyWFQ)

	const perClass = 20
	var mu sync.Mutex
	var order []model.Priority
	total := 3 * perClass
	done := make(chan struct{})

	record := func(p model.Priority) func() {
		return func() {
			mu.Lock()
			order = append(order, p)
			if len(order) == total {
				close(done)
			}
			mu.Unlock()
		}
	}

	// Backlog completo ANTES de rodar, para o escalonador ter o que ordenar.
	for i := 0; i < perClass; i++ {
		for _, p := range []model.Priority{model.LOW_PRIORITY, model.MEDIUM_PRIORITY, model.HIGH_PRIORITY} {
			if !s.Enqueue(p, nil, record(p)) {
				t.Fatalf("enqueue falhou para prioridade %d", p)
			}
		}
	}

	go s.Run()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout esperando o escoamento das tarefas")
	}
	s.Stop()

	// Janela inicial: com pesos 3/2/1, HIGH deve aparecer mais que LOW.
	window := order[:18]
	count := map[model.Priority]int{}
	for _, p := range window {
		count[p]++
	}
	t.Logf("primeiros %d servidos: HIGH=%d MED=%d LOW=%d",
		len(window), count[model.HIGH_PRIORITY], count[model.MEDIUM_PRIORITY], count[model.LOW_PRIORITY])

	if count[model.HIGH_PRIORITY] <= count[model.LOW_PRIORITY] {
		t.Fatalf("WFQ não priorizou FoV: HIGH=%d <= LOW=%d nos primeiros %d",
			count[model.HIGH_PRIORITY], count[model.LOW_PRIORITY], len(window))
	}
	if count[model.MEDIUM_PRIORITY] < count[model.LOW_PRIORITY] {
		t.Fatalf("WFQ não priorizou vizinhança: MED=%d < LOW=%d nos primeiros %d",
			count[model.MEDIUM_PRIORITY], count[model.LOW_PRIORITY], len(window))
	}
}

// SP deve esvaziar HIGH por completo antes de MED, e MED antes de LOW.
func TestSPTaskSchedulerStrictOrder(t *testing.T) {
	s := NewTaskScheduler(PolicySP)

	const perClass = 5
	var mu sync.Mutex
	var order []model.Priority
	total := 3 * perClass
	done := make(chan struct{})

	record := func(p model.Priority) func() {
		return func() {
			mu.Lock()
			order = append(order, p)
			if len(order) == total {
				close(done)
			}
			mu.Unlock()
		}
	}

	for i := 0; i < perClass; i++ {
		for _, p := range []model.Priority{model.LOW_PRIORITY, model.MEDIUM_PRIORITY, model.HIGH_PRIORITY} {
			if !s.Enqueue(p, nil, record(p)) {
				t.Fatalf("enqueue falhou para prioridade %d", p)
			}
		}
	}

	go s.Run()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout esperando o escoamento das tarefas")
	}
	s.Stop()

	for i, p := range order {
		want := model.HIGH_PRIORITY
		if i >= 2*perClass {
			want = model.LOW_PRIORITY
		} else if i >= perClass {
			want = model.MEDIUM_PRIORITY
		}
		if p != want {
			t.Fatalf("posição %d: esperado prioridade %d, veio %d (ordem=%v)", i, want, p, order)
		}
	}
}
