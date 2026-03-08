package group

import "time"

const (
	PingInterval       = 60 * time.Second
	SendTimeout        = 5 * time.Second
	BroadcastTimeout   = 5 * time.Second
	JoinTimeout        = 10 * time.Second
	ReconnectTimeout   = 8 * time.Second
	DiscoveryWait      = 6 * time.Second
	ClusterSendTimeout = 3 * time.Second
)
