package cluster

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

type ExecResult struct {
	Status string         `json:"status"`
	Result map[string]any `json:"result,omitempty"`
	Error  string         `json:"error,omitempty"`
	Pct    int            `json:"pct,omitempty"`
	Msg    string         `json:"msg,omitempty"`
}

// BinaryRunner manages a single binary process (oneshot or daemon mode)
type BinaryRunner struct {
	path string
	mode string // "oneshot" or "daemon"

	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Scanner
}

// NewBinaryRunner creates a runner for a binary (doesn't start it yet)
func NewBinaryRunner(path, mode string) *BinaryRunner {
	if mode == "" {
		mode = "oneshot"
	}
	return &BinaryRunner{
		path: path,
		mode: mode,
	}
}

// RunJob executes a single job (oneshot or daemon) with optional continuous mode
func (br *BinaryRunner) RunJob(ctx context.Context, job Job, progressFn func(pct int, msg string)) (map[string]any, error) {
	// Binary mode (oneshot vs daemon) determines process lifetime
	if br.mode == "daemon" {
		return br.runJobDaemon(ctx, job, progressFn)
	}

	// Job mode (oneshot vs continuous) determines result handling
	if job.Mode == JobContinuous {
		return br.runJobOneshotContinuous(ctx, job, progressFn)
	}
	return br.runJobOneshot(ctx, job, progressFn)
}

// Close stops the daemon binary (no-op for oneshot)
func (br *BinaryRunner) Close() error {
	br.mu.Lock()
	defer br.mu.Unlock()

	if br.cmd == nil || br.cmd.Process == nil {
		return nil
	}
	_ = br.cmd.Process.Kill()
	br.cmd.Wait()
	br.cmd = nil
	br.stdin = nil
	br.stdout = nil
	return nil
}

// ─── Oneshot Mode ───────────────────────────────────────────────────────────

func (br *BinaryRunner) runJobOneshot(ctx context.Context, job Job, progressFn func(pct int, msg string)) (map[string]any, error) {
	return br.runOneshotInternal(ctx, job, progressFn, false)
}

// Oneshot Continuous: binary emits multiple outputs, we collect them all
func (br *BinaryRunner) runJobOneshotContinuous(ctx context.Context, job Job, progressFn func(pct int, msg string)) (map[string]any, error) {
	return br.runOneshotInternal(ctx, job, progressFn, true)
}

func (br *BinaryRunner) runOneshotInternal(ctx context.Context, job Job, progressFn func(pct int, msg string), continuous bool) (map[string]any, error) {
	cmd := exec.CommandContext(ctx, br.path)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start binary: %w", err)
	}

	input := map[string]any{
		"job_id":    job.ID,
		"type":      job.Type,
		"payload":   job.Payload,
		"timeout_s": job.TimeoutS,
	}
	if err := json.NewEncoder(stdin).Encode(input); err != nil {
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("write stdin: %w", err)
	}
	stdin.Close()

	scanner := bufio.NewScanner(stdout)
	var finalResult map[string]any
	var finalErr string
	var outputs []map[string]any // Continuous mode: collect all results

scanLoop:
	for scanner.Scan() {
		var out ExecResult
		if json.Unmarshal(scanner.Bytes(), &out) != nil {
			continue
		}
		switch out.Status {
		case "progress":
			if progressFn != nil {
				progressFn(out.Pct, out.Msg)
			}
		case "done":
			if continuous {
				// Continuous: collect result and keep reading for more
				outputs = append(outputs, out.Result)
				if progressFn != nil {
					progressFn(100, "output received")
				}
			} else {
				// Oneshot: return on first result
				finalResult = out.Result
				_ = cmd.Process.Kill() // Kill binary early
				break scanLoop
			}
		case "error":
			finalErr = out.Error
			break scanLoop
		}
	}

	waitErr := cmd.Wait()

	// Handle continuous mode results
	if continuous && len(outputs) > 0 {
		if len(outputs) == 1 {
			return outputs[0], nil
		}
		// Multiple outputs: return as array
		return map[string]any{"outputs": outputs}, nil
	}

	// Handle oneshot mode results
	if finalErr != "" {
		return nil, fmt.Errorf("%s", finalErr)
	}
	if finalResult != nil {
		return finalResult, nil
	}
	if waitErr != nil {
		return nil, fmt.Errorf("binary exited: %w", waitErr)
	}
	return nil, fmt.Errorf("binary produced no result")
}

// ─── Daemon Mode ────────────────────────────────────────────────────────────

func (br *BinaryRunner) runJobDaemon(ctx context.Context, job Job, progressFn func(pct int, msg string)) (map[string]any, error) {
	// Lazy-start the daemon on first job
	if err := br.ensureDaemonRunning(); err != nil {
		return nil, fmt.Errorf("start daemon: %w", err)
	}

	// Send job to daemon
	br.mu.Lock()
	if br.stdin == nil {
		br.mu.Unlock()
		return nil, fmt.Errorf("daemon stdin not available")
	}

	input := map[string]any{
		"job_id":    job.ID,
		"type":      job.Type,
		"payload":   job.Payload,
		"timeout_s": job.TimeoutS,
	}
	if err := json.NewEncoder(br.stdin).Encode(input); err != nil {
		br.mu.Unlock()
		_ = br.Close()
		return nil, fmt.Errorf("send to daemon: %w", err)
	}
	br.mu.Unlock()

	// Read response (with timeout via context)
	resultChan := make(chan map[string]any, 1)
	errChan := make(chan error, 1)

	go func() {
		br.mu.Lock()
		scanner := br.stdout
		br.mu.Unlock()

		if scanner == nil {
			errChan <- fmt.Errorf("daemon stdout not available")
			return
		}

		for scanner.Scan() {
			var out ExecResult
			if json.Unmarshal(scanner.Bytes(), &out) != nil {
				continue
			}

			switch out.Status {
			case "progress":
				if progressFn != nil {
					progressFn(out.Pct, out.Msg)
				}
			case "done":
				resultChan <- out.Result
				return
			case "error":
				errChan <- fmt.Errorf("%s", out.Error)
				return
			}
		}

		// Reached EOF without result or error
		errChan <- fmt.Errorf("daemon produced no result")
	}()

	// Wait for result with timeout
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-resultChan:
		return result, nil
	case err := <-errChan:
		return nil, err
	}
}

func (br *BinaryRunner) ensureDaemonRunning() error {
	br.mu.Lock()
	defer br.mu.Unlock()

	if br.cmd != nil && br.cmd.Process != nil {
		return nil // Already running
	}

	cmd := exec.Command(br.path)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start binary: %w", err)
	}

	br.cmd = cmd
	br.stdin = stdin
	br.stdout = bufio.NewScanner(stdout)

	return nil
}

// ─── Legacy (backward compat) ────────────────────────────────────────────────

// RunOneshot is the legacy oneshot function (kept for backward compatibility)
func RunOneshot(ctx context.Context, binaryPath string, job Job, progressFn func(pct int, msg string)) (map[string]any, error) {
	runner := NewBinaryRunner(binaryPath, "oneshot")
	return runner.RunJob(ctx, job, progressFn)
}
