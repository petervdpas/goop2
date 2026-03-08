package listen

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/petervdpas/goop2/internal/group"
)

func (m *Manager) saveQueueToDisk() {
	if m.store == nil {
		return
	}
	groupID := ""
	if m.group != nil {
		groupID = m.group.ID
	}
	_ = m.store.Save("listen-queue", &queueState{
		GroupID: groupID,
		Paths:   m.queue,
		Index:   m.queueIdx,
	})
}

func (m *Manager) loadQueueFromDisk() *queueState {
	if m.store == nil {
		return nil
	}
	var qs queueState
	if !m.store.Load("listen-queue", &qs) {
		return nil
	}
	return &qs
}

func newStateStore(dataDir string) *group.StateStore {
	return group.NewStateStore(dataDir)
}

func generateListenID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "listen-" + hex.EncodeToString(b)
}
