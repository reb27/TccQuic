package datastructures

// A bounded circular queue.
type CircularQueue[T any] struct {
	buffer []T
	head   int
	len    int
}

// Create a new circular queue.
func NewCircularQueue[T any](capacity int) CircularQueue[T] {
	return CircularQueue[T]{
		buffer: make([]T, capacity),
	}
}

func (q *CircularQueue[T]) Enqueue(item T) bool {
	if q.len == len(q.buffer) {
		return false
	}

	tail := (q.head + q.len) % len(q.buffer)
	q.buffer[tail] = item

	q.len++

	return true
}

func (q *CircularQueue[T]) Dequeue() (val T, ok bool) {
	if q.len == 0 {
		ok = false
		return
	}

	val = q.buffer[q.head]
	ok = true

	q.head = (q.head + 1) % len(q.buffer)
	q.len--

	return
}

func (q *CircularQueue[T]) IsEmpty() bool {
	return q.len == 0
}
