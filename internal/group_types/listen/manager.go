// Package listen implements a listening group — a host streams audio in
// real-time to connected listeners via the group protocol (control) and
// a dedicated binary stream protocol (audio data).
package listen

import (
	"io"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/petervdpas/goop2/internal/group"
	"github.com/petervdpas/goop2/internal/mq"

	"github.com/libp2p/go-libp2p/core/host"
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

	// Optional encryptor for audio stream chunks.
	enc ListenEncryptor
}

// ListenEncryptor encrypts and decrypts audio stream chunks.
type ListenEncryptor interface {
	Seal(peerID string, plaintext []byte) (string, error)
	Open(peerID string, ciphertextB64 string) ([]byte, error)
}

// SetEncryptor sets the optional audio stream encryptor.
func (m *Manager) SetEncryptor(e ListenEncryptor) {
	m.enc = e
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

func isStreamURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

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

// GetGroup returns the current group state.
func (m *Manager) GetGroup() *Group {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.group == nil {
		return nil
	}

	r := *m.group
	if r.PlayState != nil && r.PlayState.Playing {
		elapsed := float64(time.Now().UnixMilli()-r.PlayState.UpdatedAt) / 1000.0
		ps := *r.PlayState
		ps.Position += elapsed
		r.PlayState = &ps
	}
	return &r
}

func (m *Manager) notifyBrowser() {
	groupID := ""
	if m.group != nil {
		groupID = m.group.ID
	}
	m.mq.PublishLocal("listen:"+groupID+":state", "", map[string]any{"group": m.group})
}

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

func (m *Manager) startStreaming(_ float64) {
	if m.filePath == "" || m.group == nil || m.group.Track == nil {
		return
	}

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

// Close shuts down the listen manager.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.stopPlaybackLocked()
	m.closeHTTPPipeLocked()

	m.group = nil
}
