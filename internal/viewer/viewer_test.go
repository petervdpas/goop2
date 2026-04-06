package viewer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/petervdpas/goop2/internal/content"
	"github.com/petervdpas/goop2/internal/p2p"
)

func TestContentTypeForPath(t *testing.T) {
	cases := []struct {
		path string
		data []byte
		want string
	}{
		{"style.css", nil, "text/css; charset=utf-8"},
		{"app.js", nil, "application/javascript; charset=utf-8"},
		{"index.html", nil, "text/html; charset=utf-8"},
		{"page.htm", nil, "text/html; charset=utf-8"},
		{"logo.svg", nil, "image/svg+xml"},
		{"STYLE.CSS", nil, "text/css; charset=utf-8"},
		{"photo.png", []byte("\x89PNG"), "image/png"},
		{"data.json", nil, "application/json"},
		{"unknown", []byte("hello world"), "text/plain; charset=utf-8"},
		{"unknown", []byte("\x00\x01\x02"), "application/octet-stream"},
	}
	for _, tc := range cases {
		got := contentTypeForPath(tc.path, tc.data)
		if got != tc.want {
			t.Errorf("contentTypeForPath(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestLogBuffer_WriteAndSnapshot(t *testing.T) {
	lb := NewLogBuffer(10)
	lb.Write([]byte("line one\nline two\n"))

	snap := lb.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(snap))
	}
	if snap[0].Msg != "line one" {
		t.Errorf("entry 0 = %q", snap[0].Msg)
	}
	if snap[1].Msg != "line two" {
		t.Errorf("entry 1 = %q", snap[1].Msg)
	}
}

func TestLogBuffer_PartialLine(t *testing.T) {
	lb := NewLogBuffer(10)
	lb.Write([]byte("partial"))
	if len(lb.Snapshot()) != 0 {
		t.Error("partial line should not appear in snapshot")
	}
	lb.Write([]byte(" rest\n"))
	snap := lb.Snapshot()
	if len(snap) != 1 || snap[0].Msg != "partial rest" {
		t.Errorf("unexpected: %+v", snap)
	}
}

func TestLogBuffer_SkipsBlankLines(t *testing.T) {
	lb := NewLogBuffer(10)
	lb.Write([]byte("one\n\n  \ntwo\n"))
	snap := lb.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("expected 2 non-blank entries, got %d", len(snap))
	}
}

func TestLogBuffer_RingOverflow(t *testing.T) {
	lb := NewLogBuffer(3)
	for i := range 5 {
		lb.Write([]byte("line " + string(rune('A'+i)) + "\n"))
	}
	snap := lb.Snapshot()
	if len(snap) != 3 {
		t.Fatalf("expected 3 entries (ring cap), got %d", len(snap))
	}
	if snap[0].Msg != "line C" {
		t.Errorf("oldest entry should be C, got %q", snap[0].Msg)
	}
}

func TestLogBuffer_DefaultMax(t *testing.T) {
	lb := NewLogBuffer(0)
	if lb == nil {
		t.Fatal("NewLogBuffer(0) should use default")
	}
}

func TestLogBuffer_Subscribe(t *testing.T) {
	lb := NewLogBuffer(10)
	ch, cancel := lb.Subscribe()
	defer cancel()

	lb.Write([]byte("live\n"))

	select {
	case e := <-ch:
		if e.Msg != "live" {
			t.Errorf("expected live, got %q", e.Msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for subscription")
	}
}

func TestLogBuffer_CancelSubscription(t *testing.T) {
	lb := NewLogBuffer(10)
	ch, cancel := lb.Subscribe()
	cancel()

	_, ok := <-ch
	if ok {
		t.Error("channel should be closed after cancel")
	}

	cancel()
}

func TestLogBuffer_ServeLogsJSON(t *testing.T) {
	lb := NewLogBuffer(10)
	lb.Write([]byte("entry\n"))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/logs", nil)
	lb.ServeLogsJSON(w, r)

	if w.Code != 200 {
		t.Fatalf("status %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Content-Type"), "application/json") {
		t.Error("expected JSON content type")
	}

	var entries []LogEntry
	if err := json.Unmarshal(w.Body.Bytes(), &entries); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(entries) != 1 || entries[0].Msg != "entry" {
		t.Errorf("unexpected entries: %+v", entries)
	}
}

func TestLogBuffer_ServeLogsJSON_MethodNotAllowed(t *testing.T) {
	lb := NewLogBuffer(10)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/logs", nil)
	lb.ServeLogsJSON(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestNoCache_SetsHeaders(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})

	handler := noCache(inner)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(w, r)

	cc := w.Header().Get("Cache-Control")
	if !strings.Contains(cc, "no-store") || !strings.Contains(cc, "no-cache") {
		t.Errorf("Cache-Control = %q", cc)
	}
	if w.Header().Get("Pragma") != "no-cache" {
		t.Errorf("Pragma = %q", w.Header().Get("Pragma"))
	}
	if w.Header().Get("Expires") != "0" {
		t.Errorf("Expires = %q", w.Header().Get("Expires"))
	}
}

func TestLogBuffer_StripsCR(t *testing.T) {
	lb := NewLogBuffer(10)
	lb.Write([]byte("windows line\r\n"))
	snap := lb.Snapshot()
	if len(snap) != 1 || snap[0].Msg != "windows line" {
		t.Errorf("expected stripped CR: %+v", snap)
	}
}

func testNode(t *testing.T) *p2p.Node {
	t.Helper()
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatalf("libp2p.New: %v", err)
	}
	t.Cleanup(func() { h.Close() })
	return &p2p.Node{Host: h}
}

func TestProxyPeerSite_SelfShortCircuit(t *testing.T) {
	node := testNode(t)
	dir := t.TempDir()
	cs, err := content.NewStore(dir, "site")
	if err != nil {
		t.Fatal(err)
	}
	cs.EnsureRoot()
	cs.Write(context.Background(), "index.html", []byte("<h1>home</h1>"), "")

	handler := proxyPeerSite(Viewer{Node: node, Content: cs})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/p/"+node.ID()+"/index.html", nil)
	handler.ServeHTTP(w, r)

	if w.Code != 200 {
		t.Fatalf("status %d, body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "<h1>home</h1>") {
		t.Errorf("body = %q", w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type = %q", ct)
	}
	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("missing X-Content-Type-Options")
	}
	if w.Header().Get("Content-Security-Policy") == "" {
		t.Error("missing CSP header")
	}
}

func TestProxyPeerSite_SelfDefaultIndex(t *testing.T) {
	node := testNode(t)
	dir := t.TempDir()
	cs, _ := content.NewStore(dir, "site")
	cs.EnsureRoot()
	cs.Write(context.Background(), "index.html", []byte("default"), "")

	handler := proxyPeerSite(Viewer{Node: node, Content: cs})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/p/"+node.ID()+"/", nil)
	handler.ServeHTTP(w, r)

	if w.Code != 200 {
		t.Fatalf("status %d", w.Code)
	}
	if w.Body.String() != "default" {
		t.Errorf("body = %q", w.Body.String())
	}
}

func TestProxyPeerSite_SelfNotFound(t *testing.T) {
	node := testNode(t)
	dir := t.TempDir()
	cs, _ := content.NewStore(dir, "site")
	cs.EnsureRoot()

	handler := proxyPeerSite(Viewer{Node: node, Content: cs})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/p/"+node.ID()+"/missing.html", nil)
	handler.ServeHTTP(w, r)

	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestProxyPeerSite_NoContentStore(t *testing.T) {
	node := testNode(t)
	handler := proxyPeerSite(Viewer{Node: node, Content: nil})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/p/"+node.ID()+"/index.html", nil)
	handler.ServeHTTP(w, r)

	if w.Code != 500 {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestProxyPeerSite_NoPeerID(t *testing.T) {
	handler := proxyPeerSite(Viewer{})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/p/", nil)
	handler.ServeHTTP(w, r)

	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestProxyPeerSite_EmptyPath(t *testing.T) {
	handler := proxyPeerSite(Viewer{})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/p/", nil)
	handler.ServeHTTP(w, r)

	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
