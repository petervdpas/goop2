package util

import (
	"testing"
	"time"
)

func TestDNSCache_ReadyFalseBeforeResolve(t *testing.T) {
	d := NewDNSCache("goop2.com", 10*time.Second, 5*time.Minute)
	if d.Ready() {
		t.Fatal("should not be ready before resolve")
	}
}

func TestDNSCache_ReadyTrueForIP(t *testing.T) {
	d := NewDNSCache("192.168.1.1", 10*time.Second, 5*time.Minute)
	if !d.Ready() {
		t.Fatal("should be ready for IP address")
	}
}

func TestDNSCache_ReadyTrueForEmpty(t *testing.T) {
	d := NewDNSCache("", 10*time.Second, 5*time.Minute)
	if !d.Ready() {
		t.Fatal("should be ready for empty host")
	}
}

func TestDNSCache_ResolveReturnsIPDirectly(t *testing.T) {
	d := NewDNSCache("1.2.3.4", 10*time.Second, 5*time.Minute)
	ip, err := d.Resolve(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if ip != "1.2.3.4" {
		t.Fatalf("expected 1.2.3.4, got %s", ip)
	}
}

func TestDNSCache_ClearResetsReady(t *testing.T) {
	d := NewDNSCache("goop2.com", 10*time.Second, 5*time.Minute)
	d.mu.Lock()
	d.ip = "1.2.3.4"
	d.expires = time.Now().Add(5 * time.Minute)
	d.mu.Unlock()

	if !d.Ready() {
		t.Fatal("should be ready with cached IP")
	}

	d.Clear()

	if d.Ready() {
		t.Fatal("should not be ready after clear")
	}
}

func TestDNSCache_ExpiredNotReady(t *testing.T) {
	d := NewDNSCache("goop2.com", 10*time.Second, 5*time.Minute)
	d.mu.Lock()
	d.ip = "1.2.3.4"
	d.expires = time.Now().Add(-time.Second)
	d.mu.Unlock()

	if d.Ready() {
		t.Fatal("should not be ready with expired cache")
	}
}

func TestDNSCache_ResolveLocalhost(t *testing.T) {
	d := NewDNSCache("localhost", 5*time.Second, 5*time.Minute)
	ip, err := d.Resolve(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if ip == "" {
		t.Fatal("expected resolved IP for localhost")
	}
}

func TestDNSCache_ResolveCachesResult(t *testing.T) {
	d := NewDNSCache("localhost", 5*time.Second, 5*time.Minute)
	ip1, _ := d.Resolve(t.Context())
	ip2, _ := d.Resolve(t.Context())
	if ip1 != ip2 {
		t.Fatalf("cached result should be same: %s vs %s", ip1, ip2)
	}
	if !d.Ready() {
		t.Fatal("should be ready after resolve")
	}
}
