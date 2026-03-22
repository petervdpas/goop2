package rendezvous

import "testing"

func TestNewClient_ExtractsHostname(t *testing.T) {
	c := NewClient("https://goop2.com")
	if c.dns.Host != "goop2.com" {
		t.Fatalf("expected Host=goop2.com, got %q", c.dns.Host)
	}
}

func TestNewClient_IPAddress(t *testing.T) {
	c := NewClient("http://192.168.1.1:8787")
	if c.dns.Host != "192.168.1.1" {
		t.Fatalf("expected Host=192.168.1.1, got %q", c.dns.Host)
	}
}

func TestDNSReady_FalseBeforeWarmup(t *testing.T) {
	c := NewClient("https://goop2.com")
	if c.DNSReady() {
		t.Fatal("DNSReady should be false before WarmDNS")
	}
}

func TestDNSReady_TrueForIPAddress(t *testing.T) {
	c := NewClient("http://192.168.1.1:8787")
	if !c.DNSReady() {
		t.Fatal("DNSReady should be true for IP-based URL")
	}
}

func TestDNSReady_TrueForLocalhost(t *testing.T) {
	c := NewClient("http://127.0.0.1:8787")
	if !c.DNSReady() {
		t.Fatal("DNSReady should be true for localhost")
	}
}
