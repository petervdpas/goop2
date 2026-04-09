package listen

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestUpdateQueueInfoLocked(t *testing.T) {
	m := NewTestManagerOpts(TestManagerOpts{SelfID: "me"})
	m.SetTestGroupFull(&Group{ID: "g", Role: "host"})
	m.queue = []string{"/music/song.mp3", "https://radio.example.com/stream"}
	m.queueIdx = 1

	m.updateQueueInfoLocked()

	g := m.group
	if g.QueueTotal != 2 {
		t.Fatalf("total = %d, want 2", g.QueueTotal)
	}
	if g.QueueIndex != 1 {
		t.Fatalf("index = %d, want 1", g.QueueIndex)
	}
	if g.QueueTypes[0] != "file" {
		t.Fatalf("type[0] = %q, want 'file'", g.QueueTypes[0])
	}
	if g.QueueTypes[1] != "stream" {
		t.Fatalf("type[1] = %q, want 'stream'", g.QueueTypes[1])
	}
	if g.Queue[0] != "song.mp3" {
		t.Fatalf("name[0] = %q, want 'song.mp3'", g.Queue[0])
	}
}

func TestUpdateQueueInfoLockedEmpty(t *testing.T) {
	m := NewTestManagerOpts(TestManagerOpts{SelfID: "me"})
	m.SetTestGroupFull(&Group{
		ID: "g", Role: "host",
		Queue: []string{"old"}, QueueTotal: 1,
	})
	m.queue = nil

	m.updateQueueInfoLocked()

	if m.group.QueueTotal != 0 {
		t.Fatalf("total = %d, want 0", m.group.QueueTotal)
	}
	if m.group.Queue != nil {
		t.Fatal("queue should be nil")
	}
}

func TestUpdateQueueInfoLockedNilGroup(t *testing.T) {
	m := NewTestManagerOpts(TestManagerOpts{SelfID: "me"})
	m.updateQueueInfoLocked()
}

func TestStreamDisplayNameInvalidURL(t *testing.T) {
	got := streamDisplayName("://bad")
	if got != "://bad" {
		t.Fatalf("expected raw string back, got %q", got)
	}
}

func TestStreamDisplayNameEmptyHost(t *testing.T) {
	got := streamDisplayName("/just/a/path")
	if got != "/just/a/path" {
		t.Fatalf("expected raw string, got %q", got)
	}
}

func writeMinimalMP3(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "test.mp3")

	var buf bytes.Buffer
	buf.Write([]byte("ID3"))
	buf.WriteByte(4)
	buf.WriteByte(0)
	buf.WriteByte(0)
	buf.Write([]byte{0, 0, 0, 0})

	frame := make([]byte, 4)
	frame[0] = 0xFF
	frame[1] = 0xFB
	frame[2] = 0x90
	frame[3] = 0x00
	buf.Write(frame)

	padding := make([]byte, 417-4)
	buf.Write(padding)

	for range 99 {
		buf.Write(frame)
		buf.Write(padding)
	}

	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestProbeMP3(t *testing.T) {
	dir := t.TempDir()
	path := writeMinimalMP3(t, dir)

	info, err := probeMP3(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Bitrate == 0 {
		t.Fatal("expected non-zero bitrate")
	}
	if info.Duration <= 0 {
		t.Fatal("expected positive duration")
	}
}

func TestProbeMP3FileNotFound(t *testing.T) {
	_, err := probeMP3("/nonexistent/file.mp3")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestProbeMP3NotMP3(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fake.mp3")
	os.WriteFile(path, []byte("this is not an mp3 file at all, just random text data"), 0644)

	_, err := probeMP3(path)
	if err == nil {
		t.Fatal("expected error for non-MP3 file")
	}
}

func TestProbeMP3TooSmall(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tiny.mp3")
	os.WriteFile(path, []byte{0xFF, 0xFB}, 0644)

	_, err := probeMP3(path)
	if err == nil {
		t.Fatal("expected error for tiny file")
	}
}

func TestRatePacerStream(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audio.raw")
	data := bytes.Repeat([]byte{0xAB}, 1024)
	os.WriteFile(path, data, 0644)

	f, _ := os.Open(path)
	defer f.Close()

	done := make(chan struct{})
	rp := &ratePacer{file: f, bitrate: 128000, done: done}

	var buf bytes.Buffer
	if err := rp.stream(&buf); err != nil {
		t.Fatal(err)
	}

	if buf.Len() != 1024 {
		t.Fatalf("expected 1024 bytes, got %d", buf.Len())
	}
}

func TestRatePacerStreamStopSignal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audio.raw")
	os.WriteFile(path, bytes.Repeat([]byte{0xAB}, 1024*1024), 0644)

	f, _ := os.Open(path)
	defer f.Close()

	done := make(chan struct{})
	close(done)

	rp := &ratePacer{file: f, bitrate: 128000, done: done}

	var buf bytes.Buffer
	if err := rp.stream(&buf); err != nil {
		t.Fatal(err)
	}
}

func TestSetEncryptor(t *testing.T) {
	m := NewTestManagerOpts(TestManagerOpts{SelfID: "me"})
	enc := &testEncryptor{}
	m.SetEncryptor(enc)
	if m.enc == nil {
		t.Fatal("encryptor should be set")
	}
}

type testEncryptor struct{}

func (e *testEncryptor) Seal(_ string, plaintext []byte) (string, error) {
	return "ENC:" + string(plaintext), nil
}

func (e *testEncryptor) Open(_ string, ciphertext string) ([]byte, error) {
	return []byte(ciphertext[4:]), nil
}
