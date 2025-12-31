// internal/app/helpers.go
package app

import (
	"fmt"
	"log"
	"net"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// NormalizeLocalViewer ensures the viewer only binds to localhost
// and returns listen addr, browser URL, and TCP check addr.
func NormalizeLocalViewer(cfgAddr string) (listenAddr string, url string, tcpAddr string) {
	a := strings.TrimSpace(cfgAddr)

	if strings.HasPrefix(a, ":") {
		a = "127.0.0.1" + a
	}
	if strings.HasPrefix(a, "0.0.0.0:") {
		a = "127.0.0.1:" + strings.TrimPrefix(a, "0.0.0.0:")
	}

	listenAddr = a
	url = "http://" + a
	tcpAddr = a
	return
}

func WaitTCP(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			_ = c.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s", addr)
}

// OpenBrowser opens the system default browser.
// Still useful for dev / debugging.
func OpenBrowser(url string) error {
	cmd := exec.Command("xdg-open", url)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Start()
}

// helper for consistent site path resolution
func siteDir(peerDir string) string {
	return filepath.Join(peerDir, "site")
}

func logBanner(peerDir, cfgPath string) {
	log.Println("────────────────────────────────────────")
	log.Println("Goop peer scope")
	log.Printf(" Peer folder : %s", peerDir)
	log.Printf(" Config file : %s", cfgPath)
	log.Println("")
	log.Println(" This process represents ONE peer.")
	log.Println(" The peer folder is the peer's boundary.")
	log.Println(" Different folder/config = different peer.")
	log.Println("────────────────────────────────────────")
}
