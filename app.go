// app.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
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

	// Theme sync
	theme     string // "dark" | "light"
	bridgeURL string // http://127.0.0.1:<port>
}

func NewApp() *App { return &App{} }

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Default theme (authoritative for the whole app lifetime)
	a.mu.Lock()
	if a.theme != "light" && a.theme != "dark" {
		a.theme = "dark"
	}
	a.mu.Unlock()

	// Start localhost theme bridge for the INTERNAL viewer (different origin)
	// Viewer will call: POST <bridgeURL>/theme  { "theme": "light" }
	// Launcher can call (via Wails binding): GetTheme/SetTheme + GetBridgeURL
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	addr := l.Addr().String()

	a.mu.Lock()
	a.bridgeURL = "http://" + addr
	a.mu.Unlock()

	mux := http.NewServeMux()

	mux.HandleFunc("/theme", func(w http.ResponseWriter, r *http.Request) {
		// Minimal headers; viewer uses fetch()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")

		switch r.Method {
		case http.MethodGet:
			a.mu.RLock()
			t := a.theme
			a.mu.RUnlock()
			_ = json.NewEncoder(w).Encode(map[string]string{"theme": t})
			return

		case http.MethodPost:
			var req struct {
				Theme string `json:"theme"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)

			t := req.Theme
			if t != "light" && t != "dark" {
				t = "dark"
			}

			a.mu.Lock()
			a.theme = t
			a.mu.Unlock()

			_ = json.NewEncoder(w).Encode(map[string]string{"theme": t})
			return

		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
			return
		}
	})

	go func() {
		// Serve until process exits
		_ = http.Serve(l, mux)
	}()
}

// -------------------------
// Frontend API (Wails bindings)
// -------------------------

func (a *App) ListPeers() ([]string, error) {
	return listPeerDirs("./peers")
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

func (a *App) GetBridgeURL() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.bridgeURL
}

func (a *App) GetTheme() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.theme != "light" && a.theme != "dark" {
		return "dark"
	}
	return a.theme
}

func (a *App) SetTheme(t string) {
	if t != "light" && t != "dark" {
		t = "dark"
	}
	a.mu.Lock()
	a.theme = t
	a.mu.Unlock()
}

func (a *App) GetStatus() map[string]string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return map[string]string{
		"started":   fmt.Sprintf("%v", a.started),
		"peerName":  a.peerName,
		"viewerURL": a.viewerURL,
		"bridgeURL": a.bridgeURL,
		"theme":     a.theme,
	}
}

// -------------------------
// Helpers
// -------------------------

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
