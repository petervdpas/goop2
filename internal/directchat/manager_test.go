package directchat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// ── Mock store ───────────────────────────────────────────────────────────────

type mockStore struct {
	mu   sync.Mutex
	msgs map[string][]Message // keyed by peerID
}

func newMockStore() *mockStore {
	return &mockStore{msgs: make(map[string][]Message)}
}

func (s *mockStore) StoreChatMessage(peerID, fromID, content string, ts int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.msgs[peerID] = append(s.msgs[peerID], Message{From: fromID, Content: content, Timestamp: ts})
	return nil
}

func (s *mockStore) GetChatHistory(peerID string, limit int) ([]Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	msgs := s.msgs[peerID]
	if msgs == nil {
		return []Message{}, nil
	}
	if limit > 0 && len(msgs) > limit {
		msgs = msgs[len(msgs)-limit:]
	}
	out := make([]Message, len(msgs))
	copy(out, msgs)
	return out, nil
}

func (s *mockStore) ClearChatHistory(peerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.msgs, peerID)
	return nil
}

func (s *mockStore) count(peerID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.msgs[peerID])
}

// ── Mock MQ ──────────────────────────────────────────────────────────────────

type mockMQ struct {
	mu   sync.Mutex
	subs []topicSub
	sent []sentMsg
}

type topicSub struct {
	prefix string
	fn     func(from, topic string, payload any)
}

type sentMsg struct {
	PeerID  string
	Topic   string
	Payload any
}

func (m *mockMQ) SubscribeTopic(prefix string, fn func(from, topic string, payload any)) func() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.subs = append(m.subs, topicSub{prefix: prefix, fn: fn})
	return func() {}
}

func (m *mockMQ) Send(_ context.Context, peerID, topic string, payload any) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, sentMsg{PeerID: peerID, Topic: topic, Payload: payload})
	return "msg-1", nil
}

func (m *mockMQ) deliver(from, topic string, payload any) {
	m.mu.Lock()
	subs := make([]topicSub, len(m.subs))
	copy(subs, m.subs)
	m.mu.Unlock()

	for _, s := range subs {
		if len(topic) >= len(s.prefix) && topic[:len(s.prefix)] == s.prefix {
			s.fn(from, topic, payload)
		}
	}
}

func (m *mockMQ) sentCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sent)
}

// ── Mock Lua dispatcher ─────────────────────────────────────────────────────

type mockLua struct {
	mu       sync.Mutex
	commands []string
}

func (l *mockLua) DispatchCommand(_ context.Context, fromPeerID, content string, reply func(context.Context, string, string) error) {
	l.mu.Lock()
	l.commands = append(l.commands, content)
	l.mu.Unlock()
	_ = reply(context.Background(), fromPeerID, "lua reply: "+content)
}

func (l *mockLua) commandCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.commands)
}

// ── extractContent ───────────────────────────────────────────────────────────

func TestExtractContent_ValidPayload(t *testing.T) {
	got := extractContent(map[string]any{"content": "hello"})
	if got != "hello" {
		t.Fatalf("expected %q, got %q", "hello", got)
	}
}

func TestExtractContent_MissingKey(t *testing.T) {
	if got := extractContent(map[string]any{"other": "val"}); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestExtractContent_NotMap(t *testing.T) {
	if got := extractContent("string"); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestExtractContent_Nil(t *testing.T) {
	if got := extractContent(nil); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestExtractContent_NonStringContent(t *testing.T) {
	if got := extractContent(map[string]any{"content": 42}); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

// ── Inbound persistence ─────────────────────────────────────────────────────

func TestHandleDirect_PersistsIncoming(t *testing.T) {
	store := newMockStore()
	mq := &mockMQ{}
	mgr := New("self", store, mq)
	mgr.Start()

	mq.deliver("peer1", "chat", map[string]any{"content": "hello"})

	if n := store.count("peer1"); n != 1 {
		t.Fatalf("expected 1 stored message, got %d", n)
	}
}

func TestHandleDirect_IgnoresEmptyFrom(t *testing.T) {
	store := newMockStore()
	mq := &mockMQ{}
	mgr := New("self", store, mq)
	mgr.Start()

	mq.deliver("", "chat", map[string]any{"content": "hello"})

	if n := store.count(""); n != 0 {
		t.Fatalf("expected 0 stored messages, got %d", n)
	}
}

func TestHandleDirect_IgnoresEmptyContent(t *testing.T) {
	store := newMockStore()
	mq := &mockMQ{}
	mgr := New("self", store, mq)
	mgr.Start()

	mq.deliver("peer1", "chat", map[string]any{"content": ""})
	mq.deliver("peer1", "chat", map[string]any{})
	mq.deliver("peer1", "chat", "not a map")

	if n := store.count("peer1"); n != 0 {
		t.Fatalf("expected 0 stored messages, got %d", n)
	}
}

// ── Outbound persistence ────────────────────────────────────────────────────

func TestPersistOutbound(t *testing.T) {
	store := newMockStore()
	mq := &mockMQ{}
	mgr := New("self", store, mq)

	mgr.PersistOutbound("peer1", "hi there")

	if n := store.count("peer1"); n != 1 {
		t.Fatalf("expected 1 stored message, got %d", n)
	}
	msgs, _ := store.GetChatHistory("peer1", 10)
	if msgs[0].From != "self" {
		t.Errorf("expected from=%q, got %q", "self", msgs[0].From)
	}
}

func TestPersistOutbound_IgnoresEmpty(t *testing.T) {
	store := newMockStore()
	mq := &mockMQ{}
	mgr := New("self", store, mq)

	mgr.PersistOutbound("peer1", "")

	if n := store.count("peer1"); n != 0 {
		t.Fatalf("expected 0 stored messages, got %d", n)
	}
}

// ── Broadcast is ephemeral (not persisted) ──────────────────────────────────

func TestBroadcast_NotPersisted(t *testing.T) {
	store := newMockStore()
	mq := &mockMQ{}
	mgr := New("self", store, mq)
	mgr.Start()

	mq.deliver("peer1", "chat.broadcast", map[string]any{"content": "hey all"})

	for _, key := range []string{"__broadcast__", "peer1"} {
		if n := store.count(key); n != 0 {
			t.Fatalf("broadcast should not be persisted (key=%q), got %d", key, n)
		}
	}
}

// ── Lua dispatch ─────────────────────────────────────────────────────────────

func TestHandleDirect_DispatchesLuaCommand(t *testing.T) {
	store := newMockStore()
	mq := &mockMQ{}
	lua := &mockLua{}
	mgr := New("self", store, mq)
	mgr.SetLuaDispatcher(lua)
	mgr.Start()

	mq.deliver("peer1", "chat", map[string]any{"content": "!help"})

	if lua.commandCount() != 1 {
		t.Fatalf("expected 1 lua command, got %d", lua.commandCount())
	}
	if mq.sentCount() != 1 {
		t.Fatalf("expected 1 reply sent via MQ, got %d", mq.sentCount())
	}
}

func TestHandleDirect_NoLua_SkipsDispatch(t *testing.T) {
	store := newMockStore()
	mq := &mockMQ{}
	mgr := New("self", store, mq)
	mgr.Start()

	mq.deliver("peer1", "chat", map[string]any{"content": "!help"})

	// Message still persisted, just no Lua dispatch.
	if n := store.count("peer1"); n != 1 {
		t.Fatalf("expected 1 stored message, got %d", n)
	}
	if mq.sentCount() != 0 {
		t.Fatalf("expected 0 MQ sends without lua, got %d", mq.sentCount())
	}
}

func TestHandleDirect_NormalMessage_NoLuaDispatch(t *testing.T) {
	store := newMockStore()
	mq := &mockMQ{}
	lua := &mockLua{}
	mgr := New("self", store, mq)
	mgr.SetLuaDispatcher(lua)
	mgr.Start()

	mq.deliver("peer1", "chat", map[string]any{"content": "just chatting"})

	if lua.commandCount() != 0 {
		t.Fatalf("expected 0 lua commands for normal message, got %d", lua.commandCount())
	}
}

// ── Peer isolation ──────────────────────────────────────────────────────────

func TestPersistence_IsolatedPerPeer(t *testing.T) {
	store := newMockStore()
	mq := &mockMQ{}
	mgr := New("self", store, mq)
	mgr.Start()

	mq.deliver("peer1", "chat", map[string]any{"content": "msg1"})
	mq.deliver("peer2", "chat", map[string]any{"content": "msg2"})
	mgr.PersistOutbound("peer1", "reply1")

	if n := store.count("peer1"); n != 2 {
		t.Fatalf("expected 2 messages for peer1, got %d", n)
	}
	if n := store.count("peer2"); n != 1 {
		t.Fatalf("expected 1 message for peer2, got %d", n)
	}
}

// ── HTTP handlers ───────────────────────────────────────────────────────────

func TestHTTP_GetHistory(t *testing.T) {
	store := newMockStore()
	mq := &mockMQ{}
	mgr := New("self", store, mq)

	_ = store.StoreChatMessage("peer1", "peer1", "hello", 1000)
	_ = store.StoreChatMessage("peer1", "self", "hi back", 2000)

	mux := http.NewServeMux()
	mgr.RegisterHTTP(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/chat/history?peer_id=peer1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var msgs []Message
	if err := json.NewDecoder(rec.Body).Decode(&msgs); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Content != "hello" {
		t.Errorf("first message = %q, want %q", msgs[0].Content, "hello")
	}
}

func TestHTTP_GetHistory_Empty(t *testing.T) {
	store := newMockStore()
	mgr := New("self", store, &mockMQ{})

	mux := http.NewServeMux()
	mgr.RegisterHTTP(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/chat/history?peer_id=nobody", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var msgs []Message
	json.NewDecoder(rec.Body).Decode(&msgs)
	if len(msgs) != 0 {
		t.Fatalf("expected 0 messages, got %d", len(msgs))
	}
}

func TestHTTP_MissingPeerID(t *testing.T) {
	mgr := New("self", newMockStore(), &mockMQ{})
	mux := http.NewServeMux()
	mgr.RegisterHTTP(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/chat/history", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHTTP_DeleteHistory(t *testing.T) {
	store := newMockStore()
	mgr := New("self", store, &mockMQ{})

	_ = store.StoreChatMessage("peer1", "peer1", "hello", 1000)

	mux := http.NewServeMux()
	mgr.RegisterHTTP(mux)

	req := httptest.NewRequest(http.MethodDelete, "/api/chat/history?peer_id=peer1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if n := store.count("peer1"); n != 0 {
		t.Fatalf("expected 0 after delete, got %d", n)
	}
}

func TestHTTP_DeleteOnlyTargetPeer(t *testing.T) {
	store := newMockStore()
	mgr := New("self", store, &mockMQ{})

	_ = store.StoreChatMessage("peer1", "peer1", "keep", 1000)
	_ = store.StoreChatMessage("peer2", "peer2", "delete", 2000)

	mux := http.NewServeMux()
	mgr.RegisterHTTP(mux)

	req := httptest.NewRequest(http.MethodDelete, "/api/chat/history?peer_id=peer2", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if n := store.count("peer1"); n != 1 {
		t.Fatalf("peer1 should be untouched, got %d", n)
	}
	if n := store.count("peer2"); n != 0 {
		t.Fatalf("peer2 should be deleted, got %d", n)
	}
}

func TestHTTP_MethodNotAllowed(t *testing.T) {
	mgr := New("self", newMockStore(), &mockMQ{})
	mux := http.NewServeMux()
	mgr.RegisterHTTP(mux)

	req := httptest.NewRequest(http.MethodPut, "/api/chat/history?peer_id=peer1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

// ── RegisterChat nil guard ──────────────────────────────────────────────────

func TestNew_NilMQ_NoPanic(t *testing.T) {
	mgr := New("self", newMockStore(), nil)
	mux := http.NewServeMux()
	mgr.RegisterHTTP(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/chat/history?peer_id=peer1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
