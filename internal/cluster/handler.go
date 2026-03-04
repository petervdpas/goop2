package cluster

import "log"

// handleGroupEvent processes group membership events.
// Called by the manager when a group.Handler event arrives.
func (m *Manager) handleGroupEvent(evt *GroupEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.groupID == "" || evt.Group != m.groupID {
		return
	}

	switch evt.Type {
	case "join":
		if m.role == roleHost && evt.From != m.selfID {
			log.Printf("CLUSTER: worker joined: %s", evt.From)
			m.scheduler.AddWorker(evt.From)
		}
	case "leave":
		if m.role == roleHost && evt.From != m.selfID {
			log.Printf("CLUSTER: worker left: %s", evt.From)
			m.scheduler.RemoveWorker(evt.From)
			// Re-queue any job assigned to this worker
			jobID := m.queue.WorkerJobID(evt.From)
			if jobID != "" {
				m.queue.Fail(jobID, "worker left")
			}
		}
	case "close":
		log.Printf("CLUSTER: group closed: %s", evt.Group)
		m.cleanup()
	}
}

// handleClusterMessage routes incoming job protocol messages.
func (m *Manager) handleClusterMessage(from, msgType string, payload any) {
	m.mu.Lock()
	role := m.role
	m.mu.Unlock()

	data, _ := payload.(map[string]any)
	if data == nil {
		return
	}

	switch {
	// Host receives from workers
	case role == roleHost:
		switch msgType {
		case "job:ack":
			jobID, _ := data["job_id"].(string)
			if jobID != "" {
				m.queue.MarkRunning(jobID)
			}
		case "job:result":
			jobID, _ := data["job_id"].(string)
			status, _ := data["status"].(string)
			if jobID == "" {
				return
			}
			switch status {
			case "completed":
				result, _ := data["result"].(map[string]any)
				m.queue.Complete(jobID, result)
				m.scheduler.UpdateWorkerStatus(from, WorkerIdle)
			case "failed":
				errMsg, _ := data["error"].(string)
				m.queue.Fail(jobID, errMsg)
				m.scheduler.UpdateWorkerStatus(from, WorkerIdle)
			}
		case "job:progress":
			// v1: log only
			jobID, _ := data["job_id"].(string)
			log.Printf("CLUSTER: progress for job %s from %s", jobID, from)
		case "worker:status":
			statusStr, _ := data["status"].(string)
			m.scheduler.UpdateWorkerStatus(from, WorkerStatus(statusStr))
		}

	// Worker receives from host
	case role == roleWorker:
		switch msgType {
		case "job:assign":
			jobID, _ := data["job_id"].(string)
			jobType, _ := data["type"].(string)
			jobPayload, _ := data["payload"].(map[string]any)
			timeoutS, _ := data["timeout_s"].(float64)
			m.worker.HandleJob(from, Job{
				ID:       jobID,
				Type:     jobType,
				Payload:  jobPayload,
				TimeoutS: int(timeoutS),
			})
		case "job:cancel":
			jobID, _ := data["job_id"].(string)
			if jobID != "" {
				m.worker.Cancel(jobID)
			}
		}
	}
}
