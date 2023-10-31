package scheduler

// A scheduler for the type T.
//
// This interface not thread-safe. Use a mutex if necessary.
type Scheduler[T any] interface {
	// Create an entry for this scheduler.
	CreateEntry(userdata T) SchedulerEntry[T]

	// Dequeue the next entry according to the scheduler policy.
	//
	// Returns nil if there's no enqueued entries.
	Dequeue() SchedulerEntry[T]
}

// The entry for a scheduler.
//
// The entry keeps track of the necessary data a scheduler associates with a
// given value.
type SchedulerEntry[T any] interface {
	// Enqueue this entry.
	//
	// Returns true on sucess, false if the scheduler is full or this entry is
	// already enqueued.
	Enqueue() bool

	// Set the priority for this entry. What this does depends on the scheduler
	// policy.
	SetPriority(priority float32)

	// Returns the user data for this entry.
	UserData() T
}
