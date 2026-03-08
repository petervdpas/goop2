package mq

import "time"

// MQ protocol timings — P2P message delivery.
const (
	AckTimeout    = 4 * time.Second        // transport ACK wait per send attempt
	RetryDelay    = 300 * time.Millisecond // delay between send retry
	ReadDeadline  = 6 * time.Second        // incoming stream read deadline
	WriteDeadline = 5 * time.Second        // outgoing response write deadline
)
