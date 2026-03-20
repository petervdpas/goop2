package util

import "time"

const (
	DefaultFetchTimeout   = 5 * time.Second
	DefaultConnectTimeout = 3 * time.Second
	ShortTimeout          = 2 * time.Second
	PollInterval          = 100 * time.Millisecond
)
