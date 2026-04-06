package routes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func siteTestMux(t *testing.T) *http.ServeMux {
	t.Helper()
	d, _ := testDeps(t)
	mux := http.NewServeMux()
	registerSiteAPIRoutes(mux, d)
	return mux
}

func TestSiteListFiles(t *testing.T) {
	mux := siteTestMux(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/site/files", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestSiteListFiles_noStore(t *testing.T) {
	mux := http.NewServeMux()
	registerSiteAPIRoutes(mux, Deps{})

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/site/files", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestSiteContent_notFound(t *testing.T) {
	mux := siteTestMux(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/site/content?path=nonexistent.html", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestSiteContent_missingPath(t *testing.T) {
	mux := siteTestMux(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/site/content", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (empty path normalizes to index.html which doesn't exist)", w.Code, http.StatusNotFound)
	}
}

func TestSiteDelete_missingPath(t *testing.T) {
	mux := siteTestMux(t)
	body := `{"path": ""}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/site/delete", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestSiteUploadLocal_missingFields(t *testing.T) {
	mux := siteTestMux(t)
	w := httptest.NewRecorder()
	body := `{"dest_path": "", "src_path": ""}`
	r := httptest.NewRequest("POST", "/api/site/upload-local", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestSiteDelete_noStore(t *testing.T) {
	mux := http.NewServeMux()
	registerSiteAPIRoutes(mux, Deps{})

	body := `{"path": "test.html"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/site/delete", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestSiteContent_rejectsPost(t *testing.T) {
	mux := siteTestMux(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/site/content", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestSiteWriteAndRead(t *testing.T) {
	d, _ := testDeps(t)
	mux := http.NewServeMux()
	registerSiteAPIRoutes(mux, d)

	d.Content.Write(context.Background(), "hello.txt", []byte("world"), "")

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/site/content?path=hello.txt", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var result map[string]string
	json.NewDecoder(w.Body).Decode(&result)
	if result["content"] != "world" {
		t.Fatalf("content = %q, want world", result["content"])
	}
}
