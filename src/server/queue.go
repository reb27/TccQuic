package server

type Queue[T any] interface {
	Enqueue(data T) (err error)
	Dequeue() (data T, err error)
}

type QueuePolicy string

const (
	WeightedFairQueue   QueuePolicy = "wfq"
	StrictPriorityQueue QueuePolicy = "sp"
)
