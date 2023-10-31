package scheduler_test

import (
	"main/src/server/task/scheduler"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test if WFQ is respecting the priorities.
func TestWFQScheduler_Weights(t *testing.T) {
	// Use the value to store the amount of dequeues
	s := scheduler.NewWFQ[*int](4)

	e1 := s.CreateEntry(new(int))
	e1.SetPriority(1)

	e2 := s.CreateEntry(new(int))
	e2.SetPriority(2)

	e3 := s.CreateEntry(new(int))
	e3.SetPriority(3)

	e10 := s.CreateEntry(new(int))
	e10.SetPriority(10)

	assert.True(t, e1.Enqueue())
	assert.True(t, e2.Enqueue())
	assert.True(t, e3.Enqueue())
	assert.True(t, e10.Enqueue())

	// 16 dequeues:
	// 1x e1
	// 2x e2
	// 3x e3
	// 10x e10

	for i := 0; i < 16; i++ {
		x := s.Dequeue()
		t.Logf("%v", x)
		assert.NotNil(t, x)
		*x.UserData()++
		assert.True(t, x.Enqueue())
	}

	assert.Equal(t, 1, *e1.UserData())
	assert.Equal(t, 2, *e2.UserData())
	assert.Equal(t, 3, *e3.UserData())
	assert.Equal(t, 10, *e10.UserData())
}

// Test if WFQ does not allow new entries to be added if it is full.
func TestWFQScheduler_Capacity(t *testing.T) {
	s := scheduler.NewWFQ[int](1)

	e20 := s.CreateEntry(1)
	e20.SetPriority(20)

	e10 := s.CreateEntry(2)
	e10.SetPriority(10)

	assert.True(t, e20.Enqueue())
	assert.False(t, e10.Enqueue())
}
