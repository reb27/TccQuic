package datastructures_test

import (
	"main/src/server/scheduler/datastructures"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPriorityQueue_EnqueueAndDequeue(t *testing.T) {
	q := datastructures.NewPriorityQueue[float32, int](3)

	assert.True(t, q.Enqueue(1, 20.0))
	assert.True(t, q.Enqueue(2, 10.0))
	assert.True(t, q.Enqueue(3, 100.0))
	assert.False(t, q.Enqueue(4, 200.0))

	// { 100.0: 3, 20.0: 1, 10.0: 2 }
	val, ok := q.Dequeue()
	assert.True(t, ok)
	assert.Equal(t, 3, val)

	// { 20.0: 1, 10.0: 2 }
	val, ok = q.Dequeue()
	assert.True(t, ok)
	assert.Equal(t, 1, val)

	// { 10.0: 2 }
	assert.True(t, q.Enqueue(5, 100.0))

	// { 500.0: 5, 10.0: 2 }
	val, ok = q.Dequeue()
	assert.True(t, ok)
	assert.Equal(t, 5, val)

	// { 10.0: 2 }
	val, ok = q.Dequeue()
	assert.True(t, ok)
	assert.Equal(t, 2, val)

	// { }
	_, ok = q.Dequeue()
	assert.False(t, ok)

	// { }
	assert.True(t, q.Enqueue(6, 100.0))

	// { 10.0: 6 }
	val, ok = q.Dequeue()
	assert.True(t, ok)
	assert.Equal(t, 6, val)
}
