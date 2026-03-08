package routes

import "time"

const (
	ServiceCheckTimeout = 3 * time.Second
	BridgeCheckTimeout  = 10 * time.Second
	ProxyTimeout        = 5 * time.Second
	EncryptionTimeout   = 3 * time.Second
	CallWriteDeadline   = 5 * time.Second
	ListenPollInterval  = 500 * time.Millisecond
)
