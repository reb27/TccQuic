package server

type WFQ[T any] struct {
	queue []T
}

func (wfq *WFQ[T]) Enqueue(data T) (err error) {
	return
}

func (wfq *WFQ[T]) Dequeue() (data T, err error) {
	return
}
