package scheduler

import "main/src/server/datastructures"

type wfqScheduler[T any] struct {
	queue             datastructures.PriorityQueue[float32, *wfqEntry[T]]
	lastVirtualFinish float32
}

type wfqEntry[T any] struct {
	scheduler         *wfqScheduler[T]
	inverseWeight     float32
	enqueued          bool
	lastVirtualFinish float32
	userdata          T
}

// Creates a new weighted fair queuing scheduler.
//
// The weighted fair queuing scheduler tries to serve entries proportionally
// to their priorities.
//
// For example, an entry of priority 2 should be served twice as much as an
// entry of priority 1.
func NewWFQ[T any](capacity int) Scheduler[T] {
	return &wfqScheduler[T]{
		queue: datastructures.NewPriorityQueue[float32, *wfqEntry[T]](
			capacity),
	}
}

func (s *wfqScheduler[T]) CreateEntry(userdata T) SchedulerEntry[T] {
	return &wfqEntry[T]{
		scheduler:         s,
		inverseWeight:     0.0,
		enqueued:          false,
		lastVirtualFinish: s.lastVirtualFinish,
		userdata:          userdata,
	}
}

func (s *wfqScheduler[T]) Dequeue() SchedulerEntry[T] {
	e, ok := s.queue.Dequeue()
	if !ok {
		return nil
	}

	e.lastVirtualFinish += e.inverseWeight
	s.lastVirtualFinish = e.lastVirtualFinish
	e.enqueued = false

	return e
}

func (e *wfqEntry[T]) Enqueue() bool {
	// Smallest lastVirtualFinish first
	if !e.enqueued && e.scheduler.queue.Enqueue(e, -e.lastVirtualFinish) {
		e.enqueued = true
		return true
	} else {
		return false
	}
}

func (e *wfqEntry[T]) SetPriority(priority float32) {
	e.inverseWeight = 1.0 / priority
}

func (e *wfqEntry[T]) UserData() T {
	return e.userdata
}
