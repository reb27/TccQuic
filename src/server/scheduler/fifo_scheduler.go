package scheduler

import "main/src/server/scheduler/datastructures"

type fifoScheduler[T any] struct {
	queue datastructures.CircularQueue[*fifoEntry[T]]
}

type fifoEntry[T any] struct {
	scheduler *fifoScheduler[T]
	enqueued  bool
	userdata  T
}

// Creates a new FIFO scheduler.
//
// The FIFO scheduler ignores the priority and serves everything in order of
// arrival.
func NewFIFO[T any](capacity int) Scheduler[T] {
	return &fifoScheduler[T]{
		queue: datastructures.NewCircularQueue[*fifoEntry[T]](capacity),
	}
}

func (s *fifoScheduler[T]) CreateEntry(userdata T) SchedulerEntry[T] {
	return &fifoEntry[T]{
		scheduler: s,
		userdata:  userdata,
	}
}

func (s *fifoScheduler[T]) Dequeue() SchedulerEntry[T] {
	val, ok := s.queue.Dequeue()
	if !ok {
		return nil
	}

	val.enqueued = false
	return val
}

func (e *fifoEntry[T]) Enqueue() bool {
	if !e.enqueued && e.scheduler.queue.Enqueue(e) {
		e.enqueued = true
		return true
	} else {
		return false
	}
}

func (e *fifoEntry[T]) SetPriority(priority float32) {}

func (e *fifoEntry[T]) UserData() T {
	return e.userdata
}
