package rendezvous

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// mockTemplatesService creates a test HTTP server that mimics the remote
// templates service with a fixed set of templates.
func mockTemplatesService(t *testing.T) *httptest.Server {
	t.Helper()

	templates := []StoreMeta{
		{Name: "Corkboard", Dir: "corkboard", Source: "store", Category: "productivity", Icon: "📋"},
		{Name: "Quiz", Dir: "quiz", Source: "store", Category: "education", Icon: "❓"},
		{Name: "Chess", Dir: "chess", Source: "store", Category: "game", Icon: "♟️"},
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	mux.HandleFunc("/api/templates/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"api_version":    1,
			"version":        "test",
			"dummy_mode":     false,
			"template_count": len(templates),
		})
	})

	mux.HandleFunc("/api/templates", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(templates)
	})

	mux.HandleFunc("/api/templates/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/templates/")
		parts := strings.SplitN(path, "/", 2)
		if len(parts) != 2 {
			http.NotFound(w, r)
			return
		}

		dir, action := parts[0], parts[1]

		// Find template
		var found *StoreMeta
		for _, m := range templates {
			if m.Dir == dir {
				found = &m
				break
			}
		}
		if found == nil {
			http.NotFound(w, r)
			return
		}

		switch action {
		case "manifest":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(found)

		case "bundle":
			// Write a minimal tar.gz with manifest + index.html
			var buf bytes.Buffer
			gw := gzip.NewWriter(&buf)
			tw := tar.NewWriter(gw)

			manifest, _ := json.Marshal(found)
			writeFile(tw, dir+"/manifest.json", manifest)
			writeFile(tw, dir+"/index.html", []byte("<h1>"+found.Name+"</h1>"))

			tw.Close()
			gw.Close()

			w.Header().Set("Content-Type", "application/gzip")
			w.Write(buf.Bytes())

		default:
			http.NotFound(w, r)
		}
	})

	return httptest.NewServer(mux)
}

func writeFile(tw *tar.Writer, name string, data []byte) {
	tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(data))})
	tw.Write(data)
}

func TestRemoteTemplatesIntegration(t *testing.T) {
	// Start mock templates service
	tplSvc := mockTemplatesService(t)
	defer tplSvc.Close()

	// Start rendezvous server with remote templates provider
	srv := New("127.0.0.1:18787", "", "", "", 0, "")
	srv.SetTemplatesProvider(NewRemoteTemplatesProvider(tplSvc.URL, ""))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		t.Fatal(err)
	}

	baseURL := srv.URL()
	time.Sleep(50 * time.Millisecond)

	t.Run("list templates via proxy", func(t *testing.T) {
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
		if len(list) != 3 {
			t.Fatalf("expected 3 templates, got %d", len(list))
		}
	})

	t.Run("get manifest via proxy", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/templates/corkboard/manifest")
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
		if meta.Name != "Corkboard" {
			t.Fatalf("expected name=Corkboard, got %q", meta.Name)
		}
	})

	t.Run("download bundle via proxy", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/templates/chess/bundle")
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
	})

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

	t.Run("client list templates", func(t *testing.T) {
		client := NewClient(baseURL)
		list, err := client.ListTemplates(ctx, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 3 {
			t.Fatalf("expected 3 templates, got %d", len(list))
		}
	})

	t.Run("client download and extract", func(t *testing.T) {
		client := NewClient(baseURL)
		rc, err := client.DownloadTemplateBundle(ctx, "quiz", "")
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
		if string(files["index.html"]) != "<h1>Quiz</h1>" {
			t.Fatalf("unexpected index.html: %q", string(files["index.html"]))
		}
	})

	t.Run("template count from status", func(t *testing.T) {
		provider := srv.templates
		count := provider.TemplateCount()
		if count != 3 {
			t.Fatalf("expected template count 3, got %d", count)
		}
	})

	t.Run("fetch templates from provider", func(t *testing.T) {
		provider := srv.templates
		list, err := provider.FetchTemplates()
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 3 {
			t.Fatalf("expected 3 templates, got %d", len(list))
		}
		dirs := map[string]bool{}
		for _, m := range list {
			dirs[m.Dir] = true
		}
		for _, want := range []string{"corkboard", "quiz", "chess"} {
			if !dirs[want] {
				t.Fatalf("missing template %q", want)
			}
		}
	})

	t.Run("no templates without provider", func(t *testing.T) {
		// Start a server without templates provider — /api/templates should 404
		srv2 := New("127.0.0.1:18788", "", "", "", 0, "")
		ctx2, cancel2 := context.WithCancel(context.Background())
		defer cancel2()
		if err := srv2.Start(ctx2); err != nil {
			t.Fatal(err)
		}
		time.Sleep(50 * time.Millisecond)

		resp, err := http.Get(srv2.URL() + "/api/templates")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 404 {
			t.Fatalf("expected 404 without templates provider, got %d", resp.StatusCode)
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
