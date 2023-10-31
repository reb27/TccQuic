package task

import (
	"main/src/server/datastructures"
	"main/src/server/task/scheduler"
	"sync"
)

type QueuePolicy string

const (
	FifoQueue           QueuePolicy = "fifo"
	StrictPriorityQueue QueuePolicy = "sp"
	WeightedFairQueue   QueuePolicy = "wfq"
)

type TaskScheduler struct {
	groups    map[int]*priorityGroup
	scheduler scheduler.Scheduler[int]

	mutex     *sync.Mutex
	cond      *sync.Cond
	isStopped bool

	capacity int // immutable
}

type priorityGroup struct {
	entry    scheduler.SchedulerEntry[int]
	streams  datastructures.CircularQueue[func()]
	priority float32
}

func NewTaskScheduler(capacity int, policy QueuePolicy) *TaskScheduler {
	var sc scheduler.Scheduler[int]
	switch policy {
	case FifoQueue:
		sc = scheduler.NewFIFO[int](capacity)
	case StrictPriorityQueue:
		sc = scheduler.NewSP[int](capacity)
	case WeightedFairQueue:
		sc = scheduler.NewWFQ[int](capacity)
	default:
		panic("invalid queue policy")
	}

	mutex := &sync.Mutex{}
	cond := sync.NewCond(mutex)

	return &TaskScheduler{
		groups:    make(map[int]*priorityGroup, capacity),
		scheduler: sc,

		mutex:     mutex,
		cond:      cond,
		isStopped: false,

		capacity: capacity,
	}
}

func (ps *TaskScheduler) Stop() {
	ps.mutex.Lock()

	ps.isStopped = true

	ps.mutex.Unlock()
	ps.cond.Broadcast()

}

func (ps *TaskScheduler) Enqueue(priorityGroupId int, priority float32, task func()) bool {
	ps.mutex.Lock()

	var group *priorityGroup
	if x, exists := ps.groups[priorityGroupId]; exists {
		group = x
	} else if len(ps.groups) != ps.capacity {
		group = &priorityGroup{
			entry:    ps.scheduler.CreateEntry(priorityGroupId),
			streams:  datastructures.NewCircularQueue[func()](ps.capacity),
			priority: priority,
		}
		ps.groups[priorityGroupId] = group
	}

	ok := false
	if group != nil {
		wasEmpty := group.streams.IsEmpty()
		ok = group.streams.Enqueue(task)
		if ok && wasEmpty {
			group.entry.Enqueue()
		}
	}

	ps.mutex.Unlock()
	ps.cond.Broadcast()

	return ok
}

func (ps *TaskScheduler) Run() {
	ps.mutex.Lock()

	for !ps.isStopped {
		entry := ps.scheduler.Dequeue()
		if entry == nil {
			ps.cond.Wait()
			continue
		}

		priorityGroupId := entry.UserData()
		group := ps.groups[priorityGroupId]

		if task, ok := group.streams.Dequeue(); ok {
			if !group.streams.IsEmpty() {
				entry.Enqueue()
			}

			// Execute task outside mutex
			ps.mutex.Unlock()
			task()
			ps.mutex.Lock()
		}
	}

	ps.mutex.Unlock()
}
