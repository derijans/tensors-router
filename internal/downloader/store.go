package downloader

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const storePragmas = "_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"

type Store struct {
	db *sql.DB
}

func OpenStore(databasePath string) (*Store, error) {
	if err := ensureDirectory(filepath.Dir(databasePath)); err != nil {
		return nil, err
	}
	if strings.Contains(databasePath, "?") {
		return nil, fmt.Errorf("downloader database path must not contain '?'")
	}
	db, err := sql.Open("sqlite", databasePath+"?"+storePragmas)
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
		`CREATE TABLE IF NOT EXISTS artifacts (path TEXT PRIMARY KEY, sha256 TEXT NOT NULL, size INTEGER NOT NULL, modified_unix_nano INTEGER NOT NULL, repository TEXT NOT NULL, repository_path TEXT NOT NULL, revision TEXT NOT NULL, verification_source TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS repositories (repository TEXT PRIMARY KEY, revision TEXT NOT NULL, local_root TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS jobs (id TEXT PRIMARY KEY, repository TEXT NOT NULL, revision TEXT NOT NULL, resolved_commit TEXT NOT NULL, state TEXT NOT NULL, total_bytes INTEGER NOT NULL, completed_bytes INTEGER NOT NULL, error TEXT NOT NULL, snapshot INTEGER NOT NULL DEFAULT 0, tree_sha256 TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS job_files (job_id TEXT NOT NULL, path TEXT NOT NULL, reason TEXT NOT NULL, expected_sha256 TEXT NOT NULL, size INTEGER NOT NULL, completed_bytes INTEGER NOT NULL, state TEXT NOT NULL, error TEXT NOT NULL, PRIMARY KEY(job_id, path), FOREIGN KEY(job_id) REFERENCES jobs(id) ON DELETE CASCADE)`,
		`CREATE TABLE IF NOT EXISTS scan_runs (id INTEGER PRIMARY KEY AUTOINCREMENT, generation INTEGER NOT NULL, state TEXT NOT NULL, completed_bytes INTEGER NOT NULL, error TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
	}
	for _, statement := range statements {
		if _, err := store.db.Exec(statement); err != nil {
			return err
		}
	}
	if err := store.migrateJobCommitColumn(); err != nil {
		return err
	}
	if err := store.ensureColumn("jobs", "snapshot", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := store.ensureColumn("jobs", "tree_sha256", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	return store.ensureColumn("job_files", "expected_git_oid", "TEXT NOT NULL DEFAULT ''")
}

func (store *Store) ensureColumn(table string, name string, definition string) error {
	rows, err := store.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var sequence int
		var columnName, columnType string
		var required, primaryKey int
		var defaultValue any
		if err := rows.Scan(&sequence, &columnName, &columnType, &required, &defaultValue, &primaryKey); err != nil {
			_ = rows.Close()
			return err
		}
		found = found || columnName == name
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if found {
		return nil
	}
	_, err = store.db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + name + ` ` + definition)
	return err
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

func (store *Store) SaveArtifact(record ArtifactRecord) (ArtifactRecord, error) {
	if record.Path == "" || !validSHA256(record.SHA256) || record.Size < 0 {
		return ArtifactRecord{}, fmt.Errorf("artifact record is invalid")
	}
	now := time.Now().UTC()
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	record.UpdatedAt = now
	var created, updated string
	err := store.db.QueryRow(`INSERT INTO artifacts(path, sha256, size, modified_unix_nano, repository, repository_path, revision, verification_source, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(path) DO UPDATE SET sha256=excluded.sha256, size=excluded.size, modified_unix_nano=excluded.modified_unix_nano, repository=excluded.repository, repository_path=excluded.repository_path, revision=excluded.revision, verification_source=excluded.verification_source, updated_at=excluded.updated_at RETURNING created_at, updated_at`, record.Path, record.SHA256, record.Size, record.ModifiedUnixNano, record.Repository, record.RepositoryPath, record.Revision, record.VerificationSource, record.CreatedAt.Format(time.RFC3339Nano), record.UpdatedAt.Format(time.RFC3339Nano)).Scan(&created, &updated)
	if err != nil {
		return ArtifactRecord{}, err
	}
	record.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	record.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return record, nil
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

func artifactFromFile(path string, hash string, repository string, repositoryPath string, revision string, verificationSource string) (ArtifactRecord, error) {
	info, err := os.Stat(path)
	if err != nil {
		return ArtifactRecord{}, err
	}
	return ArtifactRecord{Path: path, SHA256: hash, Size: info.Size(), ModifiedUnixNano: info.ModTime().UnixNano(), Repository: repository, RepositoryPath: repositoryPath, Revision: revision, VerificationSource: verificationSource}, nil
}
