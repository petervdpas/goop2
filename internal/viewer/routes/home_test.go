package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/petervdpas/goop2/internal/state"
)

func TestHomeRedirectsToPeers(t *testing.T) {
	mux := http.NewServeMux()
	d := Deps{
		Peers: state.NewPeerTable(),
	}
	registerHomeRoutes(mux, d)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusFound)
	}
	if loc := w.Header().Get("Location"); loc != "/peers" {
		t.Fatalf("Location = %q, want /peers", loc)
	}
}

func TestHomeNonRootReturns404(t *testing.T) {
	mux := http.NewServeMux()
	d := Deps{
		Peers: state.NewPeerTable(),
	}
	registerHomeRoutes(mux, d)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/nonexistent", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestAPIPeersReturnsJSON(t *testing.T) {
	mux := http.NewServeMux()
	pt := state.NewPeerTable()
	d := Deps{Peers: pt}
	registerHomeRoutes(mux, d)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/peers", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
}

func TestAPIPeersRejectsPost(t *testing.T) {
	mux := http.NewServeMux()
	d := Deps{Peers: state.NewPeerTable()}
	registerHomeRoutes(mux, d)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/peers", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestAPIPeersFavorite_missingPeerID(t *testing.T) {
	mux := http.NewServeMux()
	d, _ := testDeps(t)
	d.Peers = state.NewPeerTable()
	registerHomeRoutes(mux, d)

	body := `{"favorite": true}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/peers/favorite", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestAPIPeersFavorite_rejectsGet(t *testing.T) {
	mux := http.NewServeMux()
	d, _ := testDeps(t)
	d.Peers = state.NewPeerTable()
	registerHomeRoutes(mux, d)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/peers/favorite", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestAPITopology_withFunc(t *testing.T) {
	mux := http.NewServeMux()
	d := Deps{
		Peers: state.NewPeerTable(),
		TopologyFunc: func() any {
			return map[string]int{"peers": 5}
		},
	}
	registerHomeRoutes(mux, d)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/topology", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var got map[string]int
	json.NewDecoder(w.Body).Decode(&got)
	if got["peers"] != 5 {
		t.Fatalf("got %v", got)
	}
}

func TestAPITopology_noNode(t *testing.T) {
	mux := http.NewServeMux()
	d := Deps{Peers: state.NewPeerTable()}
	registerHomeRoutes(mux, d)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/topology", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}
