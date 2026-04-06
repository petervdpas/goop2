package routes

import (
	"archive/zip"
	"bytes"
	"testing"
)

func makeZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		fw, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		fw.Write([]byte(content))
	}
	zw.Close()
	return buf.Bytes()
}

func TestExtractZip_basic(t *testing.T) {
	data := makeZip(t, map[string]string{
		"manifest.json": `{"version":1}`,
		"site/index.html": "<h1>hi</h1>",
	})

	files, err := extractZip(data)
	if err != nil {
		t.Fatal(err)
	}
	if string(files["manifest.json"]) != `{"version":1}` {
		t.Fatalf("manifest = %q", files["manifest.json"])
	}
	if string(files["site/index.html"]) != "<h1>hi</h1>" {
		t.Fatalf("index = %q", files["site/index.html"])
	}
}

func TestExtractZip_stripWrapper(t *testing.T) {
	data := makeZip(t, map[string]string{
		"myexport/manifest.json":    `{"version":1}`,
		"myexport/site/index.html":  "<h1>wrapped</h1>",
	})

	files, err := extractZip(data)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := files["manifest.json"]; !ok {
		t.Fatal("wrapper prefix should be stripped")
	}
	if _, ok := files["site/index.html"]; !ok {
		t.Fatal("nested path should have wrapper stripped")
	}
}

func TestExtractZip_rejectsPathTraversal(t *testing.T) {
	data := makeZip(t, map[string]string{
		"../etc/passwd": "root:x:0:0",
		"safe.txt":     "ok",
	})

	files, err := extractZip(data)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := files["../etc/passwd"]; ok {
		t.Fatal("path traversal should be rejected")
	}
	if _, ok := files["safe.txt"]; !ok {
		t.Fatal("safe file should be kept")
	}
}

func TestExtractZip_invalidData(t *testing.T) {
	_, err := extractZip([]byte("not a zip"))
	if err == nil {
		t.Fatal("should fail on invalid zip data")
	}
}
