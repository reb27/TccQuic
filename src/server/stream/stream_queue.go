package stream

import (
	"main/src/server/scheduler"
	"sync"

	"github.com/lucas-clemente/quic-go"
)

type QueuePolicy string

const (
	FifoQueue           QueuePolicy = "fifo"
	StrictPriorityQueue QueuePolicy = "sp"
	WeightedFairQueue   QueuePolicy = "wfq"
)

type StreamQueue struct {
	mutex    *sync.Mutex
	isClosed bool

	scheduler   scheduler.Scheduler[*Stream]
	dequeueSync *sync.Cond
}

func NewStreamQueue(queuePolicy QueuePolicy, maxStreams int) *StreamQueue {
	var s scheduler.Scheduler[*Stream]
	switch queuePolicy {
	case FifoQueue:
		s = scheduler.NewFIFO[*Stream](maxStreams)
	case StrictPriorityQueue:
		s = scheduler.NewSP[*Stream](maxStreams)
	case WeightedFairQueue:
		s = scheduler.NewWFQ[*Stream](maxStreams)
	default:
		panic("Invalid queue policy")
	}

	mutex := &sync.Mutex{}
	dequeueSync := sync.NewCond(mutex)

	return &StreamQueue{
		mutex:       mutex,
		isClosed:    false,
		scheduler:   s,
		dequeueSync: dequeueSync,
	}
}

func (q *StreamQueue) Add(s quic.Stream, priority float32) {
	q.mutex.Lock()

	entry := q.scheduler.CreateEntry(newStream(s, priority))
	entry.SetPriority(priority)
	entry.Enqueue()

	q.mutex.Unlock()
	q.dequeueSync.Broadcast()
}

func (q *StreamQueue) Close() {
	q.mutex.Lock()

	q.isClosed = true

	q.mutex.Unlock()
	q.dequeueSync.Broadcast()
}

func (q *StreamQueue) Run(callback func(stream *Stream)) {
	for {
		entry := q.dequeue()
		if entry == nil {
			return
		}

		stream := entry.UserData()
		callback(stream)

		if !stream.isClosed {
			q.mutex.Lock()
			entry.SetPriority(stream.Priority)
			entry.Enqueue()
			q.mutex.Unlock()
		}
	}
}

func (q *StreamQueue) dequeue() (entry scheduler.SchedulerEntry[*Stream]) {
	q.mutex.Lock()
	for !q.isClosed {
		entry = q.scheduler.Dequeue()
		if entry != nil {
			break
		}
		q.dequeueSync.Wait()
	}
	q.mutex.Unlock()
	return
}
