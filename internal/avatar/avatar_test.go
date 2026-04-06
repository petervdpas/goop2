package avatar

import (
	"strings"
	"testing"
)

func TestNewStore_NoAvatar(t *testing.T) {
	s := NewStore(t.TempDir())
	if h := s.Hash(); h != "" {
		t.Errorf("expected empty hash, got %q", h)
	}
}

func TestStore_WriteAndRead(t *testing.T) {
	s := NewStore(t.TempDir())
	data := []byte("fake png data")

	if err := s.Write(data); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if h := s.Hash(); h == "" {
		t.Error("expected non-empty hash after write")
	}

	got, err := s.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("Read = %q, want %q", got, data)
	}
}

func TestStore_ReadNoAvatar(t *testing.T) {
	s := NewStore(t.TempDir())
	data, err := s.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if data != nil {
		t.Errorf("expected nil, got %q", data)
	}
}

func TestStore_Delete(t *testing.T) {
	s := NewStore(t.TempDir())
	s.Write([]byte("data"))

	if err := s.Delete(); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if h := s.Hash(); h != "" {
		t.Errorf("hash should be empty after delete, got %q", h)
	}
	data, _ := s.Read()
	if data != nil {
		t.Error("data should be nil after delete")
	}
}

func TestStore_DeleteNonExistent(t *testing.T) {
	s := NewStore(t.TempDir())
	if err := s.Delete(); err != nil {
		t.Fatalf("Delete non-existent should not error: %v", err)
	}
}

func TestStore_HashChangesOnWrite(t *testing.T) {
	s := NewStore(t.TempDir())
	s.Write([]byte("first"))
	h1 := s.Hash()
	s.Write([]byte("second"))
	h2 := s.Hash()
	if h1 == h2 {
		t.Error("hash should change when content changes")
	}
}

func TestStore_HashDeterministic(t *testing.T) {
	s := NewStore(t.TempDir())
	s.Write([]byte("same"))
	h1 := s.Hash()
	s.Write([]byte("same"))
	h2 := s.Hash()
	if h1 != h2 {
		t.Error("same content should produce same hash")
	}
}

func TestStore_InitialHashFromExistingFile(t *testing.T) {
	dir := t.TempDir()
	s1 := NewStore(dir)
	s1.Write([]byte("persisted"))
	want := s1.Hash()

	s2 := NewStore(dir)
	if got := s2.Hash(); got != want {
		t.Errorf("new store should pick up existing hash: got %q, want %q", got, want)
	}
}

func TestHashBytes_Length(t *testing.T) {
	h := hashBytes([]byte("test"))
	if len(h) != 16 {
		t.Errorf("expected 16 hex chars, got %d: %q", len(h), h)
	}
}

func TestExtractInitials(t *testing.T) {
	cases := []struct {
		label string
		want  string
	}{
		{"", "?"},
		{"  ", "?"},
		{"Alice", "AL"},
		{"a", "A"},
		{"Alice Bob", "AB"},
		{"Alice Bob Charlie", "AB"},
		{"日本語", "日本"},
		{"日", "日"},
	}
	for _, tc := range cases {
		if got := extractInitials(tc.label); got != tc.want {
			t.Errorf("extractInitials(%q) = %q, want %q", tc.label, got, tc.want)
		}
	}
}

func TestDeterministicColor(t *testing.T) {
	c1 := deterministicColor("alice@example.com")
	c2 := deterministicColor("alice@example.com")
	if c1 != c2 {
		t.Error("same input should produce same color")
	}
	if !strings.HasPrefix(c1, "#") {
		t.Errorf("color should be hex: %q", c1)
	}
}

func TestDeterministicColor_DifferentInputs(t *testing.T) {
	colors := make(map[string]bool)
	inputs := []string{"alice", "bob", "charlie", "dave", "eve", "frank"}
	for _, s := range inputs {
		colors[deterministicColor(s)] = true
	}
	if len(colors) < 2 {
		t.Error("expected some color diversity across different inputs")
	}
}

func TestInitialsSVG(t *testing.T) {
	svg := InitialsSVG("Alice Bob", "alice@test.com")
	s := string(svg)
	if !strings.Contains(s, "<svg") {
		t.Error("expected SVG tag")
	}
	if !strings.Contains(s, "AB") {
		t.Error("expected initials AB")
	}
	if !strings.Contains(s, "256") {
		t.Error("expected 256 dimension")
	}
}

func TestInitialsSVG_EmptyLabel(t *testing.T) {
	svg := InitialsSVG("", "test@test.com")
	if !strings.Contains(string(svg), "?") {
		t.Error("expected ? for empty label")
	}
}

func TestCache_PutAndGet(t *testing.T) {
	c := NewCache(t.TempDir())
	data := []byte("avatar data")
	hash := "abc123"

	if err := c.Put("peer-1", hash, data); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := c.Get("peer-1", hash)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("Get = %q, want %q", got, data)
	}
}

func TestCache_GetHashMismatch(t *testing.T) {
	c := NewCache(t.TempDir())
	c.Put("peer-1", "hash1", []byte("data"))

	got, err := c.Get("peer-1", "hash2")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != nil {
		t.Error("expected nil for hash mismatch")
	}
}

func TestCache_GetEmptyHash(t *testing.T) {
	c := NewCache(t.TempDir())
	c.Put("peer-1", "h", []byte("data"))

	got, _ := c.Get("peer-1", "")
	if got != nil {
		t.Error("expected nil for empty hash")
	}
}

func TestCache_GetMissingPeer(t *testing.T) {
	c := NewCache(t.TempDir())
	got, err := c.Get("nonexistent", "hash")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != nil {
		t.Error("expected nil for missing peer")
	}
}

func TestCache_GetAny(t *testing.T) {
	c := NewCache(t.TempDir())
	data := []byte("any avatar")
	c.Put("peer-1", "hash", data)

	got, err := c.GetAny("peer-1")
	if err != nil {
		t.Fatalf("GetAny: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("GetAny = %q, want %q", got, data)
	}
}

func TestCache_GetAnyMissing(t *testing.T) {
	c := NewCache(t.TempDir())
	got, err := c.GetAny("nonexistent")
	if err != nil {
		t.Fatalf("GetAny: %v", err)
	}
	if got != nil {
		t.Error("expected nil")
	}
}

func TestCache_Clear(t *testing.T) {
	c := NewCache(t.TempDir())
	c.Put("peer-1", "h", []byte("data"))
	c.Clear()

	got, _ := c.Get("peer-1", "h")
	if got != nil {
		t.Error("expected nil after clear")
	}
}
