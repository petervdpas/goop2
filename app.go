// app.go
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	goopapp "goop/internal/app"
	"goop/internal/config"
)

type App struct {
	ctx context.Context

	mu sync.RWMutex

	peerDir   string
	cfgPath   string
	peerName  string
	started   bool
	viewerURL string

	// UI / Theme (shared between launcher + internal viewer)
	uiMu      sync.Mutex
	bridgeURL string
}

type uiState struct {
	Theme string `json:"theme"`
}

const (
	uiPath   = "data/ui.json"
	themeKey = "goop.theme"
)

func NewApp() *App { return &App{} }

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Ensure ui.json exists with a default theme
	if _, err := a.GetTheme(); err != nil {
		log.Printf("ui theme init: %v", err)
	}

	// Start bridge so the internal viewer (http://127...) can notify Wails about theme changes.
	if err := a.startBridge(); err != nil {
		log.Printf("bridge start: %v", err)
	}
}

// -------------------------
// Theme API for Wails frontend
// -------------------------

func (a *App) GetTheme() (string, error) {
	a.uiMu.Lock()
	defer a.uiMu.Unlock()

	s, err := readUIState(uiPath)
	if err != nil {
		// If unreadable, fall back safely
		return "dark", nil
	}
	return normalizeTheme(s.Theme), nil
}

func (a *App) SetTheme(theme string) error {
	a.uiMu.Lock()
	defer a.uiMu.Unlock()

	theme = normalizeTheme(theme)
	s := uiState{Theme: theme}
	return writeUIState(uiPath, s)
}

// GetBridgeURL is used by the launcher to pass ?bridge=... to the internal viewer.
func (a *App) GetBridgeURL() string {
	a.uiMu.Lock()
	defer a.uiMu.Unlock()
	return a.bridgeURL
}

// -------------------------
// Frontend API (peers)
// -------------------------

func (a *App) ListPeers() ([]string, error) {
	return listPeerDirs("./peers")
}

func (a *App) CreatePeer(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("peer name is empty")
	}
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return "", errors.New("invalid peer name")
	}

	peerDir := filepath.Join("./peers", name)
	cfgPath := filepath.Join(peerDir, "goop.json")

	if err := os.MkdirAll(peerDir, 0o755); err != nil {
		return "", err
	}

	// Ensure config exists
	if _, _, err := config.Ensure(cfgPath); err != nil {
		return "", err
	}
	return name, nil
}

func (a *App) DeletePeer(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("peer name is empty")
	}
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return errors.New("invalid peer name")
	}

	peerDir := filepath.Join("./peers", name)

	// Prevent deleting the currently running peer
	a.mu.RLock()
	running := a.started && a.peerName == name
	a.mu.RUnlock()
	if running {
		return errors.New("cannot delete a running peer")
	}

	return os.RemoveAll(peerDir)
}

func (a *App) StartPeer(peerName string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.started {
		return fmt.Errorf("peer already started")
	}

	peerDir := filepath.Join("./peers", peerName)
	cfgPath := filepath.Join(peerDir, "goop.json")

	cfg, _, err := config.Ensure(cfgPath)
	if err != nil {
		return err
	}

	// pick free localhost port
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	cfg.Viewer.HTTPAddr = fmt.Sprintf("127.0.0.1:%d", port)

	a.peerDir = peerDir
	a.cfgPath = cfgPath
	a.peerName = peerName
	a.started = true
	a.viewerURL = "http://" + cfg.Viewer.HTTPAddr

	go func() {
		if err := goopapp.Run(a.ctx, goopapp.Options{
			PeerDir: peerDir,
			CfgPath: cfgPath,
			Cfg:     cfg,
		}); err != nil {
			log.Fatal(err)
		}
	}()

	// wait until viewer is listening
	addr := cfg.Viewer.HTTPAddr
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			_ = c.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("viewer did not start")
}

func (a *App) GetViewerURL() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.viewerURL
}

func (a *App) GetStatus() map[string]string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return map[string]string{
		"started":   fmt.Sprintf("%v", a.started),
		"peerName":  a.peerName,
		"viewerURL": a.viewerURL,
	}
}

// -------------------------
// Bridge server: internal viewer -> shared ui.json
// -------------------------

func (a *App) startBridge() error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}

	addr := ln.Addr().String()
	base := "http://" + addr

	mux := http.NewServeMux()

	// CORS helper (viewer origin is http://127..., so allow it)
	withCORS := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			h(w, r)
		}
	}

	mux.HandleFunc("/theme", withCORS(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			t, _ := a.GetTheme()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(uiState{Theme: t})
			return

		case http.MethodPost:
			b, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
			var in uiState
			if err := json.Unmarshal(b, &in); err != nil {
				http.Error(w, "bad json", http.StatusBadRequest)
				return
			}
			if err := a.SetTheme(in.Theme); err != nil {
				http.Error(w, "cannot save", http.StatusInternalServerError)
				return
			}
			// also return what we stored
			t, _ := a.GetTheme()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(uiState{Theme: t})
			return

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
	}))

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 2 * time.Second,
	}

	a.uiMu.Lock()
	a.bridgeURL = base
	a.uiMu.Unlock()

	go func() {
		_ = srv.Serve(ln)
	}()

	return nil
}

// -------------------------
// Helpers
// -------------------------

func normalizeTheme(t string) string {
	if t == "light" || t == "dark" {
		return t
	}
	return "dark"
}

func readUIState(path string) (uiState, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// default
			return uiState{Theme: "dark"}, writeUIState(path, uiState{Theme: "dark"})
		}
		return uiState{}, err
	}

	var s uiState
	if err := json.Unmarshal(b, &s); err != nil {
		// If corrupted, recover safely
		return uiState{Theme: "dark"}, writeUIState(path, uiState{Theme: "dark"})
	}
	s.Theme = normalizeTheme(s.Theme)
	return s, nil
}

func writeUIState(path string, s uiState) error {
	s.Theme = normalizeTheme(s.Theme)

	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func listPeerDirs(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	var peers []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, e.Name(), "goop.json")); err == nil {
			peers = append(peers, e.Name())
		}
	}

	sort.Strings(peers)
	return peers, nil
}
