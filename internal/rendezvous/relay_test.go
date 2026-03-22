package rendezvous

import "testing"

func TestBuildWSSAddr(t *testing.T) {
	got := buildWSSAddr("https://goop2.com", "12D3KooWTest")
	want := "/dns4/goop2.com/tcp/443/wss/p2p/12D3KooWTest"
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
