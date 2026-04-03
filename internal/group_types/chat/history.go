package chat

const maxHistory = 100

// RingBuffer is a fixed-size circular buffer for recent messages.
type RingBuffer struct {
	msgs [maxHistory]Message
	head int
	size int
}

// Add appends a message to the buffer, overwriting the oldest if full.
func (r *RingBuffer) Add(m Message) {
	r.msgs[r.head] = m
	r.head = (r.head + 1) % maxHistory
	if r.size < maxHistory {
		r.size++
	}
}

// All returns all messages in chronological order.
func (r *RingBuffer) All() []Message {
	if r.size == 0 {
		return nil
	}
	out := make([]Message, r.size)
	start := (r.head - r.size + maxHistory) % maxHistory
	for i := 0; i < r.size; i++ {
		out[i] = r.msgs[(start+i)%maxHistory]
	}
	return out
}

// Recent returns the last n messages in chronological order.
func (r *RingBuffer) Recent(n int) []Message {
	if n <= 0 || r.size == 0 {
		return nil
	}
	if n > r.size {
		n = r.size
	}
	out := make([]Message, n)
	start := (r.head - n + maxHistory) % maxHistory
	for i := 0; i < n; i++ {
		out[i] = r.msgs[(start+i)%maxHistory]
	}
	return out
}
