// Goop2 Cluster Executor — Go example
//
// Usage: go run main.go [-url http://localhost:8787]
//
// Polls for jobs, claims them, processes, reports result.
// Replace processJob() with your actual workload.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

var client = &http.Client{Timeout: 10 * time.Second}

// ── API types (matches docs/executor-api.yaml) ──────────────────────────────

type Job struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Payload  map[string]any `json:"payload,omitempty"`
	Priority int            `json:"priority"`
	TimeoutS int            `json:"timeout_s"`
	MaxRetry int            `json:"max_retry"`
}

type PendingJob struct {
	Job        Job       `json:"job"`
	ReceivedAt time.Time `json:"received_at"`
}

type JobListResponse struct {
	Pending  []PendingJob `json:"pending"`
	Accepted []PendingJob `json:"accepted"`
}

// ── API calls ───────────────────────────────────────────────────────────────

func getJobs(base string) (*JobListResponse, error) {
	resp, err := client.Get(base + "/api/cluster/job")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result JobListResponse
	return &result, json.NewDecoder(resp.Body).Decode(&result)
}

func acceptJob(base, jobID string) error {
	return postJSON(base+"/api/cluster/accept", map[string]any{"job_id": jobID})
}

func reportProgress(base, jobID string, percent int, message string, stats map[string]any) error {
	return postJSON(base+"/api/cluster/progress", map[string]any{
		"job_id":  jobID,
		"percent": percent,
		"message": message,
		"stats":   stats,
	})
}

func reportResult(base, jobID string, success bool, result map[string]any, errMsg string) error {
	return postJSON(base+"/api/cluster/result", map[string]any{
		"job_id":  jobID,
		"success": success,
		"result":  result,
		"error":   errMsg,
	})
}

func heartbeat(base string, stats map[string]any) error {
	return postJSON(base+"/api/cluster/heartbeat", map[string]any{"stats": stats})
}

func postJSON(url string, body any) error {
	b, _ := json.Marshal(body)
	resp, err := client.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

// ── Job processing (replace with your logic) ────────────────────────────────

func processJob(base string, job Job) (map[string]any, error) {
	totalSteps := 10
	if v, ok := job.Payload["steps"]; ok {
		if f, ok := v.(float64); ok {
			totalSteps = int(f)
		}
	}

	for step := 1; step <= totalSteps; step++ {
		time.Sleep(500 * time.Millisecond) // simulate work

		pct := step * 100 / totalSteps
		_ = reportProgress(base, job.ID, pct, fmt.Sprintf("step %d/%d", step, totalSteps), map[string]any{
			"step":      step,
			"memory_mb": os.Getpid() % 1000,
		})
		log.Printf("  progress: %d%%", pct)
	}

	return map[string]any{
		"type":            job.Type,
		"steps_completed": totalSteps,
	}, nil
}

// ── Main loop ───────────────────────────────────────────────────────────────

func main() {
	base := flag.String("url", "http://localhost:8787", "goop2 base URL")
	poll := flag.Duration("poll", 2*time.Second, "poll interval")
	flag.Parse()

	// Background heartbeat
	go func() {
		for {
			_ = heartbeat(*base, map[string]any{
				"executor": "go",
				"pid":      os.Getpid(),
			})
			time.Sleep(10 * time.Second)
		}
	}()

	log.Printf("polling %s for jobs...", *base)

	for {
		jobs, err := getJobs(*base)
		if err != nil {
			log.Printf("poll error: %v", err)
			time.Sleep(*poll)
			continue
		}

		if len(jobs.Pending) == 0 {
			time.Sleep(*poll)
			continue
		}

		job := jobs.Pending[0].Job
		log.Printf("found job %s (type=%s)", job.ID, job.Type)

		if err := acceptJob(*base, job.ID); err != nil {
			log.Printf("accept failed: %v", err)
			time.Sleep(*poll)
			continue
		}
		log.Printf("accepted %s", job.ID)

		result, err := processJob(*base, job)
		if err != nil {
			_ = reportResult(*base, job.ID, false, nil, err.Error())
			log.Printf("failed %s: %v", job.ID, err)
			continue
		}

		_ = reportResult(*base, job.ID, true, result, "")
		log.Printf("completed %s", job.ID)
	}
}
