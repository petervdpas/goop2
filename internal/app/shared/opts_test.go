package shared

import "testing"

func TestNormalizeLocalViewer(t *testing.T) {
	cases := []struct {
		input      string
		wantListen string
		wantURL    string
		wantTCP    string
	}{
		{":8080", "127.0.0.1:8080", "http://127.0.0.1:8080", "127.0.0.1:8080"},
		{"0.0.0.0:8080", "127.0.0.1:8080", "http://127.0.0.1:8080", "127.0.0.1:8080"},
		{"127.0.0.1:9090", "127.0.0.1:9090", "http://127.0.0.1:9090", "127.0.0.1:9090"},
		{"  :3000  ", "127.0.0.1:3000", "http://127.0.0.1:3000", "127.0.0.1:3000"},
		{"192.168.1.1:8080", "192.168.1.1:8080", "http://192.168.1.1:8080", "192.168.1.1:8080"},
	}
	for _, tc := range cases {
		listen, url, tcp := NormalizeLocalViewer(tc.input)
		if listen != tc.wantListen {
			t.Errorf("NormalizeLocalViewer(%q) listen = %q, want %q", tc.input, listen, tc.wantListen)
		}
		if url != tc.wantURL {
			t.Errorf("NormalizeLocalViewer(%q) url = %q, want %q", tc.input, url, tc.wantURL)
		}
		if tcp != tc.wantTCP {
			t.Errorf("NormalizeLocalViewer(%q) tcp = %q, want %q", tc.input, tcp, tc.wantTCP)
		}
	}
}
