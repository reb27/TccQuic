package stream_handler

import (
	"log"
	"main/src/model"
	"main/src/server/datastructures"
	"main/src/server/stream_handler/scheduler"
	"sync"
)

const maxTasksPerPriorityLevel int = 1000

type QueuePolicy string

const (
	FifoQueue           QueuePolicy = "fifo"
	StrictPriorityQueue QueuePolicy = "sp"
	WeightedFairQueue   QueuePolicy = "wfq"
)

type TaskScheduler interface {
	Stop()
	Enqueue(priority model.Priority, task func()) bool
	Run()
}

func NewTaskScheduler(policy QueuePolicy) TaskScheduler {
	if policy == FifoQueue {
		return newFifoTaskScheduler()
	} else {
		return newPriorityTaskScheduler(policy)
	}
}

// FIFO Task Scheduler

type fifoTaskScheduler struct {
	scheduler scheduler.Scheduler[func()]

	mutex     *sync.Mutex
	cond      *sync.Cond
	isStopped bool
}

func newFifoTaskScheduler() *fifoTaskScheduler {
	sc := scheduler.NewFIFO[func()](maxTasksPerPriorityLevel * model.PRIORITY_LEVEL_COUNT)

	mutex := &sync.Mutex{}
	cond := sync.NewCond(mutex)

	return &fifoTaskScheduler{
		scheduler: sc,

		mutex:     mutex,
		cond:      cond,
		isStopped: false,
	}
}

func (fs *fifoTaskScheduler) Stop() {
	fs.mutex.Lock()

	fs.isStopped = true

	fs.mutex.Unlock()
	fs.cond.Broadcast()

}

func (fs *fifoTaskScheduler) Enqueue(priority model.Priority, task func()) bool {
	fs.mutex.Lock()

	entry := fs.scheduler.CreateEntry(task)
	ok := entry != nil
	if ok {
		ok = entry.Enqueue()
	}

	fs.mutex.Unlock()
	fs.cond.Broadcast()

	return ok
}

func (fs *fifoTaskScheduler) Run() {
	fs.mutex.Lock()

	for !fs.isStopped {
		entry := fs.scheduler.Dequeue()
		if entry == nil {
			fs.cond.Wait()
			continue
		}

		task := entry.UserData()

		// Execute task outside mutex
		fs.mutex.Unlock()
		task()
		fs.mutex.Lock()
	}

	fs.mutex.Unlock()
}

// Priority Task Scheduler

type priorityTaskScheduler struct {
	groups    []*priorityGroup
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

func newPriorityTaskScheduler(policy QueuePolicy) *priorityTaskScheduler {
	var sc scheduler.Scheduler[int]
	switch policy {
	case StrictPriorityQueue:
		sc = scheduler.NewSP[int](model.PRIORITY_LEVEL_COUNT)
	case WeightedFairQueue:
		sc = scheduler.NewWFQ[int](model.PRIORITY_LEVEL_COUNT)
	default:
		panic("invalid queue policy")
	}

	groups := make([]*priorityGroup, model.PRIORITY_LEVEL_COUNT)
	for i := 0; i < model.PRIORITY_LEVEL_COUNT; i++ {
		groups[i] = &priorityGroup{
			entry:    sc.CreateEntry(i),
			tasks:    datastructures.NewCircularQueue[func()](maxTasksPerPriorityLevel),
			priority: getPriority(model.Priority(i)),
		}
	}

	mutex := &sync.Mutex{}
	cond := sync.NewCond(mutex)

	return &priorityTaskScheduler{
		groups:    groups,
		scheduler: sc,

		mutex:     mutex,
		cond:      cond,
		isStopped: false,
	}
}

func (ps *priorityTaskScheduler) Stop() {
	ps.mutex.Lock()

	ps.isStopped = true

	ps.mutex.Unlock()
	ps.cond.Broadcast()

}

func (ps *priorityTaskScheduler) Enqueue(priority model.Priority, task func()) bool {
	priorityGroupId := int(priority)

	ps.mutex.Lock()

	if priorityGroupId < 0 || priorityGroupId > model.PRIORITY_LEVEL_COUNT-1 {
		log.Printf("Invalid priority group %d. Must be between 0 and %d.\n",
			priorityGroupId, model.PRIORITY_LEVEL_COUNT-1)
		return false
	}

	group := ps.groups[priorityGroupId]

	wasEmpty := group.tasks.IsEmpty()
	ok := group.tasks.Enqueue(task)
	if ok && wasEmpty {
		group.entry.SetPriority(group.priority)
		ok = group.entry.Enqueue()
	}

	ps.mutex.Unlock()
	ps.cond.Broadcast()

	return ok
}

func (ps *priorityTaskScheduler) Run() {
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
