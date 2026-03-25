package mapper

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/petervdpas/goop2/internal/orm/schema"
)

// ── CSV Reader ──

func TestCSVReader(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.csv")
	os.WriteFile(path, []byte("Name,Age,City\nAlice,30,Amsterdam\nBob,25,Berlin\n"), 0644)

	reader, err := NewSourceReader(DataEndpoint{Type: "csv", Path: path})
	if err != nil {
		t.Fatalf("NewSourceReader: %v", err)
	}
	rows, err := reader.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if rows[0]["Name"] != "Alice" {
		t.Errorf("row[0].Name = %v", rows[0]["Name"])
	}
	if rows[0]["Age"] != "30" {
		t.Errorf("row[0].Age = %v (CSV values are strings)", rows[0]["Age"])
	}
	if rows[1]["City"] != "Berlin" {
		t.Errorf("row[1].City = %v", rows[1]["City"])
	}
}

func TestCSVReaderEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.csv")
	os.WriteFile(path, []byte("Name,Age\n"), 0644)

	reader, _ := NewSourceReader(DataEndpoint{Type: "csv", Path: path})
	rows, err := reader.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("got %d rows, want 0", len(rows))
	}
}

func TestCSVReaderMissingFile(t *testing.T) {
	reader, _ := NewSourceReader(DataEndpoint{Type: "csv", Path: "/nonexistent/file.csv"})
	_, err := reader.Read()
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestCSVReaderNoPath(t *testing.T) {
	_, err := NewSourceReader(DataEndpoint{Type: "csv"})
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

// ── JSON File Reader ──

func TestJSONFileReader(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	data := []schema.Row{
		{"name": "Alice", "score": float64(95)},
		{"name": "Bob", "score": float64(87)},
	}
	b, _ := json.Marshal(data)
	os.WriteFile(path, b, 0644)

	reader, err := NewSourceReader(DataEndpoint{Type: "json", Path: path})
	if err != nil {
		t.Fatalf("NewSourceReader: %v", err)
	}
	rows, err := reader.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if rows[0]["name"] != "Alice" {
		t.Errorf("row[0].name = %v", rows[0]["name"])
	}
	if rows[1]["score"] != float64(87) {
		t.Errorf("row[1].score = %v", rows[1]["score"])
	}
}

func TestJSONFileReaderMissing(t *testing.T) {
	reader, _ := NewSourceReader(DataEndpoint{Type: "json", Path: "/nonexistent.json"})
	_, err := reader.Read()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestJSONReaderNoPathOrURL(t *testing.T) {
	_, err := NewSourceReader(DataEndpoint{Type: "json"})
	if err == nil {
		t.Fatal("expected error for empty path and url")
	}
}

// ── CSV Writer ──

func TestCSVWriter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.csv")

	writer, err := NewTargetWriter(DataEndpoint{Type: "csv", Path: path})
	if err != nil {
		t.Fatalf("NewTargetWriter: %v", err)
	}

	rows := []schema.Row{
		{"name": "Alice", "age": "30"},
		{"name": "Bob", "age": "25"},
	}
	n, err := writer.Write(rows)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != 2 {
		t.Errorf("written = %d, want 2", n)
	}

	content, _ := os.ReadFile(path)
	s := string(content)
	if len(s) == 0 {
		t.Fatal("file is empty")
	}
}

func TestCSVWriterEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.csv")

	writer, _ := NewTargetWriter(DataEndpoint{Type: "csv", Path: path})
	n, err := writer.Write(nil)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != 0 {
		t.Errorf("written = %d, want 0", n)
	}
}

func TestCSVWriterNoPath(t *testing.T) {
	_, err := NewTargetWriter(DataEndpoint{Type: "csv"})
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

// ── JSON Writer ──

func TestJSONWriter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.json")

	writer, err := NewTargetWriter(DataEndpoint{Type: "json", Path: path})
	if err != nil {
		t.Fatalf("NewTargetWriter: %v", err)
	}

	rows := []schema.Row{
		{"id": "abc", "val": float64(42)},
		{"id": "def", "val": float64(99)},
		{"id": "ghi", "val": float64(7)},
	}
	n, err := writer.Write(rows)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != 3 {
		t.Errorf("written = %d, want 3", n)
	}

	content, _ := os.ReadFile(path)
	var loaded []schema.Row
	if err := json.Unmarshal(content, &loaded); err != nil {
		t.Fatalf("parse output: %v", err)
	}
	if len(loaded) != 3 {
		t.Errorf("loaded %d rows, want 3", len(loaded))
	}
	if loaded[0]["id"] != "abc" {
		t.Errorf("loaded[0].id = %v", loaded[0]["id"])
	}
}

func TestJSONWriterNoPath(t *testing.T) {
	_, err := NewTargetWriter(DataEndpoint{Type: "json"})
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

// ── Unknown types ──

func TestUnknownSourceType(t *testing.T) {
	_, err := NewSourceReader(DataEndpoint{Type: "xml"})
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
}

func TestUnknownTargetType(t *testing.T) {
	_, err := NewTargetWriter(DataEndpoint{Type: "xml"})
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
}

func TestTableSourceRequiresDB(t *testing.T) {
	_, err := NewSourceReader(DataEndpoint{Type: "table", Name: "users"})
	if err == nil {
		t.Fatal("expected error for table without DB")
	}
}

func TestTableTargetRequiresDB(t *testing.T) {
	_, err := NewTargetWriter(DataEndpoint{Type: "table", Name: "users"})
	if err == nil {
		t.Fatal("expected error for table without DB")
	}
}

// ── CSV round-trip: write then read ──

func TestCSVRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "roundtrip.csv")

	rows := []schema.Row{
		{"name": "Alice", "city": "Amsterdam"},
		{"name": "Bob", "city": "Berlin"},
		{"name": "Charlie", "city": "Copenhagen"},
	}

	writer, _ := NewTargetWriter(DataEndpoint{Type: "csv", Path: path})
	writer.Write(rows)

	reader, _ := NewSourceReader(DataEndpoint{Type: "csv", Path: path})
	loaded, err := reader.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(loaded) != 3 {
		t.Fatalf("got %d rows, want 3", len(loaded))
	}
	found := false
	for _, r := range loaded {
		if r["name"] == "Charlie" && r["city"] == "Copenhagen" {
			found = true
		}
	}
	if !found {
		t.Error("Charlie/Copenhagen not found in round-trip")
	}
}

// ── JSON round-trip: write then read ──

func TestJSONRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "roundtrip.json")

	rows := []schema.Row{
		{"id": "1", "value": float64(100)},
		{"id": "2", "value": float64(200)},
	}

	writer, _ := NewTargetWriter(DataEndpoint{Type: "json", Path: path})
	writer.Write(rows)

	reader, _ := NewSourceReader(DataEndpoint{Type: "json", Path: path})
	loaded, err := reader.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("got %d rows, want 2", len(loaded))
	}
	if loaded[0]["id"] != "1" || loaded[0]["value"] != float64(100) {
		t.Errorf("row 0 mismatch: %v", loaded[0])
	}
}

// ── Full pipeline: CSV → Transform → JSON ──

func TestFullPipeline_CSV_to_JSON(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "input.csv")
	jsonPath := filepath.Join(dir, "output.json")

	os.WriteFile(csvPath, []byte("first_name,last_name,score\nAlice,Smith,95\nBob,Jones,87\n"), 0644)

	tx := &Transformation{
		Name:   "csv-to-json",
		Source: DataEndpoint{Type: "csv", Path: csvPath},
		Target: DataEndpoint{Type: "json", Path: jsonPath},
		Fields: []FieldTransform{
			{Target: "full_name", Sources: []string{"first_name", "last_name"}, Transform: "concat", Args: []any{" "}},
			{Target: "score", Sources: []string{"score"}},
			{Target: "grade", Constant: "A"},
		},
	}

	reader, err := NewSourceReader(tx.Source)
	if err != nil {
		t.Fatalf("NewSourceReader: %v", err)
	}
	rows, err := reader.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	results, err := tx.ApplyMany(rows)
	if err != nil {
		t.Fatalf("ApplyMany: %v", err)
	}
	writer, err := NewTargetWriter(tx.Target)
	if err != nil {
		t.Fatalf("NewTargetWriter: %v", err)
	}
	n, err := writer.Write(results)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != 2 {
		t.Errorf("written = %d, want 2", n)
	}

	content, _ := os.ReadFile(jsonPath)
	var loaded []schema.Row
	json.Unmarshal(content, &loaded)
	if len(loaded) != 2 {
		t.Fatalf("output has %d rows", len(loaded))
	}
	if loaded[0]["full_name"] != "Alice Smith" {
		t.Errorf("full_name = %v", loaded[0]["full_name"])
	}
	if loaded[0]["grade"] != "A" {
		t.Errorf("grade = %v", loaded[0]["grade"])
	}
}
