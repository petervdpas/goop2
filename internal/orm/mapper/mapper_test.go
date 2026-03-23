package mapper

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/petervdpas/goop2/internal/orm/schema"
)

// ── Apply basics ──

func TestApplySimpleRename(t *testing.T) {
	m := &Mapping{
		Name: "test",
		Fields: []FieldMapping{
			{Target: "full_name", Sources: []string{"name"}},
			{Target: "age_years", Sources: []string{"age"}},
		},
	}

	row := schema.Row{"name": "Alice", "age": 30.0}
	out, err := m.Apply(row)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if out["full_name"] != "Alice" {
		t.Errorf("full_name = %v, want Alice", out["full_name"])
	}
	if out["age_years"] != 30.0 {
		t.Errorf("age_years = %v, want 30", out["age_years"])
	}
}

func TestApplyConstant(t *testing.T) {
	m := &Mapping{
		Name: "test",
		Fields: []FieldMapping{
			{Target: "currency", Constant: "EUR"},
			{Target: "status", Constant: float64(1)},
			{Target: "active", Constant: true},
		},
	}

	out, err := m.Apply(schema.Row{})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if out["currency"] != "EUR" {
		t.Errorf("currency = %v", out["currency"])
	}
	if out["status"] != float64(1) {
		t.Errorf("status = %v", out["status"])
	}
	if out["active"] != true {
		t.Errorf("active = %v", out["active"])
	}
}

func TestApplyMissingSource(t *testing.T) {
	m := &Mapping{
		Name: "test",
		Fields: []FieldMapping{
			{Target: "out", Sources: []string{"nonexistent"}},
		},
	}
	out, err := m.Apply(schema.Row{"other": "value"})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if out["out"] != nil {
		t.Errorf("out = %v, want nil", out["out"])
	}
}

func TestApplyMultipleSourcesNoTransform(t *testing.T) {
	m := &Mapping{
		Name: "test",
		Fields: []FieldMapping{
			{Target: "both", Sources: []string{"a", "b"}},
		},
	}
	out, err := m.Apply(schema.Row{"a": "x", "b": "y"})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	arr, ok := out["both"].([]any)
	if !ok {
		t.Fatalf("both is not []any: %T", out["both"])
	}
	if len(arr) != 2 || arr[0] != "x" || arr[1] != "y" {
		t.Errorf("both = %v", arr)
	}
}

func TestApplyNoSourcesNoTransform(t *testing.T) {
	m := &Mapping{
		Name: "test",
		Fields: []FieldMapping{
			{Target: "empty", Constant: "ok"},
		},
	}
	out, err := m.Apply(schema.Row{})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if out["empty"] != "ok" {
		t.Errorf("empty = %v", out["empty"])
	}
}

func TestApplyUnknownTransform(t *testing.T) {
	m := &Mapping{
		Name: "test",
		Fields: []FieldMapping{
			{Target: "x", Sources: []string{"a"}, Transform: "bogus"},
		},
	}
	_, err := m.Apply(schema.Row{"a": "val"})
	if err == nil {
		t.Fatal("expected error for unknown transform")
	}
	if !strings.Contains(err.Error(), "unknown transform") {
		t.Errorf("error = %v", err)
	}
}

// ── ApplyMany ──

func TestApplyMany(t *testing.T) {
	m := &Mapping{
		Name: "test",
		Fields: []FieldMapping{
			{Target: "name", Sources: []string{"label"}, Transform: "uppercase"},
		},
	}

	rows := []schema.Row{
		{"label": "alice"},
		{"label": "bob"},
		{"label": "charlie"},
	}
	results, err := m.ApplyMany(rows)
	if err != nil {
		t.Fatalf("ApplyMany: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}
	expected := []string{"ALICE", "BOB", "CHARLIE"}
	for i, want := range expected {
		if results[i]["name"] != want {
			t.Errorf("results[%d] = %v, want %v", i, results[i]["name"], want)
		}
	}
}

func TestApplyManyEmpty(t *testing.T) {
	m := &Mapping{
		Name:   "test",
		Fields: []FieldMapping{{Target: "x", Constant: 1}},
	}
	results, err := m.ApplyMany(nil)
	if err != nil {
		t.Fatalf("ApplyMany: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("got %d results, want 0", len(results))
	}
}

func TestApplyManyErrorPropagation(t *testing.T) {
	m := &Mapping{
		Name: "test",
		Fields: []FieldMapping{
			{Target: "n", Sources: []string{"v"}, Transform: "to_int"},
		},
	}
	rows := []schema.Row{
		{"v": "123"},
		{"v": "not_a_number"},
	}
	_, err := m.ApplyMany(rows)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "row 1") {
		t.Errorf("error should mention row index: %v", err)
	}
}

// ── Transform: uppercase ──

func TestUppercase(t *testing.T) {
	tests := []struct {
		input any
		want  string
	}{
		{"hello", "HELLO"},
		{"ALREADY", "ALREADY"},
		{"MiXeD", "MIXED"},
		{"", ""},
		{nil, ""},
		{float64(42), "42"},
	}
	for _, tt := range tests {
		m := &Mapping{Name: "t", Fields: []FieldMapping{{Target: "o", Sources: []string{"i"}, Transform: "uppercase"}}}
		out, err := m.Apply(schema.Row{"i": tt.input})
		if err != nil {
			t.Errorf("uppercase(%v): %v", tt.input, err)
			continue
		}
		if out["o"] != tt.want {
			t.Errorf("uppercase(%v) = %v, want %v", tt.input, out["o"], tt.want)
		}
	}
}

// ── Transform: lowercase ──

func TestLowercase(t *testing.T) {
	tests := []struct {
		input any
		want  string
	}{
		{"HELLO", "hello"},
		{"already", "already"},
		{"MiXeD", "mixed"},
		{"", ""},
		{nil, ""},
	}
	for _, tt := range tests {
		m := &Mapping{Name: "t", Fields: []FieldMapping{{Target: "o", Sources: []string{"i"}, Transform: "lowercase"}}}
		out, err := m.Apply(schema.Row{"i": tt.input})
		if err != nil {
			t.Errorf("lowercase(%v): %v", tt.input, err)
			continue
		}
		if out["o"] != tt.want {
			t.Errorf("lowercase(%v) = %v, want %v", tt.input, out["o"], tt.want)
		}
	}
}

// ── Transform: trim ──

func TestTrim(t *testing.T) {
	tests := []struct {
		input any
		want  string
	}{
		{"  hello  ", "hello"},
		{"\t\n spaced \n\t", "spaced"},
		{"nospace", "nospace"},
		{"", ""},
		{nil, ""},
	}
	for _, tt := range tests {
		m := &Mapping{Name: "t", Fields: []FieldMapping{{Target: "o", Sources: []string{"i"}, Transform: "trim"}}}
		out, err := m.Apply(schema.Row{"i": tt.input})
		if err != nil {
			t.Errorf("trim(%v): %v", tt.input, err)
			continue
		}
		if out["o"] != tt.want {
			t.Errorf("trim(%v) = %v, want %v", tt.input, out["o"], tt.want)
		}
	}
}

// ── Transform: concat ──

func TestConcat(t *testing.T) {
	m := &Mapping{Name: "t", Fields: []FieldMapping{
		{Target: "full", Sources: []string{"first", "last"}, Transform: "concat", Args: []any{" "}},
	}}
	out, err := m.Apply(schema.Row{"first": "John", "last": "Doe"})
	if err != nil {
		t.Fatalf("concat: %v", err)
	}
	if out["full"] != "John Doe" {
		t.Errorf("full = %v", out["full"])
	}
}

func TestConcatNoSeparator(t *testing.T) {
	m := &Mapping{Name: "t", Fields: []FieldMapping{
		{Target: "out", Sources: []string{"a", "b"}, Transform: "concat"},
	}}
	out, err := m.Apply(schema.Row{"a": "foo", "b": "bar"})
	if err != nil {
		t.Fatalf("concat: %v", err)
	}
	if out["out"] != "foobar" {
		t.Errorf("out = %v", out["out"])
	}
}

func TestConcatWithNil(t *testing.T) {
	m := &Mapping{Name: "t", Fields: []FieldMapping{
		{Target: "out", Sources: []string{"a", "b"}, Transform: "concat", Args: []any{"-"}},
	}}
	out, err := m.Apply(schema.Row{"a": "hello", "b": nil})
	if err != nil {
		t.Fatalf("concat: %v", err)
	}
	if out["out"] != "hello-" {
		t.Errorf("out = %v", out["out"])
	}
}

func TestConcatMixedTypes(t *testing.T) {
	m := &Mapping{Name: "t", Fields: []FieldMapping{
		{Target: "out", Sources: []string{"a", "b", "c"}, Transform: "concat", Args: []any{"|"}},
	}}
	out, err := m.Apply(schema.Row{"a": "text", "b": float64(42), "c": true})
	if err != nil {
		t.Fatalf("concat: %v", err)
	}
	if out["out"] != "text|42|true" {
		t.Errorf("out = %v", out["out"])
	}
}

// ── Transform: round ──

func TestRound(t *testing.T) {
	tests := []struct {
		input    any
		decimals int
		want     float64
	}{
		{3.14159, 2, 3.14},
		{3.14159, 0, 3.0},
		{3.14159, 4, 3.1416},
		{2.5, 0, 3.0},
		{float64(100), 2, 100.0},
	}
	for _, tt := range tests {
		m := &Mapping{Name: "t", Fields: []FieldMapping{
			{Target: "o", Sources: []string{"v"}, Transform: "round", Args: []any{float64(tt.decimals)}},
		}}
		out, err := m.Apply(schema.Row{"v": tt.input})
		if err != nil {
			t.Errorf("round(%v, %d): %v", tt.input, tt.decimals, err)
			continue
		}
		if out["o"] != tt.want {
			t.Errorf("round(%v, %d) = %v, want %v", tt.input, tt.decimals, out["o"], tt.want)
		}
	}
}

func TestRoundNil(t *testing.T) {
	m := &Mapping{Name: "t", Fields: []FieldMapping{
		{Target: "o", Sources: []string{"v"}, Transform: "round"},
	}}
	out, err := m.Apply(schema.Row{"v": nil})
	if err != nil {
		t.Fatalf("round(nil): %v", err)
	}
	if out["o"] != nil {
		t.Errorf("round(nil) = %v, want nil", out["o"])
	}
}

func TestRoundString(t *testing.T) {
	m := &Mapping{Name: "t", Fields: []FieldMapping{
		{Target: "o", Sources: []string{"v"}, Transform: "round"},
	}}
	out, err := m.Apply(schema.Row{"v": "not_a_number"})
	if err != nil {
		t.Fatalf("round(string): %v", err)
	}
	if out["o"] != "not_a_number" {
		t.Errorf("round(string) = %v", out["o"])
	}
}

// ── Transform: default ──

func TestDefault(t *testing.T) {
	m := &Mapping{Name: "t", Fields: []FieldMapping{
		{Target: "s", Sources: []string{"s"}, Transform: "default", Args: []any{"fallback"}},
	}}

	out, _ := m.Apply(schema.Row{"s": nil})
	if out["s"] != "fallback" {
		t.Errorf("nil → %v, want fallback", out["s"])
	}

	out, _ = m.Apply(schema.Row{"s": ""})
	if out["s"] != "fallback" {
		t.Errorf("empty → %v, want fallback", out["s"])
	}

	out, _ = m.Apply(schema.Row{"s": "present"})
	if out["s"] != "present" {
		t.Errorf("present → %v, want present", out["s"])
	}

	out, _ = m.Apply(schema.Row{"s": float64(0)})
	if out["s"] != float64(0) {
		t.Errorf("0 → %v, want 0 (numeric zero is non-empty)", out["s"])
	}
}

func TestDefaultNoArgs(t *testing.T) {
	m := &Mapping{Name: "t", Fields: []FieldMapping{
		{Target: "s", Sources: []string{"s"}, Transform: "default"},
	}}
	out, _ := m.Apply(schema.Row{"s": nil})
	if out["s"] != nil {
		t.Errorf("default with no args on nil = %v, want nil", out["s"])
	}
}

// ── Transform: to_int ──

func TestToInt(t *testing.T) {
	tests := []struct {
		input   any
		want    int64
		wantErr bool
	}{
		{float64(42), 42, false},
		{"123", 123, false},
		{float64(3.9), 3, false},
		{int64(7), 7, false},
		{"not_a_number", 0, true},
	}
	for _, tt := range tests {
		m := &Mapping{Name: "t", Fields: []FieldMapping{
			{Target: "o", Sources: []string{"v"}, Transform: "to_int"},
		}}
		out, err := m.Apply(schema.Row{"v": tt.input})
		if tt.wantErr {
			if err == nil {
				t.Errorf("to_int(%v): expected error", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("to_int(%v): %v", tt.input, err)
			continue
		}
		if out["o"] != tt.want {
			t.Errorf("to_int(%v) = %v, want %v", tt.input, out["o"], tt.want)
		}
	}
}

func TestToIntNil(t *testing.T) {
	m := &Mapping{Name: "t", Fields: []FieldMapping{
		{Target: "o", Sources: []string{"v"}, Transform: "to_int"},
	}}
	out, err := m.Apply(schema.Row{"v": nil})
	if err != nil {
		t.Fatalf("to_int(nil): %v", err)
	}
	if out["o"] != nil {
		t.Errorf("to_int(nil) = %v", out["o"])
	}
}

// ── Transform: to_text ──

func TestToText(t *testing.T) {
	tests := []struct {
		input any
		want  string
	}{
		{float64(42), "42"},
		{float64(3.14), "3.14"},
		{"hello", "hello"},
		{true, "true"},
		{nil, ""},
		{int64(99), "99"},
	}
	for _, tt := range tests {
		m := &Mapping{Name: "t", Fields: []FieldMapping{
			{Target: "o", Sources: []string{"v"}, Transform: "to_text"},
		}}
		out, err := m.Apply(schema.Row{"v": tt.input})
		if err != nil {
			t.Errorf("to_text(%v): %v", tt.input, err)
			continue
		}
		if out["o"] != tt.want {
			t.Errorf("to_text(%v) = %v, want %v", tt.input, out["o"], tt.want)
		}
	}
}

// ── Transform: to_float ──

func TestToFloat(t *testing.T) {
	tests := []struct {
		input   any
		want    float64
		wantErr bool
	}{
		{float64(3.14), 3.14, false},
		{"2.5", 2.5, false},
		{int64(10), 10.0, false},
		{"bad", 0, true},
	}
	for _, tt := range tests {
		m := &Mapping{Name: "t", Fields: []FieldMapping{
			{Target: "o", Sources: []string{"v"}, Transform: "to_float"},
		}}
		out, err := m.Apply(schema.Row{"v": tt.input})
		if tt.wantErr {
			if err == nil {
				t.Errorf("to_float(%v): expected error", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("to_float(%v): %v", tt.input, err)
			continue
		}
		if out["o"] != tt.want {
			t.Errorf("to_float(%v) = %v, want %v", tt.input, out["o"], tt.want)
		}
	}
}

func TestToFloatNil(t *testing.T) {
	m := &Mapping{Name: "t", Fields: []FieldMapping{
		{Target: "o", Sources: []string{"v"}, Transform: "to_float"},
	}}
	out, err := m.Apply(schema.Row{"v": nil})
	if err != nil {
		t.Fatalf("to_float(nil): %v", err)
	}
	if out["o"] != nil {
		t.Errorf("to_float(nil) = %v", out["o"])
	}
}

// ── Transform: substring ──

func TestSubstring(t *testing.T) {
	tests := []struct {
		input  string
		start  int
		length int
		want   string
	}{
		{"hello world", 0, 5, "hello"},
		{"hello world", 6, 5, "world"},
		{"hello", 0, 100, "hello"},
		{"hello", 10, 5, ""},
		{"hello", 2, 2, "ll"},
	}
	for _, tt := range tests {
		m := &Mapping{Name: "t", Fields: []FieldMapping{
			{Target: "o", Sources: []string{"v"}, Transform: "substring", Args: []any{float64(tt.start), float64(tt.length)}},
		}}
		out, err := m.Apply(schema.Row{"v": tt.input})
		if err != nil {
			t.Errorf("substring(%q, %d, %d): %v", tt.input, tt.start, tt.length, err)
			continue
		}
		if out["o"] != tt.want {
			t.Errorf("substring(%q, %d, %d) = %v, want %v", tt.input, tt.start, tt.length, out["o"], tt.want)
		}
	}
}

func TestSubstringNegativeStart(t *testing.T) {
	m := &Mapping{Name: "t", Fields: []FieldMapping{
		{Target: "o", Sources: []string{"v"}, Transform: "substring", Args: []any{float64(-5), float64(3)}},
	}}
	out, err := m.Apply(schema.Row{"v": "hello"})
	if err != nil {
		t.Fatalf("substring negative start: %v", err)
	}
	if out["o"] != "hel" {
		t.Errorf("substring = %v, want 'hel'", out["o"])
	}
}

// ── Transform: prefix / suffix ──

func TestPrefix(t *testing.T) {
	m := &Mapping{Name: "t", Fields: []FieldMapping{
		{Target: "o", Sources: []string{"id"}, Transform: "prefix", Args: []any{"INV-"}},
	}}
	out, _ := m.Apply(schema.Row{"id": "100"})
	if out["o"] != "INV-100" {
		t.Errorf("prefix = %v", out["o"])
	}
}

func TestPrefixNilSource(t *testing.T) {
	m := &Mapping{Name: "t", Fields: []FieldMapping{
		{Target: "o", Sources: []string{"id"}, Transform: "prefix", Args: []any{"X-"}},
	}}
	out, _ := m.Apply(schema.Row{"id": nil})
	if out["o"] != "X-" {
		t.Errorf("prefix(nil) = %v", out["o"])
	}
}

func TestSuffix(t *testing.T) {
	m := &Mapping{Name: "t", Fields: []FieldMapping{
		{Target: "o", Sources: []string{"name"}, Transform: "suffix", Args: []any{" Jr."}},
	}}
	out, _ := m.Apply(schema.Row{"name": "Bob"})
	if out["o"] != "Bob Jr." {
		t.Errorf("suffix = %v", out["o"])
	}
}

// ── Transform: now ──

func TestNow(t *testing.T) {
	m := &Mapping{Name: "t", Fields: []FieldMapping{
		{Target: "ts", Transform: "now"},
	}}
	out, err := m.Apply(schema.Row{})
	if err != nil {
		t.Fatalf("now: %v", err)
	}
	s, ok := out["ts"].(string)
	if !ok {
		t.Fatalf("now result is %T, not string", out["ts"])
	}
	if len(s) < 20 {
		t.Errorf("now result too short: %v", s)
	}
}

// ── Transform: coalesce ──

func TestCoalesce(t *testing.T) {
	m := &Mapping{Name: "t", Fields: []FieldMapping{
		{Target: "val", Sources: []string{"a", "b", "c"}, Transform: "coalesce"},
	}}

	out, _ := m.Apply(schema.Row{"a": nil, "b": "", "c": "found"})
	if out["val"] != "found" {
		t.Errorf("coalesce = %v, want 'found'", out["val"])
	}

	out, _ = m.Apply(schema.Row{"a": "first", "b": "second"})
	if out["val"] != "first" {
		t.Errorf("coalesce = %v, want 'first'", out["val"])
	}

	out, _ = m.Apply(schema.Row{"a": nil, "b": nil, "c": nil})
	if out["val"] != nil {
		t.Errorf("coalesce all nil = %v, want nil", out["val"])
	}
}

// ── Transform: replace ──

func TestReplace(t *testing.T) {
	m := &Mapping{Name: "t", Fields: []FieldMapping{
		{Target: "o", Sources: []string{"raw"}, Transform: "replace", Args: []any{"-", "_"}},
	}}
	out, _ := m.Apply(schema.Row{"raw": "foo-bar-baz"})
	if out["o"] != "foo_bar_baz" {
		t.Errorf("replace = %v", out["o"])
	}
}

func TestReplaceNoMatch(t *testing.T) {
	m := &Mapping{Name: "t", Fields: []FieldMapping{
		{Target: "o", Sources: []string{"raw"}, Transform: "replace", Args: []any{"xyz", "!"}},
	}}
	out, _ := m.Apply(schema.Row{"raw": "hello"})
	if out["o"] != "hello" {
		t.Errorf("replace no match = %v", out["o"])
	}
}

// ── Transform: split ──

func TestSplitIndex(t *testing.T) {
	m := &Mapping{Name: "t", Fields: []FieldMapping{
		{Target: "first", Sources: []string{"csv"}, Transform: "split", Args: []any{",", float64(0)}},
		{Target: "second", Sources: []string{"csv"}, Transform: "split", Args: []any{",", float64(1)}},
		{Target: "third", Sources: []string{"csv"}, Transform: "split", Args: []any{",", float64(2)}},
	}}
	out, err := m.Apply(schema.Row{"csv": "alpha, beta, gamma"})
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if out["first"] != "alpha" {
		t.Errorf("first = %v", out["first"])
	}
	if out["second"] != "beta" {
		t.Errorf("second = %v", out["second"])
	}
	if out["third"] != "gamma" {
		t.Errorf("third = %v", out["third"])
	}
}

func TestSplitNoIndex(t *testing.T) {
	m := &Mapping{Name: "t", Fields: []FieldMapping{
		{Target: "parts", Sources: []string{"csv"}, Transform: "split", Args: []any{","}},
	}}
	out, err := m.Apply(schema.Row{"csv": "a,b,c"})
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	arr, ok := out["parts"].([]any)
	if !ok {
		t.Fatalf("parts is %T", out["parts"])
	}
	if len(arr) != 3 {
		t.Errorf("parts len = %d", len(arr))
	}
}

func TestSplitOutOfBounds(t *testing.T) {
	m := &Mapping{Name: "t", Fields: []FieldMapping{
		{Target: "o", Sources: []string{"csv"}, Transform: "split", Args: []any{",", float64(99)}},
	}}
	out, err := m.Apply(schema.Row{"csv": "a,b"})
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	arr, ok := out["o"].([]any)
	if !ok {
		t.Fatalf("expected []any for out-of-bounds index, got %T", out["o"])
	}
	if len(arr) != 2 {
		t.Errorf("len = %d", len(arr))
	}
}

// ── Transform: join ──

func TestJoin(t *testing.T) {
	m := &Mapping{Name: "t", Fields: []FieldMapping{
		{Target: "o", Sources: []string{"a", "b", "c"}, Transform: "join", Args: []any{" - "}},
	}}
	out, _ := m.Apply(schema.Row{"a": "x", "b": "y", "c": "z"})
	if out["o"] != "x - y - z" {
		t.Errorf("join = %v", out["o"])
	}
}

func TestJoinDefaultSep(t *testing.T) {
	m := &Mapping{Name: "t", Fields: []FieldMapping{
		{Target: "o", Sources: []string{"a", "b"}, Transform: "join"},
	}}
	out, _ := m.Apply(schema.Row{"a": "x", "b": "y"})
	if out["o"] != "x,y" {
		t.Errorf("join default = %v", out["o"])
	}
}

// ── Transform: length ──

func TestLength(t *testing.T) {
	tests := []struct {
		input any
		want  int64
	}{
		{"hello", 5},
		{"", 0},
		{nil, 0},
		{float64(123), 3},
	}
	for _, tt := range tests {
		m := &Mapping{Name: "t", Fields: []FieldMapping{
			{Target: "o", Sources: []string{"v"}, Transform: "length"},
		}}
		out, _ := m.Apply(schema.Row{"v": tt.input})
		if out["o"] != tt.want {
			t.Errorf("length(%v) = %v, want %v", tt.input, out["o"], tt.want)
		}
	}
}

// ── toString helper ──

func TestToStringHelper(t *testing.T) {
	tests := []struct {
		input any
		want  string
	}{
		{nil, ""},
		{"hello", "hello"},
		{float64(42), "42"},
		{float64(3.14), "3.14"},
		{int64(7), "7"},
		{int(99), "99"},
		{true, "true"},
		{false, "false"},
	}
	for _, tt := range tests {
		got := toString(tt.input)
		if got != tt.want {
			t.Errorf("toString(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ── File I/O ──

func TestSaveLoadDelete(t *testing.T) {
	dir := t.TempDir()

	m := &Mapping{
		Name:        "order-to-invoice",
		Description: "Test mapping",
		Fields: []FieldMapping{
			{Target: "id", Sources: []string{"order_id"}, Transform: "prefix", Args: []any{"INV-"}},
			{Target: "currency", Constant: "EUR"},
		},
	}

	if err := Save(dir, m); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "order-to-invoice.json")); err != nil {
		t.Fatalf("file not created: %v", err)
	}

	loaded, err := Load(dir, "order-to-invoice")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Name != m.Name {
		t.Errorf("name = %v, want %v", loaded.Name, m.Name)
	}
	if loaded.Description != m.Description {
		t.Errorf("description = %v, want %v", loaded.Description, m.Description)
	}
	if len(loaded.Fields) != 2 {
		t.Errorf("fields = %d, want 2", len(loaded.Fields))
	}

	all, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("LoadDir got %d, want 1", len(all))
	}

	if err := Delete(dir, "order-to-invoice"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "order-to-invoice.json")); !os.IsNotExist(err) {
		t.Error("file should be deleted")
	}
}

func TestLoadDirNonexistent(t *testing.T) {
	mappings, err := LoadDir("/nonexistent/path/that/does/not/exist")
	if err != nil {
		t.Fatalf("LoadDir nonexistent: %v", err)
	}
	if mappings != nil {
		t.Errorf("expected nil, got %d mappings", len(mappings))
	}
}

func TestLoadDirSkipsInvalid(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "valid.json"), []byte(`{"name":"valid","fields":[{"target":"x","constant":"y"}]}`), 0644)
	os.WriteFile(filepath.Join(dir, "invalid.json"), []byte(`not json`), 0644)
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte(`not a mapping`), 0644)

	mappings, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if len(mappings) != 1 {
		t.Errorf("got %d mappings, want 1 (valid only)", len(mappings))
	}
}

func TestLoadNonexistent(t *testing.T) {
	_, err := Load(t.TempDir(), "does-not-exist")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSaveCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sub", "dir")
	m := &Mapping{Name: "test", Fields: []FieldMapping{{Target: "x", Constant: 1}}}
	if err := Save(dir, m); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "test.json")); err != nil {
		t.Fatalf("file not created: %v", err)
	}
}

func TestSaveValidationError(t *testing.T) {
	dir := t.TempDir()
	m := &Mapping{Name: "", Fields: nil}
	if err := Save(dir, m); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestDeleteNonexistent(t *testing.T) {
	err := Delete(t.TempDir(), "nope")
	if err == nil {
		t.Fatal("expected error")
	}
}

// ── Multiple saves (overwrite) ──

func TestSaveOverwrite(t *testing.T) {
	dir := t.TempDir()

	m1 := &Mapping{Name: "test", Fields: []FieldMapping{{Target: "v1", Constant: 1}}}
	Save(dir, m1)

	m2 := &Mapping{Name: "test", Fields: []FieldMapping{{Target: "v2", Constant: 2}}}
	Save(dir, m2)

	loaded, err := Load(dir, "test")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Fields[0].Target != "v2" {
		t.Errorf("not overwritten: target = %v", loaded.Fields[0].Target)
	}
}

// ── Validation ──

func TestValidation(t *testing.T) {
	tests := []struct {
		name    string
		mapping Mapping
		wantErr bool
	}{
		{"empty name", Mapping{Name: "", Fields: []FieldMapping{{Target: "x", Constant: 1}}}, true},
		{"whitespace name", Mapping{Name: "  ", Fields: []FieldMapping{{Target: "x", Constant: 1}}}, true},
		{"no fields", Mapping{Name: "test", Fields: nil}, true},
		{"empty fields", Mapping{Name: "test", Fields: []FieldMapping{}}, true},
		{"no target", Mapping{Name: "test", Fields: []FieldMapping{{Sources: []string{"a"}}}}, true},
		{"whitespace target", Mapping{Name: "test", Fields: []FieldMapping{{Target: "  ", Sources: []string{"a"}}}}, true},
		{"duplicate target", Mapping{Name: "test", Fields: []FieldMapping{
			{Target: "x", Constant: 1},
			{Target: "x", Constant: 2},
		}}, true},
		{"no source or constant", Mapping{Name: "test", Fields: []FieldMapping{{Target: "x"}}}, true},
		{"unknown transform", Mapping{Name: "test", Fields: []FieldMapping{
			{Target: "x", Sources: []string{"a"}, Transform: "nonexistent"},
		}}, true},
		{"valid with sources", Mapping{Name: "test", Fields: []FieldMapping{
			{Target: "x", Sources: []string{"a"}},
		}}, false},
		{"valid with constant", Mapping{Name: "test", Fields: []FieldMapping{
			{Target: "x", Constant: "hello"},
		}}, false},
		{"valid with transform", Mapping{Name: "test", Fields: []FieldMapping{
			{Target: "x", Sources: []string{"a"}, Transform: "uppercase"},
		}}, false},
		{"valid now transform no sources", Mapping{Name: "test", Fields: []FieldMapping{
			{Target: "ts", Transform: "now"},
		}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.mapping.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// ── TransformNames ──

func TestTransformNames(t *testing.T) {
	names := TransformNames()
	if len(names) == 0 {
		t.Fatal("no transform names")
	}
	expected := []string{"uppercase", "lowercase", "trim", "concat", "round", "default",
		"to_int", "to_text", "to_float", "substring", "prefix", "suffix", "now",
		"coalesce", "replace", "split", "join", "length"}
	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}
	for _, e := range expected {
		if !nameSet[e] {
			t.Errorf("missing transform: %v", e)
		}
	}
}

// ── Complex mapping (real-world scenario) ──

func TestComplexMapping(t *testing.T) {
	m := &Mapping{
		Name:        "order-to-invoice",
		Description: "Convert paid orders to invoice records",
		Fields: []FieldMapping{
			{Target: "invoice_number", Sources: []string{"order_id"}, Transform: "prefix", Args: []any{"INV-"}},
			{Target: "customer", Sources: []string{"first_name", "last_name"}, Transform: "concat", Args: []any{" "}},
			{Target: "email", Sources: []string{"email"}, Transform: "lowercase"},
			{Target: "total", Sources: []string{"amount"}, Transform: "round", Args: []any{float64(2)}},
			{Target: "currency", Constant: "EUR"},
			{Target: "status", Constant: "pending"},
			{Target: "created_at", Transform: "now"},
			{Target: "notes", Sources: []string{"notes"}, Transform: "default", Args: []any{"No notes"}},
		},
	}

	if err := m.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	row := schema.Row{
		"order_id":   "42",
		"first_name": "Jan",
		"last_name":  "de Vries",
		"email":      "JAN@EXAMPLE.COM",
		"amount":     199.956,
		"notes":      nil,
	}

	out, err := m.Apply(row)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if out["invoice_number"] != "INV-42" {
		t.Errorf("invoice_number = %v", out["invoice_number"])
	}
	if out["customer"] != "Jan de Vries" {
		t.Errorf("customer = %v", out["customer"])
	}
	if out["email"] != "jan@example.com" {
		t.Errorf("email = %v", out["email"])
	}
	if out["total"] != 199.96 {
		t.Errorf("total = %v", out["total"])
	}
	if out["currency"] != "EUR" {
		t.Errorf("currency = %v", out["currency"])
	}
	if out["status"] != "pending" {
		t.Errorf("status = %v", out["status"])
	}
	if out["notes"] != "No notes" {
		t.Errorf("notes = %v", out["notes"])
	}
	if out["created_at"] == nil || out["created_at"] == "" {
		t.Error("created_at should be set")
	}
}

// ── Round-trip: Save → Load → Apply ──

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()

	m := &Mapping{
		Name: "sensor-normalize",
		Fields: []FieldMapping{
			{Target: "device", Sources: []string{"device_id"}, Transform: "uppercase"},
			{Target: "temp_c", Sources: []string{"temp_raw"}, Transform: "round", Args: []any{float64(1)}},
			{Target: "unit", Constant: "celsius"},
		},
	}

	if err := Save(dir, m); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(dir, "sensor-normalize")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	out, err := loaded.Apply(schema.Row{"device_id": "sensor-a", "temp_raw": 23.456})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if out["device"] != "SENSOR-A" {
		t.Errorf("device = %v", out["device"])
	}
	if out["temp_c"] != 23.5 {
		t.Errorf("temp_c = %v", out["temp_c"])
	}
	if out["unit"] != "celsius" {
		t.Errorf("unit = %v", out["unit"])
	}
}
