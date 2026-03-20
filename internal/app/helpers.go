package app

import (
	"fmt"
	"log"
	"net"
	"time"

	"github.com/petervdpas/goop2/internal/util"
)

func WaitTCP(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", addr, TCPDialAttemptTimeout)
		if err == nil {
			_ = c.Close()
			return nil
		}
		time.Sleep(util.PollInterval)
	}
	return fmt.Errorf("timeout waiting for %s", addr)
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
