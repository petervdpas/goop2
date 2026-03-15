package listen

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"

	"github.com/petervdpas/goop2/internal/proto"
)

// JoinGroup joins a remote listening group.
func (m *Manager) JoinGroup(hostPeerID, groupID string) error {
	m.mu.Lock()
	lg := m.group

	if lg != nil && lg.Role == "host" {
		m.mu.Unlock()
		return fmt.Errorf("already hosting a listen group")
	}

	// Auto-leave current listener group before joining new one.
	if lg != nil && lg.Role == "listener" {
		m.closeHTTPPipeLocked()
		m.mu.Unlock()
		_ = m.grp.LeaveGroup(lg.ID)
	} else {
		m.mu.Unlock()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := m.grp.JoinRemoteGroup(ctx, hostPeerID, groupID); err != nil {
		return err
	}

	m.mu.Lock()
	m.group = &Group{
		ID:   groupID,
		Name: groupID,
		Role: "listener",
	}
	m.mu.Unlock()

	log.Printf("LISTEN: Joined group %s as listener", groupID)
	m.notifyBrowserLocked()
	return nil
}

// LeaveGroup leaves the current listening group.
func (m *Manager) LeaveGroup() error {
	m.mu.Lock()
	lg := m.group

	if lg == nil || lg.Role != "listener" {
		m.mu.Unlock()
		return fmt.Errorf("not in a listening group")
	}

	m.closeHTTPPipeLocked()
	m.group = nil
	m.mu.Unlock()

	log.Printf("LISTEN: Left group %s", lg.ID)
	m.notifyBrowserLocked()

	return m.grp.LeaveGroup(lg.ID)
}

// AudioReader returns an io.ReadCloser that streams audio from the host.
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
	m.httpPipeMu.Lock()
	if m.httpPipeR != nil {
		m.httpPipeR.Close()
	}
	r, w := io.Pipe()
	m.httpPipeR = r
	m.httpPipeW = w
	m.httpPipeMu.Unlock()

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

		if isStreamURL(filePath) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

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
				return
			}
			defer resp.Body.Close()
			buf := make([]byte, 32*1024)
			io.CopyBuffer(httpW, resp.Body, buf) //nolint:errcheck
			return
		}

		ff, err := os.Open(filePath)
		if err != nil {
			return
		}
		defer ff.Close()
		byteOffset := int64(pos * float64(bitrate) / 8.0)
		if byteOffset > 0 {
			ff.Seek(byteOffset, io.SeekStart) //nolint:errcheck
		}
		buf := make([]byte, 32*1024)
		io.CopyBuffer(httpW, ff, buf) //nolint:errcheck
	}()

	return r, nil
}

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

	fmt.Fprintf(s, "LISTEN %s\n", lg.ID)

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

	if strings.HasPrefix(line, "EAOK") && m.enc != nil {
		return &decryptingReader{stream: s, enc: m.enc, peerID: hostPeerID}, nil
	}

	if !strings.HasPrefix(line, "OK") {
		s.Close()
		return nil, fmt.Errorf("unexpected response: %s", line)
	}

	return s, nil
}

type decryptingReader struct {
	stream network.Stream
	enc    ListenEncryptor
	peerID string
	buf    []byte
	pos    int
}

func (r *decryptingReader) Read(p []byte) (int, error) {
	if r.pos < len(r.buf) {
		n := copy(p, r.buf[r.pos:])
		r.pos += n
		return n, nil
	}

	header := make([]byte, 4)
	if _, err := io.ReadFull(r.stream, header); err != nil {
		return 0, err
	}
	size := binary.BigEndian.Uint32(header)
	if size > 10*1024*1024 {
		return 0, fmt.Errorf("encrypted audio chunk too large: %d", size)
	}

	ciphertext := make([]byte, size)
	if _, err := io.ReadFull(r.stream, ciphertext); err != nil {
		return 0, err
	}

	plaintext, err := r.enc.Open(r.peerID, string(ciphertext))
	if err != nil {
		return 0, fmt.Errorf("decrypt audio chunk: %w", err)
	}

	r.buf = plaintext
	r.pos = 0
	n := copy(p, r.buf)
	r.pos = n
	return n, nil
}

func (r *decryptingReader) Close() error {
	return r.stream.Close()
}
