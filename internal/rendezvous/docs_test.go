package rendezvous

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func startTestServer(t *testing.T) (baseURL string, cancel context.CancelFunc) {
	t.Helper()
	srv := New("127.0.0.1:18789", "", "", "", 0, "", RelayTimingConfig{})
	ctx, c := context.WithCancel(context.Background())
	if err := srv.Start(ctx); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	return srv.URL(), c
}

func TestDocSiteLoadsPages(t *testing.T) {
	site := newDocSite()
	if len(site.Pages) == 0 {
		t.Fatal("expected at least one doc page")
	}

	slugs := map[string]bool{}
	for _, p := range site.Pages {
		slugs[p.Slug] = true
		if p.Title == "" {
			t.Errorf("page %q has empty title", p.Slug)
		}
		if p.HTML == "" {
			t.Errorf("page %q has empty HTML", p.Slug)
		}
	}

	for _, want := range []string{"overview", "quickstart", "configuration", "api", "executor"} {
		if !slugs[want] {
			t.Errorf("missing expected page %q; have %v", want, slugs)
		}
	}
}

func TestDocSitePageOrder(t *testing.T) {
	site := newDocSite()
	for i := 1; i < len(site.Pages); i++ {
		if site.Pages[i].Order <= site.Pages[i-1].Order {
			t.Errorf("pages out of order: %q (order %d) after %q (order %d)",
				site.Pages[i].Slug, site.Pages[i].Order,
				site.Pages[i-1].Slug, site.Pages[i-1].Order)
		}
	}
}

func TestDocSiteBySlugIndex(t *testing.T) {
	site := newDocSite()
	for _, p := range site.Pages {
		got, ok := site.BySlug[p.Slug]
		if !ok {
			t.Errorf("slug %q not in BySlug index", p.Slug)
			continue
		}
		if got.Title != p.Title {
			t.Errorf("BySlug[%q].Title = %q, want %q", p.Slug, got.Title, p.Title)
		}
	}
}

func TestDocsHTTPRedirect(t *testing.T) {
	base, cancel := startTestServer(t)
	defer cancel()

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	resp, err := client.Get(base + "/docs")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 302, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if !strings.HasPrefix(loc, "/docs/") {
		t.Fatalf("expected redirect to /docs/<slug>, got %q", loc)
	}
}

func TestDocsHTTPPage(t *testing.T) {
	base, cancel := startTestServer(t)
	defer cancel()

	resp, err := http.Get(base + "/docs/overview")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	html := string(body)

	if !strings.Contains(html, "Goop2") {
		t.Error("page body doesn't mention Goop2")
	}
	if !strings.Contains(html, "sidebar-link") {
		t.Error("page missing sidebar navigation")
	}
}

func TestDocsHTTP404(t *testing.T) {
	base, cancel := startTestServer(t)
	defer cancel()

	resp, err := http.Get(base + "/docs/nonexistent-page")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestOpenAPISpecEndpoint(t *testing.T) {
	base, cancel := startTestServer(t)
	defer cancel()

	resp, err := http.Get(base + "/api/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if ct != "application/json" {
		t.Fatalf("expected content-type application/json, got %q", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	if len(body) < 100 {
		t.Fatal("openapi spec seems too small")
	}
	if !strings.Contains(string(body), "swagger") && !strings.Contains(string(body), "openapi") {
		t.Error("response doesn't look like an OpenAPI spec")
	}
}

func TestOpenAPISpecMethodNotAllowed(t *testing.T) {
	base, cancel := startTestServer(t)
	defer cancel()

	resp, err := http.Post(base+"/api/openapi.json", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", resp.StatusCode)
	}
}

func TestExecutorAPISpecEndpoint(t *testing.T) {
	base, cancel := startTestServer(t)
	defer cancel()

	resp, err := http.Get(base + "/api/executor-api.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if ct != "application/x-yaml" {
		t.Fatalf("expected content-type application/x-yaml, got %q", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "openapi") {
		t.Error("response doesn't look like an OpenAPI spec")
	}
	if !strings.Contains(string(body), "Cluster Binary Protocol") {
		t.Error("response doesn't mention Cluster Binary Protocol")
	}
}

func TestExecutorAPISpecMethodNotAllowed(t *testing.T) {
	base, cancel := startTestServer(t)
	defer cancel()

	resp, err := http.Post(base+"/api/executor-api.yaml", "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", resp.StatusCode)
	}
}

func TestDocsAPIPage(t *testing.T) {
	base, cancel := startTestServer(t)
	defer cancel()

	resp, err := http.Get(base + "/docs/api")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	html := string(body)

	if !strings.Contains(html, "swagger-ui") {
		t.Error("API docs page doesn't contain swagger-ui div")
	}
}

func TestDocsExecutorPage(t *testing.T) {
	base, cancel := startTestServer(t)
	defer cancel()

	resp, err := http.Get(base + "/docs/executor")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	html := string(body)

	if !strings.Contains(html, "redoc-container") {
		t.Error("Executor docs page doesn't contain redoc-container div")
	}
}
