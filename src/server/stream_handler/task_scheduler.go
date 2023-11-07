package stream_handler

import (
	"log"
	"main/src/model"
	"main/src/server/datastructures"
	"main/src/server/stream_handler/scheduler"
	"sync"
)

const maxTasksPerPriorityLevel int = 100

type QueuePolicy string

const (
	FifoQueue           QueuePolicy = "fifo"
	StrictPriorityQueue QueuePolicy = "sp"
	WeightedFairQueue   QueuePolicy = "wfq"
)

type TaskScheduler struct {
	groups    [model.PRIORITY_LEVEL_COUNT]priorityGroup
	scheduler scheduler.Scheduler[int]

	mutex     *sync.Mutex
	cond      *sync.Cond
	isStopped bool
}

type priorityGroup struct {
	entry    scheduler.SchedulerEntry[int]
	tasks    datastructures.CircularQueue[func()]
	priority float32
}

func getPriority(priorityGroup model.Priority) float32 {
	switch priorityGroup {
	case model.HIGH_PRIORITY:
		return 10.0
	case model.LOW_PRIORITY:
		return 1.0
	default:
		panic("invalid priority group")
	}
}

func NewTaskScheduler(policy QueuePolicy) *TaskScheduler {
	var sc scheduler.Scheduler[int]
	switch policy {
	case FifoQueue:
		sc = scheduler.NewFIFO[int](model.PRIORITY_LEVEL_COUNT)
	case StrictPriorityQueue:
		sc = scheduler.NewSP[int](model.PRIORITY_LEVEL_COUNT)
	case WeightedFairQueue:
		sc = scheduler.NewWFQ[int](model.PRIORITY_LEVEL_COUNT)
	default:
		panic("invalid queue policy")
	}

	groups := [model.PRIORITY_LEVEL_COUNT]priorityGroup{}
	for i := 0; i < model.PRIORITY_LEVEL_COUNT; i++ {
		groups[i] = priorityGroup{
			entry:    sc.CreateEntry(i),
			tasks:    datastructures.NewCircularQueue[func()](maxTasksPerPriorityLevel),
			priority: getPriority(model.Priority(i)),
		}
	}

	mutex := &sync.Mutex{}
	cond := sync.NewCond(mutex)

	return &TaskScheduler{
		scheduler: sc,

		mutex:     mutex,
		cond:      cond,
		isStopped: false,
	}
}

func (ps *TaskScheduler) Stop() {
	ps.mutex.Lock()

	ps.isStopped = true

	ps.mutex.Unlock()
	ps.cond.Broadcast()

}

func (ps *TaskScheduler) Enqueue(priorityGroupId int, task func()) bool {
	ps.mutex.Lock()

	if priorityGroupId < 0 || priorityGroupId > model.PRIORITY_LEVEL_COUNT-1 {
		log.Printf("Invalid priority group %d. Must be between 0 and %d.\n",
			priorityGroupId, model.PRIORITY_LEVEL_COUNT-1)
		return false
	}

	group := &ps.groups[priorityGroupId]

	wasEmpty := group.tasks.IsEmpty()
	ok := group.tasks.Enqueue(task)
	if ok && wasEmpty {
		group.entry.SetPriority(group.priority)
		group.entry.Enqueue()
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

		if task, ok := group.tasks.Dequeue(); ok {
			if !group.tasks.IsEmpty() {
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
