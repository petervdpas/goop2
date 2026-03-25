package mapper

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/petervdpas/goop2/internal/orm/schema"
)

type SourceReader interface {
	Read() ([]schema.Row, error)
}

type TargetWriter interface {
	Write(rows []schema.Row) (int, error)
}

func NewSourceReader(ep DataEndpoint) (SourceReader, error) {
	switch ep.Type {
	case "csv":
		if ep.Path == "" {
			return nil, fmt.Errorf("csv source requires path")
		}
		return &csvReader{path: ep.Path}, nil
	case "json":
		if ep.Path != "" {
			return &jsonFileReader{path: ep.Path}, nil
		}
		if ep.URL != "" {
			return &jsonAPIReader{url: ep.URL}, nil
		}
		return nil, fmt.Errorf("json source requires path or url")
	case "table":
		return nil, fmt.Errorf("table source requires database — use ExecuteWithDB")
	default:
		return nil, fmt.Errorf("unknown source type %q", ep.Type)
	}
}

func NewTargetWriter(ep DataEndpoint) (TargetWriter, error) {
	switch ep.Type {
	case "csv":
		if ep.Path == "" {
			return nil, fmt.Errorf("csv target requires path")
		}
		return &csvWriter{path: ep.Path}, nil
	case "json":
		if ep.Path == "" {
			return nil, fmt.Errorf("json target requires path")
		}
		return &jsonFileWriter{path: ep.Path}, nil
	case "table":
		return nil, fmt.Errorf("table target requires database — use ExecuteWithDB")
	default:
		return nil, fmt.Errorf("unknown target type %q", ep.Type)
	}
}

type csvReader struct {
	path string
}

func (r *csvReader) Read() ([]schema.Row, error) {
	f, err := os.Open(r.path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	cr := csv.NewReader(f)
	headers, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("csv: read header: %w", err)
	}
	for i := range headers {
		headers[i] = strings.TrimSpace(headers[i])
	}

	var rows []schema.Row
	for {
		record, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("csv: read row: %w", err)
		}
		row := make(schema.Row, len(headers))
		for i, h := range headers {
			if i < len(record) {
				row[h] = record[i]
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

type jsonFileReader struct {
	path string
}

func (r *jsonFileReader) Read() ([]schema.Row, error) {
	data, err := os.ReadFile(r.path)
	if err != nil {
		return nil, err
	}
	var rows []schema.Row
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, fmt.Errorf("json: parse: %w", err)
	}
	return rows, nil
}

type jsonAPIReader struct {
	url string
}

func (r *jsonAPIReader) Read() ([]schema.Row, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(r.url)
	if err != nil {
		return nil, fmt.Errorf("api: fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api: status %d", resp.StatusCode)
	}
	var rows []schema.Row
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return nil, fmt.Errorf("api: parse: %w", err)
	}
	return rows, nil
}

type csvWriter struct {
	path string
}

func (w *csvWriter) Write(rows []schema.Row) (int, error) {
	if len(rows) == 0 {
		return 0, nil
	}

	var headers []string
	for k := range rows[0] {
		headers = append(headers, k)
	}

	f, err := os.Create(w.path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	cw := csv.NewWriter(f)
	if err := cw.Write(headers); err != nil {
		return 0, err
	}
	for _, row := range rows {
		record := make([]string, len(headers))
		for i, h := range headers {
			if v, ok := row[h]; ok && v != nil {
				record[i] = fmt.Sprintf("%v", v)
			}
		}
		if err := cw.Write(record); err != nil {
			return 0, err
		}
	}
	cw.Flush()
	return len(rows), cw.Error()
}

type jsonFileWriter struct {
	path string
}

func (w *jsonFileWriter) Write(rows []schema.Row) (int, error) {
	data, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return 0, err
	}
	if err := os.WriteFile(w.path, append(data, '\n'), 0644); err != nil {
		return 0, err
	}
	return len(rows), nil
}
