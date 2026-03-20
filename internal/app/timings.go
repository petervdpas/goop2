package app

import "time"

const (
	ProgressEmitDelay     = 200 * time.Millisecond   // delay between startup progress steps
	TCPDialAttemptTimeout = 200 * time.Millisecond   // per-attempt TCP dial in WaitTCP
)
