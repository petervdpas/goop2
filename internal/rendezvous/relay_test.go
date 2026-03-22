package rendezvous

import "testing"

func TestBuildWSSAddr(t *testing.T) {
	got := buildWSSAddr("https://1.2.3.4", "12D3KooWTest")
	want := "/ip4/1.2.3.4/tcp/443/tls/sni/1.2.3.4/ws/p2p/12D3KooWTest"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestBuildWSSAddr_EmptyURL(t *testing.T) {
	if got := buildWSSAddr("", "12D3KooWTest"); got != "" {
		t.Fatalf("expected empty for empty URL, got %q", got)
	}
}

func TestBuildWSSAddr_NoHostname(t *testing.T) {
	if got := buildWSSAddr("://", "12D3KooWTest"); got != "" {
		t.Fatalf("expected empty for bad URL, got %q", got)
	}
}

func TestBuildPublicAddr_IPv4(t *testing.T) {
	got := buildPublicAddr("https://1.2.3.4", 4001, "12D3KooWTest")
	want := "/ip4/1.2.3.4/tcp/4001/p2p/12D3KooWTest"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestBuildExternalAddrs_TCPOnly(t *testing.T) {
	addrs := buildExternalAddrs("https://1.2.3.4", 4001, 0)
	if len(addrs) != 1 {
		t.Fatalf("expected 1 addr, got %d", len(addrs))
	}
	if addrs[0].String() != "/ip4/1.2.3.4/tcp/4001" {
		t.Fatalf("expected TCP addr, got %s", addrs[0])
	}
}

func TestBuildExternalAddrs_TCPAndWSS(t *testing.T) {
	addrs := buildExternalAddrs("https://1.2.3.4", 4001, 4002)
	if len(addrs) != 2 {
		t.Fatalf("expected 2 addrs, got %d", len(addrs))
	}
	if addrs[0].String() != "/ip4/1.2.3.4/tcp/4001" {
		t.Fatalf("expected TCP addr first, got %s", addrs[0])
	}
	if addrs[1].String() != "/ip4/1.2.3.4/tcp/443/tls/sni/1.2.3.4/ws" {
		t.Fatalf("expected WSS addr second, got %s", addrs[1])
	}
}

func TestBuildExternalAddrs_Domain(t *testing.T) {
	addrs := buildExternalAddrs("https://localhost", 4001, 4002)
	if len(addrs) != 2 {
		t.Fatalf("expected 2 addrs, got %d", len(addrs))
	}
	wss := addrs[1].String()
	if wss != "/ip4/127.0.0.1/tcp/443/tls/sni/localhost/ws" {
		t.Fatalf("expected WSS with resolved IP + SNI, got %s", wss)
	}
}

func TestBuildExternalAddrs_EmptyURL(t *testing.T) {
	addrs := buildExternalAddrs("", 4001, 4002)
	if len(addrs) != 0 {
		t.Fatalf("expected 0 addrs for empty URL, got %d", len(addrs))
	}
}
