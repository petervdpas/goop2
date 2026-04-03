package files

import (
	"strings"
	"testing"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSaveAndRead(t *testing.T) {
	s := testStore(t)
	data := []byte("hello world")

	hash, err := s.Save("g1", "readme.txt", data)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "sha256:") {
		t.Fatalf("expected sha256 hash, got %q", hash)
	}

	got, gotHash, err := s.Read("g1", "readme.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello world" {
		t.Fatalf("got %q, want 'hello world'", got)
	}
	if gotHash != hash {
		t.Fatalf("hash mismatch: save=%q read=%q", hash, gotHash)
	}
}

func TestReadNotFound(t *testing.T) {
	s := testStore(t)
	_, _, err := s.Read("g1", "nope.txt")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestDelete(t *testing.T) {
	s := testStore(t)
	s.Save("g1", "tmp.txt", []byte("x"))

	if err := s.Delete("g1", "tmp.txt"); err != nil {
		t.Fatal(err)
	}
	_, _, err := s.Read("g1", "tmp.txt")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestDeleteNotFound(t *testing.T) {
	s := testStore(t)
	if err := s.Delete("g1", "nope.txt"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestList(t *testing.T) {
	s := testStore(t)
	s.Save("g1", "a.txt", []byte("aaa"))
	s.Save("g1", "b.txt", []byte("bbb"))

	files, err := s.List("g1")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}

	names := map[string]bool{}
	for _, f := range files {
		names[f.Name] = true
		if f.Size == 0 {
			t.Fatalf("file %s has zero size", f.Name)
		}
		if !strings.HasPrefix(f.Hash, "sha256:") {
			t.Fatalf("file %s has bad hash: %q", f.Name, f.Hash)
		}
	}
	if !names["a.txt"] || !names["b.txt"] {
		t.Fatalf("expected a.txt and b.txt, got %v", names)
	}
}

func TestListEmpty(t *testing.T) {
	s := testStore(t)
	files, err := s.List("nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("expected 0 files, got %d", len(files))
	}
}

func TestListGroups(t *testing.T) {
	s := testStore(t)
	s.Save("g1", "a.txt", []byte("a"))
	s.Save("g2", "b.txt", []byte("b"))

	groups, err := s.ListGroups()
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
}

func TestHasFiles(t *testing.T) {
	s := testStore(t)
	if s.HasFiles("g1") {
		t.Fatal("should have no files initially")
	}
	s.Save("g1", "x.txt", []byte("x"))
	if !s.HasFiles("g1") {
		t.Fatal("should have files after save")
	}
}

func TestSaveOverwrite(t *testing.T) {
	s := testStore(t)
	s.Save("g1", "f.txt", []byte("v1"))
	s.Save("g1", "f.txt", []byte("v2"))

	got, _, _ := s.Read("g1", "f.txt")
	if string(got) != "v2" {
		t.Fatalf("expected 'v2', got %q", got)
	}
}

func TestFileTooLarge(t *testing.T) {
	s := testStore(t)
	big := make([]byte, MaxFileSize+1)
	_, err := s.Save("g1", "big.bin", big)
	if err != ErrTooLarge {
		t.Fatalf("expected ErrTooLarge, got %v", err)
	}
}

func TestBadFilenames(t *testing.T) {
	s := testStore(t)
	bad := []string{"", ".", "..", ".hidden", "a/b", "a\\b"}
	for _, name := range bad {
		_, err := s.Save("g1", name, []byte("x"))
		if err != ErrBadName {
			t.Fatalf("filename %q: expected ErrBadName, got %v", name, err)
		}
	}
}

func TestListJSON(t *testing.T) {
	s := testStore(t)
	s.Save("g1", "doc.txt", []byte("content"))

	data, err := s.ListJSON("g1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "doc.txt") {
		t.Fatalf("JSON should contain filename, got %s", data)
	}
}
