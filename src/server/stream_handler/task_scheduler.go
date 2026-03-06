package stream_handler

import (
	"log"
	"sync"
	"time"

	"main/src/model"
	"main/src/server/metrics"
	"main/src/server/stream_handler/scheduler"
)

// ----------------------------- Tipos públicos -----------------------------

type QueuePolicy string

const (
	PolicyFIFO   QueuePolicy = "fifo"
	PolicySP     QueuePolicy = "sp"     // strict priority (não-preemptivo)
	PolicyWFQ    QueuePolicy = "wfq"    // weighted fair queuing (virtual finish time)
	PolicyVOI_SP QueuePolicy = "voi_sp" // VoI com Strict Priority
)

// TaskScheduler é a interface usada pelo stream_handler.go
type TaskScheduler interface {
	Enqueue(p model.Priority, req *model.VideoPacketRequest, fn func()) bool
	SetThroughput(bytesPerSec float64)
	Run()
	Stop()
}

// ----------------------------- Implementação -----------------------------

type task struct {
	prio     model.Priority
	req      *model.VideoPacketRequest
	fn       func()
	enqueued time.Time
}

type Scheduler struct {
	policy QueuePolicy

	// filas por prioridade (low=0, med=1, high=2) — usadas por FIFO, SP, VoI_SP
	queues [int(model.PRIORITY_LEVEL_COUNT)][]task

	// WFQ original (virtual finish time) — usado apenas quando policy == PolicyWFQ
	wfqSched scheduler.Scheduler[task]
	wfqCount int // contagem de itens no WFQ (para totalQueuedLocked)

	// controle de execução
	mu      sync.Mutex
	cond    *sync.Cond
	stopped bool
	running bool

	// Bandwidth estimation para VoI
	throughput float64
}

// NewTaskScheduler cria um escalonador com a política desejada
func NewTaskScheduler(policy QueuePolicy) TaskScheduler {
	s := &Scheduler{
		policy: policy,
	}

	s.cond = sync.NewCond(&s.mu)

	// Inicializar WFQ original se necessário
	if policy == PolicyWFQ {
		s.wfqSched = scheduler.NewWFQ[task](10000) // capacidade alta
		metrics.SetWFQWeights(map[int]float64{
			int(model.LOW_PRIORITY):    1,
			int(model.MEDIUM_PRIORITY): 2,
			int(model.HIGH_PRIORITY):   3,
		})
	}

	// Expor pesos semânticos ao módulo de WFQ utilization (para VoI)
	if policy == PolicyVOI_SP {
		metrics.SetWFQWeights(map[int]float64{
			int(model.LOW_PRIORITY):    float64(model.WFQWeightFromSemantic(model.SemanticPiBackground)),
			int(model.MEDIUM_PRIORITY): float64(model.WFQWeightFromSemantic(model.SemanticPiNearFoV)),
			int(model.HIGH_PRIORITY):   float64(model.WFQWeightFromSemantic(model.SemanticPiFoV)),
		})
	}

	return s
}

// ----------------------------- API pública -------------------------------

func (s *Scheduler) SetThroughput(bytesPerSec float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.throughput = bytesPerSec
}

func (s *Scheduler) Enqueue(p model.Priority, req *model.VideoPacketRequest, fn func()) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return false
	}

	// Lógica de descarte baseada em VoI (se aplicável)
	if s.policy == PolicyVOI_SP {
		if req != nil && s.throughput > 0 {
			// Calcular VoI para decidir se aceita a tarefa
			tileSize := model.EstimateTileSize(req)
			// Por enquanto, calculamos o VoI usando o helper do modelo
			deadline := time.Now().Add(time.Duration(req.Timeout) * time.Millisecond)
			voi := model.CalculateVoIForRequest(req, time.Now(), deadline, tileSize, s.throughput)

			if voi <= 0 {
				log.Printf("[SCHED] DISCARDING task seg=%d tile=%d due to VoI=%.2f (throughput=%.2f)",
					req.Segment, req.Tile, voi, s.throughput)
				return true // Retornamos true para indicar que a requisição foi "processada" (descartada)
			}
		}
	}

	// enfileira
	if s.policy == PolicyWFQ {
		// Usar o WFQ original (virtual finish time)
		entry := s.wfqSched.CreateEntry(task{
			prio:     p,
			req:      req,
			fn:       fn,
			enqueued: time.Now(),
		})
		// Prioridade WFQ: LOW=1, MED=2, HIGH=3
		entry.SetPriority(float32(p) + 1)
		if !entry.Enqueue() {
			log.Println("[SCHED] WFQ queue full")
			return false
		}
		s.wfqCount++
	} else {
		s.queues[int(p)] = append(s.queues[int(p)], task{
			prio:     p,
			req:      req,
			fn:       fn,
			enqueued: time.Now(),
		})
	}

	// backlog mudou (soma de todas as filas)
	metrics.UpdateBacklog(s.totalQueuedLocked())

	// acorda a goroutine do Run
	s.cond.Signal()
	return true
}

func (s *Scheduler) Run() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	log.Printf("[SCHED] running with policy=%s", s.policy)

	for {
		// escolhe próxima tarefa (bloqueando se necessário)
		t, ok := s.nextTaskBlocking()
		if !ok {
			// parado
			break
		}

		// executa fora do lock
		t.fn()
	}
	log.Printf("[SCHED] stopped")
}

func (s *Scheduler) Stop() {
	s.mu.Lock()
	s.stopped = true
	s.mu.Unlock()
	s.cond.Broadcast()
}

// ----------------------- QueueLenProvider (opcional) ----------------------

// QueueLenPerClass expõe comprimentos das filas por classe (para sampling)
func (s *Scheduler) QueueLenPerClass() map[model.Priority]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := make(map[model.Priority]int, int(model.PRIORITY_LEVEL_COUNT))
	for i := 0; i < int(model.PRIORITY_LEVEL_COUNT); i++ {
		m[model.Priority(i)] = len(s.queues[i])
	}
	return m
}

// ----------------------------- Seleção ------------------------------------

func (s *Scheduler) nextTaskBlocking() (task, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for {
		if s.stopped {
			return task{}, false
		}

		if total := s.totalQueuedLocked(); total > 0 {
			var t task
			switch s.policy {
			case PolicyFIFO:
				t = s.pickFIFO()
			case PolicySP, PolicyVOI_SP:
				t = s.pickSP()
			case PolicyWFQ:
				t = s.pickWFQ()
			default:
				t = s.pickFIFO()
			}

			// após remover a task das filas, o backlog mudou
			metrics.UpdateBacklog(s.totalQueuedLocked())
			// vamos começar a processar => estado busy
			metrics.UpdateServiceState(true)

			return t, true
		}

		// filas vazias → servidor idle (antes de bloquear)
		metrics.UpdateServiceState(false)

		s.cond.Wait()
	}
}

// total de itens enfileirados (com lock)
func (s *Scheduler) totalQueuedLocked() int {
	n := 0
	for i := 0; i < int(model.PRIORITY_LEVEL_COUNT); i++ {
		n += len(s.queues[i])
	}
	n += s.wfqCount
	return n
}

// ----------------------------- Políticas ----------------------------------

// FIFO global: pega o mais antigo entre todas as filas.
func (s *Scheduler) pickFIFO() task {
	// encontra o task com menor enqueued
	found := false
	var bestIdx int
	var bestQ int
	var bestEnq time.Time

	for q := 0; q < int(model.PRIORITY_LEVEL_COUNT); q++ {
		if len(s.queues[q]) == 0 {
			continue
		}
		t := s.queues[q][0]
		if !found || t.enqueued.Before(bestEnq) {
			found = true
			bestEnq = t.enqueued
			bestIdx = 0
			bestQ = q
		}
	}
	t := s.queues[bestQ][bestIdx]
	// remove da fila
	s.queues[bestQ] = s.queues[bestQ][1:]
	return t
}

// SP (strict priority, não-preemptivo): sempre escolhe a maior prioridade disponível.
func (s *Scheduler) pickSP() task {
	// high → medium → low
	for q := int(model.HIGH_PRIORITY); q >= int(model.LOW_PRIORITY); q-- {
		if len(s.queues[q]) > 0 {
			t := s.queues[q][0]
			s.queues[q] = s.queues[q][1:]
			return t
		}
	}
	return task{}
}

// WFQ (weighted fair queuing): usa a implementação original com virtual finish time.
func (s *Scheduler) pickWFQ() task {
	entry := s.wfqSched.Dequeue()
	if entry == nil {
		return task{}
	}
	s.wfqCount--
	return entry.UserData()
}

// ----------------------------- Utilidades ---------------------------------

// (Opcional) Se em algum momento você adicionar preempção real ao SP, chame isto:
func (s *Scheduler) notifyPreemption(preempted, preemptor model.Priority) {
	metrics.M().OnPreempt(preempted, preemptor)
}
