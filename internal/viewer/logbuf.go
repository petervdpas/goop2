// internal/viewer/logbuf.go
package viewer

import (
	"bytes"
	"encoding/json"
	"github.com/petervdpas/goop2/internal/util"
	"net/http"
	"strings"
	"sync"
	"time"
)

type LogEntry struct {
	TS  time.Time `json:"ts"`
	Msg string    `json:"msg"`
}

type LogBuffer struct {
	mu      sync.Mutex
	entries *util.RingBuffer[LogEntry]

	subs map[chan LogEntry]struct{}

	partial bytes.Buffer
}

func NewLogBuffer(max int) *LogBuffer {
	if max <= 0 {
		max = 500
	}
	return &LogBuffer{
		entries: util.NewRingBuffer[LogEntry](max),
		subs:    make(map[chan LogEntry]struct{}),
	}
}

// Write implements io.Writer for log.SetOutput/io.MultiWriter.
func (b *LogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.partial.Write(p)

	for {
		data := b.partial.Bytes()
		i := bytes.IndexByte(data, '\n')
		if i == -1 {
			break
		}

		line := string(data[:i])
		b.partial.Next(i + 1)

		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}

		e := LogEntry{TS: time.Now(), Msg: line}
		b.appendLocked(e)
		b.broadcastLocked(e)
	}

	return len(p), nil
}

func (b *LogBuffer) appendLocked(e LogEntry) {
	b.entries.Push(e)
}

func (b *LogBuffer) broadcastLocked(e LogEntry) {
	for ch := range b.subs {
		select {
		case ch <- e:
		default:
			// drop on slow subscriber
		}
	}
}

func (b *LogBuffer) Snapshot() []LogEntry {
	return b.entries.Snapshot()
}

func (b *LogBuffer) Subscribe() (ch chan LogEntry, cancel func()) {
	ch = make(chan LogEntry, 64)

	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()

	cancel = func() {
		b.mu.Lock()
		if _, ok := b.subs[ch]; ok {
			delete(b.subs, ch)
			close(ch)
		}
		b.mu.Unlock()
	}
	return ch, cancel
}

// GET /api/logs
func (b *LogBuffer) ServeLogsJSON(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(b.Snapshot())
}

// GET /api/logs/stream  (Server-Sent Events) - tail only (no snapshot)
func (b *LogBuffer) ServeLogsSSE(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch, cancel := b.Subscribe()
	defer cancel()

	// NO snapshot here. Tail only.
	for {
		select {
		case <-r.Context().Done():
			return
		case e, ok := <-ch:
			if !ok {
				return
			}
			writeSSE(w, e)
			flusher.Flush()
		}
	}
}

func writeSSE(w http.ResponseWriter, e LogEntry) {
	b, _ := json.Marshal(e)
	_, _ = w.Write([]byte("event: message\n"))
	_, _ = w.Write([]byte("data: " + string(b) + "\n\n"))
}
