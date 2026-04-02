package group

import "github.com/petervdpas/goop2/internal/storage"

// NewTestManager creates a minimal Manager backed only by a DB,
// suitable for unit tests that don't need P2P or MQ transport.
func NewTestManager(db *storage.DB, selfID string) *Manager {
	return &Manager{
		db:           db,
		selfID:       selfID,
		groups:       make(map[string]*hostedGroup),
		activeConns:  make(map[string]*clientConn),
		pendingJoins: make(map[string]chan joinResult),
		handlers:     make(map[string]TypeHandler),
	}
}
