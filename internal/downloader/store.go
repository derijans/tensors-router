package downloader

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func OpenStore(databasePath string) (*Store, error) {
	if err := ensureDirectory(filepath.Dir(databasePath)); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		return nil, err
	}
	store := &Store{db: db}
	if err := store.initialize(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (store *Store) Close() error {
	if store == nil || store.db == nil {
		return nil
	}
	return store.db.Close()
}

func (store *Store) initialize() error {
	statements := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
		`CREATE TABLE IF NOT EXISTS artifacts (path TEXT PRIMARY KEY, sha256 TEXT NOT NULL, size INTEGER NOT NULL, modified_unix_nano INTEGER NOT NULL, repository TEXT NOT NULL, repository_path TEXT NOT NULL, revision TEXT NOT NULL, verification_source TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS repositories (repository TEXT PRIMARY KEY, revision TEXT NOT NULL, local_root TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS jobs (id TEXT PRIMARY KEY, repository TEXT NOT NULL, revision TEXT NOT NULL, resolved_commit TEXT NOT NULL, state TEXT NOT NULL, total_bytes INTEGER NOT NULL, completed_bytes INTEGER NOT NULL, error TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS job_files (job_id TEXT NOT NULL, path TEXT NOT NULL, reason TEXT NOT NULL, expected_sha256 TEXT NOT NULL, size INTEGER NOT NULL, completed_bytes INTEGER NOT NULL, state TEXT NOT NULL, error TEXT NOT NULL, PRIMARY KEY(job_id, path), FOREIGN KEY(job_id) REFERENCES jobs(id) ON DELETE CASCADE)`,
		`CREATE TABLE IF NOT EXISTS scan_runs (id INTEGER PRIMARY KEY AUTOINCREMENT, generation INTEGER NOT NULL, state TEXT NOT NULL, completed_bytes INTEGER NOT NULL, error TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
	}
	for _, statement := range statements {
		if _, err := store.db.Exec(statement); err != nil {
			return err
		}
	}
	return store.migrateJobCommitColumn()
}

func (store *Store) migrateJobCommitColumn() error {
	rows, err := store.db.Query(`PRAGMA table_info(jobs)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	legacyCommit := false
	resolvedCommit := false
	for rows.Next() {
		var sequence int
		var name, columnType string
		var required, primaryKey int
		var defaultValue any
		if err := rows.Scan(&sequence, &name, &columnType, &required, &defaultValue, &primaryKey); err != nil {
			return err
		}
		legacyCommit = legacyCommit || name == "commit"
		resolvedCommit = resolvedCommit || name == "resolved_commit"
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if legacyCommit && !resolvedCommit {
		_, err := store.db.Exec(`ALTER TABLE jobs RENAME COLUMN "commit" TO resolved_commit`)
		return err
	}
	return nil
}

func (store *Store) Artifact(path string) (ArtifactRecord, bool, error) {
	row := store.db.QueryRow(`SELECT path, sha256, size, modified_unix_nano, repository, repository_path, revision, verification_source, created_at, updated_at FROM artifacts WHERE path = ?`, path)
	var record ArtifactRecord
	var created, updated string
	err := row.Scan(&record.Path, &record.SHA256, &record.Size, &record.ModifiedUnixNano, &record.Repository, &record.RepositoryPath, &record.Revision, &record.VerificationSource, &created, &updated)
	if err == sql.ErrNoRows {
		return ArtifactRecord{}, false, nil
	}
	if err != nil {
		return ArtifactRecord{}, false, err
	}
	record.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	record.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return record, true, nil
}

func (store *Store) SaveArtifact(record ArtifactRecord) error {
	if record.Path == "" || !validSHA256(record.SHA256) || record.Size < 0 {
		return fmt.Errorf("artifact record is invalid")
	}
	now := time.Now().UTC()
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	record.UpdatedAt = now
	_, err := store.db.Exec(`INSERT INTO artifacts(path, sha256, size, modified_unix_nano, repository, repository_path, revision, verification_source, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(path) DO UPDATE SET sha256=excluded.sha256, size=excluded.size, modified_unix_nano=excluded.modified_unix_nano, repository=excluded.repository, repository_path=excluded.repository_path, revision=excluded.revision, verification_source=excluded.verification_source, updated_at=excluded.updated_at`, record.Path, record.SHA256, record.Size, record.ModifiedUnixNano, record.Repository, record.RepositoryPath, record.Revision, record.VerificationSource, record.CreatedAt.Format(time.RFC3339Nano), record.UpdatedAt.Format(time.RFC3339Nano))
	return err
}

func (store *Store) ListArtifacts() ([]ArtifactRecord, error) {
	rows, err := store.db.Query(`SELECT path, sha256, size, modified_unix_nano, repository, repository_path, revision, verification_source, created_at, updated_at FROM artifacts ORDER BY path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []ArtifactRecord{}
	for rows.Next() {
		var record ArtifactRecord
		var created, updated string
		if err := rows.Scan(&record.Path, &record.SHA256, &record.Size, &record.ModifiedUnixNano, &record.Repository, &record.RepositoryPath, &record.Revision, &record.VerificationSource, &created, &updated); err != nil {
			return nil, err
		}
		record.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		record.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		result = append(result, record)
	}
	return result, rows.Err()
}

func (store *Store) DeleteArtifact(path string) error {
	_, err := store.db.Exec(`DELETE FROM artifacts WHERE path = ?`, path)
	return err
}

func (store *Store) SaveJob(job DownloadJob) error {
	if job.ID == "" || job.Repository == "" || job.Commit == "" || !validJobState(job.State) || job.TotalBytes < 0 || job.CompletedBytes < 0 {
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
	if _, err := transaction.Exec(`INSERT INTO jobs(id, repository, revision, resolved_commit, state, total_bytes, completed_bytes, error, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET state=excluded.state, total_bytes=excluded.total_bytes, completed_bytes=excluded.completed_bytes, error=excluded.error, updated_at=excluded.updated_at`, job.ID, job.Repository, job.Revision, job.Commit, job.State, job.TotalBytes, job.CompletedBytes, job.Error, job.CreatedAt.Format(time.RFC3339Nano), job.UpdatedAt.Format(time.RFC3339Nano)); err != nil {
		return err
	}
	if _, err := transaction.Exec(`DELETE FROM job_files WHERE job_id = ?`, job.ID); err != nil {
		return err
	}
	for _, file := range job.Files {
		if _, err := transaction.Exec(`INSERT INTO job_files(job_id, path, reason, expected_sha256, size, completed_bytes, state, error) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`, job.ID, file.Path, file.Reason, file.ExpectedSHA256, file.Size, file.CompletedBytes, file.State, file.Error); err != nil {
			return err
		}
	}
	return transaction.Commit()
}

func (store *Store) Job(id string) (DownloadJob, bool, error) {
	row := store.db.QueryRow(`SELECT id, repository, revision, resolved_commit, state, total_bytes, completed_bytes, error, created_at, updated_at FROM jobs WHERE id = ?`, id)
	var job DownloadJob
	var created, updated string
	err := row.Scan(&job.ID, &job.Repository, &job.Revision, &job.Commit, &job.State, &job.TotalBytes, &job.CompletedBytes, &job.Error, &created, &updated)
	if err == sql.ErrNoRows {
		return DownloadJob{}, false, nil
	}
	if err != nil {
		return DownloadJob{}, false, err
	}
	job.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	job.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	files, err := store.jobFiles(id)
	if err != nil {
		return DownloadJob{}, false, err
	}
	job.Files = files
	return job, true, nil
}

func (store *Store) Jobs() ([]DownloadJob, error) {
	rows, err := store.db.Query(`SELECT id FROM jobs ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := []DownloadJob{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		job, found, err := store.Job(id)
		if err != nil {
			return nil, err
		}
		if found {
			jobs = append(jobs, job)
		}
	}
	return jobs, rows.Err()
}

func (store *Store) jobFiles(jobID string) ([]JobFile, error) {
	rows, err := store.db.Query(`SELECT path, reason, expected_sha256, size, completed_bytes, state, error FROM job_files WHERE job_id = ? ORDER BY path`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	files := []JobFile{}
	for rows.Next() {
		var file JobFile
		if err := rows.Scan(&file.Path, &file.Reason, &file.ExpectedSHA256, &file.Size, &file.CompletedBytes, &file.State, &file.Error); err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, rows.Err()
}

func validJobState(state JobState) bool {
	switch state {
	case JobQueued, JobRunning, JobPaused, JobCancelled, JobFailed, JobCompleted:
		return true
	default:
		return false
	}
}

func artifactFromFile(path string, hash string, repository string, repositoryPath string, revision string, verificationSource string) (ArtifactRecord, error) {
	info, err := os.Stat(path)
	if err != nil {
		return ArtifactRecord{}, err
	}
	return ArtifactRecord{Path: path, SHA256: hash, Size: info.Size(), ModifiedUnixNano: info.ModTime().UnixNano(), Repository: repository, RepositoryPath: repositoryPath, Revision: revision, VerificationSource: verificationSource}, nil
}
