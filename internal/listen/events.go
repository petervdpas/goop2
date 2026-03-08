package listen

import (
	"encoding/json"
	"log"
	"time"

	"github.com/petervdpas/goop2/internal/group"
)

func (m *Manager) sendControl(msg ControlMsg) {
	if m.group == nil {
		return
	}
	payload := map[string]any{
		"app_type": "listen",
		"listen":   msg,
	}
	if m.group.Role == "host" {
		_ = m.grp.SendToGroupAsHost(m.group.ID, payload)
	} else {
		_ = m.grp.SendToGroup(m.group.ID, payload)
	}
}

// HandleGroupEvent implements group.Handler for app_type "listen".
func (m *Manager) HandleGroupEvent(evt *group.Event) {
	m.mu.RLock()
	lg := m.group
	m.mu.RUnlock()

	if lg == nil {
		hostPeerID, connected := m.grp.ActiveGroup(evt.Group)
		if connected {
			groupName := evt.Group
			if evt.Type == "welcome" {
				if wp, ok := evt.Payload.(map[string]any); ok {
					if n, ok := wp["group_name"].(string); ok && n != "" {
						groupName = n
					}
				}
			}
			m.mu.Lock()
			if m.group == nil {
				m.group = &Group{
					ID:   evt.Group,
					Name: groupName,
					Role: "listener",
				}
				lg = m.group
				log.Printf("LISTEN: Auto-restored listener state for group %s on host %s", evt.Group, hostPeerID)
			} else {
				lg = m.group
			}
			m.mu.Unlock()
			m.notifyBrowserLocked()
		}
		if lg == nil {
			return
		}
	}

	if evt.Group != lg.ID {
		return
	}

	if evt.From == m.selfID {
		return
	}

	switch evt.Type {
	case "msg":
		m.handleControlEvent(evt.Payload)
	case "close":
		m.mu.Lock()
		m.closeHTTPPipeLocked()
		m.group = nil
		m.mu.Unlock()
		m.notifyBrowserLocked()
		log.Printf("LISTEN: Group closed by host")
	case "leave":
		if lg.Role == "host" {
			log.Printf("LISTEN: Listener %s left", evt.From)
		}
	case "members":
		if lg.Role == "host" {
			if mp, ok := evt.Payload.(map[string]any); ok {
				if members, ok := mp["members"].([]any); ok {
					m.mu.Lock()

					oldSet := make(map[string]bool, len(m.group.Listeners))
					for _, pid := range m.group.Listeners {
						oldSet[pid] = true
					}

					m.group.Listeners = make([]string, 0, len(members))
					for _, member := range members {
						if mi, ok := member.(map[string]any); ok {
							if pid, ok := mi["peer_id"].(string); ok && pid != m.selfID {
								m.group.Listeners = append(m.group.Listeners, pid)
							}
						}
					}

					hasNewListeners := false
					for _, pid := range m.group.Listeners {
						if !oldSet[pid] {
							hasNewListeners = true
							break
						}
					}

					var syncTrack *Track
					var syncQueue []string
					var syncQueueTypes []string
					var syncQueueIdx, syncQueueTotal int
					var syncPos float64
					var syncPlaying bool
					if hasNewListeners && m.group.Track != nil {
						syncTrack = m.group.Track
						syncQueue = append([]string(nil), m.group.Queue...)
						syncQueueTypes = append([]string(nil), m.group.QueueTypes...)
						syncQueueIdx = m.group.QueueIndex
						syncQueueTotal = m.group.QueueTotal
						if ps := m.group.PlayState; ps != nil {
							syncPlaying = ps.Playing
							if ps.Playing {
								elapsed := float64(time.Now().UnixMilli()-ps.UpdatedAt) / 1000.0
								syncPos = ps.Position + elapsed
							} else {
								syncPos = ps.Position
							}
							if syncPos < 0 {
								syncPos = 0
							}
						}
					}

					m.mu.Unlock()
					m.notifyBrowserLocked()

					if syncTrack != nil {
						m.sendControl(ControlMsg{
							Action:     "load",
							Track:      syncTrack,
							Queue:      syncQueue,
							QueueTypes: syncQueueTypes,
							QueueIndex: syncQueueIdx,
							QueueTotal: syncQueueTotal,
						})
						if syncPlaying {
							m.sendControl(ControlMsg{Action: "play", Position: syncPos})
						} else {
							m.sendControl(ControlMsg{Action: "pause", Position: syncPos})
						}
					}
				}
			}
		}
	}
}

func (m *Manager) handleControlEvent(payload any) {
	mp, ok := payload.(map[string]any)
	if !ok {
		return
	}

	listenRaw, ok := mp["listen"]
	if !ok {
		return
	}

	data, err := json.Marshal(listenRaw)
	if err != nil {
		return
	}
	var ctrl ControlMsg
	if err := json.Unmarshal(data, &ctrl); err != nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.group == nil {
		return
	}

	switch ctrl.Action {
	case "load":
		m.group.Track = ctrl.Track
		m.group.PlayState = &PlayState{
			Playing:   false,
			Position:  0,
			UpdatedAt: time.Now().UnixMilli(),
		}
		if ctrl.QueueTotal > 0 {
			m.group.Queue = ctrl.Queue
			m.group.QueueTypes = ctrl.QueueTypes
			m.group.QueueIndex = ctrl.QueueIndex
			m.group.QueueTotal = ctrl.QueueTotal
		}
		log.Printf("LISTEN: Host loaded track: %s", ctrl.Track.Name)

	case "play":
		m.group.PlayState = &PlayState{
			Playing:   true,
			Position:  ctrl.Position,
			UpdatedAt: time.Now().UnixMilli(),
		}
		log.Printf("LISTEN: Host started playback at %.1fs", ctrl.Position)

	case "pause":
		m.group.PlayState = &PlayState{
			Playing:   false,
			Position:  ctrl.Position,
			UpdatedAt: time.Now().UnixMilli(),
		}
		log.Printf("LISTEN: Host paused at %.1fs", ctrl.Position)

	case "seek":
		wasPlaying := m.group.PlayState != nil && m.group.PlayState.Playing
		m.group.PlayState = &PlayState{
			Playing:   wasPlaying,
			Position:  ctrl.Position,
			UpdatedAt: time.Now().UnixMilli(),
		}
		m.closeHTTPPipeLocked()
		log.Printf("LISTEN: Host seeked to %.1fs", ctrl.Position)

	case "sync":
		if ctrl.Track != nil {
			m.group.Track = ctrl.Track
			if ctrl.QueueTotal > 0 {
				m.group.Queue = ctrl.Queue
				m.group.QueueTypes = ctrl.QueueTypes
				m.group.QueueIndex = ctrl.QueueIndex
				m.group.QueueTotal = ctrl.QueueTotal
			}
		}
		m.group.PlayState = &PlayState{
			Playing:   true,
			Position:  ctrl.Position,
			UpdatedAt: time.Now().UnixMilli(),
		}

	case "close":
		m.closeHTTPPipeLocked()
		m.group = nil
		log.Printf("LISTEN: Group closed by host")
	}

	m.notifyBrowser()
}
