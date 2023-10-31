package datastructures

import (
	"container/heap"

	"golang.org/x/exp/constraints"
)

type heapImpl[K constraints.Ordered, T any] []heapItem[K, T]

type heapItem[K constraints.Ordered, T any] struct {
	value    T
	priority K
}

func (q heapImpl[K, T]) Len() int { return len(q) }

func (q heapImpl[K, T]) Less(i, j int) bool {
	return q[i].priority > q[j].priority
}

func (q heapImpl[K, T]) Swap(i, j int) {
	q[i], q[j] = q[j], q[i]
}

func (q *heapImpl[K, T]) Push(x any) {
	item := x.(heapItem[K, T])
	*q = append(*q, item)
}

func (q *heapImpl[K, T]) Pop() any {
	old := *q
	n := len(old)
	item := old[n-1]
	old[n-1] = heapItem[K, T]{} // avoid memory leak
	*q = old[0 : n-1]
	return item
}

type PriorityQueue[K constraints.Ordered, T any] struct {
	heap heapImpl[K, T]
}

func NewPriorityQueue[K constraints.Ordered, T any](
	capacity int,
) PriorityQueue[K, T] {
	return PriorityQueue[K, T]{
		heap: make(heapImpl[K, T], 0, capacity),
	}
}

func (q *PriorityQueue[K, T]) Enqueue(value T, priority K) bool {
	if len(q.heap) == cap(q.heap) {
		return false
	}

	heap.Push(&q.heap, heapItem[K, T]{
		value:    value,
		priority: priority,
	})
	return true
}

func (q *PriorityQueue[K, T]) Dequeue() (val T, ok bool) {
	if len(q.heap) == 0 {
		ok = false
		return
	}

	item := heap.Pop(&q.heap).(heapItem[K, T])
	val = item.value
	ok = true
	return
}
