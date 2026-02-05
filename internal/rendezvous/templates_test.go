package rendezvous

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTemplateStoreEndToEnd(t *testing.T) {
	// Create a temp directory with a test template
	dir := t.TempDir()
	tplDir := filepath.Join(dir, "templates", "hello-store")
	if err := os.MkdirAll(filepath.Join(tplDir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}

	manifest := `{"name":"Hello Store","description":"A test template","category":"test","icon":"🏪"}`
	os.WriteFile(filepath.Join(tplDir, "manifest.json"), []byte(manifest), 0o644)
	os.WriteFile(filepath.Join(tplDir, "src", "index.html"), []byte("<h1>hello</h1>"), 0o644)
	os.WriteFile(filepath.Join(tplDir, "src", "style.css"), []byte("body{}"), 0o644)

	// Start rendezvous server with template store
	srv := New("127.0.0.1:18787", []string{filepath.Join(dir, "templates")}, "", "", "", false, "", SMTPConfig{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		t.Fatal(err)
	}

	baseURL := srv.URL()
	t.Logf("server at %s", baseURL)

	// Give server a moment to start
	time.Sleep(50 * time.Millisecond)

	// Test 1: GET /api/templates — should return embedded + disk templates
	t.Run("list templates", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/templates")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		var list []StoreMeta
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			t.Fatal(err)
		}

		// 3 embedded (corkboard, quiz, photobook) + 1 disk (hello-store)
		if len(list) < 4 {
			t.Fatalf("expected at least 4 templates, got %d", len(list))
		}

		dirs := map[string]bool{}
		for _, m := range list {
			dirs[m.Dir] = true
			if m.Source != "store" {
				t.Fatalf("expected source=store for %q, got %q", m.Dir, m.Source)
			}
		}
		for _, want := range []string{"corkboard", "quiz", "photobook", "hello-store"} {
			if !dirs[want] {
				t.Fatalf("missing expected template %q", want)
			}
		}
	})

	// Test 2: GET /api/templates/hello-store/manifest.json
	t.Run("get manifest", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/templates/hello-store/manifest.json")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		var meta StoreMeta
		if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
			t.Fatal(err)
		}
		if meta.Name != "Hello Store" {
			t.Fatalf("expected name=Hello Store, got %q", meta.Name)
		}
	})

	// Test 3: GET /api/templates/hello-store/bundle — tar.gz download
	t.Run("download bundle", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/templates/hello-store/bundle")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		ct := resp.Header.Get("Content-Type")
		if ct != "application/gzip" {
			t.Fatalf("expected content-type application/gzip, got %q", ct)
		}

		// Extract and verify contents
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if len(body) == 0 {
			t.Fatal("empty bundle")
		}

		// Verify we can extract it
		files, err := testExtractTarGz(bytes.NewReader(body))
		if err != nil {
			t.Fatalf("failed to extract bundle: %v", err)
		}

		if _, ok := files["manifest.json"]; !ok {
			t.Fatal("bundle missing manifest.json")
		}
		if _, ok := files["src/index.html"]; !ok {
			t.Fatal("bundle missing src/index.html")
		}
		if _, ok := files["src/style.css"]; !ok {
			t.Fatal("bundle missing src/style.css")
		}
	})

	// Test 4: GET /api/templates/nonexistent/bundle — 404
	t.Run("missing template 404", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/templates/nonexistent/bundle")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()

		if resp.StatusCode != 404 {
			t.Fatalf("expected 404, got %d", resp.StatusCode)
		}
	})

	// Test 5: Client ListTemplates
	t.Run("client list templates", func(t *testing.T) {
		client := NewClient(baseURL)
		list, err := client.ListTemplates(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(list) < 4 {
			t.Fatalf("expected at least 4 templates, got %d", len(list))
		}
	})

	// Test 6: Download embedded template bundle (corkboard)
	t.Run("download embedded bundle", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/templates/corkboard/bundle")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		files, err := testExtractTarGz(resp.Body)
		if err != nil {
			t.Fatalf("extract: %v", err)
		}

		if _, ok := files["manifest.json"]; !ok {
			t.Fatal("bundle missing manifest.json")
		}
		if _, ok := files["index.html"]; !ok {
			t.Fatal("bundle missing index.html")
		}
		if _, ok := files["schema.sql"]; !ok {
			t.Fatal("bundle missing schema.sql")
		}
	})

	// Test 7: Client DownloadTemplateBundle + testExtractTarGz
	t.Run("client download and extract", func(t *testing.T) {
		client := NewClient(baseURL)
		rc, err := client.DownloadTemplateBundle(ctx, "hello-store")
		if err != nil {
			t.Fatal(err)
		}
		defer rc.Close()

		files, err := testExtractTarGz(rc)
		if err != nil {
			t.Fatalf("extract: %v", err)
		}

		if _, ok := files["manifest.json"]; !ok {
			t.Fatal("missing manifest.json")
		}
		if string(files["src/index.html"]) != "<h1>hello</h1>" {
			t.Fatalf("unexpected index.html content: %q", string(files["src/index.html"]))
		}
	})
}

// testExtractTarGz is a test helper that extracts a tar.gz stream to a map.
func testExtractTarGz(r io.Reader) (map[string][]byte, error) {
	gr, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	files := make(map[string][]byte)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		// Strip top-level dir prefix
		name := filepath.ToSlash(hdr.Name)
		if i := strings.IndexByte(name, '/'); i >= 0 {
			name = name[i+1:]
		}
		if name == "" {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return nil, err
		}
		files[name] = data
	}
	return files, nil
}
