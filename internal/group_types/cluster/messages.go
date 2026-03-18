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
			m.dispatcher.AddWorker(evt.From)
		}
	case "leave":
		if m.role == roleHost && evt.From != m.selfID {
			log.Printf("CLUSTER: worker left: %s", evt.From)
			m.dispatcher.RemoveWorker(evt.From)
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
				log.Printf("CLUSTER: job %s acknowledged by %s (running)", jobID, from)
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
				log.Printf("CLUSTER: job %s completed by %s", jobID, from)
				m.queue.Complete(jobID, result)
				m.dispatcher.UpdateWorkerStatus(from, WorkerIdle)
			case "failed":
				errMsg, _ := data["error"].(string)
				log.Printf("CLUSTER: job %s failed on %s: %s", jobID, from, errMsg)
				m.queue.Fail(jobID, errMsg)
				m.dispatcher.UpdateWorkerStatus(from, WorkerIdle)
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
			m.dispatcher.SetWorkerVerified(from, ok, types, int(capacity))
		case "worker:binary":
			path, _ := data["path"].(string)
			mode, _ := data["mode"].(string)
			log.Printf("CLUSTER: worker %s set binary %s (mode=%s)", from, path, mode)
			m.dispatcher.SetWorkerBinary(from, path, mode)
		case "worker:status":
			statusStr, _ := data["status"].(string)
			m.dispatcher.UpdateWorkerStatus(from, WorkerStatus(statusStr))
		}

	case r == roleWorker:
		switch msgType {
		case "job:assign":
			jobID, _ := data["job_id"].(string)
			jobType, _ := data["type"].(string)
			jobPayload, _ := data["payload"].(map[string]any)
			timeoutS, _ := data["timeout_s"].(float64)
			log.Printf("CLUSTER: received job %s (type=%s) from host", jobID, jobType)
			m.worker.HandleJob(from, Job{
				ID:       jobID,
				Type:     jobType,
				Payload:  jobPayload,
				TimeoutS: int(timeoutS),
			})
		case "job:cancel":
			jobID, _ := data["job_id"].(string)
			if jobID != "" {
				log.Printf("CLUSTER: job %s cancelled by host", jobID)
				m.worker.Cancel(jobID)
			}
		case "worker:pause":
			log.Printf("CLUSTER: paused by host")
			m.worker.Pause()
		case "worker:resume":
			log.Printf("CLUSTER: resumed by host")
			m.worker.Resume()
		}
	}
}
