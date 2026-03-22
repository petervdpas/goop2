package p2p

import (
	"testing"
	"time"

	ma "github.com/multiformats/go-multiaddr"

	"github.com/petervdpas/goop2/internal/rendezvous"
)

func TestIsCircuitAddr(t *testing.T) {
	for _, tc := range []struct {
		addr string
		want bool
	}{
		{"/ip4/192.168.1.1/tcp/4001", false},
		{"/ip4/10.0.0.1/tcp/4001/p2p/12D3KooWApD9NTZytvvSrELPrW6Wa5HGRVKKmsHskMcUrFDF11Mz/p2p-circuit", true},
		{"/ip6/::1/tcp/4001", false},
	} {
		a, err := ma.NewMultiaddr(tc.addr)
		if err != nil {
			t.Fatalf("bad multiaddr %q: %v", tc.addr, err)
		}
		if got := isCircuitAddr(a); got != tc.want {
			t.Errorf("isCircuitAddr(%q) = %v, want %v", tc.addr, got, tc.want)
		}
	}
}

func TestDurOrDefault(t *testing.T) {
	for _, tc := range []struct {
		sec  int
		def  time.Duration
		want time.Duration
	}{
		{0, 5 * time.Second, 5 * time.Second},
		{-1, 3 * time.Second, 3 * time.Second},
		{10, 3 * time.Second, 10 * time.Second},
		{1, time.Minute, 1 * time.Second},
	} {
		got := durOrDefault(tc.sec, tc.def)
		if got != tc.want {
			t.Errorf("durOrDefault(%d, %s) = %s, want %s", tc.sec, tc.def, got, tc.want)
		}
	}
}

func TestRelayInfoToAddrInfo_Valid(t *testing.T) {
	ri := &rendezvous.RelayInfo{
		PeerID: "12D3KooWApD9NTZytvvSrELPrW6Wa5HGRVKKmsHskMcUrFDF11Mz",
		Addrs: []string{
			"/ip4/192.168.178.50/tcp/4001",
			"/ip4/84.86.207.172/tcp/4001",
		},
	}

	ai, err := relayInfoToAddrInfo(ri)
	if err != nil {
		t.Fatal(err)
	}
	if ai.ID.String() != ri.PeerID {
		t.Fatalf("peer ID mismatch: got %s, want %s", ai.ID, ri.PeerID)
	}
	if len(ai.Addrs) != 2 {
		t.Fatalf("expected 2 addresses, got %d", len(ai.Addrs))
	}
}

func TestRelayInfoToAddrInfo_InvalidPeerID(t *testing.T) {
	ri := &rendezvous.RelayInfo{PeerID: "not-a-real-peer-id"}
	_, err := relayInfoToAddrInfo(ri)
	if err == nil {
		t.Fatal("expected error for invalid peer ID")
	}
}

func TestRelayInfoToAddrInfo_BadAddrsSkipped(t *testing.T) {
	ri := &rendezvous.RelayInfo{
		PeerID: "12D3KooWApD9NTZytvvSrELPrW6Wa5HGRVKKmsHskMcUrFDF11Mz",
		Addrs: []string{
			"not-a-multiaddr",
			"/ip4/192.168.1.1/tcp/4001",
			"also-bad",
		},
	}

	ai, err := relayInfoToAddrInfo(ri)
	if err != nil {
		t.Fatal(err)
	}
	if len(ai.Addrs) != 1 {
		t.Fatalf("expected 1 valid address (bad ones skipped), got %d", len(ai.Addrs))
	}
}

func TestRelayInfoToAddrInfo_EmptyAddrs(t *testing.T) {
	ri := &rendezvous.RelayInfo{
		PeerID: "12D3KooWApD9NTZytvvSrELPrW6Wa5HGRVKKmsHskMcUrFDF11Mz",
		Addrs:  nil,
	}

	ai, err := relayInfoToAddrInfo(ri)
	if err != nil {
		t.Fatal(err)
	}
	if len(ai.Addrs) != 0 {
		t.Fatalf("expected 0 addresses, got %d", len(ai.Addrs))
	}
}

func TestSiteResponseParsing_OKFormat(t *testing.T) {
	for _, tc := range []struct {
		header   string
		wantMime string
		wantSize string
		wantOK   bool
	}{
		{"OK text/html 1234", "text/html", "1234", true},
		{"OK application/json; charset=utf-8 5678", "application/json; charset=utf-8", "5678", true},
		{"OK text/css 0", "text/css", "0", true},
		{"ERR not found", "", "", false},
		{"GARBAGE", "", "", false},
		{"OK ", "", "", false},
	} {
		t.Run(tc.header, func(t *testing.T) {
			h := tc.header
			ok := len(h) > 3 && h[:3] == "OK "
			if ok != tc.wantOK {
				if !tc.wantOK {
					return
				}
				t.Fatalf("expected OK parse for %q", h)
			}
			if !ok {
				return
			}

			lastSpace := -1
			for i := len(h) - 1; i >= 0; i-- {
				if h[i] == ' ' {
					lastSpace = i
					break
				}
			}
			if lastSpace <= 3 {
				t.Fatalf("no size field in %q", h)
			}

			mime := h[3:lastSpace]
			size := h[lastSpace+1:]

			if mime != tc.wantMime {
				t.Errorf("mime = %q, want %q", mime, tc.wantMime)
			}
			if size != tc.wantSize {
				t.Errorf("size = %q, want %q", size, tc.wantSize)
			}
		})
	}
}

func TestSiteResponseParsing_EOKFormat(t *testing.T) {
	for _, tc := range []struct {
		header   string
		wantMime string
		wantSize string
		wantErr  bool
	}{
		{"EOK text/html 1234", "text/html", "1234", false},
		{"EOK application/octet-stream 0", "application/octet-stream", "0", false},
		{"EOK ", "", "", true},
		{"EOK nosizefield", "", "", true},
	} {
		t.Run(tc.header, func(t *testing.T) {
			h := tc.header
			if len(h) < 4 || h[:4] != "EOK " {
				if !tc.wantErr {
					t.Fatal("expected EOK prefix")
				}
				return
			}

			lastSpace := -1
			for i := len(h) - 1; i >= 0; i-- {
				if h[i] == ' ' {
					lastSpace = i
					break
				}
			}

			if lastSpace <= 4 {
				if !tc.wantErr {
					t.Fatalf("expected error for %q", h)
				}
				return
			}
			if tc.wantErr {
				t.Fatalf("expected error but parsed OK for %q", h)
			}

			mime := h[4:lastSpace]
			size := h[lastSpace+1:]

			if mime != tc.wantMime {
				t.Errorf("mime = %q, want %q", mime, tc.wantMime)
			}
			if size != tc.wantSize {
				t.Errorf("size = %q, want %q", size, tc.wantSize)
			}
		})
	}
}

func TestSitePathTraversal(t *testing.T) {
	for _, tc := range []struct {
		path    string
		blocked bool
	}{
		{"/index.html", false},
		{"/css/style.css", false},
		{"/../../../etc/passwd", true},
		{"/lua/scripts.lua", true},
		{"/lua", true},
		{"/images/photo.jpg", false},
	} {
		t.Run(tc.path, func(t *testing.T) {
			// Replicate the path cleaning logic from handleSiteStream.
			reqPath := tc.path
			if reqPath == "" || reqPath == "/" {
				reqPath = "/index.html"
			}

			clean := reqPath
			for len(clean) > 0 && (clean[0] == '/' || clean[0] == '\\') {
				clean = clean[1:]
			}

			isLua := len(clean) >= 4 && clean[:4] == "lua/" || clean == "lua"

			// Path traversal: after cleaning, should not start with ..
			hasTraversal := len(clean) >= 2 && clean[:2] == ".."

			blocked := isLua || hasTraversal
			if blocked != tc.blocked {
				t.Errorf("path %q: blocked=%v, want %v (clean=%q)", tc.path, blocked, tc.blocked, clean)
			}
		})
	}
}

func TestRelayInfoToAddrInfo_DropsLocalhost(t *testing.T) {
	ri := &rendezvous.RelayInfo{
		PeerID: "12D3KooWApD9NTZytvvSrELPrW6Wa5HGRVKKmsHskMcUrFDF11Mz",
		Addrs: []string{
			"/ip4/127.0.0.1/tcp/4001",
			"/ip4/192.168.178.42/tcp/4001",
			"/ip4/84.86.207.172/tcp/4001",
		},
	}

	ai, err := relayInfoToAddrInfo(ri)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range ai.Addrs {
		if a.String() == "/ip4/127.0.0.1/tcp/4001" {
			t.Fatal("localhost address should have been filtered out")
		}
	}
	if len(ai.Addrs) != 2 {
		t.Fatalf("expected 2 addresses (localhost dropped), got %d", len(ai.Addrs))
	}
}

func TestRelayInfoToAddrInfo_LANBeforeWAN(t *testing.T) {
	ri := &rendezvous.RelayInfo{
		PeerID: "12D3KooWApD9NTZytvvSrELPrW6Wa5HGRVKKmsHskMcUrFDF11Mz",
		Addrs: []string{
			"/ip4/84.86.207.172/tcp/4001",
			"/ip4/192.168.178.42/tcp/4001",
		},
	}

	ai, err := relayInfoToAddrInfo(ri)
	if err != nil {
		t.Fatal(err)
	}
	if len(ai.Addrs) != 2 {
		t.Fatalf("expected 2 addresses, got %d", len(ai.Addrs))
	}
	first := ai.Addrs[0].String()
	if first != "/ip4/192.168.178.42/tcp/4001" {
		t.Fatalf("expected LAN address first, got %s", first)
	}
}

func TestRelayInfoToAddrInfo_WSSPassedThrough(t *testing.T) {
	ri := &rendezvous.RelayInfo{
		PeerID: "12D3KooWApD9NTZytvvSrELPrW6Wa5HGRVKKmsHskMcUrFDF11Mz",
		Addrs: []string{
			"/ip4/192.168.178.42/tcp/4001",
			"/dns4/goop2.com/tcp/443/wss",
		},
	}

	ai, err := relayInfoToAddrInfo(ri)
	if err != nil {
		t.Fatal(err)
	}
	if len(ai.Addrs) != 2 {
		t.Fatalf("expected 2 addresses, got %d", len(ai.Addrs))
	}
	last := ai.Addrs[1].String()
	if last != "/dns4/goop2.com/tcp/443/wss" {
		t.Fatalf("expected WSS address last, got %s", last)
	}
}
