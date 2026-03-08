package lua

import "time"

const (
	HTTPTimeout      = 10 * time.Second
	ShutdownTimeout  = 500 * time.Millisecond
	MemCheckInterval = 100 * time.Millisecond
)
