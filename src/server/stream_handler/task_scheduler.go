package stream_handler

import (
	"log"
	"math"
	"os"
	"strconv"
	"sync"
	"time"

	"main/src/model"
	"main/src/server/metrics"
	"main/src/server/stream_handler/scheduler"
)

// WFQBytesRecorder é implementada pelo Scheduler quando policy == PolicyWFQ.
// O stream_handler usa type-assertion para reportar bytes enviados por classe.
type WFQBytesRecorder interface {
	RecordWFQBytes(prio model.Priority, n int)
}

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

	// filas por prioridade — indexadas por model.Priority
	// (HIGH=0, MED=1, LOW=2, ordem do iota) — usadas por FIFO, SP, VoI_SP
	queues [int(model.PRIORITY_LEVEL_COUNT)][]task

	// WFQ (virtual finish time) — usado apenas quando policy == PolicyWFQ.
	// O wfqScheduler exige UMA entry persistente por fluxo (o virtual finish
	// acumula por entry); usamos 1 entry por CLASSE + uma fila FIFO por classe.
	wfqSched   scheduler.Scheduler[int]                                 // userdata = int(model.Priority)
	wfqEntries [int(model.PRIORITY_LEVEL_COUNT)]scheduler.SchedulerEntry[int]
	wfqQueues  [int(model.PRIORITY_LEVEL_COUNT)][]task                  // FIFO interna por classe
	wfqCount   int                                                      // total enfileirado no WFQ

	// Pesos dinâmicos WFQ (Algoritmo 1 do paper)
	// Todos os arrays abaixo são indexados por model.Priority (HIGH=0, MED=1, LOW=2).
	wfqBytes        [3]int64   // bytes acumulados na rodada atual
	wfqPhi          [3]float64 // share normalizado φ atual por classe (estado EMA)
	wfqRoundDequeues int       // quantidade de dequeues na rodada atual

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

	// Inicializar WFQ com pesos dinâmicos
	if policy == PolicyWFQ {
		s.wfqSched = scheduler.NewWFQ[int](16) // 1 entry persistente por classe
		// φ_base_p = W_p / W_total — indexado por model.Priority (HIGH=0).
		// FoV(HIGH)=3/6, vizinhança(MED)=2/6, fundo(LOW)=1/6.
		s.wfqPhi[int(model.HIGH_PRIORITY)] = 3.0 / 6.0
		s.wfqPhi[int(model.MEDIUM_PRIORITY)] = 2.0 / 6.0
		s.wfqPhi[int(model.LOW_PRIORITY)] = 1.0 / 6.0
		for p := 0; p < int(model.PRIORITY_LEVEL_COUNT); p++ {
			e := s.wfqSched.CreateEntry(p)
			e.SetPriority(float32(s.wfqPhi[p] * 6.0))
			s.wfqEntries[p] = e
		}
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
		// FIFO interna da classe + garante a entry persistente da classe no WFQ.
		// (Entry já enfileirada → Enqueue é no-op; o peso dinâmico é atualizado
		// pelo recalcWFQWeights via SetPriority.)
		s.wfqQueues[int(p)] = append(s.wfqQueues[int(p)], task{
			prio:     p,
			req:      req,
			fn:       fn,
			enqueued: time.Now(),
		})
		s.wfqEntries[int(p)].Enqueue()
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

// QueueLenPerClass expõe comprimentos das filas por classe (para sampling).
// Cobre tanto as filas FIFO/SP quanto as filas internas do WFQ.
func (s *Scheduler) QueueLenPerClass() map[model.Priority]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := make(map[model.Priority]int, int(model.PRIORITY_LEVEL_COUNT))
	for i := 0; i < int(model.PRIORITY_LEVEL_COUNT); i++ {
		m[model.Priority(i)] = len(s.queues[i]) + len(s.wfqQueues[i])
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
// HIGH_PRIORITY=0, MEDIUM_PRIORITY=1, LOW_PRIORITY=2 (iota order).
func (s *Scheduler) pickSP() task {
	for q := int(model.HIGH_PRIORITY); q <= int(model.LOW_PRIORITY); q++ {
		if len(s.queues[q]) > 0 {
			t := s.queues[q][0]
			s.queues[q] = s.queues[q][1:]
			return t
		}
	}
	return task{}
}

// WFQ (weighted fair queuing): recalcula pesos dinâmicos por rodada.
// Uma rodada fecha a cada 3 dequeues (P=3 classes).
func (s *Scheduler) pickWFQ() task {
	entry := s.wfqSched.Dequeue()
	if entry == nil {
		return task{}
	}
	p := entry.UserData() // classe (int(model.Priority))
	if len(s.wfqQueues[p]) == 0 {
		// invariante quebrada (não deveria ocorrer): entry sem tarefa pendente
		log.Printf("[SCHED] WFQ entry da classe %d sem tarefas", p)
		return task{}
	}
	t := s.wfqQueues[p][0]
	s.wfqQueues[p] = s.wfqQueues[p][1:]
	s.wfqCount--
	// classe ainda tem backlog → mantém a entry na disputa (virtual finish acumula)
	if len(s.wfqQueues[p]) > 0 {
		entry.Enqueue()
	}

	s.wfqRoundDequeues++
	const wfqRoundSize = 3
	if s.wfqRoundDequeues >= wfqRoundSize {
		s.recalcWFQWeights()
		s.wfqBytes = [3]int64{}
		s.wfqRoundDequeues = 0
	}

	return t
}

// wfqBeta lê WFQ_BETA do ambiente (padrão 1.0; valores válidos: 0.5, 1.0, 2.0).
func wfqBeta() float64 {
	if v := os.Getenv("WFQ_BETA"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			return f
		}
	}
	return 1.0
}

// recalcWFQWeights implementa o Algoritmo 1 do paper para P=3 classes.
// Roda dentro do lock de nextTaskBlocking (via pickWFQ), portanto acessa
// wfqBytes e wfqPhi sem necessidade de lock adicional.
func (s *Scheduler) recalcWFQWeights() {
	const (
		K      = 1.0  // ganho global
		alpha  = 0.3  // taxa EMA
		epsMin = 0.05 // share mínimo por fila
		epsMax = 0.05 // headroom máximo
		Wtotal = 6.0  // 1+2+3 — escala dos pesos de saída
	)
	beta := wfqBeta()
	// φ_base_p = W_p_inicial / W_total (âncora fixa, não muda).
	// Indexado por model.Priority (HIGH=0): FoV=3/6, vizinhança=2/6, fundo=1/6.
	var phiBase [3]float64
	phiBase[int(model.HIGH_PRIORITY)] = 3.0 / Wtotal
	phiBase[int(model.MEDIUM_PRIORITY)] = 2.0 / Wtotal
	phiBase[int(model.LOW_PRIORITY)] = 1.0 / Wtotal

// Passo 1: total de bytes acumulados na rodada
	var total int64
	for _, b := range s.wfqBytes {
		total += b
	}
	if total == 0 {
		return // sem tráfego na rodada, manter pesos atuais
	}

	// Passos 2–4: para cada classe p ∈ {LOW, MED, HIGH}
	phi := s.wfqPhi
	for p := 0; p < 3; p++ {
		// Passo 1: workload share medido
		xp := float64(s.wfqBytes[p]) / float64(total)
		// Passo 2: candidato (fórmula do paper)
		phiCand := K * math.Pow(xp, beta) * phiBase[p]
		// Passo 3: clamp — nenhuma fila some ou domina
		if phiCand < epsMin {
			phiCand = epsMin
		}
		if phiCand > 1.0-epsMax {
			phiCand = 1.0 - epsMax
		}
		// Passo 4: suavização EMA
		phi[p] = (1.0-alpha)*phi[p] + alpha*phiCand
	}

	// Passo 5: normalizar para que sum(φ_p) == 1
	var phiSum float64
	for _, v := range phi {
		phiSum += v
	}
	for p := range phi {
		phi[p] /= phiSum
	}

	// Passo 6: converter φ → peso WFQ e publicar.
	// phi é indexado por model.Priority (HIGH=0, MED=1, LOW=2).
	adapted := phi != s.wfqPhi
	s.wfqPhi = phi

	// Atualiza o peso das entries persistentes (vale para os próximos dequeues)
	for p := 0; p < int(model.PRIORITY_LEVEL_COUNT); p++ {
		s.wfqEntries[p].SetPriority(float32(phi[p] * Wtotal))
	}

	newWeights := map[int]float64{
		int(model.LOW_PRIORITY):    phi[int(model.LOW_PRIORITY)] * Wtotal,
		int(model.MEDIUM_PRIORITY): phi[int(model.MEDIUM_PRIORITY)] * Wtotal,
		int(model.HIGH_PRIORITY):   phi[int(model.HIGH_PRIORITY)] * Wtotal,
	}
	metrics.SetWFQWeights(newWeights)
	metrics.SetWFQAdapted(adapted)

	hi, med, lo := int(model.HIGH_PRIORITY), int(model.MEDIUM_PRIORITY), int(model.LOW_PRIORITY)
	log.Printf("[WFQ-DYN] x(H/M/L)=%.2f/%.2f/%.2f φ(H/M/L)=%.3f/%.3f/%.3f W(H/M/L)=%.2f/%.2f/%.2f adapted=%v",
		float64(s.wfqBytes[hi])/float64(total),
		float64(s.wfqBytes[med])/float64(total),
		float64(s.wfqBytes[lo])/float64(total),
		phi[hi], phi[med], phi[lo],
		phi[hi]*Wtotal, phi[med]*Wtotal, phi[lo]*Wtotal,
		adapted)
}

// RecordWFQBytes acumula bytes enviados por classe para a rodada atual.
// Implementa a interface WFQBytesRecorder.
func (s *Scheduler) RecordWFQBytes(prio model.Priority, n int) {
	if s.policy != PolicyWFQ {
		return
	}
	s.mu.Lock()
	s.wfqBytes[int(prio)] += int64(n)
	s.mu.Unlock()
}

// ----------------------------- Utilidades ---------------------------------

// (Opcional) Se em algum momento você adicionar preempção real ao SP, chame isto:
func (s *Scheduler) notifyPreemption(preempted, preemptor model.Priority) {
	metrics.M().OnPreempt(preempted, preemptor)
}
