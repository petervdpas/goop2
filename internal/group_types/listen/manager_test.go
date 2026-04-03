package listen

import (
	"testing"
)

func TestIsStreamURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"http://radio.example.com/stream", true},
		{"https://live.example.com/audio", true},
		{"/home/user/music.mp3", false},
		{"", false},
	}
	for _, tc := range tests {
		if got := isStreamURL(tc.url); got != tc.want {
			t.Errorf("isStreamURL(%q) = %v, want %v", tc.url, got, tc.want)
		}
	}
}

func TestStreamDisplayName(t *testing.T) {
	got := streamDisplayName("https://radio.example.com/live/stream")
	if got != "radio.example.com/live/stream" {
		t.Fatalf("expected 'radio.example.com/live/stream', got %q", got)
	}
}

func TestStreamDisplayNameLong(t *testing.T) {
	long := "https://radio.example.com/very/long/path/that/exceeds/sixty/characters/total/in/the/host/plus/path"
	got := streamDisplayName(long)
	if len(got) > 60 {
		t.Fatalf("expected truncated to 60 chars, got %d: %q", len(got), got)
	}
}
