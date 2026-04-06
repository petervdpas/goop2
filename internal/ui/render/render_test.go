package render

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHighlight_Go(t *testing.T) {
	out := Highlight(`fmt.Println("hello")`, "go")
	if !strings.Contains(out, "<pre") {
		t.Errorf("expected HTML output with <pre>, got: %s", out)
	}
	if !strings.Contains(out, "Println") {
		t.Errorf("expected token Println in output")
	}
}

func TestHighlight_UnknownLanguage(t *testing.T) {
	out := Highlight("some code", "nonexistent-lang-xyz")
	if !strings.Contains(out, "some code") {
		t.Errorf("fallback should preserve code: %s", out)
	}
}

func TestHighlight_JavaScript(t *testing.T) {
	out := Highlight(`const x = 42;`, "javascript")
	if !strings.Contains(out, "42") {
		t.Errorf("expected literal in output: %s", out)
	}
}

func TestHighlight_EmptyCode(t *testing.T) {
	out := Highlight("", "go")
	if !strings.Contains(out, "<pre") {
		t.Errorf("empty code should still produce valid HTML: %s", out)
	}
}

func TestHighlight_HTML(t *testing.T) {
	out := Highlight(`<div class="test">hello</div>`, "html")
	if !strings.Contains(out, "test") {
		t.Errorf("expected content in output: %s", out)
	}
}

func TestHighlight_CSS(t *testing.T) {
	out := Highlight(`body { color: red; }`, "css")
	if !strings.Contains(out, "color") {
		t.Errorf("expected CSS property: %s", out)
	}
}

func TestInitTemplates(t *testing.T) {
	if err := InitTemplates(); err != nil {
		t.Fatalf("InitTemplates: %v", err)
	}
}

func TestInitTemplates_Idempotent(t *testing.T) {
	if err := InitTemplates(); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := InitTemplates(); err != nil {
		t.Fatalf("second: %v", err)
	}
}

func TestRenderStandalone(t *testing.T) {
	if err := InitTemplates(); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	RenderStandalone(w, "page.error_unreachable", struct{ Detail string }{Detail: "test error"})

	if w.Code != 200 {
		t.Fatalf("status %d, body: %s", w.Code, w.Body.String())
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type = %q", ct)
	}
	if !strings.Contains(w.Body.String(), "test error") {
		t.Error("expected error detail in output")
	}
}

func TestRenderStandalone_UnknownTemplate(t *testing.T) {
	if err := InitTemplates(); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	RenderStandalone(w, "nonexistent.template", nil)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for unknown template, got %d", w.Code)
	}
}
