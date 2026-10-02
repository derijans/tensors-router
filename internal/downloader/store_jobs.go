package downloader

import (
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"time"
)

const listedJobLimit = 500

func (store *Store) SaveJob(job DownloadJob) error {
	if job.ID == "" || job.Repository == "" || job.Commit == "" || !validJobState(job.State) || job.TotalBytes < 0 || job.CompletedBytes < 0 || job.TreeSHA256 != "" && !validSHA256(job.TreeSHA256) {
		return fmt.Errorf("download job is invalid")
	}
	now := time.Now().UTC()
	if job.CreatedAt.IsZero() {
		job.CreatedAt = now
	}
	job.UpdatedAt = now
	transaction, err := store.db.Begin()
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	if _, err := transaction.Exec(`INSERT INTO jobs(id, repository, revision, resolved_commit, state, total_bytes, completed_bytes, error, snapshot, tree_sha256, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET state=excluded.state, total_bytes=excluded.total_bytes, completed_bytes=excluded.completed_bytes, error=excluded.error, snapshot=excluded.snapshot, tree_sha256=excluded.tree_sha256, updated_at=excluded.updated_at`, job.ID, job.Repository, job.Revision, job.Commit, job.State, job.TotalBytes, job.CompletedBytes, job.Error, job.Snapshot, job.TreeSHA256, job.CreatedAt.Format(time.RFC3339Nano), job.UpdatedAt.Format(time.RFC3339Nano)); err != nil {
		return err
	}
	if _, err := transaction.Exec(`DELETE FROM job_files WHERE job_id = ?`, job.ID); err != nil {
		return err
	}
	for _, file := range job.Files {
		if _, err := transaction.Exec(`INSERT INTO job_files(job_id, path, reason, expected_sha256, expected_git_oid, size, completed_bytes, state, error) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`, job.ID, file.Path, file.Reason, file.ExpectedSHA256, file.ExpectedGitOID, file.Size, file.CompletedBytes, file.State, file.Error); err != nil {
			return err
		}
	}
	return transaction.Commit()
}

func (store *Store) Job(id string) (DownloadJob, bool, error) {
	row := store.db.QueryRow(`SELECT id, repository, revision, resolved_commit, state, total_bytes, error, snapshot, tree_sha256, created_at, updated_at FROM jobs WHERE id = ?`, id)
	var job DownloadJob
	var created, updated string
	err := row.Scan(&job.ID, &job.Repository, &job.Revision, &job.Commit, &job.State, &job.TotalBytes, &job.Error, &job.Snapshot, &job.TreeSHA256, &created, &updated)
	if err == sql.ErrNoRows {
		return DownloadJob{}, false, nil
	}
	if err != nil {
		return DownloadJob{}, false, err
	}
	job.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	job.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	if job.Files, err = store.jobFiles(id); err != nil {
		return DownloadJob{}, false, err
	}
	for _, file := range job.Files {
		job.CompletedBytes += file.CompletedBytes
	}
	return job, true, nil
}

func (store *Store) Jobs() ([]DownloadJob, error) {
	return store.jobsWhere(`ORDER BY updated_at DESC LIMIT ?`, listedJobLimit)
}

func (store *Store) ActiveJobFor(repository string, commit string, snapshot bool, paths []string) (DownloadJob, bool, error) {
	candidates, err := store.jobsWhere(`WHERE repository = ? AND resolved_commit = ? AND snapshot = ? AND state IN (?, ?, ?) ORDER BY updated_at DESC`, repository, commit, snapshot, JobQueued, JobRunning, JobPaused)
	if err != nil {
		return DownloadJob{}, false, err
	}
	wanted := slices.Sorted(slices.Values(paths))
	for _, candidate := range candidates {
		existing := make([]string, 0, len(candidate.Files))
		for _, file := range candidate.Files {
			existing = append(existing, file.Path)
		}
		if slices.Equal(slices.Sorted(slices.Values(existing)), wanted) {
			return candidate, true, nil
		}
	}
	return DownloadJob{}, false, nil
}

func (store *Store) jobsWhere(clause string, arguments ...any) ([]DownloadJob, error) {
	rows, err := store.db.Query(`SELECT id FROM jobs `+clause, arguments...)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := closeRows(rows); err != nil {
		return nil, err
	}
	jobs := make([]DownloadJob, 0, len(ids))
	for _, id := range ids {
		job, found, err := store.Job(id)
		if err != nil {
			return nil, err
		}
		if found {
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

func closeRows(rows *sql.Rows) error {
	iterationError := rows.Err()
	if closeError := rows.Close(); iterationError == nil {
		return closeError
	}
	return iterationError
}

func (store *Store) TransitionJob(id string, to JobState, message string, from ...JobState) (bool, error) {
	placeholders := strings.TrimSuffix(strings.Repeat("?, ", len(from)), ", ")
	arguments := []any{to, message, nowText(), id}
	for _, state := range from {
		arguments = append(arguments, state)
	}
	result, err := store.db.Exec(`UPDATE jobs SET state = ?, error = ?, updated_at = ? WHERE id = ? AND state IN (`+placeholders+`)`, arguments...)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected == 1, err
}

func (store *Store) UpdateFile(jobID string, path string, state JobState, message string, completed int64) error {
	return store.updateFilesAndTouch(jobID, `UPDATE job_files SET state = ?, error = ?, completed_bytes = ? WHERE job_id = ? AND path = ?`, state, message, completed, jobID, path)
}

func (store *Store) UpdateFileProgress(jobID string, path string, completed int64) error {
	return store.updateFilesAndTouch(jobID, `UPDATE job_files SET completed_bytes = ? WHERE job_id = ? AND path = ?`, completed, jobID, path)
}

func (store *Store) FailUnfinishedFiles(jobID string, message string) error {
	return store.updateFilesAndTouch(jobID, `UPDATE job_files SET state = ?, error = ? WHERE job_id = ? AND state IN (?, ?)`, JobFailed, message, jobID, JobQueued, JobRunning)
}

func (store *Store) ResetUnfinishedFiles(jobID string) error {
	return store.updateFilesAndTouch(jobID, `UPDATE job_files SET state = ?, error = '' WHERE job_id = ? AND state != ?`, JobQueued, jobID, JobCompleted)
}

func (store *Store) CompleteAllFiles(jobID string) error {
	return store.updateFilesAndTouch(jobID, `UPDATE job_files SET state = ?, error = '', completed_bytes = size WHERE job_id = ?`, JobCompleted, jobID)
}

func (store *Store) SetTreeDigest(jobID string, digest string) error {
	if !validSHA256(digest) {
		return fmt.Errorf("snapshot tree digest is invalid")
	}
	_, err := store.db.Exec(`UPDATE jobs SET tree_sha256 = ?, updated_at = ? WHERE id = ?`, digest, nowText(), jobID)
	return err
}

func (store *Store) updateFilesAndTouch(jobID string, statement string, arguments ...any) error {
	transaction, err := store.db.Begin()
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	if _, err := transaction.Exec(statement, arguments...); err != nil {
		return err
	}
	if _, err := transaction.Exec(`UPDATE jobs SET updated_at = ? WHERE id = ?`, nowText(), jobID); err != nil {
		return err
	}
	return transaction.Commit()
}

func (store *Store) RequeueInterrupted() ([]string, error) {
	transaction, err := store.db.Begin()
	if err != nil {
		return nil, err
	}
	defer transaction.Rollback()
	rows, err := transaction.Query(`SELECT id FROM jobs WHERE state IN (?, ?) ORDER BY created_at`, JobQueued, JobRunning)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := closeRows(rows); err != nil {
		return nil, err
	}
	if _, err := transaction.Exec(`UPDATE jobs SET state = ?, error = '', updated_at = ? WHERE state = ?`, JobQueued, nowText(), JobRunning); err != nil {
		return nil, err
	}
	if _, err := transaction.Exec(`UPDATE job_files SET state = ? WHERE state = ?`, JobQueued, JobRunning); err != nil {
		return nil, err
	}
	return ids, transaction.Commit()
}

func (store *Store) PruneFinishedJobs(updatedBefore time.Time) (int64, error) {
	result, err := store.db.Exec(`DELETE FROM jobs WHERE state IN (?, ?, ?) AND updated_at < ?`, JobCompleted, JobFailed, JobCancelled, updatedBefore.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (store *Store) jobFiles(jobID string) ([]JobFile, error) {
	rows, err := store.db.Query(`SELECT path, reason, expected_sha256, expected_git_oid, size, completed_bytes, state, error FROM job_files WHERE job_id = ? ORDER BY path`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	files := []JobFile{}
	for rows.Next() {
		var file JobFile
		if err := rows.Scan(&file.Path, &file.Reason, &file.ExpectedSHA256, &file.ExpectedGitOID, &file.Size, &file.CompletedBytes, &file.State, &file.Error); err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, rows.Err()
}

func nowText() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func validJobState(state JobState) bool {
	switch state {
	case JobQueued, JobRunning, JobPaused, JobCancelled, JobFailed, JobCompleted:
		return true
	default:
		return false
	}
}
