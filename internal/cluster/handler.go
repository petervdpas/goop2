package cluster

import "log"

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
			for _, jobID := range m.queue.WorkerJobIDs(evt.From) {
				m.queue.Fail(jobID, "worker left")
			}
		}
	case "close":
		log.Printf("CLUSTER: group closed: %s", evt.Group)
		m.cleanup()
	}
}

func (m *Manager) handleClusterMessage(from, msgType string, payload any) {
	m.mu.Lock()
	r := m.role
	m.mu.Unlock()

	data, _ := payload.(map[string]any)
	if data == nil {
		return
	}

	switch {
	case r == roleHost:
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
			jobID, _ := data["job_id"].(string)
			pct, _ := data["percent"].(float64)
			msg, _ := data["message"].(string)
			if jobID != "" {
				m.queue.UpdateProgress(jobID, int(pct), msg)
			}
		case "worker:verified":
			ok, _ := data["ok"].(bool)
			capacity, _ := data["capacity"].(float64)
			var types []string
			if raw, _ := data["types"].([]any); raw != nil {
				for _, v := range raw {
					if s, _ := v.(string); s != "" {
						types = append(types, s)
					}
				}
			}
			m.scheduler.SetWorkerVerified(from, ok, types, int(capacity))
		case "worker:binary":
			path, _ := data["path"].(string)
			mode, _ := data["mode"].(string)
			m.scheduler.SetWorkerBinary(from, path, mode)
		case "worker:status":
			statusStr, _ := data["status"].(string)
			m.scheduler.UpdateWorkerStatus(from, WorkerStatus(statusStr))
		}

	case r == roleWorker:
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
