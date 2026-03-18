package cluster

import (
	"github.com/petervdpas/goop2/internal/storage"
)

type dbJobStore struct {
	db *storage.DB
}

func NewJobStore(db *storage.DB) JobStore {
	return &dbJobStore{db: db}
}

func (s *dbJobStore) SaveJob(groupID string, js *JobState) error {
	return s.db.SaveClusterJob(groupID,
		js.Job.ID, js.Job.Type, string(js.Job.Mode), storage.MarshalJSON(js.Job.Payload),
		js.Job.Priority, js.Job.TimeoutS, js.Job.MaxRetry,
		string(js.Status), js.WorkerID, storage.MarshalJSON(js.Result), js.Error,
		js.Progress, js.ProgressMsg, js.Retries,
		storage.FormatTime(js.CreatedAt), storage.FormatTime(js.StartedAt), storage.FormatTime(js.DoneAt),
		js.ElapsedMs)
}

func (s *dbJobStore) LoadJobs(groupID string) ([]*JobState, error) {
	rows, err := s.db.LoadClusterJobs(groupID)
	if err != nil {
		return nil, err
	}
	out := make([]*JobState, 0, len(rows))
	for _, r := range rows {
		out = append(out, &JobState{
			Job: Job{
				ID:       r.ID,
				Type:     r.Type,
				Mode:     JobMode(r.Mode),
				Payload:  storage.UnmarshalJSON(r.Payload),
				Priority: r.Priority,
				TimeoutS: r.TimeoutS,
				MaxRetry: r.MaxRetry,
			},
			Status:      JobStatus(r.Status),
			WorkerID:    r.WorkerID,
			Result:      storage.UnmarshalJSON(r.Result),
			Error:       r.Error,
			Progress:    r.Progress,
			ProgressMsg: r.ProgressMsg,
			Retries:     r.Retries,
			CreatedAt:   storage.ParseTime(r.CreatedAt),
			StartedAt:   storage.ParseTime(r.StartedAt),
			DoneAt:      storage.ParseTime(r.DoneAt),
			ElapsedMs:   r.ElapsedMs,
		})
	}
	return out, nil
}

func (s *dbJobStore) DeleteJob(groupID, jobID string) error {
	return s.db.DeleteClusterJob(groupID, jobID)
}

func (s *dbJobStore) DeleteJobs(groupID string) error {
	return s.db.DeleteClusterJobs(groupID)
}
