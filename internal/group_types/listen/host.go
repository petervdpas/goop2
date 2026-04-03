package listen

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/petervdpas/goop2/internal/group"
	"github.com/petervdpas/goop2/internal/mq"
	"github.com/petervdpas/goop2/internal/proto"

	libhost "github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
)

// New creates a new listen manager. It registers the binary stream handler
// and subscribes to group events for listen control messages.
func New(h libhost.Host, grp *group.Manager, mqMgr *mq.Manager, selfID, dataDir string) *Manager {
	m := &Manager{
		host:   h,
		grp:    grp,
		mq:     mqMgr,
		selfID: selfID,
		store:  newStateStore(dataDir),
		pipes:  make(map[string]*listenerPipe),
	}

	// Recover any listen group left over from a previous session.
	if rows, err := grp.ListHostedGroups(); err == nil {
		for _, g := range rows {
			if strings.HasPrefix(g.ID, "listen-") && m.group == nil {
				_ = grp.JoinOwnGroup(g.ID)
				m.group = &Group{
					ID:   g.ID,
					Name: g.Name,
					Role: "host",
				}
				m.paused = true
				m.stopCh = make(chan struct{})
				log.Printf("LISTEN: Recovered group %s (%s) from previous session", g.ID, g.Name)

				if qs := m.loadQueueFromDisk(); qs != nil && len(qs.Paths) > 0 {
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

// CreateGroup creates a new listening group. Only one group at a time.
// The listen-specific setup happens in the OnCreate lifecycle hook.
func (m *Manager) CreateGroup(name string) (*Group, error) {
	id := generateListenID()
	if err := m.grp.CreateGroup(id, name, "listen", name, 0); err != nil {
		return nil, fmt.Errorf("create group: %w", err)
	}
	if err := m.grp.JoinOwnGroup(id); err != nil {
		m.grp.CloseGroup(id) //nolint:errcheck
		return nil, fmt.Errorf("join own group: %w", err)
	}
	return m.GetGroup(), nil
}

// LoadTrack loads a single MP3 file, replacing any existing queue.
func (m *Manager) LoadTrack(filePath string) (*Track, error) {
	return m.LoadQueue([]string{filePath})
}

// LoadQueue loads one or more MP3 files as a playlist.
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

// AddToQueue appends one or more files to the playlist.
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
		m.stopPlaybackLocked()
		m.queue = paths
		m.queueIdx = 0
		_, err := m.loadTrackAtLocked(0)
		m.saveQueueToDisk()
		return err
	}

	m.queue = append(m.queue, paths...)
	m.updateQueueInfoLocked()
	m.saveQueueToDisk()
	m.notifyBrowser()
	return nil
}

func (m *Manager) loadTrackAtLocked(idx int) (*Track, error) {
	if idx >= len(m.queue) {
		return nil, fmt.Errorf("queue index out of range")
	}
	filePath := m.queue[idx]

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

// Next skips to the next track.
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
func (m *Manager) RemoveFromQueue(idx int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.group == nil || m.group.Role != "host" {
		return fmt.Errorf("not hosting a group")
	}
	if idx < 0 || idx >= len(m.queue) {
		return fmt.Errorf("invalid queue index")
	}

	newQueue := make([]string, 0, len(m.queue)-1)
	newQueue = append(newQueue, m.queue[:idx]...)
	newQueue = append(newQueue, m.queue[idx+1:]...)
	m.queue = newQueue

	if idx == m.queueIdx {
		wasPlaying := !m.paused
		m.stopPlaybackLocked()

		if len(m.queue) == 0 {
			m.queueIdx = 0
			m.filePath = ""
			m.group.Track = nil
			m.group.Queue = nil
			m.group.QueueIndex = 0
			m.group.QueueTotal = 0
			m.group.PlayState = &PlayState{Playing: false, Position: 0, UpdatedAt: time.Now().UnixMilli()}
			m.sendControl(ControlMsg{Action: "pause", Position: 0})
		} else {
			if m.queueIdx >= len(m.queue) {
				m.queueIdx = len(m.queue) - 1
			}

			_, err := m.loadTrackAtLocked(m.queueIdx)
			if err != nil {
				m.queueIdx = 0
				m.loadTrackAtLocked(0) //nolint:errcheck
			}

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

func (m *Manager) advanceQueue() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.group == nil || m.group.Role != "host" {
		return
	}

	for {
		nextIdx := m.queueIdx + 1
		if nextIdx >= len(m.queue) {
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

const syncPulseTicks = 10

func (m *Manager) trackTimerGoroutine(stopCh chan struct{}) {
	ticker := time.NewTicker(StreamPollInterval)
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

			if g == nil || paused || g.PlayState == nil || !g.PlayState.Playing || g.Track == nil {
				if paused {
					log.Printf("LISTEN: Timer exiting because paused=true")
				}
				return
			}

			elapsed := float64(time.Now().UnixMilli()-g.PlayState.UpdatedAt) / 1000.0
			pos := g.PlayState.Position + elapsed
			if g.Track.Duration > 0 && pos >= g.Track.Duration {
				log.Printf("LISTEN: Track ended, advancing queue")
				m.advanceQueue()
				return
			}

			if ticks%syncPulseTicks == 0 {
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
	m.startStreaming(pos)

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
// The listen-specific cleanup happens in the OnClose lifecycle hook.
func (m *Manager) CloseGroup() error {
	m.mu.RLock()
	lg := m.group
	m.mu.RUnlock()

	if lg == nil {
		return nil
	}

	m.sendControl(ControlMsg{Action: "close"})

	if lg.Role == "host" {
		return m.grp.CloseGroup(lg.ID)
	}
	return m.grp.LeaveGroup(lg.ID)
}

// writeAudioChunk writes audio data to the stream, encrypting if needed.
func (m *Manager) writeAudioChunk(s network.Stream, peerID string, encrypted bool, data []byte) error {
	if encrypted && m.enc != nil {
		sealed, err := m.enc.Seal(peerID, data)
		if err == nil {
			sealedBytes := []byte(sealed)
			header := make([]byte, 4)
			binary.BigEndian.PutUint32(header, uint32(len(sealedBytes)))
			if _, err := s.Write(header); err != nil {
				return err
			}
			for len(sealedBytes) > 0 {
				nw, err := s.Write(sealedBytes)
				if err != nil {
					return err
				}
				sealedBytes = sealedBytes[nw:]
			}
			return nil
		}
	}
	for len(data) > 0 {
		nw, err := s.Write(data)
		if err != nil {
			return err
		}
		data = data[nw:]
	}
	return nil
}

func (m *Manager) handleAudioStream(s network.Stream) {
	remotePeer := s.Conn().RemotePeer().String()
	defer s.Close()

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

	encrypted := false
	if m.enc != nil {
		if _, err := m.enc.Seal(remotePeer, []byte("test")); err == nil {
			encrypted = true
		}
	}
	if encrypted {
		fmt.Fprintf(s, "EAOK %s %d %.2f\n", lg.Track.Format, lg.Track.Bitrate, lg.Track.Duration)
	} else {
		fmt.Fprintf(s, "OK %s %d %.2f\n", lg.Track.Format, lg.Track.Bitrate, lg.Track.Duration)
	}

	log.Printf("LISTEN: Audio stream started for %s", remotePeer)

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
		fmt.Fprintf(s, "")
		return
	}

	if isStreamURL(filePath) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go func() {
			ticker := time.NewTicker(StreamPollInterval)
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
				if werr := m.writeAudioChunk(s, remotePeer, encrypted, audioBuffer[:n]); werr != nil {
					log.Printf("LISTEN: Stream to %s ended (write error): %v", remotePeer, werr)
					return
				}
			}
			if err != nil {
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

	byteOffset := int64(pos * float64(lg.Track.Bitrate) / 8.0)
	if byteOffset > 0 {
		f.Seek(byteOffset, io.SeekStart) //nolint:errcheck
	}

	audioBuffer := make([]byte, 64*1024)
	checkCounter := 0
	for {
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
			if werr := m.writeAudioChunk(s, remotePeer, encrypted, audioBuffer[:n]); werr != nil {
				log.Printf("LISTEN: Stream to %s ended (write error): %v", remotePeer, werr)
				return
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
