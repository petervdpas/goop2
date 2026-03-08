package listen

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
)

func (m *Manager) queueFilePath() string {
	if m.dataDir == "" {
		return ""
	}
	return filepath.Join(m.dataDir, "listen-queue.json")
}

func (m *Manager) queueFilePathForGroup(_ string) string {
	return m.queueFilePath()
}

func (m *Manager) saveQueueToDisk() {
	p := m.queueFilePath()
	if p == "" {
		return
	}
	groupID := ""
	if m.group != nil {
		groupID = m.group.ID
	}
	qs := queueState{GroupID: groupID, Paths: m.queue, Index: m.queueIdx}
	data, err := json.Marshal(qs)
	if err != nil {
		return
	}
	_ = os.WriteFile(p, data, 0644)
}

func (m *Manager) loadQueueFromDiskForGroup(groupID string) *queueState {
	p := m.queueFilePathForGroup(groupID)
	if p == "" {
		return nil
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	var qs queueState
	if err := json.Unmarshal(data, &qs); err != nil {
		return nil
	}
	return &qs
}

func generateListenID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "listen-" + hex.EncodeToString(b)
}
