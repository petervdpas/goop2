package bridge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/petervdpas/goop2/internal/state"
)

func TestNew(t *testing.T) {
	peers := state.NewPeerTable()
	c := New("https://bridge.example.com/", "a@b.com", "tok", "peer-1", "Alice", "pk", true, peers)

	if c.bridgeURL != "https://bridge.example.com" {
		t.Errorf("bridgeURL = %q (trailing slash not trimmed)", c.bridgeURL)
	}
	if c.email != "a@b.com" {
		t.Errorf("email = %q", c.email)
	}
	if c.peerID != "peer-1" {
		t.Errorf("peerID = %q", c.peerID)
	}
	if c.dns == nil {
		t.Error("dns cache should be initialized")
	}
	if c.httpClient == nil {
		t.Error("httpClient should be initialized")
	}
}

func TestRegister_Success(t *testing.T) {
	var gotHeaders http.Header
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header.Clone()
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"session_id": "sess-123"})
	}))
	defer srv.Close()

	c := New(srv.URL, "alice@test.com", "my-token", "peer-1", "Alice", "pk-xyz", true, state.NewPeerTable())

	sessionID, err := c.Register(context.Background())
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if sessionID != "sess-123" {
		t.Errorf("sessionID = %q", sessionID)
	}
	if gotHeaders.Get("X-Goop-Email") != "alice@test.com" {
		t.Errorf("X-Goop-Email = %q", gotHeaders.Get("X-Goop-Email"))
	}
	if gotHeaders.Get("X-Bridge-Token") != "my-token" {
		t.Errorf("X-Bridge-Token = %q", gotHeaders.Get("X-Bridge-Token"))
	}
	if gotBody["peer_id"] != "peer-1" {
		t.Errorf("peer_id = %v", gotBody["peer_id"])
	}
	if gotBody["label"] != "Alice" {
		t.Errorf("label = %v", gotBody["label"])
	}
}

func TestRegister_NonCreatedStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := New(srv.URL, "a@b.com", "tok", "p", "L", "", false, state.NewPeerTable())
	_, err := c.Register(context.Background())
	if err == nil {
		t.Error("expected error for non-201 status")
	}
}

func TestConnectOnce_ReceivesPresence(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		evt := map[string]any{
			"type": "presence",
			"data": map[string]string{"peer": "remote-1"},
		}
		b, _ := json.Marshal(evt)
		conn.WriteMessage(websocket.TextMessage, b)
		time.Sleep(100 * time.Millisecond)
		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	}))
	defer srv.Close()

	c := New(srv.URL, "a@b.com", "tok", "peer-1", "Alice", "", false, state.NewPeerTable())

	var gotData json.RawMessage
	var mu sync.Mutex
	err := c.connectOnce(context.Background(), func(data json.RawMessage) {
		mu.Lock()
		gotData = data
		mu.Unlock()
	})
	if err == nil {
		t.Log("connectOnce returned nil (server closed normally)")
	}

	mu.Lock()
	defer mu.Unlock()
	if gotData == nil {
		t.Fatal("expected presence callback")
	}
	var parsed map[string]string
	json.Unmarshal(gotData, &parsed)
	if parsed["peer"] != "remote-1" {
		t.Errorf("peer = %q", parsed["peer"])
	}
}

func TestConnectOnce_IgnoresUnknownEventTypes(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		unknown := map[string]any{"type": "unknown", "data": "x"}
		b, _ := json.Marshal(unknown)
		conn.WriteMessage(websocket.TextMessage, b)

		evt := map[string]any{"type": "presence", "data": "valid"}
		b, _ = json.Marshal(evt)
		conn.WriteMessage(websocket.TextMessage, b)
		time.Sleep(50 * time.Millisecond)
		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	}))
	defer srv.Close()

	c := New(srv.URL, "a@b.com", "tok", "p", "L", "", false, state.NewPeerTable())

	callCount := 0
	c.connectOnce(context.Background(), func(data json.RawMessage) {
		callCount++
	})
	if callCount != 1 {
		t.Errorf("expected 1 presence callback, got %d", callCount)
	}
}

func TestConnect_ReconnectsAfterFailure(t *testing.T) {
	var mu sync.Mutex
	attempts := 0
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/bridge/peers" {
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{"session_id": "s"})
			return
		}

		mu.Lock()
		attempts++
		n := attempts
		mu.Unlock()

		if n == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		evt := map[string]any{"type": "presence", "data": "ok"}
		b, _ := json.Marshal(evt)
		conn.WriteMessage(websocket.TextMessage, b)
		time.Sleep(100 * time.Millisecond)
	}))
	defer srv.Close()

	c := New(srv.URL, "a@b.com", "tok", "peer-1", "Alice", "", false, state.NewPeerTable())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	received := make(chan struct{}, 1)
	go c.Connect(ctx, func(data json.RawMessage) {
		select {
		case received <- struct{}{}:
		default:
		}
		cancel()
	})

	select {
	case <-received:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for reconnect")
	}

	mu.Lock()
	if attempts < 2 {
		t.Errorf("expected at least 2 connection attempts, got %d", attempts)
	}
	mu.Unlock()
}
