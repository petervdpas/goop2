package listen

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/petervdpas/goop2/internal/group"
	"github.com/petervdpas/goop2/internal/proto"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
)

// Manager manages a single listening room (hosting or listening).
type Manager struct {
	host   host.Host
	grp    *group.Manager
	selfID string

	mu   sync.RWMutex
	room *Room

	// Host-side state
	filePath string   // path to the loaded MP3
	file     *os.File // open file handle for streaming
	paused   bool
	stopCh   chan struct{} // closed to stop streaming goroutines
	seekGen  int64         // incremented on seek to signal reconnect

	// Per-listener audio pipes (listener peerID -> pipe)
	pipesMu sync.RWMutex
	pipes   map[string]*listenerPipe

	// Local HTTP audio pipe (for the listener viewer)
	httpPipeMu sync.Mutex
	httpPipeR  *io.PipeReader
	httpPipeW  *io.PipeWriter

	// SSE listeners for state changes
	sseMu     sync.RWMutex
	sseChans  map[chan *Room]struct{}
}

type listenerPipe struct {
	w      io.WriteCloser
	cancel func()
}

// New creates a new listen manager. It registers the binary stream handler
// and subscribes to group events for listen control messages.
func New(h host.Host, grp *group.Manager, selfID string) *Manager {
	m := &Manager{
		host:     h,
		grp:      grp,
		selfID:   selfID,
		pipes:    make(map[string]*listenerPipe),
		sseChans: make(map[chan *Room]struct{}),
	}

	// Clean up any stale listen- groups from previous sessions
	if rows, err := grp.ListHostedGroups(); err == nil {
		for _, g := range rows {
			if strings.HasPrefix(g.ID, "listen-") {
				_ = grp.CloseGroup(g.ID)
				log.Printf("LISTEN: Cleaned up stale room %s on startup", g.ID)
			}
		}
	}

	h.SetStreamHandler(protocol.ID(proto.ListenProtoID), m.handleAudioStream)
	go m.forwardGroupEvents()

	return m
}

// ── Host methods ─────────────────────────────────────────────────────────────

// CreateRoom creates a new listening room. Only one room at a time.
func (m *Manager) CreateRoom(name string) (*Room, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.room != nil {
		return nil, fmt.Errorf("already in a room")
	}

	id := generateRoomID()

	if err := m.grp.CreateGroup(id, name, "listen", 0); err != nil {
		return nil, fmt.Errorf("create group: %w", err)
	}
	if err := m.grp.JoinOwnGroup(id); err != nil {
		m.grp.CloseGroup(id)
		return nil, fmt.Errorf("join own group: %w", err)
	}

	m.room = &Room{
		ID:   id,
		Name: name,
		Role: "host",
	}
	m.paused = true
	m.stopCh = make(chan struct{})

	log.Printf("LISTEN: Created room %s (%s)", id, name)
	m.notifySSE()
	return m.room, nil
}

// LoadTrack loads an MP3 file for streaming.
func (m *Manager) LoadTrack(filePath string) (*Track, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.room == nil || m.room.Role != "host" {
		return nil, fmt.Errorf("not hosting a room")
	}

	// Stop any current playback
	m.stopPlaybackLocked()

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
	}
	m.room.Track = track
	m.room.PlayState = &PlayState{
		Playing:   false,
		Position:  0,
		UpdatedAt: time.Now().UnixMilli(),
	}

	// Broadcast track load to listeners
	m.sendControl(ControlMsg{Action: "load", Track: track})

	log.Printf("LISTEN: Loaded track %s (%d kbps, %.1fs)", track.Name, track.Bitrate/1000, track.Duration)
	m.notifySSE()
	return track, nil
}

// Play starts or resumes playback.
func (m *Manager) Play() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.room == nil || m.room.Role != "host" {
		return fmt.Errorf("not hosting a room")
	}
	if m.room.Track == nil {
		return fmt.Errorf("no track loaded")
	}

	pos := 0.0
	if m.room.PlayState != nil {
		pos = m.room.PlayState.Position
	}

	m.paused = false
	m.room.PlayState = &PlayState{
		Playing:   true,
		Position:  pos,
		UpdatedAt: time.Now().UnixMilli(),
	}

	m.sendControl(ControlMsg{Action: "play", Position: pos})

	// Start streaming to all connected listeners
	m.startStreaming(pos)

	log.Printf("LISTEN: Play from %.1fs", pos)
	m.notifySSE()
	return nil
}

// Pause pauses playback.
func (m *Manager) Pause() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.room == nil || m.room.Role != "host" {
		return fmt.Errorf("not hosting a room")
	}

	m.stopPlaybackLocked()
	m.paused = true

	pos := m.currentPosition()
	m.room.PlayState = &PlayState{
		Playing:   false,
		Position:  pos,
		UpdatedAt: time.Now().UnixMilli(),
	}

	m.sendControl(ControlMsg{Action: "pause", Position: pos})

	log.Printf("LISTEN: Paused at %.1fs", pos)
	m.notifySSE()
	return nil
}

// Seek jumps to a position in seconds.
func (m *Manager) Seek(position float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.room == nil || m.room.Role != "host" {
		return fmt.Errorf("not hosting a room")
	}
	if m.room.Track == nil {
		return fmt.Errorf("no track loaded")
	}

	wasPlaying := m.room.PlayState != nil && m.room.PlayState.Playing
	m.stopPlaybackLocked()

	m.room.PlayState = &PlayState{
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
	m.notifySSE()
	return nil
}

// CloseRoom closes the listening room and disconnects all listeners.
func (m *Manager) CloseRoom() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.room == nil {
		return nil
	}

	m.stopPlaybackLocked()

	if m.room.Role == "host" {
		m.sendControl(ControlMsg{Action: "close"})
		_ = m.grp.CloseGroup(m.room.ID)
	} else {
		_ = m.grp.LeaveGroup()
	}

	m.closeHTTPPipeLocked()
	m.room = nil
	m.filePath = ""

	log.Printf("LISTEN: Room closed")
	m.notifySSE()
	return nil
}

// ── Listener methods ─────────────────────────────────────────────────────────

// JoinRoom joins a remote listening room.
func (m *Manager) JoinRoom(hostPeerID, roomID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.room != nil {
		return fmt.Errorf("already in a room")
	}

	if err := m.grp.JoinRemoteGroup(context.TODO(), hostPeerID, roomID); err != nil {
		return fmt.Errorf("join group: %w", err)
	}

	m.room = &Room{
		ID:   roomID,
		Name: roomID,
		Role: "listener",
	}

	log.Printf("LISTEN: Joined room %s on host %s", roomID, hostPeerID)
	m.notifySSE()
	return nil
}

// LeaveRoom leaves the current listening room.
func (m *Manager) LeaveRoom() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.room == nil || m.room.Role != "listener" {
		return fmt.Errorf("not in a listening room")
	}

	_ = m.grp.LeaveGroup()
	m.closeHTTPPipeLocked()
	m.room = nil

	log.Printf("LISTEN: Left room")
	m.notifySSE()
	return nil
}

// AudioReader returns an io.ReadCloser that streams audio from the host.
// The caller is responsible for closing it.
func (m *Manager) AudioReader() (io.ReadCloser, error) {
	m.mu.RLock()
	room := m.room
	m.mu.RUnlock()

	if room == nil {
		return nil, fmt.Errorf("not in a room")
	}

	if room.Role == "listener" {
		return m.connectAudioStream()
	}

	// Host can also listen to their own stream (local playback)
	m.httpPipeMu.Lock()
	defer m.httpPipeMu.Unlock()

	if m.httpPipeR != nil {
		// Close old pipe
		m.httpPipeR.Close()
	}

	r, w := io.Pipe()
	m.httpPipeR = r
	m.httpPipeW = w
	return r, nil
}

// GetRoom returns the current room state.
func (m *Manager) GetRoom() *Room {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.room == nil {
		return nil
	}

	// Return a copy with up-to-date position
	r := *m.room
	if r.PlayState != nil && r.PlayState.Playing {
		elapsed := float64(time.Now().UnixMilli()-r.PlayState.UpdatedAt) / 1000.0
		ps := *r.PlayState
		ps.Position += elapsed
		r.PlayState = &ps
	}
	return &r
}

// ── SSE subscription ─────────────────────────────────────────────────────────

// SubscribeSSE returns a channel that receives room state updates.
func (m *Manager) SubscribeSSE() (ch chan *Room, cancel func()) {
	ch = make(chan *Room, 16)

	m.sseMu.Lock()
	m.sseChans[ch] = struct{}{}
	m.sseMu.Unlock()

	cancel = func() {
		m.sseMu.Lock()
		if _, ok := m.sseChans[ch]; ok {
			delete(m.sseChans, ch)
			close(ch)
		}
		m.sseMu.Unlock()
	}
	return ch, cancel
}

func (m *Manager) notifySSE() {
	room := m.room // caller holds mu

	m.sseMu.RLock()
	defer m.sseMu.RUnlock()

	for ch := range m.sseChans {
		select {
		case ch <- room:
		default:
		}
	}
}

// ── Streaming (host → listeners) ─────────────────────────────────────────────

// handleAudioStream processes incoming listen protocol streams from listeners.
// Wire format: "LISTEN <room_id>\n" → host sends raw MP3 bytes.
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
	roomID := parts[1]

	m.mu.RLock()
	room := m.room
	m.mu.RUnlock()

	if room == nil || room.ID != roomID || room.Role != "host" {
		fmt.Fprintf(s, "ERR not found\n")
		return
	}

	if room.Track == nil {
		fmt.Fprintf(s, "ERR no track\n")
		return
	}

	// Send OK with track info
	fmt.Fprintf(s, "OK %s %d %.2f\n", room.Track.Format, room.Track.Bitrate, room.Track.Duration)

	log.Printf("LISTEN: Audio stream started for %s", remotePeer)

	// Open file and seek to current position
	m.mu.RLock()
	pos := 0.0
	if room.PlayState != nil {
		pos = room.PlayState.Position
		if room.PlayState.Playing {
			elapsed := float64(time.Now().UnixMilli()-room.PlayState.UpdatedAt) / 1000.0
			pos += elapsed
		}
	}
	filePath := m.filePath
	paused := m.paused
	gen := m.seekGen
	m.mu.RUnlock()

	if paused {
		// Playback hasn't started yet. Close the stream so the listener
		// knows to retry once a "play" control message arrives.
		fmt.Fprintf(s, "")
		return
	}

	f, err := os.Open(filePath)
	if err != nil {
		log.Printf("LISTEN: Failed to open file for streaming: %v", err)
		return
	}
	defer f.Close()

	// Seek to byte position based on current playback position
	byteOffset := int64(pos * float64(room.Track.Bitrate) / 8.0)
	if byteOffset > 0 {
		f.Seek(byteOffset, io.SeekStart)
	}

	// Create a done channel that closes when room state changes
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				m.mu.RLock()
				currentGen := m.seekGen
				currentPaused := m.paused
				currentRoom := m.room
				m.mu.RUnlock()

				if currentRoom == nil || currentRoom.ID != roomID || currentPaused || currentGen != gen {
					close(done)
					return
				}
			}
		}
	}()

	rp := &ratePacer{
		file:    f,
		bitrate: room.Track.Bitrate,
		done:    done,
	}

	if err := rp.stream(s); err != nil {
		log.Printf("LISTEN: Stream to %s ended: %v", remotePeer, err)
	} else {
		log.Printf("LISTEN: Stream to %s finished", remotePeer)
	}
}

// connectAudioStream opens a listen protocol stream to the host and returns
// a reader for the audio data.
func (m *Manager) connectAudioStream() (io.ReadCloser, error) {
	m.mu.RLock()
	room := m.room
	m.mu.RUnlock()

	if room == nil || room.Role != "listener" {
		return nil, fmt.Errorf("not a listener")
	}

	hostPeerID, _, connected := m.grp.ActiveGroup()
	if !connected {
		return nil, fmt.Errorf("not connected to host")
	}

	pid, err := peer.Decode(hostPeerID)
	if err != nil {
		return nil, fmt.Errorf("invalid host peer ID: %w", err)
	}

	s, err := m.host.NewStream(context.TODO(), pid, protocol.ID(proto.ListenProtoID))
	if err != nil {
		return nil, fmt.Errorf("open stream: %w", err)
	}

	// Send request
	fmt.Fprintf(s, "LISTEN %s\n", room.ID)

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

// startStreaming opens the file and starts rate-pacing audio to the HTTP pipe.
func (m *Manager) startStreaming(position float64) {
	if m.filePath == "" || m.room == nil || m.room.Track == nil {
		return
	}

	// Close old stop channel, create new one
	if m.stopCh != nil {
		select {
		case <-m.stopCh:
		default:
			close(m.stopCh)
		}
	}
	m.stopCh = make(chan struct{})

	if m.file != nil {
		m.file.Close()
	}

	f, err := os.Open(m.filePath)
	if err != nil {
		log.Printf("LISTEN: Failed to open file: %v", err)
		return
	}
	m.file = f

	// Seek to byte position
	byteOffset := int64(position * float64(m.room.Track.Bitrate) / 8.0)
	if byteOffset > 0 {
		f.Seek(byteOffset, io.SeekStart)
	}

	stopCh := m.stopCh

	// Stream to local HTTP pipe if connected
	m.httpPipeMu.Lock()
	httpW := m.httpPipeW
	m.httpPipeMu.Unlock()

	if httpW != nil {
		go func() {
			ff, err := os.Open(m.filePath)
			if err != nil {
				return
			}
			defer ff.Close()
			if byteOffset > 0 {
				ff.Seek(byteOffset, io.SeekStart)
			}
			rp := &ratePacer{file: ff, bitrate: m.room.Track.Bitrate, done: stopCh}
			rp.stream(httpW)
		}()
	}
}

func (m *Manager) stopPlaybackLocked() {
	if m.stopCh != nil {
		select {
		case <-m.stopCh:
		default:
			close(m.stopCh)
		}
	}
	if m.file != nil {
		m.file.Close()
		m.file = nil
	}
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
	if m.room == nil || m.room.PlayState == nil {
		return 0
	}
	pos := m.room.PlayState.Position
	if m.room.PlayState.Playing {
		elapsed := float64(time.Now().UnixMilli()-m.room.PlayState.UpdatedAt) / 1000.0
		pos += elapsed
	}
	return pos
}

func (m *Manager) sendControl(msg ControlMsg) {
	if m.room == nil {
		return
	}
	payload := map[string]any{
		"app_type": "listen",
		"listen":   msg,
	}
	if m.room.Role == "host" {
		_ = m.grp.SendToGroupAsHost(m.room.ID, payload)
	} else {
		_ = m.grp.SendToGroup(payload)
	}
}

// forwardGroupEvents listens to group events and handles listen control messages.
func (m *Manager) forwardGroupEvents() {
	evtCh := m.grp.Subscribe()

	for evt := range evtCh {
		m.mu.RLock()
		room := m.room
		m.mu.RUnlock()

		if room == nil {
			// Check for welcome events with app_type "listen"
			if evt.Type == "welcome" {
				if wp, ok := evt.Payload.(map[string]any); ok {
					if appType, _ := wp["app_type"].(string); appType == "listen" {
						m.mu.Lock()
						if m.room != nil && m.room.ID == evt.Group {
							if name, ok := wp["group_name"].(string); ok {
								m.room.Name = name
							}
						}
						m.mu.Unlock()
						m.notifySSELocked()
					}
				}
			}
			continue
		}

		if evt.Group != room.ID {
			continue
		}

		// Skip own messages
		if evt.From == m.selfID {
			continue
		}

		switch evt.Type {
		case "msg":
			m.handleControlEvent(evt.Payload)
		case "close":
			m.mu.Lock()
			m.closeHTTPPipeLocked()
			m.room = nil
			m.mu.Unlock()
			m.notifySSELocked()
			log.Printf("LISTEN: Room closed by host")
		case "leave":
			if room.Role == "host" {
				// A listener left
				log.Printf("LISTEN: Listener %s left", evt.From)
			}
		case "members":
			// Update listener list
			if room.Role == "host" {
				if mp, ok := evt.Payload.(map[string]any); ok {
					if members, ok := mp["members"].([]any); ok {
						m.mu.Lock()
						m.room.Listeners = make([]string, 0, len(members))
						for _, member := range members {
							if mi, ok := member.(map[string]any); ok {
								if pid, ok := mi["peer_id"].(string); ok && pid != m.selfID {
									m.room.Listeners = append(m.room.Listeners, pid)
								}
							}
						}
						m.mu.Unlock()
						m.notifySSELocked()
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

	if m.room == nil {
		return
	}

	switch ctrl.Action {
	case "load":
		m.room.Track = ctrl.Track
		m.room.PlayState = &PlayState{
			Playing:   false,
			Position:  0,
			UpdatedAt: time.Now().UnixMilli(),
		}
		log.Printf("LISTEN: Host loaded track: %s", ctrl.Track.Name)

	case "play":
		m.room.PlayState = &PlayState{
			Playing:   true,
			Position:  ctrl.Position,
			UpdatedAt: time.Now().UnixMilli(),
		}
		log.Printf("LISTEN: Host started playback at %.1fs", ctrl.Position)

	case "pause":
		m.room.PlayState = &PlayState{
			Playing:   false,
			Position:  ctrl.Position,
			UpdatedAt: time.Now().UnixMilli(),
		}
		log.Printf("LISTEN: Host paused at %.1fs", ctrl.Position)

	case "seek":
		wasPlaying := m.room.PlayState != nil && m.room.PlayState.Playing
		m.room.PlayState = &PlayState{
			Playing:   wasPlaying,
			Position:  ctrl.Position,
			UpdatedAt: time.Now().UnixMilli(),
		}
		// Close existing audio pipe so the viewer reconnects
		m.closeHTTPPipeLocked()
		log.Printf("LISTEN: Host seeked to %.1fs", ctrl.Position)

	case "sync":
		m.room.PlayState = &PlayState{
			Playing:   true,
			Position:  ctrl.Position,
			UpdatedAt: time.Now().UnixMilli(),
		}

	case "close":
		m.closeHTTPPipeLocked()
		m.room = nil
		log.Printf("LISTEN: Room closed by host")
	}

	m.notifySSE()
}

func (m *Manager) notifySSELocked() {
	m.mu.RLock()
	room := m.room
	m.mu.RUnlock()

	m.sseMu.RLock()
	defer m.sseMu.RUnlock()

	for ch := range m.sseChans {
		select {
		case ch <- room:
		default:
		}
	}
}

// Close shuts down the listen manager.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.stopPlaybackLocked()
	m.closeHTTPPipeLocked()

	m.sseMu.Lock()
	for ch := range m.sseChans {
		close(ch)
	}
	m.sseChans = nil
	m.sseMu.Unlock()

	m.room = nil
}

func generateRoomID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "listen-" + hex.EncodeToString(b)
}
