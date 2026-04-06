package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNormalizeRel(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", "index.html"},
		{".", "index.html"},
		{" ", "index.html"},
		{"index.html", "index.html"},
		{"/index.html", "index.html"},
		{"  /foo/bar.html  ", "foo/bar.html"},
		{`foo\bar.html`, "foo/bar.html"},
		{"./css/style.css", "css/style.css"},
		{"//double//slash", "double/slash"},
	}
	for _, tc := range tests {
		got := normalizeRel(tc.in)
		if got != tc.want {
			t.Errorf("normalizeRel(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDirOf(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", ""},
		{".", ""},
		{"index.html", ""},
		{"foo/bar.html", "foo"},
		{"/foo/bar/baz.html", "foo/bar"},
		{`foo\bar\baz.html`, "foo/bar"},
	}
	for _, tc := range tests {
		got := dirOf(tc.in)
		if got != tc.want {
			t.Errorf("dirOf(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestAtoiOrNeg(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"0", 0},
		{"42", 42},
		{"100", 100},
		{"", 0},
		{"abc", -1},
		{"12x", -1},
		{"-1", -1},
	}
	for _, tc := range tests {
		got := atoiOrNeg(tc.in)
		if got != tc.want {
			t.Errorf("atoiOrNeg(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestIsImageExt(t *testing.T) {
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".ico", ".bmp"} {
		if !isImageExt("file" + ext) {
			t.Errorf("isImageExt(%q) should be true", "file"+ext)
		}
	}
	for _, ext := range []string{".html", ".css", ".js", ".go", ".txt", ""} {
		if isImageExt("file" + ext) {
			t.Errorf("isImageExt(%q) should be false", "file"+ext)
		}
	}
	if !isImageExt("photo.PNG") {
		t.Error("isImageExt should be case-insensitive")
	}
}

func TestIsValidTheme(t *testing.T) {
	if !isValidTheme("light") {
		t.Error("light should be valid")
	}
	if !isValidTheme("dark") {
		t.Error("dark should be valid")
	}
	if isValidTheme("") {
		t.Error("empty should be invalid")
	}
	if isValidTheme("blue") {
		t.Error("blue should be invalid")
	}
}

func TestFormBool(t *testing.T) {
	tests := []struct {
		val  string
		want bool
	}{
		{"on", true},
		{"1", true},
		{"true", true},
		{"yes", true},
		{"ON", true},
		{"True", true},
		{"YES", true},
		{"", false},
		{"off", false},
		{"0", false},
		{"false", false},
		{"no", false},
	}
	for _, tc := range tests {
		form := map[string][]string{"key": {tc.val}}
		got := formBool(form, "key")
		if got != tc.want {
			t.Errorf("formBool(%q) = %v, want %v", tc.val, got, tc.want)
		}
	}
	if formBool(nil, "missing") {
		t.Error("missing key should return false")
	}
}

func TestGetTrimmedPostFormValue(t *testing.T) {
	form := map[string][]string{
		"name":  {"  Alice  "},
		"empty": {""},
	}
	if got := getTrimmedPostFormValue(form, "name"); got != "Alice" {
		t.Errorf("got %q, want %q", got, "Alice")
	}
	if got := getTrimmedPostFormValue(form, "empty"); got != "" {
		t.Errorf("got %q, want empty", got)
	}
	if got := getTrimmedPostFormValue(form, "missing"); got != "" {
		t.Errorf("got %q, want empty for missing key", got)
	}
}

func TestSafeCall(t *testing.T) {
	if got := safeCall(nil); got != "" {
		t.Errorf("safeCall(nil) = %q, want empty", got)
	}
	if got := safeCall(func() string { return "hello" }); got != "hello" {
		t.Errorf("safeCall(fn) = %q, want %q", got, "hello")
	}
}

func TestNewToken(t *testing.T) {
	tok := newToken(16)
	if len(tok) != 32 {
		t.Errorf("newToken(16) length = %d, want 32 hex chars", len(tok))
	}
	tok2 := newToken(16)
	if tok == tok2 {
		t.Error("consecutive tokens should differ")
	}
}

func TestIsLocalRequest(t *testing.T) {
	local := httptest.NewRequest("GET", "/", nil)
	local.RemoteAddr = "127.0.0.1:12345"
	if !isLocalRequest(local) {
		t.Error("127.0.0.1 should be local")
	}

	local6 := httptest.NewRequest("GET", "/", nil)
	local6.RemoteAddr = "[::1]:12345"
	if !isLocalRequest(local6) {
		t.Error("::1 should be local")
	}

	remote := httptest.NewRequest("GET", "/", nil)
	remote.RemoteAddr = "192.168.1.100:12345"
	if isLocalRequest(remote) {
		t.Error("192.168.1.100 should not be local")
	}

	bad := httptest.NewRequest("GET", "/", nil)
	bad.RemoteAddr = "invalid"
	if isLocalRequest(bad) {
		t.Error("invalid RemoteAddr should not be local")
	}
}

func TestRequireMethod(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	if !requireMethod(w, r, http.MethodGet) {
		t.Error("GET should match GET")
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest("POST", "/", nil)
	if requireMethod(w, r, http.MethodGet) {
		t.Error("POST should not match GET")
	}
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestRequireLocal(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "127.0.0.1:9999"
	if !requireLocal(w, r) {
		t.Error("localhost should pass")
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.1:9999"
	if requireLocal(w, r) {
		t.Error("remote should fail")
	}
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestRequireContentStore(t *testing.T) {
	w := httptest.NewRecorder()
	if requireContentStore(w, nil) {
		t.Error("nil store should return false")
	}
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}

	w = httptest.NewRecorder()
	if !requireContentStore(w, "something") {
		t.Error("non-nil store should return true")
	}
}

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, map[string]string{"key": "val"})

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got map[string]string
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got["key"] != "val" {
		t.Errorf("got %v", got)
	}
}

func TestTopologyHandler_noNode(t *testing.T) {
	d := Deps{}
	handler := topologyHandler(d)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/topology", nil)
	handler(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestTopologyHandler_withFunc(t *testing.T) {
	d := Deps{
		TopologyFunc: func() any {
			return map[string]string{"nodes": "3"}
		},
	}
	handler := topologyHandler(d)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/topology", nil)
	handler(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var got map[string]string
	json.NewDecoder(w.Body).Decode(&got)
	if got["nodes"] != "3" {
		t.Errorf("got %v", got)
	}
}

func TestHandleGet_rejectsPost(t *testing.T) {
	mux := http.NewServeMux()
	handleGet(mux, "/test", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/test", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandlePostAction_rejectsGet(t *testing.T) {
	mux := http.NewServeMux()
	handlePostAction(mux, "/test", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/test", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestFetchServiceHealth_emptyURL(t *testing.T) {
	result := fetchServiceHealth(http.DefaultClient, "", "", nil)
	if result["ok"] != false {
		t.Error("empty URL should return ok=false")
	}
}
