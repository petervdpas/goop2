package cluster

import (
	"sync"
	"testing"
	"time"
)

// ── Queue tests ─────────────────────────────────────────────────────────────

func TestQueueSubmitAndNextPending(t *testing.T) {
	q := NewQueue()

	id := q.Submit(Job{Type: "render", Priority: 1})
	if id == "" {
		t.Fatal("expected non-empty job ID")
	}

	job := q.NextPending()
	if job == nil {
		t.Fatal("expected a pending job")
	}
	if job.ID != id {
		t.Fatalf("got job ID %s, want %s", job.ID, id)
	}
}

func TestQueuePriority(t *testing.T) {
	q := NewQueue()

	q.Submit(Job{Type: "low", Priority: 1})
	highID := q.Submit(Job{Type: "high", Priority: 10})

	job := q.NextPending()
	if job.ID != highID {
		t.Fatalf("expected high-priority job %s, got %s", highID, job.ID)
	}
}

func TestQueueAssignAndComplete(t *testing.T) {
	q := NewQueue()
	id := q.Submit(Job{Type: "test"})

	q.Assign(id, "worker-1")
	js, ok := q.Get(id)
	if !ok {
		t.Fatal("job not found")
	}
	if js.Status != StatusAssigned {
		t.Fatalf("expected assigned, got %s", js.Status)
	}

	q.MarkRunning(id)
	js, _ = q.Get(id)
	if js.Status != StatusRunning {
		t.Fatalf("expected running, got %s", js.Status)
	}

	q.Complete(id, map[string]any{"output": "done"})
	js, _ = q.Get(id)
	if js.Status != StatusCompleted {
		t.Fatalf("expected completed, got %s", js.Status)
	}
	if js.Result["output"] != "done" {
		t.Fatalf("unexpected result: %v", js.Result)
	}
}

func TestQueueFailRetry(t *testing.T) {
	q := NewQueue()
	id := q.Submit(Job{Type: "flaky", MaxRetry: 2})

	q.Assign(id, "w1")
	q.Fail(id, "oops")
	js, _ := q.Get(id)
	if js.Status != StatusPending {
		t.Fatalf("expected re-queued to pending, got %s", js.Status)
	}
	if js.Retries != 1 {
		t.Fatalf("expected 1 retry, got %d", js.Retries)
	}

	// Second failure
	q.Assign(id, "w2")
	q.Fail(id, "oops again")
	js, _ = q.Get(id)
	if js.Status != StatusPending {
		t.Fatalf("expected pending after 2nd fail, got %s", js.Status)
	}

	// Third failure — exhausted
	q.Assign(id, "w3")
	q.Fail(id, "final fail")
	js, _ = q.Get(id)
	if js.Status != StatusFailed {
		t.Fatalf("expected failed after exhausting retries, got %s", js.Status)
	}
}

func TestQueueCancel(t *testing.T) {
	q := NewQueue()
	id := q.Submit(Job{Type: "cancelme"})

	if err := q.Cancel(id); err != nil {
		t.Fatalf("cancel failed: %v", err)
	}
	js, _ := q.Get(id)
	if js.Status != StatusCancelled {
		t.Fatalf("expected cancelled, got %s", js.Status)
	}

	// Can't cancel again
	if err := q.Cancel(id); err == nil {
		t.Fatal("expected error cancelling terminal job")
	}
}

func TestQueueStats(t *testing.T) {
	q := NewQueue()
	q.Submit(Job{Type: "a"})
	q.Submit(Job{Type: "b"})
	id := q.Submit(Job{Type: "c"})
	q.Assign(id, "w1")
	q.MarkRunning(id)

	s := q.Stats()
	if s.Pending != 2 {
		t.Fatalf("expected 2 pending, got %d", s.Pending)
	}
	if s.Running != 1 {
		t.Fatalf("expected 1 running, got %d", s.Running)
	}
}

// ── Scheduler tests ─────────────────────────────────────────────────────────

func TestSchedulerDispatch(t *testing.T) {
	q := NewQueue()
	jobID := q.Submit(Job{Type: "render", Priority: 1})

	var mu sync.Mutex
	sent := make(map[string]string) // peerID → topic

	sendFn := func(peerID, topic string, payload any) error {
		mu.Lock()
		sent[peerID] = topic
		mu.Unlock()
		return nil
	}

	s := NewScheduler(q, sendFn)
	s.AddWorker("peer-A")

	// Manually call dispatch instead of running the loop
	s.dispatch("test-group")

	mu.Lock()
	topic, ok := sent["peer-A"]
	mu.Unlock()

	if !ok {
		t.Fatal("expected job to be dispatched to peer-A")
	}
	if topic != "cluster:test-group:job:assign" {
		t.Fatalf("unexpected topic: %s", topic)
	}

	// Job should be assigned now
	js, _ := q.Get(jobID)
	if js.Status != StatusAssigned {
		t.Fatalf("expected assigned, got %s", js.Status)
	}
	if js.WorkerID != "peer-A" {
		t.Fatalf("expected worker peer-A, got %s", js.WorkerID)
	}
}

func TestSchedulerNoIdleWorkers(t *testing.T) {
	q := NewQueue()
	q.Submit(Job{Type: "test"})

	sendFn := func(peerID, topic string, payload any) error {
		t.Fatal("should not send when no workers")
		return nil
	}

	s := NewScheduler(q, sendFn)
	s.dispatch("g1") // no workers registered — should be a no-op
}

func TestSchedulerRemoveWorker(t *testing.T) {
	q := NewQueue()
	q.Submit(Job{Type: "test"})

	sendFn := func(peerID, topic string, payload any) error {
		t.Fatal("should not send after worker removed")
		return nil
	}

	s := NewScheduler(q, sendFn)
	s.AddWorker("peer-A")
	s.RemoveWorker("peer-A")
	s.dispatch("g1")
}

// ── Worker tests ────────────────────────────────────────────────────────────

func TestWorkerHandleJob(t *testing.T) {
	var mu sync.Mutex
	messages := make([]map[string]any, 0)

	sendFn := func(peerID, topic string, payload any) error {
		mu.Lock()
		data, _ := payload.(map[string]any)
		messages = append(messages, map[string]any{
			"peer":  peerID,
			"topic": topic,
			"data":  data,
		})
		mu.Unlock()
		return nil
	}

	w := NewWorker(sendFn, "g1")
	w.HandleJob("host-peer", Job{ID: "j1", Type: "echo", TimeoutS: 5})

	// Wait for the goroutine to complete
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(messages) < 2 {
		t.Fatalf("expected at least 2 messages (ack + result), got %d", len(messages))
	}

	// First message should be ack
	if messages[0]["topic"] != "cluster:g1:job:ack" {
		t.Fatalf("expected ack topic, got %s", messages[0]["topic"])
	}

	// Second message should be result
	if messages[1]["topic"] != "cluster:g1:job:result" {
		t.Fatalf("expected result topic, got %s", messages[1]["topic"])
	}

	if w.Status() != WorkerIdle {
		t.Fatalf("expected idle after job, got %s", w.Status())
	}
}

func TestWorkerCancel(t *testing.T) {
	sendFn := func(peerID, topic string, payload any) error { return nil }

	w := NewWorker(sendFn, "g1")
	// Submit a job with long timeout so we can cancel it
	w.HandleJob("host", Job{ID: "j1", Type: "slow", TimeoutS: 60})

	time.Sleep(50 * time.Millisecond) // let goroutine start
	w.Cancel("j1")

	time.Sleep(50 * time.Millisecond)
	if w.RunningCount() != 0 {
		t.Fatalf("expected 0 running jobs after cancel, got %d", w.RunningCount())
	}
}

func TestWorkerClose(t *testing.T) {
	sendFn := func(peerID, topic string, payload any) error { return nil }

	w := NewWorker(sendFn, "g1")
	w.HandleJob("host", Job{ID: "j1", Type: "test", TimeoutS: 60})
	time.Sleep(50 * time.Millisecond)

	w.Close()
	if w.Status() != WorkerOffline {
		t.Fatalf("expected offline after close, got %s", w.Status())
	}
}

// ── Manager tests ───────────────────────────────────────────────────────────

func TestManagerCreateAndJoinCluster(t *testing.T) {
	sendFn := func(peerID, topic string, payload any) error { return nil }
	subFn := func(fn func(from, topic string, payload any)) func() {
		return func() {} // no-op unsubscribe
	}

	m := New("self", sendFn, subFn)
	defer m.Close()

	if err := m.CreateCluster("g1"); err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if m.Role() != "host" {
		t.Fatalf("expected host role, got %s", m.Role())
	}

	// Can't join while hosting
	if err := m.JoinCluster("g2"); err == nil {
		t.Fatal("expected error joining while already in cluster")
	}

	m.LeaveCluster()
	if m.Role() != "" {
		t.Fatalf("expected empty role after leave, got %s", m.Role())
	}

	// Now can join
	if err := m.JoinCluster("g2"); err != nil {
		t.Fatalf("join failed: %v", err)
	}
	if m.Role() != "worker" {
		t.Fatalf("expected worker role, got %s", m.Role())
	}
}

func TestManagerSubmitJob(t *testing.T) {
	sendFn := func(peerID, topic string, payload any) error { return nil }
	subFn := func(fn func(from, topic string, payload any)) func() {
		return func() {}
	}

	m := New("self", sendFn, subFn)
	defer m.Close()

	// Can't submit without being host
	_, err := m.SubmitJob(Job{Type: "test"})
	if err == nil {
		t.Fatal("expected error submitting as non-host")
	}

	m.CreateCluster("g1")

	id, err := m.SubmitJob(Job{Type: "render", Priority: 5})
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty job ID")
	}

	jobs := m.GetJobs()
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}

	stats := m.GetStats()
	if stats.Pending != 1 {
		t.Fatalf("expected 1 pending, got %d", stats.Pending)
	}
}

func TestManagerGroupEvents(t *testing.T) {
	var mu sync.Mutex
	sent := make(map[string]bool)

	sendFn := func(peerID, topic string, payload any) error {
		mu.Lock()
		sent[peerID] = true
		mu.Unlock()
		return nil
	}
	subFn := func(fn func(from, topic string, payload any)) func() {
		return func() {}
	}

	m := New("host-peer", sendFn, subFn)
	defer m.Close()

	m.CreateCluster("g1")
	m.SubmitJob(Job{Type: "test"})

	// Simulate worker join
	m.HandleGroupEvent(&GroupEvent{
		Type:  "join",
		Group: "g1",
		From:  "worker-1",
	})

	// Give scheduler time to dispatch
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	dispatched := sent["worker-1"]
	mu.Unlock()

	if !dispatched {
		t.Fatal("expected job to be dispatched to worker-1 after join")
	}

	workers := m.GetWorkers()
	if len(workers) != 1 {
		t.Fatalf("expected 1 worker, got %d", len(workers))
	}

	// Simulate worker leave
	m.HandleGroupEvent(&GroupEvent{
		Type:  "leave",
		Group: "g1",
		From:  "worker-1",
	})

	time.Sleep(50 * time.Millisecond)
	workers = m.GetWorkers()
	if len(workers) != 0 {
		t.Fatalf("expected 0 workers after leave, got %d", len(workers))
	}
}

// ── Handler tests ───────────────────────────────────────────────────────────

func TestHandlerJobAckAndResult(t *testing.T) {
	sendFn := func(peerID, topic string, payload any) error { return nil }
	subFn := func(fn func(from, topic string, payload any)) func() {
		return func() {}
	}

	m := New("host", sendFn, subFn)
	defer m.Close()

	m.CreateCluster("g1")
	id, _ := m.SubmitJob(Job{Type: "test"})

	// Manually assign to simulate scheduler
	m.mu.Lock()
	m.queue.Assign(id, "worker-1")
	m.mu.Unlock()

	// Simulate ack
	m.handleClusterMessage("worker-1", "job:ack", map[string]any{"job_id": id})
	js, _ := m.queue.Get(id)
	if js.Status != StatusRunning {
		t.Fatalf("expected running after ack, got %s", js.Status)
	}

	// Simulate result
	m.handleClusterMessage("worker-1", "job:result", map[string]any{
		"job_id": id,
		"status": "completed",
		"result": map[string]any{"output": "42"},
	})
	js, _ = m.queue.Get(id)
	if js.Status != StatusCompleted {
		t.Fatalf("expected completed, got %s", js.Status)
	}
}
