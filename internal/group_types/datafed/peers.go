package datafed

import (
	"encoding/json"
	"log"
)

func (m *Manager) handlePeerGone(from string, payload any) {
	peerID := extractPeerID(payload)
	if peerID == "" || peerID == m.selfID {
		return
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	changed := false
	for groupID, fg := range m.groups {
		fg.rwmu.Lock()
		if c, ok := fg.contributions[peerID]; ok {
			if fg.suspended == nil {
				fg.suspended = make(map[string]*PeerContribution)
			}
			fg.suspended[peerID] = c
			delete(fg.contributions, peerID)
			changed = true
			log.Printf("DATA-FED: suspended %s from group %s (peer gone)", peerID, groupID)
		}
		fg.rwmu.Unlock()
	}

	if changed {
		for groupID := range m.groups {
			m.publishSync(groupID)
		}
		m.notifyChange()
	}
}

func (m *Manager) handlePeerAnnounce(from string, payload any) {
	peerID := extractPeerID(payload)
	if peerID == "" || peerID == m.selfID {
		return
	}

	if isOffline(payload) {
		return
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	changed := false
	for groupID, fg := range m.groups {
		fg.rwmu.Lock()
		if c, ok := fg.suspended[peerID]; ok {
			fg.contributions[peerID] = c
			delete(fg.suspended, peerID)
			changed = true
			log.Printf("DATA-FED: restored %s to group %s (peer back)", peerID, groupID)
		}
		fg.rwmu.Unlock()
	}

	if changed {
		for groupID := range m.groups {
			m.publishSync(groupID)
		}
		m.notifyChange()
	}
}

func extractPeerID(payload any) string {
	switch p := payload.(type) {
	case map[string]any:
		if id, ok := p["peerID"].(string); ok {
			return id
		}
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	var obj struct {
		PeerID string `json:"peerID"`
	}
	if json.Unmarshal(data, &obj) == nil {
		return obj.PeerID
	}
	return ""
}

func isOffline(payload any) bool {
	switch p := payload.(type) {
	case map[string]any:
		if offline, ok := p["offline"].(bool); ok {
			return offline
		}
	}
	return false
}
