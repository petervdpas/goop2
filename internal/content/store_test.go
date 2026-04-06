package content

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := NewStore(dir, "site")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := s.EnsureRoot(); err != nil {
		t.Fatalf("EnsureRoot: %v", err)
	}
	return s
}

func TestNewStore_Defaults(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir, "")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if !strings.HasSuffix(s.RootAbs(), "site") {
		t.Errorf("expected root ending in site, got %q", s.RootAbs())
	}
}

func TestNewStore_AbsoluteSiteRel(t *testing.T) {
	dir := t.TempDir()
	absPath := filepath.Join(dir, "custom")
	s, err := NewStore(dir, absPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if s.RootAbs() != absPath {
		t.Errorf("expected %q, got %q", absPath, s.RootAbs())
	}
}

func TestWriteAndRead(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	data := []byte("hello world")

	etag, err := s.Write(ctx, "test.txt", data, "")
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !strings.HasPrefix(etag, "sha256:") {
		t.Errorf("bad etag: %q", etag)
	}

	got, gotEtag, err := s.Read(ctx, "test.txt")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(got) != "hello world" {
		t.Errorf("Read = %q", got)
	}
	if gotEtag != etag {
		t.Errorf("etag mismatch: %q vs %q", gotEtag, etag)
	}
}

func TestRead_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, _, err := s.Read(context.Background(), "missing.txt")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestWrite_EtagConflict(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	etag, _ := s.Write(ctx, "test.txt", []byte("v1"), "")
	s.Write(ctx, "test.txt", []byte("v2"), "")

	_, err := s.Write(ctx, "test.txt", []byte("v3"), etag)
	if err != ErrConflict {
		t.Errorf("expected ErrConflict, got %v", err)
	}
}

func TestWrite_EtagNoneForNewFile(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.Write(ctx, "new.txt", []byte("data"), "none")
	if err != nil {
		t.Errorf("Write with ifMatch=none for new file should succeed: %v", err)
	}

	_, err = s.Write(ctx, "new.txt", []byte("data2"), "none")
	if err != ErrConflict {
		t.Errorf("expected ErrConflict when file exists and ifMatch=none, got %v", err)
	}
}

func TestWrite_ImagePathEnforcement(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.Write(ctx, "photo.png", []byte("img"), "")
	if err != ErrImagePath {
		t.Errorf("expected ErrImagePath for image outside images/, got %v", err)
	}

	_, err = s.Write(ctx, "images/photo.png", []byte("img"), "")
	if err != nil {
		t.Errorf("image in images/ should succeed: %v", err)
	}
}

func TestWrite_PathTraversal(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.Write(ctx, "../escape.txt", []byte("bad"), "")
	if err != ErrOutsideRoot {
		t.Errorf("expected ErrOutsideRoot, got %v", err)
	}
}

func TestWrite_DirConflict(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	s.Mkdir(ctx, "folder")
	_, err := s.Write(ctx, "folder", []byte("data"), "")
	if err != ErrConflict {
		t.Errorf("expected ErrConflict writing to existing directory, got %v", err)
	}
}

func TestDelete(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	s.Write(ctx, "del.txt", []byte("data"), "")
	if err := s.Delete(ctx, "del.txt"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, _, err := s.Read(ctx, "del.txt")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestDelete_NotFound(t *testing.T) {
	s := newTestStore(t)
	if err := s.Delete(context.Background(), "missing.txt"); err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestMkdir(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.Mkdir(ctx, "sub/deep"); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	abs := filepath.Join(s.RootAbs(), "sub", "deep")
	st, err := os.Stat(abs)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if !st.IsDir() {
		t.Error("expected directory")
	}
}

func TestMkdir_PathTraversal(t *testing.T) {
	s := newTestStore(t)
	if err := s.Mkdir(context.Background(), "../../escape"); err != ErrOutsideRoot {
		t.Errorf("expected ErrOutsideRoot, got %v", err)
	}
}

func TestList(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	s.Write(ctx, "a.txt", []byte("a"), "")
	s.Write(ctx, "b.txt", []byte("bb"), "")
	s.Mkdir(ctx, "sub")

	items, err := s.List(ctx, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}

	names := make(map[string]bool)
	for _, fi := range items {
		names[fi.Path] = true
	}
	if !names["a.txt"] || !names["b.txt"] || !names["sub"] {
		t.Errorf("unexpected items: %v", names)
	}
}

func TestList_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.List(context.Background(), "nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestListTree(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	s.Write(ctx, "root.txt", []byte("r"), "")
	s.Write(ctx, "images/logo.png", []byte("img"), "")

	tree, err := s.ListTree(ctx, "")
	if err != nil {
		t.Fatalf("ListTree: %v", err)
	}
	if len(tree) < 3 {
		t.Fatalf("expected at least 3 items (images dir, logo.png, root.txt), got %d", len(tree))
	}

	var found bool
	for _, item := range tree {
		if item.Path == "images/logo.png" && !item.IsDir && item.Depth == 1 {
			found = true
		}
	}
	if !found {
		t.Error("expected images/logo.png at depth 1")
	}
}

func TestListTree_DirsBeforeFiles(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	s.Write(ctx, "z-file.txt", []byte("f"), "")
	s.Mkdir(ctx, "a-dir")

	tree, err := s.ListTree(ctx, "")
	if err != nil {
		t.Fatalf("ListTree: %v", err)
	}
	if len(tree) < 2 {
		t.Fatalf("expected at least 2, got %d", len(tree))
	}
	if !tree[0].IsDir {
		t.Error("expected directory first in listing")
	}
}

func TestDeletePath_File(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	s.Write(ctx, "del.txt", []byte("data"), "")
	if err := s.DeletePath(ctx, "del.txt", false); err != nil {
		t.Fatalf("DeletePath: %v", err)
	}
}

func TestDeletePath_DirRecursive(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	s.Write(ctx, "dir/sub/file.txt", []byte("data"), "")
	if err := s.DeletePath(ctx, "dir", true); err != nil {
		t.Fatalf("DeletePath recursive: %v", err)
	}
	_, err := s.List(ctx, "dir")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after recursive delete, got %v", err)
	}
}

func TestDeletePath_DirNonRecursive(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	s.Write(ctx, "dir/file.txt", []byte("data"), "")
	err := s.DeletePath(ctx, "dir", false)
	if err == nil {
		t.Error("expected error for non-recursive delete of non-empty dir")
	}
}

func TestDeletePath_NotFound(t *testing.T) {
	s := newTestStore(t)
	if err := s.DeletePath(context.Background(), "missing", false); err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestRename(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	s.Write(ctx, "old.txt", []byte("content"), "")
	if err := s.Rename(ctx, "old.txt", "new.txt"); err != nil {
		t.Fatalf("Rename: %v", err)
	}

	_, _, err := s.Read(ctx, "old.txt")
	if err != ErrNotFound {
		t.Error("old file should not exist")
	}
	data, _, err := s.Read(ctx, "new.txt")
	if err != nil {
		t.Fatalf("Read new: %v", err)
	}
	if string(data) != "content" {
		t.Errorf("content = %q", data)
	}
}

func TestRename_NotFound(t *testing.T) {
	s := newTestStore(t)
	if err := s.Rename(context.Background(), "missing.txt", "new.txt"); err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestRename_PathTraversal(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	s.Write(ctx, "test.txt", []byte("data"), "")

	if err := s.Rename(ctx, "test.txt", "../../escape.txt"); err != ErrOutsideRoot {
		t.Errorf("expected ErrOutsideRoot, got %v", err)
	}
}

func TestNormalizeDir_File(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	s.Write(ctx, "index.html", []byte("hi"), "")

	dir, err := s.NormalizeDir(ctx, "index.html")
	if err != nil {
		t.Fatalf("NormalizeDir: %v", err)
	}
	if dir != "" {
		t.Errorf("expected empty (root parent), got %q", dir)
	}
}

func TestNormalizeDir_Directory(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	s.Mkdir(ctx, "pages")

	dir, err := s.NormalizeDir(ctx, "pages")
	if err != nil {
		t.Fatalf("NormalizeDir: %v", err)
	}
	if dir != "pages" {
		t.Errorf("expected pages, got %q", dir)
	}
}

func TestNormalizeDir_Empty(t *testing.T) {
	s := newTestStore(t)
	dir, err := s.NormalizeDir(context.Background(), "")
	if err != nil {
		t.Fatalf("NormalizeDir: %v", err)
	}
	if dir != "" {
		t.Errorf("expected empty, got %q", dir)
	}
}

func TestMkdirUnder(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	result, err := s.MkdirUnder(ctx, "", "newfolder")
	if err != nil {
		t.Fatalf("MkdirUnder: %v", err)
	}
	if result != "newfolder" {
		t.Errorf("expected newfolder, got %q", result)
	}
}

func TestMkdirUnder_EmptyName(t *testing.T) {
	s := newTestStore(t)
	_, err := s.MkdirUnder(context.Background(), "", "")
	if err == nil {
		t.Error("expected error for empty folder name")
	}
}

func TestMkdirUnder_SlashInName(t *testing.T) {
	s := newTestStore(t)
	_, err := s.MkdirUnder(context.Background(), "", "a/b")
	if err == nil {
		t.Error("expected error for slash in folder name")
	}
}

func TestMkdirUnder_DotDot(t *testing.T) {
	s := newTestStore(t)
	_, err := s.MkdirUnder(context.Background(), "", "..")
	if err == nil {
		t.Error("expected error for .. folder name")
	}
}

func TestNormalizeRelPath(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"/", ""},
		{"  /foo/bar  ", "foo/bar"},
		{`foo\bar`, "foo/bar"},
		{"./foo", "foo"},
		{"/foo/../bar", "bar"},
	}
	for _, tc := range cases {
		if got := normalizeRelPath(tc.in); got != tc.want {
			t.Errorf("normalizeRelPath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestEtagBytes(t *testing.T) {
	e := etagBytes([]byte("test"))
	if !strings.HasPrefix(e, "sha256:") {
		t.Errorf("bad prefix: %q", e)
	}
	if len(e) != 7+64 {
		t.Errorf("expected len 71, got %d", len(e))
	}

	e2 := etagBytes([]byte("test"))
	if e != e2 {
		t.Error("same input should produce same etag")
	}
}

func TestCleanAbs_PathTraversal(t *testing.T) {
	s := newTestStore(t)
	_, err := s.cleanAbs("../../../etc/passwd")
	if err != ErrOutsideRoot {
		t.Errorf("expected ErrOutsideRoot, got %v", err)
	}
}

func TestWrite_CreatesParentDirs(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.Write(ctx, "deep/nested/file.txt", []byte("data"), "")
	if err != nil {
		t.Fatalf("Write nested: %v", err)
	}
	data, _, _ := s.Read(ctx, "deep/nested/file.txt")
	if string(data) != "data" {
		t.Errorf("content = %q", data)
	}
}
