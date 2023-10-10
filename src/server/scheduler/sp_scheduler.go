package scheduler

import "main/src/server/scheduler/datastructures"

type spScheduler[T any] struct {
	queue datastructures.PriorityQueue[float32, *spEntry[T]]
}

type spEntry[T any] struct {
	scheduler *spScheduler[T]
	priority  float32
	enqueued  bool
	userdata  T
}

// Creates a new strict priority scheduler.
//
// The strict priority scheduler always yields the entry with largest priority
// first.
func NewSP[T any](capacity int) Scheduler[T] {
	return &spScheduler[T]{
		queue: datastructures.NewPriorityQueue[float32, *spEntry[T]](capacity),
	}
}

func (s *spScheduler[T]) CreateEntry(userdata T) SchedulerEntry[T] {
	return &spEntry[T]{
		scheduler: s,
		priority:  0.0,
		enqueued:  false,
		userdata:  userdata,
	}
}

func (s *spScheduler[T]) Dequeue() SchedulerEntry[T] {
	val, ok := s.queue.Dequeue()
	if !ok {
		return nil
	}

	val.enqueued = false
	return val
}

func (e *spEntry[T]) Enqueue() bool {
	if !e.enqueued && e.scheduler.queue.Enqueue(e, e.priority) {
		e.enqueued = true
		return true
	} else {
		return false
	}
}

func (e *spEntry[T]) SetPriority(priority float32) {
	e.priority = priority
}

func (e *spEntry[T]) UserData() T {
	return e.userdata
}
