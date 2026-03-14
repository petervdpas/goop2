package storage

import (
	"encoding/json"
	"time"
)

func (d *DB) SaveClusterJob(groupID, id, typ, mode, payload string, priority, timeoutS, maxRetry int,
	status, workerID, result, errMsg string, progress int, progressMsg string, retries int,
	createdAt, startedAt, doneAt string, elapsedMs int64) error {
	_, err := d.db.Exec(`
		INSERT INTO _cluster_jobs (id, group_id, type, mode, payload, priority, timeout_s, max_retry,
			status, worker_id, result, error, progress, progress_msg, retries, created_at, started_at, done_at, elapsed_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			status=excluded.status, worker_id=excluded.worker_id, result=excluded.result,
			error=excluded.error, progress=excluded.progress, progress_msg=excluded.progress_msg,
			retries=excluded.retries, started_at=excluded.started_at, done_at=excluded.done_at, elapsed_ms=excluded.elapsed_ms`,
		id, groupID, typ, mode, payload, priority, timeoutS, maxRetry,
		status, workerID, result, errMsg, progress, progressMsg, retries,
		createdAt, startedAt, doneAt, elapsedMs)
	return err
}

type ClusterJobRow struct {
	ID, Type, Mode, Payload                         string
	Priority, TimeoutS, MaxRetry                    int
	Status, WorkerID, Result, Error, ProgressMsg    string
	Progress, Retries                               int
	CreatedAt, StartedAt, DoneAt                    string
	ElapsedMs                                       int64
}

func (d *DB) LoadClusterJobs(groupID string) ([]ClusterJobRow, error) {
	rows, err := d.db.Query(`SELECT id, type, mode, payload, priority, timeout_s, max_retry,
		status, worker_id, result, error, progress, progress_msg, retries, created_at, started_at, done_at, elapsed_ms
		FROM _cluster_jobs WHERE group_id = ? ORDER BY created_at DESC`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ClusterJobRow
	for rows.Next() {
		var j ClusterJobRow
		if err := rows.Scan(&j.ID, &j.Type, &j.Mode, &j.Payload, &j.Priority, &j.TimeoutS, &j.MaxRetry,
			&j.Status, &j.WorkerID, &j.Result, &j.Error, &j.Progress, &j.ProgressMsg, &j.Retries,
			&j.CreatedAt, &j.StartedAt, &j.DoneAt, &j.ElapsedMs); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, nil
}

func (d *DB) DeleteClusterJobs(groupID string) error {
	_, err := d.db.Exec(`DELETE FROM _cluster_jobs WHERE group_id = ?`, groupID)
	return err
}

func MarshalJSON(v any) string {
	if v == nil {
		return "{}"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func UnmarshalJSON(s string) map[string]any {
	if s == "" || s == "{}" {
		return nil
	}
	var m map[string]any
	json.Unmarshal([]byte(s), &m)
	return m
}

func FormatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func ParseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339, s)
	return t
}
