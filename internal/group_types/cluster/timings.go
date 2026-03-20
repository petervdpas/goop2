package cluster

import "time"

const (
	DispatchTick       = 100 * time.Millisecond // job dispatch loop interval
	WorkerCheckTimeout = 5 * time.Second        // binary verification check-job timeout
)
