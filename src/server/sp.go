package server

type SP[T any] struct {
	queue []T
}

func (sp *SP[T]) Enqueue(data T) (err error) {
	return
}

func (sp *SP[T]) Dequeue() (data T, err error) {
	return
}
