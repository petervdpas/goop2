// app.go
package main

import (
	"context"
	"fmt"
	"log"
	"net"
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
}

func NewApp() *App { return &App{} }

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// -------------------------
// Frontend API
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
