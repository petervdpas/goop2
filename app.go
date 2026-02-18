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
	"sync"
	"time"

	goopapp "github.com/petervdpas/goop2/internal/app"
	"github.com/petervdpas/goop2/internal/config"
	"github.com/petervdpas/goop2/internal/util"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx    context.Context
	cancel context.CancelFunc

	mu sync.RWMutex

	peerDir          string
	cfgPath          string
	peerName         string
	started          bool
	viewerURL        string
	isRendezvousOnly bool

	// UI / Theme (shared between launcher + internal viewer)
	uiMu      sync.Mutex
	bridgeURL string
}

// PeerInfo is returned by ListPeers to the Wails frontend.
type PeerInfo struct {
	Name           string `json:"name"`
	RendezvousOnly bool   `json:"rendezvous_only"`
	Splash         string `json:"splash"`
}

type uiState struct {
	Theme string `json:"theme"`
}

const (
	uiPath   = "data/ui.json"
	themeKey = "goop.theme"
)

const defaultIndexHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>Goop² Peer</title>
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <link rel="stylesheet" href="style.css">
</head>
<body>
  <main>
    <h1>Hello from Goop²</h1>
    <p>This peer is live.</p>
  </main>
</body>
</html>
`

const defaultStyleCSS = `:root {
  --bg: #0f1115;
  --panel: #151924;
  --text: #e6e9ef;
  --muted: #9aa3b2;
  --accent: #7aa2ff;
  --radius: 14px;
}

html, body {
  margin: 0;
  padding: 0;
  background: radial-gradient(1200px 700px at 65% 18%, rgba(122,162,255,0.14), transparent 55%),
              radial-gradient(900px 600px at 30% 35%, rgba(160,120,255,0.10), transparent 60%),
              var(--bg);
  color: var(--text);
  font-family: system-ui, -apple-system, Segoe UI, Roboto, Ubuntu, Cantarell, "Noto Sans", Arial, sans-serif;
}

main {
  max-width: 860px;
  margin: 0 auto;
  padding: 3rem 1.25rem;
}

h1 {
  margin: 0 0 0.75rem 0;
  color: var(--accent);
  letter-spacing: -0.02em;
}

p {
  margin: 0;
  color: var(--muted);
}
`

func NewApp() *App { return &App{} }

func (a *App) startup(ctx context.Context) {
	// Create cancellable context for peer lifecycle
	a.ctx, a.cancel = context.WithCancel(ctx)

	// Ensure ui.json exists with a default theme
	if _, err := a.GetTheme(); err != nil {
		log.Printf("ui theme init: %v", err)
	}

	// Start bridge so the internal viewer (http://127...) can notify Wails about theme changes.
	if err := a.startBridge(); err != nil {
		log.Printf("bridge start: %v", err)
	}
}

func (a *App) shutdown(ctx context.Context) {
	// Cancel the peer context to trigger cleanup and offline messages
	if a.cancel != nil {
		log.Println("========================================")
		log.Println("SHUTDOWN: Cancelling peer context...")
		log.Println("========================================")
		a.cancel()

		// Give the peer time to send offline message
		time.Sleep(500 * time.Millisecond)
		log.Println("SHUTDOWN: Complete")
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

// OpenInBrowser opens a URL in the default browser.
func (a *App) OpenInBrowser(url string) {
	runtime.BrowserOpenURL(a.ctx, url)
}

// SelectSiteFolder opens a native directory picker and returns the chosen path.
// Returns empty string if the user cancels.
func (a *App) SelectSiteFolder() (string, error) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Choose site folder",
	})
	if err != nil {
		return "", err
	}
	return dir, nil
}

// -------------------------
// Frontend API (peers)
// -------------------------

func (a *App) ListPeers() ([]PeerInfo, error) {
	return listPeerInfos("./peers")
}

func (a *App) CreatePeer(name string, siteFolder string) (string, error) {
	name, err := util.ValidatePeerName(name)
	if err != nil {
		return "", err
	}

	peerDir := filepath.Join("./peers", name)
	cfgPath := filepath.Join(peerDir, "goop.json")

	if err := os.MkdirAll(peerDir, 0o755); err != nil {
		return "", err
	}

	// Ensure config exists
	cfg, created, err := config.Ensure(cfgPath)
	if err != nil {
		return "", err
	}

	// If a site folder was chosen, update all paths that derive from site root
	if siteFolder != "" && created {
		cfg.Paths.SiteRoot = siteFolder
		cfg.Paths.SiteSource = filepath.Join(siteFolder, "src")
		cfg.Paths.SiteStage = filepath.Join(siteFolder, "stage")
		cfg.Lua.ScriptDir = filepath.Join(siteFolder, "lua")
		if err := config.Save(cfgPath, cfg); err != nil {
			return "", err
		}
	}

	// Ensure default site files exist (index.html + style.css)
	siteDir := filepath.Join(peerDir, "site")
	if siteFolder != "" {
		siteDir = siteFolder
	}
	if err := ensureDefaultPeerSite(siteDir); err != nil {
		return "", err
	}

	// Ensure default store templates exist
	if err := ensureDefaultStoreTemplates(peerDir); err != nil {
		return "", err
	}

	return name, nil
}

func (a *App) DeletePeer(name string) error {
	name, err := util.ValidatePeerName(name)
	if err != nil {
		return err
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
	a.isRendezvousOnly = cfg.Presence.RendezvousOnly

	progress := func(step, total int, label string) {
		runtime.EventsEmit(a.ctx, "startup:progress", map[string]interface{}{
			"step":  step,
			"total": total,
			"label": label,
		})
	}

	go func() {
		if err := goopapp.Run(a.ctx, goopapp.Options{
			PeerDir:   peerDir,
			CfgPath:   cfgPath,
			Cfg:       cfg,
			BridgeURL: a.GetBridgeURL(),
			Progress:  progress,
		}); err != nil {
			log.Fatal(err)
		}
	}()

	// wait until viewer is listening (30s — progress bar keeps user informed)
	if err := goopapp.WaitTCP(cfg.Viewer.HTTPAddr, 30*time.Second); err != nil {
		runtime.EventsEmit(a.ctx, "startup:error", "Viewer did not start in time")
		return fmt.Errorf("viewer did not start")
	}

	return nil
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
		"started":        fmt.Sprintf("%v", a.started),
		"peerName":       a.peerName,
		"viewerURL":      a.viewerURL,
		"rendezvousOnly": fmt.Sprintf("%v", a.isRendezvousOnly),
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

	// Save export zip via native file dialog
	mux.HandleFunc("/export-save", withCORS(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if err := r.ParseMultipartForm(50 << 20); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		filename := r.FormValue("filename")
		if filename == "" {
			filename = "goop-export.zip"
		}

		file, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "file required", http.StatusBadRequest)
			return
		}
		defer file.Close()

		data, err := io.ReadAll(io.LimitReader(file, 50<<20+1))
		if err != nil {
			http.Error(w, "read error", http.StatusInternalServerError)
			return
		}

		savePath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
			Title:           "Save export archive",
			DefaultFilename: filename,
			Filters: []runtime.FileFilter{
				{DisplayName: "Zip Archives (*.zip)", Pattern: "*.zip"},
			},
		})
		if err != nil {
			http.Error(w, "dialog error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if savePath == "" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{"cancelled": true})
			return
		}

		if err := os.WriteFile(savePath, data, 0o644); err != nil {
			http.Error(w, "write error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"cancelled": false,
			"path":      savePath,
		})
	}))

	// Select directory via native dialog
	mux.HandleFunc("/select-dir", withCORS(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		title := r.URL.Query().Get("title")
		if title == "" {
			title = "Choose directory"
		}
		dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
			Title: title,
		})
		if err != nil {
			http.Error(w, "dialog error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"cancelled": dir == "",
			"path":      dir,
		})
	}))

	// Select a single file via native dialog
	mux.HandleFunc("/select-file", withCORS(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		title := r.URL.Query().Get("title")
		if title == "" {
			title = "Choose file"
		}
		file, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
			Title: title,
		})
		if err != nil {
			http.Error(w, "dialog error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"cancelled": file == "",
			"path":      file,
		})
	}))

	// Select multiple files via native dialog
	mux.HandleFunc("/select-files", withCORS(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		title := r.URL.Query().Get("title")
		if title == "" {
			title = "Choose files"
		}
		files, err := runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{
			Title: title,
		})
		if err != nil {
			http.Error(w, "dialog error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"cancelled": len(files) == 0,
			"paths":     files,
		})
	}))

	// Open URL in browser endpoint
	mux.HandleFunc("/open", withCORS(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		url := r.URL.Query().Get("url")
		if url == "" {
			http.Error(w, "missing url parameter", http.StatusBadRequest)
			return
		}
		runtime.BrowserOpenURL(a.ctx, url)
		w.WriteHeader(http.StatusOK)
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
	return util.WriteJSONFile(path, s)
}

func ensureDefaultPeerSite(siteDir string) error {
	if err := os.MkdirAll(siteDir, 0o755); err != nil {
		return err
	}

	indexPath := filepath.Join(siteDir, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if err := os.WriteFile(indexPath, []byte(defaultIndexHTML), 0o644); err != nil {
			return err
		}
	}

	if err := os.MkdirAll(filepath.Join(siteDir, "images"), 0o755); err != nil {
		return err
	}

	stylePath := filepath.Join(siteDir, "style.css")
	if _, err := os.Stat(stylePath); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if err := os.WriteFile(stylePath, []byte(defaultStyleCSS), 0o644); err != nil {
			return err
		}
	}

	return nil
}

func ensureDefaultStoreTemplates(peerDir string) error {
	tplDir := filepath.Join(peerDir, "templates", "hello-store")

	// Skip if already exists
	if _, err := os.Stat(filepath.Join(tplDir, "manifest.json")); err == nil {
		return nil
	}

	if err := os.MkdirAll(filepath.Join(tplDir, "css"), 0o755); err != nil {
		return err
	}

	manifest := `{
  "name": "Hello Store",
  "description": "A simple starter template served from the template store.",
  "category": "starter",
  "icon": "🏪"
}
`
	if err := os.WriteFile(filepath.Join(tplDir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		return err
	}

	indexHTML := `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>Hello from the Store</title>
  <link rel="stylesheet" href="css/style.css">
</head>
<body>
  <h1>Hello from the Template Store</h1>
  <p>This site was installed from a store template.</p>
</body>
</html>
`
	if err := os.WriteFile(filepath.Join(tplDir, "index.html"), []byte(indexHTML), 0o644); err != nil {
		return err
	}

	css := `body {
  font-family: system-ui, sans-serif;
  max-width: 600px;
  margin: 2rem auto;
  padding: 0 1rem;
}
h1 { color: #2a6; }
`
	if err := os.WriteFile(filepath.Join(tplDir, "css", "style.css"), []byte(css), 0o644); err != nil {
		return err
	}

	return nil
}

func listPeerInfos(root string) ([]PeerInfo, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []PeerInfo{}, nil
		}
		return nil, err
	}

	var peers []PeerInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		cfgPath := filepath.Join(root, e.Name(), "goop.json")
		if _, err := os.Stat(cfgPath); err != nil {
			continue
		}

		info := PeerInfo{Name: e.Name()}

		// Read config to check rendezvous_only flag.
		// Use LoadPartial (no validation) so the pill shows even when
		// the config has validation issues (e.g. missing rendezvous_host).
		if cfg, err := config.LoadPartial(cfgPath); err == nil {
			info.RendezvousOnly = cfg.Presence.RendezvousOnly
			info.Splash = cfg.Viewer.Splash
		}

		peers = append(peers, info)
	}

	sort.Slice(peers, func(i, j int) bool { return peers[i].Name < peers[j].Name })
	return peers, nil
}
