package mq

import "time"

const (
	AckTimeout    = 4 * time.Second
	RetryDelay    = 300 * time.Millisecond
	ReadDeadline  = 6 * time.Second
	WriteDeadline = 5 * time.Second
)
