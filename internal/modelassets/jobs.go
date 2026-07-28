package modelassets

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

const (
	JobQueued    = "queued"
	JobResolving = "resolving"
	JobCompleted = "completed"
	JobFailed    = "failed"
)

type ResolutionJob struct {
	ID            string        `json:"id"`
	ConfigID      string        `json:"config_id"`
	NodeID        string        `json:"node_id"`
	State         string        `json:"state"`
	Source        string        `json:"source,omitempty"`
	ProgressBytes int64         `json:"progress_bytes"`
	TotalBytes    int64         `json:"total_bytes"`
	Error         string        `json:"error,omitempty"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
	Results       []FieldResult `json:"results,omitempty"`
}

func (index *Index) CreateResolutionJob(configID string, nodeID string) (ResolutionJob, error) {
	id, err := resolutionJobID()
	if err != nil {
		return ResolutionJob{}, err
	}
	now := time.Now().UTC()
	job := ResolutionJob{ID: id, ConfigID: configID, NodeID: nodeID, State: JobQueued, CreatedAt: now, UpdatedAt: now}
	_, err = index.db.Exec(`INSERT INTO resolution_jobs(id, config_id, node_id, state, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?)`, job.ID, job.ConfigID, job.NodeID, job.State, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	return job, err
}

func (index *Index) UpdateResolutionJob(job ResolutionJob) error {
	if job.ID == "" || !resolutionJobState(job.State) {
		return fmt.Errorf("invalid resolution job")
	}
	job.UpdatedAt = time.Now().UTC()
	transaction, err := index.db.Begin()
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	if _, err := transaction.Exec(`UPDATE resolution_jobs SET state=?, source=?, progress_bytes=?, total_bytes=?, error=?, updated_at=? WHERE id=?`, job.State, job.Source, job.ProgressBytes, job.TotalBytes, job.Error, job.UpdatedAt.Format(time.RFC3339Nano), job.ID); err != nil {
		return err
	}
	if _, err := transaction.Exec(`DELETE FROM resolution_results WHERE job_id=?`, job.ID); err != nil {
		return err
	}
	for _, result := range job.Results {
		source := result.Source
		if source == "" {
			source = job.Source
		}
		if _, err := transaction.Exec(`INSERT INTO resolution_results(job_id, field_name, sha256, resolved, source, verification, commit_hash, error) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`, job.ID, result.Field, result.Hash, result.Resolved, source, result.Verification, result.Commit, result.Failure); err != nil {
			return err
		}
	}
	return transaction.Commit()
}

func (index *Index) ResolutionJob(id string) (ResolutionJob, bool, error) {
	var job ResolutionJob
	var created, updated string
	err := index.db.QueryRow(`SELECT id, config_id, node_id, state, source, progress_bytes, total_bytes, error, created_at, updated_at FROM resolution_jobs WHERE id=?`, id).Scan(&job.ID, &job.ConfigID, &job.NodeID, &job.State, &job.Source, &job.ProgressBytes, &job.TotalBytes, &job.Error, &created, &updated)
	if err == sql.ErrNoRows {
		return ResolutionJob{}, false, nil
	}
	if err != nil {
		return ResolutionJob{}, false, err
	}
	job.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	job.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	rows, err := index.db.Query(`SELECT field_name, sha256, resolved, source, verification, commit_hash, error FROM resolution_results WHERE job_id=? ORDER BY field_name, sha256`, id)
	if err != nil {
		return ResolutionJob{}, false, err
	}
	defer rows.Close()
	for rows.Next() {
		var result FieldResult
		if err := rows.Scan(&result.Field, &result.Hash, &result.Resolved, &result.Source, &result.Verification, &result.Commit, &result.Failure); err != nil {
			return ResolutionJob{}, false, err
		}
		job.Results = append(job.Results, result)
	}
	return job, true, rows.Err()
}

func (index *Index) LatestResolutionStates() (map[string]ResolutionJob, error) {
	rows, err := index.db.Query(`SELECT id, config_id, node_id, state, source, progress_bytes, total_bytes, error, created_at, updated_at FROM resolution_jobs WHERE rowid IN (SELECT MAX(rowid) FROM resolution_jobs GROUP BY config_id)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	states := map[string]ResolutionJob{}
	for rows.Next() {
		var job ResolutionJob
		var created, updated string
		if err := rows.Scan(&job.ID, &job.ConfigID, &job.NodeID, &job.State, &job.Source, &job.ProgressBytes, &job.TotalBytes, &job.Error, &created, &updated); err != nil {
			return nil, err
		}
		job.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		job.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		states[job.ConfigID] = job
	}
	return states, rows.Err()
}

func resolutionJobID() (string, error) {
	content := make([]byte, 16)
	if _, err := rand.Read(content); err != nil {
		return "", err
	}
	return hex.EncodeToString(content), nil
}

func resolutionJobState(value string) bool {
	return value == JobQueued || value == JobResolving || value == JobCompleted || value == JobFailed
}
