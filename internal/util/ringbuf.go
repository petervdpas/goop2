package util

import "sync"

// RingBuffer is a fixed-capacity circular buffer. When full, Push overwrites
// the oldest element. All methods are safe for concurrent use.
type RingBuffer[T any] struct {
	mu    sync.RWMutex
	buf   []T
	head  int
	count int
}

// NewRingBuffer creates a ring buffer with the given capacity.
func NewRingBuffer[T any](capacity int) *RingBuffer[T] {
	return &RingBuffer[T]{buf: make([]T, capacity)}
}

// Push appends an item, overwriting the oldest if full.
func (r *RingBuffer[T]) Push(item T) {
	r.mu.Lock()
	idx := (r.head + r.count) % len(r.buf)
	r.buf[idx] = item
	if r.count == len(r.buf) {
		r.head = (r.head + 1) % len(r.buf)
	} else {
		r.count++
	}
	r.mu.Unlock()
}

// Snapshot returns a copy of all elements in order (oldest first).
func (r *RingBuffer[T]) Snapshot() []T {
	r.mu.RLock()
	out := make([]T, r.count)
	for i := 0; i < r.count; i++ {
		out[i] = r.buf[(r.head+i)%len(r.buf)]
	}
	r.mu.RUnlock()
	return out
}

// Len returns the number of elements stored.
func (r *RingBuffer[T]) Len() int {
	r.mu.RLock()
	n := r.count
	r.mu.RUnlock()
	return n
}
