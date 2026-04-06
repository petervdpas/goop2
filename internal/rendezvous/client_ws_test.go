package rendezvous

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/petervdpas/goop2/internal/proto"
)

func TestIsWSUnsupported(t *testing.T) {
	cases := []struct {
		err  string
		want bool
	}{
		{"bad handshake", true},
		{"ws dial wss://x (status 404): bad handshake", true},
		{"ws dial wss://x (status 403): forbidden", true},
		{"ws dial wss://x (status 501): not implemented", true},
		{"ws read: connection reset", false},
		{"ws dial wss://x (status 425): too early", false},
	}
	for _, tc := range cases {
		if got := isWSUnsupported(errors.New(tc.err)); got != tc.want {
			t.Errorf("isWSUnsupported(%q) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

func TestIsWSTooEarly(t *testing.T) {
	cases := []struct {
		err  string
		want bool
	}{
		{"ws dial wss://x (status 425): too early", true},
		{"ws dial wss://x (status 404): bad handshake", false},
		{"connection refused", false},
	}
	for _, tc := range cases {
		if got := isWSTooEarly(errors.New(tc.err)); got != tc.want {
			t.Errorf("isWSTooEarly(%q) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

func TestWsBase(t *testing.T) {
	cases := []struct {
		baseURL string
		want    string
	}{
		{"https://goop2.com", "wss://goop2.com/ws"},
		{"http://127.0.0.1:8787", "ws://127.0.0.1:8787/ws"},
		{"http://localhost:8787", "ws://localhost:8787/ws"},
		{"http://::1:8787", "wss://::1:8787/ws"},
		{"http://192.168.1.100:8787", "wss://192.168.1.100:8787/ws"},
	}
	for _, tc := range cases {
		c := &Client{BaseURL: tc.baseURL}
		if got := c.wsBase(); got != tc.want {
			t.Errorf("wsBase(%q) = %q, want %q", tc.baseURL, got, tc.want)
		}
	}
}

func TestWsURL(t *testing.T) {
	c := &Client{BaseURL: "https://goop2.com"}
	got := c.wsURL("peer-123")
	want := "wss://goop2.com/ws?peer_id=peer-123"
	if got != want {
		t.Errorf("wsURL = %q, want %q", got, want)
	}
}

func TestWsProbeURL(t *testing.T) {
	c := &Client{BaseURL: "https://goop2.com"}
	got := c.wsProbeURL()
	want := "wss://goop2.com/ws?probe=1"
	if got != want {
		t.Errorf("wsProbeURL = %q, want %q", got, want)
	}
}

func TestPublishWS_NoConnection(t *testing.T) {
	c := &Client{}
	if c.PublishWS(proto.PresenceMsg{Type: proto.TypeOnline, PeerID: "p"}) {
		t.Error("PublishWS should return false when no WS connected")
	}
}

func TestPublishWS_BufferFull(t *testing.T) {
	ch := make(chan []byte) // unbuffered = always full
	c := &Client{wsSend: ch}
	if c.PublishWS(proto.PresenceMsg{Type: proto.TypeOnline, PeerID: "p"}) {
		t.Error("PublishWS should return false when send buffer is full")
	}
}

func TestPublishWS_Success(t *testing.T) {
	ch := make(chan []byte, 1)
	c := &Client{wsSend: ch}
	if !c.PublishWS(proto.PresenceMsg{Type: proto.TypeOnline, PeerID: "p"}) {
		t.Error("PublishWS should return true when message is queued")
	}
	if len(ch) != 1 {
		t.Error("expected message in send channel")
	}
}

func TestSubscribeOnce_ParsesSSE(t *testing.T) {
	msgs := []proto.PresenceMsg{
		{Type: proto.TypeOnline, PeerID: "peer-1", Content: "peer-1"},
		{Type: proto.TypeUpdate, PeerID: "peer-2", Content: "peer-2"},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		fmt.Fprintln(w, ": comment, should be ignored")
		flusher.Flush()
		for _, m := range msgs {
			b, _ := json.Marshal(m)
			fmt.Fprintf(w, "data: %s\n\n", b)
			flusher.Flush()
		}
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL}
	var got []proto.PresenceMsg
	var mu sync.Mutex
	err := c.subscribeOnce(context.Background(), func(pm proto.PresenceMsg) {
		mu.Lock()
		got = append(got, pm)
		mu.Unlock()
	})
	if err != nil {
		t.Fatalf("subscribeOnce: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(got))
	}
	if got[0].PeerID != "peer-1" || got[1].PeerID != "peer-2" {
		t.Errorf("unexpected messages: %+v", got)
	}
}

func TestSubscribeOnce_SkipsInvalidMessages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		fmt.Fprintln(w, "data: not-json")
		flusher.Flush()
		fmt.Fprintln(w, "data: {}")
		flusher.Flush()
		b, _ := json.Marshal(proto.PresenceMsg{Type: "", PeerID: "p"})
		fmt.Fprintf(w, "data: %s\n", b)
		flusher.Flush()
		b, _ = json.Marshal(proto.PresenceMsg{Type: proto.TypeOnline, PeerID: ""})
		fmt.Fprintf(w, "data: %s\n", b)
		flusher.Flush()
		b, _ = json.Marshal(proto.PresenceMsg{Type: proto.TypeOnline, PeerID: "valid"})
		fmt.Fprintf(w, "data: %s\n", b)
		flusher.Flush()
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL}
	var got []proto.PresenceMsg
	c.subscribeOnce(context.Background(), func(pm proto.PresenceMsg) {
		got = append(got, pm)
	})
	if len(got) != 1 || got[0].PeerID != "valid" {
		t.Errorf("expected 1 valid message (peer=valid), got %+v", got)
	}
}

func TestSubscribeOnce_NonSuccessStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL}
	err := c.subscribeOnce(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for non-2xx status")
	}
	if !strings.Contains(err.Error(), "503") {
		t.Errorf("expected 503 in error, got: %v", err)
	}
}

func TestSubscribeEvents_ReconnectsOnFailure(t *testing.T) {
	var attempts int
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attempts++
		n := attempts
		mu.Unlock()
		if n <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		b, _ := json.Marshal(proto.PresenceMsg{Type: proto.TypeOnline, PeerID: "p1", Content: "p1"})
		fmt.Fprintf(w, "data: %s\n\n", b)
		flusher.Flush()
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	received := make(chan proto.PresenceMsg, 1)
	go c.SubscribeEvents(ctx, func(pm proto.PresenceMsg) {
		select {
		case received <- pm:
		default:
		}
		cancel()
	})

	select {
	case pm := <-received:
		if pm.PeerID != "p1" {
			t.Errorf("unexpected peer: %s", pm.PeerID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for SSE message after reconnects")
	}

	mu.Lock()
	if attempts < 3 {
		t.Errorf("expected at least 3 attempts, got %d", attempts)
	}
	mu.Unlock()
}

func TestConnectWebSocket_ReceivesMessages(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ws" {
			http.NotFound(w, r)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		msg := proto.PresenceMsg{Type: proto.TypeOnline, PeerID: "remote", Content: "remote"}
		b, _ := json.Marshal(msg)
		conn.WriteMessage(websocket.TextMessage, b)
		time.Sleep(100 * time.Millisecond)
		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	received := make(chan proto.PresenceMsg, 1)
	go c.ConnectWebSocket(ctx, "local-peer", func(pm proto.PresenceMsg) {
		select {
		case received <- pm:
		default:
		}
		cancel()
	})

	select {
	case pm := <-received:
		if pm.PeerID != "remote" {
			t.Errorf("expected remote, got %s", pm.PeerID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for WS message")
	}
}

func TestConnectWebSocket_FallsBackToSSE(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

	var wsMu sync.Mutex
	wsEnabled := false

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ws" {
			wsMu.Lock()
			enabled := wsEnabled
			wsMu.Unlock()
			if !enabled {
				http.Error(w, "not implemented", http.StatusNotImplemented)
				return
			}
			if r.URL.Query().Get("probe") == "1" {
				conn, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					return
				}
				conn.WriteMessage(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, "probe-ok"))
				conn.Close()
				return
			}
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()
			msg := proto.PresenceMsg{Type: proto.TypeUpdate, PeerID: "ws-peer", Content: "ws-peer"}
			b, _ := json.Marshal(msg)
			conn.WriteMessage(websocket.TextMessage, b)
			time.Sleep(200 * time.Millisecond)
		}
		if r.URL.Path == "/events" {
			w.Header().Set("Content-Type", "text/event-stream")
			flusher := w.(http.Flusher)
			msg := proto.PresenceMsg{Type: proto.TypeOnline, PeerID: "sse-peer", Content: "sse-peer"}
			b, _ := json.Marshal(msg)
			fmt.Fprintf(w, "data: %s\n\n", b)
			flusher.Flush()
			// Keep connection open briefly so SSE doesn't reconnect loop too fast
			time.Sleep(500 * time.Millisecond)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	received := make(chan proto.PresenceMsg, 4)
	go c.ConnectWebSocket(ctx, "local", func(pm proto.PresenceMsg) {
		received <- pm
	})

	// Should receive SSE message after WS fallback
	select {
	case pm := <-received:
		if pm.PeerID != "sse-peer" {
			t.Errorf("expected sse-peer during fallback, got %s", pm.PeerID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for SSE fallback message")
	}

	// Enable WS so the probe succeeds and triggers switch-back
	wsMu.Lock()
	wsEnabled = true
	wsMu.Unlock()

	// Should eventually receive WS message after probe detects WS availability
	deadline := time.After(8 * time.Second)
	for {
		select {
		case pm := <-received:
			if pm.PeerID == "ws-peer" {
				cancel()
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for WS switch-back message")
		}
	}
}

func TestConnectWebSocket_425Retry(t *testing.T) {
	var attempts int
	var mu sync.Mutex
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ws" {
			http.NotFound(w, r)
			return
		}
		mu.Lock()
		attempts++
		n := attempts
		mu.Unlock()
		if n == 1 {
			http.Error(w, "publish first", http.StatusTooEarly)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		msg := proto.PresenceMsg{Type: proto.TypeOnline, PeerID: "p", Content: "p"}
		b, _ := json.Marshal(msg)
		conn.WriteMessage(websocket.TextMessage, b)
		time.Sleep(100 * time.Millisecond)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	received := make(chan proto.PresenceMsg, 1)
	go c.ConnectWebSocket(ctx, "local", func(pm proto.PresenceMsg) {
		select {
		case received <- pm:
		default:
		}
		cancel()
	})

	select {
	case pm := <-received:
		if pm.PeerID != "p" {
			t.Errorf("expected p, got %s", pm.PeerID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for message after 425 retry")
	}

	mu.Lock()
	if attempts < 2 {
		t.Errorf("expected at least 2 attempts (425 then success), got %d", attempts)
	}
	mu.Unlock()
}

func TestConnectWebSocket_ReconnectsOnClose(t *testing.T) {
	var attempts int
	var mu sync.Mutex
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ws" {
			http.NotFound(w, r)
			return
		}
		mu.Lock()
		attempts++
		n := attempts
		mu.Unlock()

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		msg := proto.PresenceMsg{Type: proto.TypeOnline, PeerID: fmt.Sprintf("p-%d", n), Content: fmt.Sprintf("p-%d", n)}
		b, _ := json.Marshal(msg)
		conn.WriteMessage(websocket.TextMessage, b)
		time.Sleep(50 * time.Millisecond)
		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	received := make(chan string, 10)
	go c.ConnectWebSocket(ctx, "local", func(pm proto.PresenceMsg) {
		received <- pm.PeerID
	})

	seen := make(map[string]bool)
	deadline := time.After(4 * time.Second)
	for len(seen) < 2 {
		select {
		case pid := <-received:
			seen[pid] = true
		case <-deadline:
			cancel()
			t.Fatalf("expected at least 2 distinct connections, got %d: %v", len(seen), seen)
		}
	}
	cancel()
}

func TestIsWebSocketConnected(t *testing.T) {
	c := &Client{}
	if c.IsWebSocketConnected() {
		t.Error("should be false initially")
	}
}
