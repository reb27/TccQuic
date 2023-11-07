package scheduler_test

import (
	"main/src/server/stream_handler/scheduler"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Tests if FIFO dequeues in first-come-first-served order.
func TestFIFOScheduler_Order(t *testing.T) {
	s := scheduler.NewFIFO[int](3)

	e20 := s.CreateEntry(1)
	e20.SetPriority(20)

	e10 := s.CreateEntry(2)
	e10.SetPriority(10)

	e100 := s.CreateEntry(3)
	e100.SetPriority(100)

	assert.True(t, e20.Enqueue())
	assert.True(t, e10.Enqueue())
	assert.True(t, e100.Enqueue())

	assert.Equal(t, e20, s.Dequeue())
	assert.Equal(t, e10, s.Dequeue())
	assert.Equal(t, e100, s.Dequeue())

	assert.Nil(t, s.Dequeue())
}

// Test if FIFO does not allow new entries to be added if it is full.
func TestFIFOScheduler_Capacity(t *testing.T) {
	s := scheduler.NewFIFO[int](1)

	e20 := s.CreateEntry(1)
	e20.SetPriority(20)

	e10 := s.CreateEntry(2)
	e10.SetPriority(10)

	assert.True(t, e20.Enqueue())
	assert.False(t, e10.Enqueue())
}
