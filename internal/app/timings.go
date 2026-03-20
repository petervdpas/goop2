package app

import "time"

const (
	ProgressEmitDelay     = time.Second              // delay between startup progress steps
	TCPDialAttemptTimeout = 200 * time.Millisecond   // per-attempt TCP dial in WaitTCP
)
