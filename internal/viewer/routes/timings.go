package routes

import "time"

// Viewer HTTP route timings.
const (
	ServiceCheckTimeout = 3 * time.Second       // health check to microservices
	BridgeCheckTimeout  = 3 * time.Second       // bridge token request
	ProxyTimeout        = 3 * time.Second       // proxy/capability check to rendezvous
	EncryptionTimeout   = 3 * time.Second       // encryption service check
	CallWriteDeadline   = 5 * time.Second       // WebSocket write deadline for call streams
	ListenPollInterval  = 500 * time.Millisecond // listen stream position poll
)
