package listen

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/petervdpas/goop2/internal/group"
	"github.com/petervdpas/goop2/internal/mq"
	"github.com/petervdpas/goop2/internal/proto"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
)

// Manager manages a single listening group (hosting or listening).
type Manager struct {
	host    host.Host
	grp     *group.Manager
	mq      *mq.Manager
	selfID  string
	dataDir string // directory for persisting queue state

	mu    sync.RWMutex
	group *Group

	// Host-side state
	filePath string // path to the loaded MP3
	paused   bool
	stopCh   chan struct{} // closed to stop streaming goroutines
	seekGen  int64        // incremented on seek to signal reconnect

	// Queue
	queue    []string // file paths for the playlist
	queueIdx int      // current index

	// Per-listener audio pipes (listener peerID -> pipe)
	pipesMu sync.RWMutex
	pipes   map[string]*listenerPipe

	// Local HTTP audio pipe (for the host/listener viewer)
	httpPipeMu sync.Mutex
	httpPipeR  *io.PipeReader
	httpPipeW  *io.PipeWriter
}

type listenerPipe struct {
	w      io.WriteCloser
	cancel func()
}

// queueState is serialised to disk to persist the playlist across restarts.
type queueState struct {
	GroupID string   `json:"group_id"`
	Paths   []string `json:"paths"`
	Index   int      `json:"index"`
}

// isStreamURL detects HTTP/HTTPS stream URLs.
func isStreamURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// streamDisplayName extracts a display name from a stream URL.
func streamDisplayName(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return rawURL
	}
	name := u.Host + u.Path
	if len(name) > 60 {
		name = name[:57] + "..."
	}
	return name
}

// New creates a new listen manager. It registers the binary stream handler
// and subscribes to group events for listen control messages.
func New(h host.Host, grp *group.Manager, mqMgr *mq.Manager, selfID, dataDir string) *Manager {
	m := &Manager{
		host:    h,
		grp:     grp,
		mq:      mqMgr,
		selfID:  selfID,
		dataDir: dataDir,
		pipes:   make(map[string]*listenerPipe),
	}

	// Recover any listen group left over from a previous session.
	// The group protocol already reloaded it from the DB; we just need to
	// re-attach the listen manager's state so the host can load tracks again.
	if rows, err := grp.ListHostedGroups(); err == nil {
		for _, g := range rows {
			if strings.HasPrefix(g.ID, "listen-") && m.group == nil {
				_ = grp.JoinOwnGroup(g.ID) // no-op if already joined
				m.group = &Group{
					ID:   g.ID,
					Name: g.Name,
					Role: "host",
				}
				m.paused = true
				m.stopCh = make(chan struct{})
				log.Printf("LISTEN: Recovered group %s (%s) from previous session", g.ID, g.Name)

				// Restore saved queue
				if qs := m.loadQueueFromDiskForGroup(g.ID); qs != nil && len(qs.Paths) > 0 {
					m.queue = qs.Paths
					m.queueIdx = qs.Index
					if m.queueIdx >= len(m.queue) {
						m.queueIdx = 0
					}
					_, err := m.loadTrackAtLocked(m.queueIdx)
					if err != nil {
						log.Printf("LISTEN: Could not reload queue track: %v", err)
						m.queue = nil
						m.queueIdx = 0
					} else {
						log.Printf("LISTEN: Restored queue (%d tracks, current=%d)", len(m.queue), m.queueIdx)
					}
				}
			}
		}
	}

	h.SetStreamHandler(protocol.ID(proto.ListenProtoID), m.handleAudioStream)

	return m
}

// ── Host methods ─────────────────────────────────────────────────────────────

// CreateGroup creates a new listening group. Only one group at a time.
func (m *Manager) CreateGroup(name string) (*Group, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.group != nil {
		return nil, fmt.Errorf("already in a group")
	}

	id := generateListenID()

	if err := m.grp.CreateGroup(id, name, "listen", 0, false); err != nil {
		return nil, fmt.Errorf("create group: %w", err)
	}
	if err := m.grp.JoinOwnGroup(id); err != nil {
		m.grp.CloseGroup(id)
		return nil, fmt.Errorf("join own group: %w", err)
	}

	m.group = &Group{
		ID:   id,
		Name: name,
		Role: "host",
	}
	m.paused = true
	m.stopCh = make(chan struct{})

	log.Printf("LISTEN: Created group %s (%s)", id, name)
	m.notifyBrowser()
	return m.group, nil
}

// LoadTrack loads a single MP3 file, replacing any existing queue.
func (m *Manager) LoadTrack(filePath string) (*Track, error) {
	return m.LoadQueue([]string{filePath})
}

// LoadQueue loads one or more MP3 files as a playlist. Playback of the first
// track begins when Play is called; subsequent tracks auto-advance.
func (m *Manager) LoadQueue(paths []string) (*Track, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("no paths provided")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.group == nil || m.group.Role != "host" {
		return nil, fmt.Errorf("not hosting a group")
	}

	m.stopPlaybackLocked()
	m.queue = paths
	m.queueIdx = 0

	track, err := m.loadTrackAtLocked(0)
	m.saveQueueToDisk()
	return track, err
}

// AddToQueue appends one or more files to the playlist. If no queue exists yet
// the first file is loaded immediately (ready to play). Safe to call while
// playback is running — does not interrupt the current track.
func (m *Manager) AddToQueue(paths []string) error {
	if len(paths) == 0 {
		return fmt.Errorf("no paths provided")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.group == nil || m.group.Role != "host" {
		return fmt.Errorf("not hosting a group")
	}

	if len(m.queue) == 0 {
		// No queue yet — start fresh from the first file
		m.stopPlaybackLocked()
		m.queue = paths
		m.queueIdx = 0
		_, err := m.loadTrackAtLocked(0)
		m.saveQueueToDisk()
		return err
	}

	// Append without interrupting current playback
	m.queue = append(m.queue, paths...)
	m.updateQueueInfoLocked()
	m.saveQueueToDisk()
	m.notifyBrowser()
	return nil
}

// loadTrackAtLocked loads the track at queue index idx. Caller must hold m.mu.
func (m *Manager) loadTrackAtLocked(idx int) (*Track, error) {
	if idx >= len(m.queue) {
		return nil, fmt.Errorf("queue index out of range")
	}
	filePath := m.queue[idx]

	// Branch for stream URLs
	if isStreamURL(filePath) {
		m.filePath = filePath
		m.paused = true

		track := &Track{
			Name:     streamDisplayName(filePath),
			Duration: 0,
			Bitrate:  0,
			Format:   "stream",
			IsStream: true,
		}
		m.group.Track = track
		m.group.PlayState = &PlayState{
			Playing:   false,
			Position:  0,
			UpdatedAt: time.Now().UnixMilli(),
		}
		m.updateQueueInfoLocked()

		// Broadcast track load to listeners
		m.sendControl(ControlMsg{
			Action:     "load",
			Track:      track,
			Queue:      m.group.Queue,
			QueueTypes: m.group.QueueTypes,
			QueueIndex: m.group.QueueIndex,
			QueueTotal: m.group.QueueTotal,
		})

		log.Printf("LISTEN: Loaded stream %s [%d/%d]", track.Name, idx+1, len(m.queue))
		m.notifyBrowser()
		return track, nil
	}

	info, err := probeMP3(filePath)
	if err != nil {
		return nil, fmt.Errorf("probe mp3: %w", err)
	}

	m.filePath = filePath
	m.paused = true

	track := &Track{
		Name:     filepath.Base(filePath),
		Duration: info.Duration,
		Bitrate:  info.Bitrate,
		Format:   "mp3",
		IsStream: false,
	}
	m.group.Track = track
	m.group.PlayState = &PlayState{
		Playing:   false,
		Position:  0,
		UpdatedAt: time.Now().UnixMilli(),
	}
	m.updateQueueInfoLocked()

	// Broadcast track load to listeners
	m.sendControl(ControlMsg{
		Action:     "load",
		Track:      track,
		Queue:      m.group.Queue,
		QueueTypes: m.group.QueueTypes,
		QueueIndex: m.group.QueueIndex,
		QueueTotal: m.group.QueueTotal,
	})

	log.Printf("LISTEN: Loaded track %s (%d kbps, %.1fs) [%d/%d]",
		track.Name, track.Bitrate/1000, track.Duration, idx+1, len(m.queue))
	m.notifyBrowser()
	return track, nil
}

// updateQueueInfoLocked refreshes the queue display names on m.group.
// Caller must hold m.mu.
func (m *Manager) updateQueueInfoLocked() {
	if m.group == nil {
		return
	}
	if len(m.queue) == 0 {
		m.group.Queue = nil
		m.group.QueueTypes = nil
		m.group.QueueIndex = 0
		m.group.QueueTotal = 0
		return
	}
	types := make([]string, len(m.queue))
	names := make([]string, len(m.queue))
	for i, p := range m.queue {
		if isStreamURL(p) {
			types[i] = "stream"
			names[i] = streamDisplayName(p)
		} else {
			types[i] = "file"
			names[i] = filepath.Base(p)
		}
	}
	m.group.Queue = names
	m.group.QueueTypes = types
	m.group.QueueIndex = m.queueIdx
	m.group.QueueTotal = len(m.queue)
}

// Next skips to the next track. Safe to call from the HTTP handler.
func (m *Manager) Next() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.group == nil || m.group.Role != "host" {
		return fmt.Errorf("not hosting a group")
	}
	nextIdx := m.queueIdx + 1
	if nextIdx >= len(m.queue) {
		return fmt.Errorf("already at last track")
	}

	wasPlaying := !m.paused
	m.stopPlaybackLocked()
	m.queueIdx = nextIdx

	if _, err := m.loadTrackAtLocked(nextIdx); err != nil {
		return err
	}

	if wasPlaying {
		m.paused = false
		m.group.PlayState = &PlayState{Playing: true, Position: 0, UpdatedAt: time.Now().UnixMilli()}
		m.sendControl(ControlMsg{Action: "play", Position: 0})
		m.startStreaming(0)
		stopCh := m.stopCh
		go m.trackTimerGoroutine(stopCh)
	}

	m.saveQueueToDisk()
	m.notifyBrowser()
	return nil
}

// RemoveFromQueue removes a track at the specified index.
// If the currently playing track is removed, it stops playback.
func (m *Manager) RemoveFromQueue(idx int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.group == nil || m.group.Role != "host" {
		return fmt.Errorf("not hosting a group")
	}
	if idx < 0 || idx >= len(m.queue) {
		return fmt.Errorf("invalid queue index")
	}

	// Remove the track from the queue
	newQueue := make([]string, 0, len(m.queue)-1)
	newQueue = append(newQueue, m.queue[:idx]...)
	newQueue = append(newQueue, m.queue[idx+1:]...)
	m.queue = newQueue

	// If removing the currently playing track
	if idx == m.queueIdx {
		wasPlaying := !m.paused
		m.stopPlaybackLocked()

		// Queue is now empty - clear state but stay in group
		if len(m.queue) == 0 {
			m.queueIdx = 0
			m.filePath = ""
			m.group.Track = nil
			m.group.Queue = nil
			m.group.QueueIndex = 0
			m.group.QueueTotal = 0
			m.group.PlayState = &PlayState{Playing: false, Position: 0, UpdatedAt: time.Now().UnixMilli()}
			// Send pause to listeners so they stop playing
			m.sendControl(ControlMsg{Action: "pause", Position: 0})
		} else {
			// Adjust queue index if needed
			if m.queueIdx >= len(m.queue) {
				m.queueIdx = len(m.queue) - 1
			}

			// Load the new current track
			_, err := m.loadTrackAtLocked(m.queueIdx)
			if err != nil {
				m.queueIdx = 0
				m.loadTrackAtLocked(0)
			}

			// Resume playback if it was playing
			if wasPlaying {
				m.paused = false
				m.group.PlayState = &PlayState{Playing: true, Position: 0, UpdatedAt: time.Now().UnixMilli()}
				m.sendControl(ControlMsg{Action: "play", Position: 0})
				m.startStreaming(0)
				stopCh := m.stopCh
				go m.trackTimerGoroutine(stopCh)
			}
		}
	} else {
		// Adjust index if needed for non-current tracks
		if idx < m.queueIdx {
			m.queueIdx--
		}
		m.updateQueueInfoLocked()
	}

	m.saveQueueToDisk()
	m.notifyBrowser()
	log.Printf("LISTEN: Removed track at index %d", idx)
	return nil
}

// SkipToTrack jumps to a specific track in the queue by index.
func (m *Manager) SkipToTrack(idx int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.group == nil || m.group.Role != "host" {
		return fmt.Errorf("not hosting a group")
	}
	if idx < 0 || idx >= len(m.queue) {
		return fmt.Errorf("invalid queue index")
	}

	wasPlaying := !m.paused
	m.stopPlaybackLocked()
	m.queueIdx = idx

	_, err := m.loadTrackAtLocked(idx)
	if err != nil {
		return err
	}

	if wasPlaying {
		m.paused = false
		m.group.PlayState = &PlayState{Playing: true, Position: 0, UpdatedAt: time.Now().UnixMilli()}
		m.sendControl(ControlMsg{Action: "play", Position: 0})
		m.startStreaming(0)
		stopCh := m.stopCh
		go m.trackTimerGoroutine(stopCh)
	}

	m.saveQueueToDisk()
	m.notifyBrowser()
	log.Printf("LISTEN: Skipped to track %d", idx)
	return nil
}

// Prev goes to the previous track (or restarts current if past 3 seconds).
func (m *Manager) Prev() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.group == nil || m.group.Role != "host" {
		return fmt.Errorf("not hosting a group")
	}

	// If more than 3 seconds in, restart current track
	pos := m.currentPosition()
	if pos > 3.0 || m.queueIdx == 0 {
		wasPlaying := !m.paused
		m.stopPlaybackLocked()
		if _, err := m.loadTrackAtLocked(m.queueIdx); err != nil {
			return err
		}
		if wasPlaying {
			m.paused = false
			m.group.PlayState = &PlayState{Playing: true, Position: 0, UpdatedAt: time.Now().UnixMilli()}
			m.sendControl(ControlMsg{Action: "play", Position: 0})
			m.startStreaming(0)
			stopCh := m.stopCh
			go m.trackTimerGoroutine(stopCh)
		}
		m.saveQueueToDisk()
		m.notifyBrowser()
		return nil
	}

	// Go to previous track
	prevIdx := m.queueIdx - 1
	wasPlaying := !m.paused
	m.stopPlaybackLocked()
	m.queueIdx = prevIdx

	if _, err := m.loadTrackAtLocked(prevIdx); err != nil {
		return err
	}

	if wasPlaying {
		m.paused = false
		m.group.PlayState = &PlayState{Playing: true, Position: 0, UpdatedAt: time.Now().UnixMilli()}
		m.sendControl(ControlMsg{Action: "play", Position: 0})
		m.startStreaming(0)
		stopCh := m.stopCh
		go m.trackTimerGoroutine(stopCh)
	}

	m.saveQueueToDisk()
	m.notifyBrowser()
	return nil
}

// advanceQueue advances to the next track and starts playing.
// Safe to call from a goroutine (takes its own lock).
func (m *Manager) advanceQueue() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.group == nil || m.group.Role != "host" {
		return
	}

	for {
		nextIdx := m.queueIdx + 1
		if nextIdx >= len(m.queue) {
			// Playlist exhausted
			m.paused = true
			if m.group.PlayState != nil {
				m.group.PlayState.Playing = false
			}
			log.Printf("LISTEN: Playlist finished")
			m.notifyBrowser()
			return
		}

		m.stopPlaybackLocked()
		m.queueIdx = nextIdx

		_, err := m.loadTrackAtLocked(nextIdx)
		if err != nil {
			log.Printf("LISTEN: Skipping bad track %s: %v", m.queue[nextIdx], err)
			continue
		}

		// Auto-play next track
		m.paused = false
		m.group.PlayState = &PlayState{
			Playing:   true,
			Position:  0,
			UpdatedAt: time.Now().UnixMilli(),
		}
		m.saveQueueToDisk()
		m.sendControl(ControlMsg{Action: "play", Position: 0})
		m.startStreaming(0)
		stopCh := m.stopCh
		go m.trackTimerGoroutine(stopCh)
		m.notifyBrowser()
		return
	}
}

// syncPulseEvery controls how often the host broadcasts a sync pulse to listeners.
const syncPulseTicks = 10 // × 500 ms = 5 seconds

// trackTimerGoroutine polls playback position, auto-advances when a track ends,
// and emits a periodic sync pulse so late-joining listeners catch up.
func (m *Manager) trackTimerGoroutine(stopCh chan struct{}) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	var ticks int
	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			ticks++
			m.mu.RLock()
			g := m.group
			paused := m.paused
			m.mu.RUnlock()

			// Exit immediately if paused, no sync pulses while paused
			if g == nil || paused || g.PlayState == nil || !g.PlayState.Playing || g.Track == nil {
				if paused {
					log.Printf("LISTEN: Timer exiting because paused=true")
				}
				return
			}

			elapsed := float64(time.Now().UnixMilli()-g.PlayState.UpdatedAt) / 1000.0
			pos := g.PlayState.Position + elapsed
			// Don't auto-advance streams (Duration: 0), only advance finite-duration tracks
			if g.Track.Duration > 0 && pos >= g.Track.Duration {
				log.Printf("LISTEN: Track ended, advancing queue")
				m.advanceQueue()
				return
			}

			// Periodic sync pulse: broadcast current track + position to all
			// listeners so anyone who joined late or missed the initial load/play
			// messages catches up within syncPulseTicks × 500ms.
			if ticks%syncPulseTicks == 0 {
				// Double-check paused before sending sync
				m.mu.RLock()
				if m.paused {
					m.mu.RUnlock()
					log.Printf("LISTEN: Paused during sync, exiting timer")
					return
				}
				track := g.Track
				queue := append([]string(nil), g.Queue...)
				queueTypes := append([]string(nil), g.QueueTypes...)
				queueIdx := g.QueueIndex
				queueTotal := g.QueueTotal
				m.mu.RUnlock()
				m.sendControl(ControlMsg{
					Action:     "sync",
					Track:      track,
					Position:   pos,
					Queue:      queue,
					QueueTypes: queueTypes,
					QueueIndex: queueIdx,
					QueueTotal: queueTotal,
				})
			}
		}
	}
}

// Play starts or resumes playback.
func (m *Manager) Play() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.group == nil || m.group.Role != "host" {
		return fmt.Errorf("not hosting a group")
	}
	if m.group.Track == nil {
		return fmt.Errorf("no track loaded")
	}

	pos := 0.0
	if m.group.PlayState != nil {
		pos = m.group.PlayState.Position
	}

	m.paused = false
	m.group.PlayState = &PlayState{
		Playing:   true,
		Position:  pos,
		UpdatedAt: time.Now().UnixMilli(),
	}

	m.sendControl(ControlMsg{Action: "play", Position: pos})

	// Start streaming to all connected listeners
	m.startStreaming(pos)

	// Start track-end timer for auto-advance
	stopCh := m.stopCh
	go m.trackTimerGoroutine(stopCh)

	log.Printf("LISTEN: Play from %.1fs", pos)
	m.notifyBrowser()
	return nil
}

// Pause pauses playback.
func (m *Manager) Pause() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.group == nil || m.group.Role != "host" {
		return fmt.Errorf("not hosting a group")
	}

	// Set paused FIRST before stopping, so timer checks see it immediately
	m.paused = true
	log.Printf("LISTEN: Set paused=true")

	m.stopPlaybackLocked()
	log.Printf("LISTEN: Stopped playback")

	pos := m.currentPosition()
	m.group.PlayState = &PlayState{
		Playing:   false,
		Position:  pos,
		UpdatedAt: time.Now().UnixMilli(),
	}
	log.Printf("LISTEN: Updated PlayState to Playing=false")

	m.sendControl(ControlMsg{Action: "pause", Position: pos})
	log.Printf("LISTEN: Sent pause control")

	m.notifyBrowser()
	log.Printf("LISTEN: Paused at %.1fs", pos)
	return nil
}

// Seek jumps to a position in seconds.
func (m *Manager) Seek(position float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.group == nil || m.group.Role != "host" {
		return fmt.Errorf("not hosting a group")
	}
	if m.group.Track == nil {
		return fmt.Errorf("no track loaded")
	}

	wasPlaying := m.group.PlayState != nil && m.group.PlayState.Playing
	m.stopPlaybackLocked()

	m.group.PlayState = &PlayState{
		Playing:   wasPlaying,
		Position:  position,
		UpdatedAt: time.Now().UnixMilli(),
	}

	m.sendControl(ControlMsg{Action: "seek", Position: position})
	m.seekGen++

	if wasPlaying {
		m.paused = false
		m.startStreaming(position)
	}

	log.Printf("LISTEN: Seek to %.1fs (playing=%v)", position, wasPlaying)
	m.notifyBrowser()
	return nil
}

// CloseGroup closes the listening group and disconnects all listeners.
func (m *Manager) CloseGroup() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.group == nil {
		return nil
	}

	m.stopPlaybackLocked()

	if m.group.Role == "host" {
		m.sendControl(ControlMsg{Action: "close"})
		_ = m.grp.CloseGroup(m.group.ID)
	} else {
		_ = m.grp.LeaveGroup(m.group.ID)
	}

	m.closeHTTPPipeLocked()
	m.group = nil
	m.filePath = ""
	m.queue = nil
	m.queueIdx = 0
	m.saveQueueToDisk() // clear persisted queue

	log.Printf("LISTEN: Group closed")
	m.notifyBrowser()
	return nil
}

// ── Listener methods ─────────────────────────────────────────────────────────

// JoinGroup joins a remote listening group.
func (m *Manager) JoinGroup(hostPeerID, groupID string) error {
	// A listener can only be in one listen group at a time — like being at
	// someone's house, you have to leave before going to another.
	// Auto-leave any current listener group before joining the new one.
	// Hosting your own group is a different role; that blocks joining.
	m.mu.Lock()
	if m.group != nil {
		if m.group.Role == "listener" {
			log.Printf("LISTEN: Auto-leaving %s to join %s", m.group.ID, groupID)
			_ = m.grp.LeaveGroup(m.group.ID)
			m.closeHTTPPipeLocked()
			m.group = nil
			m.notifyBrowser()
		} else {
			// Role == "host": can't abandon your own party.
			m.mu.Unlock()
			return fmt.Errorf("already hosting a listen group")
		}
	}

	// Set m.group BEFORE calling JoinRemoteGroup so that forwardGroupEvents
	// can process control messages (load, play) that the host sends in
	// response to the join — those messages arrive via the group protocol
	// stream during or immediately after the handshake, before this
	// goroutine resumes to set m.group. Without this, the event loop sees
	// lg == nil and silently drops the messages, leaving the listener stuck
	// on "Waiting for host to play a track..." forever.
	m.group = &Group{
		ID:   groupID,
		Name: groupID,
		Role: "listener",
	}
	m.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := m.grp.JoinRemoteGroup(ctx, hostPeerID, groupID); err != nil {
		// Roll back the optimistic group assignment.
		m.mu.Lock()
		if m.group != nil && m.group.ID == groupID && m.group.Role == "listener" {
			m.group = nil
		}
		m.mu.Unlock()
		return fmt.Errorf("join group: %w", err)
	}

	log.Printf("LISTEN: Joined group %s on host %s", groupID, hostPeerID)
	m.notifyBrowser()
	return nil
}

// LeaveGroup leaves the current listening group.
func (m *Manager) LeaveGroup() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.group == nil || m.group.Role != "listener" {
		return fmt.Errorf("not in a listening group")
	}

	_ = m.grp.LeaveGroup(m.group.ID)
	m.closeHTTPPipeLocked()
	m.group = nil

	log.Printf("LISTEN: Left group")
	m.notifyBrowser()
	return nil
}

// AudioReader returns an io.ReadCloser that streams audio from the host.
// The caller is responsible for closing it.
func (m *Manager) AudioReader() (io.ReadCloser, error) {
	m.mu.RLock()
	lg := m.group
	m.mu.RUnlock()

	if lg == nil {
		return nil, fmt.Errorf("not in a group")
	}

	if lg.Role == "listener" {
		return m.connectAudioStream()
	}

	// Host can also listen to their own stream (local playback).
	// Create a new pipe; if playback is already running, restart streaming to it.
	m.httpPipeMu.Lock()
	if m.httpPipeR != nil {
		m.httpPipeR.Close()
	}
	r, w := io.Pipe()
	m.httpPipeR = r
	m.httpPipeW = w
	m.httpPipeMu.Unlock()

	// If the host is already playing, start streaming to the new pipe.
	// We deliberately do NOT call startStreaming() here because that would
	// replace m.stopCh, killing any timer goroutine launched by Next/Prev/Play.
	go func() {
		m.mu.RLock()
		playing := m.group != nil && !m.paused && m.filePath != "" && m.group.Track != nil
		var filePath string
		var bitrate int
		var pos float64
		var stopCh chan struct{}
		if playing {
			filePath = m.filePath
			bitrate = m.group.Track.Bitrate
			pos = m.currentPosition()
			stopCh = m.stopCh
		}
		m.mu.RUnlock()

		if !playing {
			return
		}

		m.httpPipeMu.Lock()
		httpW := m.httpPipeW
		m.httpPipeMu.Unlock()
		if httpW == nil {
			return
		}

		// Branch for stream URLs - use cancellable context to stop on pause
		if isStreamURL(filePath) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Cancel context when stop signal arrives
			go func() {
				<-stopCh
				cancel()
			}()

			req, err := http.NewRequestWithContext(ctx, "GET", filePath, nil)
			if err != nil {
				httpW.CloseWithError(err)
				return
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				// Context cancelled or other error - just exit
				return
			}
			defer resp.Body.Close()
			buf := make([]byte, 32*1024)
			io.CopyBuffer(httpW, resp.Body, buf)
			return
		}

		ff, err := os.Open(filePath)
		if err != nil {
			return
		}
		defer ff.Close()
		byteOffset := int64(pos * float64(bitrate) / 8.0)
		if byteOffset > 0 {
			ff.Seek(byteOffset, io.SeekStart)
		}
		// Local HTTP — copy at full speed. The browser's audio element plays at
		// natural speed so currentTime stays in sync with the server clock.
		// stopCh / pipe close (on pause/next) will make Write return ErrClosedPipe.
		buf := make([]byte, 32*1024)
		io.CopyBuffer(httpW, ff, buf)
	}()

	return r, nil
}

// GetGroup returns the current group state.
func (m *Manager) GetGroup() *Group {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.group == nil {
		return nil
	}

	// Return a copy with up-to-date position
	r := *m.group
	if r.PlayState != nil && r.PlayState.Playing {
		elapsed := float64(time.Now().UnixMilli()-r.PlayState.UpdatedAt) / 1000.0
		ps := *r.PlayState
		ps.Position += elapsed
		r.PlayState = &ps
	}
	return &r
}

// notifyBrowser publishes the current group state to the browser via MQ PublishLocal.
// Caller must hold m.mu (at minimum read lock).
func (m *Manager) notifyBrowser() {
	groupID := ""
	if m.group != nil {
		groupID = m.group.ID
	}
	m.mq.PublishLocal("listen:"+groupID+":state", "", map[string]any{"group": m.group})
}

// ── Streaming (host → listeners) ─────────────────────────────────────────────

// handleAudioStream processes incoming listen protocol streams from listeners.
// Wire format: "LISTEN <group_id>\n" → host sends raw MP3 bytes.
func (m *Manager) handleAudioStream(s network.Stream) {
	remotePeer := s.Conn().RemotePeer().String()
	defer s.Close()

	// Read request line
	buf := make([]byte, 256)
	n := 0
	for n < len(buf) {
		b := make([]byte, 1)
		_, err := s.Read(b)
		if err != nil {
			return
		}
		if b[0] == '\n' {
			break
		}
		buf[n] = b[0]
		n++
	}
	line := string(buf[:n])
	parts := strings.SplitN(line, " ", 2)
	if len(parts) != 2 || parts[0] != "LISTEN" {
		fmt.Fprintf(s, "ERR bad request\n")
		return
	}
	groupID := parts[1]

	m.mu.RLock()
	lg := m.group
	m.mu.RUnlock()

	if lg == nil || lg.ID != groupID || lg.Role != "host" {
		fmt.Fprintf(s, "ERR not found\n")
		return
	}

	if lg.Track == nil {
		fmt.Fprintf(s, "ERR no track\n")
		return
	}

	// Send OK with track info
	fmt.Fprintf(s, "OK %s %d %.2f\n", lg.Track.Format, lg.Track.Bitrate, lg.Track.Duration)

	log.Printf("LISTEN: Audio stream started for %s", remotePeer)

	// Open file and seek to current position
	m.mu.RLock()
	pos := 0.0
	if lg.PlayState != nil {
		pos = lg.PlayState.Position
		if lg.PlayState.Playing {
			elapsed := float64(time.Now().UnixMilli()-lg.PlayState.UpdatedAt) / 1000.0
			pos += elapsed
		}
	}
	filePath := m.filePath
	paused := m.paused
	m.mu.RUnlock()

	if paused {
		// Playback hasn't started yet. Close the stream so the listener
		// knows to retry once a "play" control message arrives.
		fmt.Fprintf(s, "")
		return
	}

	// Branch for stream URLs - use context to support immediate stop
	if isStreamURL(filePath) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Monitor pause flag and cancel context if needed
		go func() {
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					m.mu.RLock()
					shouldStop := m.paused || m.group == nil || m.group.ID != groupID
					m.mu.RUnlock()
					if shouldStop {
						cancel()
						return
					}
				case <-ctx.Done():
					return
				}
			}
		}()

		req, err := http.NewRequestWithContext(ctx, "GET", filePath, nil)
		if err != nil {
			log.Printf("LISTEN: Failed to create request for stream for %s: %v", remotePeer, err)
			return
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Printf("LISTEN: Failed to fetch stream for %s: %v", remotePeer, err)
			return
		}
		defer resp.Body.Close()

		audioBuffer := make([]byte, 64*1024)
		for {
			n, err := resp.Body.Read(audioBuffer)
			if n > 0 {
				data := audioBuffer[:n]
				for len(data) > 0 {
					nw, werr := s.Write(data)
					if werr != nil {
						log.Printf("LISTEN: Stream to %s ended (write error): %v", remotePeer, werr)
						return
					}
					data = data[nw:]
				}
			}
			if err != nil {
				// For streams, EOF is normal or context cancelled
				log.Printf("LISTEN: Stream to %s finished", remotePeer)
				return
			}
		}
	}

	f, err := os.Open(filePath)
	if err != nil {
		log.Printf("LISTEN: Failed to open file for streaming: %v", err)
		return
	}
	defer f.Close()

	// Seek to byte position based on current playback position
	byteOffset := int64(pos * float64(lg.Track.Bitrate) / 8.0)
	if byteOffset > 0 {
		f.Seek(byteOffset, io.SeekStart)
	}

	// Stream audio data. Stop if playback is paused or group is closed.
	audioBuffer := make([]byte, 64*1024)
	checkCounter := 0
	for {
		// Every 10 reads, check if playback should stop (roughly every 640KB, or ~0.5 seconds at 320kbps)
		checkCounter++
		if checkCounter >= 10 {
			checkCounter = 0
			m.mu.RLock()
			shouldStop := m.paused || m.group == nil || m.group.ID != groupID
			m.mu.RUnlock()
			if shouldStop {
				log.Printf("LISTEN: Stream to %s stopped (playback ended)", remotePeer)
				return
			}
		}

		n, err := f.Read(audioBuffer)
		if n > 0 {
			data := audioBuffer[:n]
			for len(data) > 0 {
				nw, werr := s.Write(data)
				if werr != nil {
					log.Printf("LISTEN: Stream to %s ended (write error): %v", remotePeer, werr)
					return
				}
				data = data[nw:]
			}
		}
		if err == io.EOF {
			log.Printf("LISTEN: Stream to %s finished", remotePeer)
			return
		}
		if err != nil {
			log.Printf("LISTEN: Stream to %s ended (read error): %v", remotePeer, err)
			return
		}
	}
}

// connectAudioStream opens a listen protocol stream to the host and returns
// a reader for the audio data.
func (m *Manager) connectAudioStream() (io.ReadCloser, error) {
	m.mu.RLock()
	lg := m.group
	m.mu.RUnlock()

	if lg == nil || lg.Role != "listener" {
		return nil, fmt.Errorf("not a listener")
	}

	hostPeerID, connected := m.grp.ActiveGroup(lg.ID)
	if !connected {
		return nil, fmt.Errorf("not connected to host")
	}

	pid, err := peer.Decode(hostPeerID)
	if err != nil {
		return nil, fmt.Errorf("invalid host peer ID: %w", err)
	}

	sCtx, sCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer sCancel()
	s, err := m.host.NewStream(sCtx, pid, protocol.ID(proto.ListenProtoID))
	if err != nil {
		return nil, fmt.Errorf("open stream: %w", err)
	}

	// Send request
	fmt.Fprintf(s, "LISTEN %s\n", lg.ID)

	// Read response line
	buf := make([]byte, 256)
	n := 0
	for n < len(buf) {
		b := make([]byte, 1)
		_, err := s.Read(b)
		if err != nil {
			s.Close()
			return nil, fmt.Errorf("read response: %w", err)
		}
		if b[0] == '\n' {
			break
		}
		buf[n] = b[0]
		n++
	}
	line := string(buf[:n])

	if strings.HasPrefix(line, "ERR") {
		s.Close()
		return nil, fmt.Errorf("host: %s", line)
	}

	if !strings.HasPrefix(line, "OK") {
		s.Close()
		return nil, fmt.Errorf("unexpected response: %s", line)
	}

	return s, nil
}

// startStreaming signals that playback has started at the given position.
// Audio delivery to listeners is handled per-stream in handleAudioStream;
// audio delivery to the local browser is handled in AudioReader.
// This method only resets the stop channel so goroutines watching it restart.
func (m *Manager) startStreaming(_ float64) {
	if m.filePath == "" || m.group == nil || m.group.Track == nil {
		return
	}

	// Close old stop channel, create new one so trackTimerGoroutine restarts.
	if m.stopCh != nil {
		select {
		case <-m.stopCh:
		default:
			close(m.stopCh)
		}
	}
	m.stopCh = make(chan struct{})
}

func (m *Manager) stopPlaybackLocked() {
	if m.stopCh != nil {
		select {
		case <-m.stopCh:
		default:
			close(m.stopCh)
		}
	}
	// Close the HTTP pipe so any goroutine blocked writing to it unblocks
	// immediately with ErrClosedPipe. The stream handler exits on its next Read.
	m.httpPipeMu.Lock()
	if m.httpPipeW != nil {
		m.httpPipeW.CloseWithError(io.ErrClosedPipe)
		m.httpPipeW = nil
	}
	if m.httpPipeR != nil {
		m.httpPipeR.Close()
		m.httpPipeR = nil
	}
	m.httpPipeMu.Unlock()
}

func (m *Manager) closeHTTPPipeLocked() {
	m.httpPipeMu.Lock()
	if m.httpPipeW != nil {
		m.httpPipeW.Close()
		m.httpPipeW = nil
	}
	if m.httpPipeR != nil {
		m.httpPipeR.Close()
		m.httpPipeR = nil
	}
	m.httpPipeMu.Unlock()
}

func (m *Manager) currentPosition() float64 {
	if m.group == nil || m.group.PlayState == nil {
		return 0
	}
	pos := m.group.PlayState.Position
	if m.group.PlayState.Playing {
		elapsed := float64(time.Now().UnixMilli()-m.group.PlayState.UpdatedAt) / 1000.0
		pos += elapsed
	}
	return pos
}

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
// It is called by the group manager for every event whose group has app_type "listen".
func (m *Manager) HandleGroupEvent(evt *group.Event) {
	m.mu.RLock()
	lg := m.group
	m.mu.RUnlock()

	if lg == nil {
		// No active listen session — but the group manager may have reconnected
		// to this listen group via reconnectSubscriptions() without going through
		// listen.Manager.JoinGroup() (which is the only path that sets m.group).
		// If the group manager has an active client connection for this group,
		// auto-restore m.group so we can process control messages immediately.
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

	// Skip own messages
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
			// A listener left
			log.Printf("LISTEN: Listener %s left", evt.From)
		}
	case "members":
		// Update listener list; sync state to any newly joined listener.
		if lg.Role == "host" {
			if mp, ok := evt.Payload.(map[string]any); ok {
				if members, ok := mp["members"].([]any); ok {
					m.mu.Lock()

					// Remember who was already connected so we can spot new joiners.
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

					// Detect new listeners.
					hasNewListeners := false
					for _, pid := range m.group.Listeners {
						if !oldSet[pid] {
							hasNewListeners = true
							break
						}
					}

					// Capture state to sync (outside the lock below).
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

					// Bring newly joined listeners up to speed.
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

	// Re-marshal and unmarshal to get typed ControlMsg
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
		// Close existing audio pipe so the viewer reconnects
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

// notifyBrowserLocked reads m.group under its own RLock and publishes to the browser.
// Use this when the caller does NOT already hold m.mu.
func (m *Manager) notifyBrowserLocked() {
	m.mu.RLock()
	groupID := ""
	if m.group != nil {
		groupID = m.group.ID
	}
	lg := m.group
	m.mu.RUnlock()
	m.mq.PublishLocal("listen:"+groupID+":state", "", map[string]any{"group": lg})
}

// Close shuts down the listen manager.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.stopPlaybackLocked()
	m.closeHTTPPipeLocked()

	m.group = nil
}

// ── Queue persistence ─────────────────────────────────────────────────────────

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
