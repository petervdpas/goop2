package storage

import (
	"testing"
	"time"
)

func TestSaveAndLoadClusterJobs(t *testing.T) {
	db := testDB(t)

	err := db.SaveClusterJob("g1", "job1", "compute", "oneshot", `{"input":"data"}`,
		5, 60, 3, "pending", "", "{}", "", 0, "", 0,
		"2026-01-01T00:00:00Z", "", "", 0)
	if err != nil {
		t.Fatal(err)
	}

	jobs, err := db.LoadClusterJobs("g1")
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}

	j := jobs[0]
	if j.ID != "job1" {
		t.Fatalf("id = %q", j.ID)
	}
	if j.Type != "compute" {
		t.Fatalf("type = %q", j.Type)
	}
	if j.Mode != "oneshot" {
		t.Fatalf("mode = %q", j.Mode)
	}
	if j.Payload != `{"input":"data"}` {
		t.Fatalf("payload = %q", j.Payload)
	}
	if j.Priority != 5 {
		t.Fatalf("priority = %d", j.Priority)
	}
	if j.TimeoutS != 60 {
		t.Fatalf("timeout_s = %d", j.TimeoutS)
	}
	if j.MaxRetry != 3 {
		t.Fatalf("max_retry = %d", j.MaxRetry)
	}
	if j.Status != "pending" {
		t.Fatalf("status = %q", j.Status)
	}
}

func TestSaveClusterJobUpsert(t *testing.T) {
	db := testDB(t)

	db.SaveClusterJob("g1", "job1", "compute", "oneshot", "{}", 0, 0, 0,
		"pending", "", "{}", "", 0, "", 0, "", "", "", 0)

	db.SaveClusterJob("g1", "job1", "compute", "oneshot", "{}", 0, 0, 0,
		"running", "worker1", "{}", "", 50, "half done", 0, "", "2026-01-01T00:01:00Z", "", 0)

	jobs, _ := db.LoadClusterJobs("g1")
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job after upsert, got %d", len(jobs))
	}
	if jobs[0].Status != "running" {
		t.Fatalf("status = %q, want 'running'", jobs[0].Status)
	}
	if jobs[0].WorkerID != "worker1" {
		t.Fatalf("worker_id = %q", jobs[0].WorkerID)
	}
	if jobs[0].Progress != 50 {
		t.Fatalf("progress = %d", jobs[0].Progress)
	}
}

func TestLoadClusterJobsEmpty(t *testing.T) {
	db := testDB(t)

	jobs, err := db.LoadClusterJobs("nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 0 {
		t.Fatalf("expected 0 jobs, got %d", len(jobs))
	}
}

func TestLoadClusterJobsIsolation(t *testing.T) {
	db := testDB(t)

	db.SaveClusterJob("g1", "j1", "compute", "oneshot", "{}", 0, 0, 0,
		"pending", "", "{}", "", 0, "", 0, "", "", "", 0)
	db.SaveClusterJob("g2", "j2", "compute", "oneshot", "{}", 0, 0, 0,
		"pending", "", "{}", "", 0, "", 0, "", "", "", 0)

	jobs, _ := db.LoadClusterJobs("g1")
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job for g1, got %d", len(jobs))
	}
	if jobs[0].ID != "j1" {
		t.Fatalf("id = %q, want 'j1'", jobs[0].ID)
	}
}

func TestDeleteClusterJob(t *testing.T) {
	db := testDB(t)

	db.SaveClusterJob("g1", "j1", "compute", "oneshot", "{}", 0, 0, 0,
		"done", "", "{}", "", 100, "", 0, "", "", "", 0)

	if err := db.DeleteClusterJob("g1", "j1"); err != nil {
		t.Fatal(err)
	}

	jobs, _ := db.LoadClusterJobs("g1")
	if len(jobs) != 0 {
		t.Fatalf("expected 0 jobs after delete, got %d", len(jobs))
	}
}

func TestDeleteClusterJobs(t *testing.T) {
	db := testDB(t)

	db.SaveClusterJob("g1", "j1", "compute", "oneshot", "{}", 0, 0, 0,
		"done", "", "{}", "", 0, "", 0, "", "", "", 0)
	db.SaveClusterJob("g1", "j2", "compute", "oneshot", "{}", 0, 0, 0,
		"done", "", "{}", "", 0, "", 0, "", "", "", 0)

	if err := db.DeleteClusterJobs("g1"); err != nil {
		t.Fatal(err)
	}

	jobs, _ := db.LoadClusterJobs("g1")
	if len(jobs) != 0 {
		t.Fatalf("expected 0 jobs after bulk delete, got %d", len(jobs))
	}
}

func TestMarshalJSON(t *testing.T) {
	if got := MarshalJSON(nil); got != "{}" {
		t.Fatalf("MarshalJSON(nil) = %q, want '{}'", got)
	}

	m := map[string]any{"key": "value"}
	got := MarshalJSON(m)
	if got != `{"key":"value"}` {
		t.Fatalf("got %q", got)
	}
}

func TestUnmarshalJSON(t *testing.T) {
	if got := UnmarshalJSON(""); got != nil {
		t.Fatalf("UnmarshalJSON(\"\") = %v, want nil", got)
	}
	if got := UnmarshalJSON("{}"); got != nil {
		t.Fatalf("UnmarshalJSON(\"{}\") = %v, want nil", got)
	}

	got := UnmarshalJSON(`{"key":"value"}`)
	if got["key"] != "value" {
		t.Fatalf("got %v", got)
	}
}

func TestFormatTime(t *testing.T) {
	if got := FormatTime(time.Time{}); got != "" {
		t.Fatalf("FormatTime(zero) = %q, want ''", got)
	}

	ts := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
	got := FormatTime(ts)
	if got != "2026-01-15T10:30:00Z" {
		t.Fatalf("FormatTime = %q", got)
	}
}

func TestParseTime(t *testing.T) {
	if got := ParseTime(""); !got.IsZero() {
		t.Fatalf("ParseTime(\"\") should be zero, got %v", got)
	}

	got := ParseTime("2026-01-15T10:30:00Z")
	if got.Year() != 2026 || got.Month() != 1 || got.Day() != 15 {
		t.Fatalf("ParseTime = %v", got)
	}
}
