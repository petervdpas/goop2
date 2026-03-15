package listen

import (
	"fmt"
	"log"
	"time"

	"github.com/petervdpas/goop2/internal/group"
)

func (m *Manager) sendControl(msg ControlMsg) {
	if m.group == nil {
		return
	}
	_ = m.grp.SendControl(m.group.ID, "listen", msg)
}

func (m *Manager) Flags() group.TypeFlags {
	return group.TypeFlags{HostCanJoin: true}
}

// OnCreate is called when a listen group is created.
func (m *Manager) OnCreate(groupID, name string, _ int, _ bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.group != nil {
		return fmt.Errorf("already in a group")
	}

	m.group = &Group{
		ID:   groupID,
		Name: name,
		Role: "host",
	}
	m.paused = true
	m.stopCh = make(chan struct{})

	log.Printf("LISTEN: Initialized host state for group %s (%s)", groupID, name)
	m.notifyBrowser()
	return nil
}

// OnJoin is called on the host when a member joins the listen group.
func (m *Manager) OnJoin(_ string, peerID string, _ bool) {
	log.Printf("LISTEN: Listener %s joined", peerID)
}

// OnLeave is called on the host when a member leaves the listen group.
func (m *Manager) OnLeave(_ string, peerID string, _ bool) {
	log.Printf("LISTEN: Listener %s left", peerID)
}

// OnClose is called when a listen group is closed.
func (m *Manager) OnClose(groupID string) {
	m.mu.Lock()
	if m.group != nil && m.group.ID == groupID {
		m.stopPlaybackLocked()
		m.closeHTTPPipeLocked()
		m.group = nil
		m.filePath = ""
		m.queue = nil
		m.queueIdx = 0
	}
	m.mu.Unlock()
	m.saveQueueToDisk()

	log.Printf("LISTEN: Group %s closed", groupID)
	m.notifyBrowserLocked()
}

// OnEvent is called for all group events (msg, members, meta, etc.).
func (m *Manager) OnEvent(evt *group.Event) {
	m.mu.RLock()
	lg := m.group
	m.mu.RUnlock()

	if lg == nil {
		// Auto-restore listener state if the group manager reconnected
		// via reconnectSubscriptions without going through listen's JoinGroup.
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
	case "leave":
		if lg.Role == "listener" {
			m.mu.Lock()
			m.closeHTTPPipeLocked()
			m.group = nil
			m.mu.Unlock()
			log.Printf("LISTEN: Left group %s", evt.Group)
			m.notifyBrowserLocked()
		}
	case "msg":
		m.handleControlEvent(evt.Payload)
	case "members":
		if lg.Role == "host" {
			m.handleMembersEvent(evt)
		}
	}
}

func (m *Manager) handleMembersEvent(evt *group.Event) {
	m.mu.Lock()

	members, hasNewListeners := group.ParseMembers(evt.Payload, m.selfID, m.group.Listeners)
	if members == nil {
		m.mu.Unlock()
		return
	}
	m.group.Listeners = members

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

func (m *Manager) handleControlEvent(payload any) {
	var ctrl ControlMsg
	if !group.ParseControl(payload, "listen", &ctrl) {
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
		log.Printf("LISTEN: Group closed by host (control)")
	}

	m.notifyBrowser()
}
