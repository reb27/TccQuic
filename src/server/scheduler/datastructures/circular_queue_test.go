package datastructures_test

import (
	"main/src/server/scheduler/datastructures"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCircularQueue_EnqueueAndDequeue(t *testing.T) {
	q := datastructures.NewCircularQueue[int](3)

	assert.True(t, q.Enqueue(1))
	assert.True(t, q.Enqueue(2))
	assert.True(t, q.Enqueue(3))
	assert.False(t, q.Enqueue(4))

	// { 1, 2, 3 }
	val, ok := q.Dequeue()
	assert.True(t, ok)
	assert.Equal(t, 1, val)

	// { 2, 3 }
	val, ok = q.Dequeue()
	assert.True(t, ok)
	assert.Equal(t, 2, val)

	// { 3 }
	assert.True(t, q.Enqueue(5))

	// { 3, 5 }
	val, ok = q.Dequeue()
	assert.True(t, ok)
	assert.Equal(t, 3, val)

	// { 5 }
	val, ok = q.Dequeue()
	assert.True(t, ok)
	assert.Equal(t, 5, val)

	// { }
	_, ok = q.Dequeue()
	assert.False(t, ok)

	// { }
	assert.True(t, q.Enqueue(6))

	// { 6 }
	val, ok = q.Dequeue()
	assert.True(t, ok)
	assert.Equal(t, 6, val)
}
