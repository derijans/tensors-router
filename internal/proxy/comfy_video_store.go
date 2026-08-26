package proxy

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Finished ComfyUI video jobs are held on disk rather than in memory: a single
// generation can run to hundreds of megabytes, and the store keeps completed
// jobs for a full day so a client can come back for them. Holding those bytes
// resident would let a handful of concurrent jobs exhaust the router process.
//
// The directory is operator-controlled through ffmpeg.scratch_dir precisely
// because container deployments run read-only with a small /tmp; pointing it
// at the data volume is what makes the size caps below meaningful rather than
// an immediate tmpfs failure.
const (
	comfyVideoJobLifetime    = 24 * time.Hour
	maxComfyVideoJobs        = 256
	maxComfyVideoBytes       = 2 << 30
	maxComfyVideoStoreBytes  = 8 << 30
	maxComfyVideoUploadBytes = 32 << 20
)

var errComfyVideoTooLarge = errors.New("generated video exceeded the router's size cap")

type comfyVideoStatus string

const (
	comfyVideoQueued    comfyVideoStatus = "queued"
	comfyVideoRunning   comfyVideoStatus = "running"
	comfyVideoCompleted comfyVideoStatus = "completed"
	comfyVideoFailed    comfyVideoStatus = "failed"
)

type comfyVideoJob struct {
	mu       sync.Mutex
	id       string
	filename string
	path     string
	status   comfyVideoStatus
	errMsg   string
	size     int64
}

type comfyVideoSnapshot struct {
	status   comfyVideoStatus
	errMsg   string
	filename string
	path     string
	size     int64
}

func (job *comfyVideoJob) snapshot() comfyVideoSnapshot {
	job.mu.Lock()
	defer job.mu.Unlock()
	return comfyVideoSnapshot{status: job.status, errMsg: job.errMsg, filename: job.filename, path: job.path, size: job.size}
}

func (job *comfyVideoJob) setRunning() {
	job.mu.Lock()
	defer job.mu.Unlock()
	job.status = comfyVideoRunning
}

func (job *comfyVideoJob) fail(message string) {
	job.mu.Lock()
	defer job.mu.Unlock()
	job.status = comfyVideoFailed
	job.errMsg = message
}

func (job *comfyVideoJob) complete(size int64) {
	job.mu.Lock()
	defer job.mu.Unlock()
	job.status = comfyVideoCompleted
	job.size = size
}

type comfyVideoEntry struct {
	job       *comfyVideoJob
	expiresAt time.Time
}

type comfyVideoUpload struct {
	path      string
	expiresAt time.Time
}

type comfyVideoJobStore struct {
	mu             sync.Mutex
	dir            string
	jobs           map[string]*comfyVideoEntry
	uploads        map[string]comfyVideoUpload
	pendingRemoval []string
	maxVideoBytes  int64
	now            func() time.Time
}

func newComfyVideoJobStore(scratchDir string) *comfyVideoJobStore {
	dir := strings.TrimSpace(scratchDir)
	if dir == "" {
		dir = os.TempDir()
	}
	return &comfyVideoJobStore{
		dir:           filepath.Join(dir, "tensors-router-comfy-video"),
		jobs:          map[string]*comfyVideoEntry{},
		uploads:       map[string]comfyVideoUpload{},
		maxVideoBytes: maxComfyVideoBytes,
		now:           time.Now,
	}
}

// create mints a job id and reserves its output file path. The file itself is
// only written once generation finishes, so a queued or failed job costs no
// disk.
func (store *comfyVideoJobStore) create() (string, *comfyVideoJob, error) {
	id := newComfyVideoID()
	store.mu.Lock()
	defer store.mu.Unlock()
	if err := os.MkdirAll(store.dir, 0o700); err != nil {
		return "", nil, fmt.Errorf("video scratch directory %q is not usable: %w", store.dir, err)
	}
	store.sweepLocked()
	if len(store.jobs) >= maxComfyVideoJobs {
		store.evictOldestLocked()
	}
	job := &comfyVideoJob{
		id:       id,
		filename: id + ".mp4",
		path:     filepath.Join(store.dir, id+".mp4"),
		status:   comfyVideoQueued,
	}
	store.jobs[id] = &comfyVideoEntry{job: job, expiresAt: store.now().Add(comfyVideoJobLifetime)}
	return id, job, nil
}

func (store *comfyVideoJobStore) videoCap() int64 {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.maxVideoBytes > 0 {
		return store.maxVideoBytes
	}
	return maxComfyVideoBytes
}

func (store *comfyVideoJobStore) get(id string) (*comfyVideoJob, bool) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.sweepLocked()
	entry, ok := store.jobs[id]
	if !ok {
		return nil, false
	}
	return entry.job, true
}

func (store *comfyVideoJobStore) jobForFilename(filename string) (*comfyVideoJob, bool) {
	id := strings.TrimSuffix(filename, ".mp4")
	if id == filename {
		return nil, false
	}
	job, ok := store.get(id)
	if !ok || job.snapshot().filename != filename {
		return nil, false
	}
	return job, true
}

// writeVideo streams produce's output straight to the job's file, capped so a
// runaway generation cannot fill the volume. The file is removed if produce
// fails, so a failed job never leaves bytes behind.
func (store *comfyVideoJobStore) writeVideo(job *comfyVideoJob, produce func(io.Writer) error) (int64, error) {
	file, err := os.OpenFile(job.path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, fmt.Errorf("could not open video output file: %w", err)
	}
	writer := &cappedWriter{target: file, remaining: store.videoCap(), overflow: errComfyVideoTooLarge}
	produceErr := produce(writer)
	closeErr := file.Close()
	if produceErr == nil && closeErr != nil {
		produceErr = closeErr
	}
	if produceErr != nil {
		_ = os.Remove(job.path)
		return 0, produceErr
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.enforceStoreBudgetLocked(job.id)
	return writer.written, nil
}

// rememberUpload keeps a copy of an uploaded reference image under the name
// the backend assigned it, so a later video workflow naming that image can be
// satisfied locally. The stored path is derived from a hash of the name
// rather than the name itself, so a hostile name cannot escape the scratch
// directory.
func (store *comfyVideoJobStore) rememberUpload(name string, content []byte) error {
	name = strings.TrimSpace(name)
	if name == "" || len(content) == 0 {
		return fmt.Errorf("upload name and content are required")
	}
	if len(content) > maxComfyVideoUploadBytes {
		return fmt.Errorf("uploaded image exceeded the router's %d byte size cap", int64(maxComfyVideoUploadBytes))
	}
	store.mu.Lock()
	if err := os.MkdirAll(store.dir, 0o700); err != nil {
		store.mu.Unlock()
		return fmt.Errorf("video scratch directory %q is not usable: %w", store.dir, err)
	}
	store.sweepLocked()
	path := filepath.Join(store.dir, "upload-"+uploadPathKey(name))
	store.uploads[name] = comfyVideoUpload{path: path, expiresAt: store.now().Add(comfyVideoJobLifetime)}
	store.mu.Unlock()

	if err := os.WriteFile(path, content, 0o600); err != nil {
		store.mu.Lock()
		delete(store.uploads, name)
		store.mu.Unlock()
		return fmt.Errorf("could not write upload copy: %w", err)
	}
	return nil
}

func (store *comfyVideoJobStore) uploadBytes(name string) ([]byte, bool) {
	store.mu.Lock()
	upload, ok := store.uploads[strings.TrimSpace(name)]
	store.mu.Unlock()
	if !ok {
		return nil, false
	}
	data, err := os.ReadFile(upload.path)
	if err != nil {
		return nil, false
	}
	return data, true
}

func uploadPathKey(name string) string {
	sum := sha256.Sum256([]byte(name))
	return hex.EncodeToString(sum[:16])
}

func (store *comfyVideoJobStore) sweepLocked() {
	now := store.now()
	for id, entry := range store.jobs {
		if !now.Before(entry.expiresAt) {
			store.removeJobLocked(id, entry)
		}
	}
	for name, upload := range store.uploads {
		if !now.Before(upload.expiresAt) {
			store.queueRemovalLocked(upload.path)
			delete(store.uploads, name)
		}
	}
	store.retryPendingRemovalLocked()
}

func (store *comfyVideoJobStore) evictOldestLocked() {
	oldestID := ""
	var oldestExpiry time.Time
	for id, entry := range store.jobs {
		if oldestID == "" || entry.expiresAt.Before(oldestExpiry) {
			oldestID = id
			oldestExpiry = entry.expiresAt
		}
	}
	if oldestID != "" {
		store.removeJobLocked(oldestID, store.jobs[oldestID])
	}
}

// enforceStoreBudgetLocked evicts the oldest completed jobs until the stored
// video bytes fit the budget, never evicting the job that just finished.
func (store *comfyVideoJobStore) enforceStoreBudgetLocked(protectedID string) {
	type sized struct {
		id        string
		expiresAt time.Time
		size      int64
	}
	entries := make([]sized, 0, len(store.jobs))
	total := int64(0)
	for id, entry := range store.jobs {
		size := entry.job.snapshot().size
		total += size
		entries = append(entries, sized{id: id, expiresAt: entry.expiresAt, size: size})
	}
	if total <= maxComfyVideoStoreBytes {
		return
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].expiresAt.Before(entries[right].expiresAt) })
	for _, candidate := range entries {
		if total <= maxComfyVideoStoreBytes {
			return
		}
		if candidate.id == protectedID || candidate.size == 0 {
			continue
		}
		store.removeJobLocked(candidate.id, store.jobs[candidate.id])
		total -= candidate.size
	}
}

func (store *comfyVideoJobStore) removeJobLocked(id string, entry *comfyVideoEntry) {
	if entry == nil {
		delete(store.jobs, id)
		return
	}
	store.queueRemovalLocked(entry.job.path)
	delete(store.jobs, id)
}

// queueRemovalLocked deletes a file, retrying later when it cannot be removed
// now. Windows refuses to unlink a file a concurrent /view is still reading,
// so a failed delete must not leak the path.
func (store *comfyVideoJobStore) queueRemovalLocked(path string) {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		store.pendingRemoval = append(store.pendingRemoval, path)
	}
}

func (store *comfyVideoJobStore) retryPendingRemovalLocked() {
	if len(store.pendingRemoval) == 0 {
		return
	}
	remaining := store.pendingRemoval[:0]
	for _, path := range store.pendingRemoval {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			remaining = append(remaining, path)
		}
	}
	store.pendingRemoval = remaining
}

type cappedWriter struct {
	target    io.Writer
	remaining int64
	written   int64
	overflow  error
}

func (writer *cappedWriter) Write(payload []byte) (int, error) {
	if int64(len(payload)) > writer.remaining {
		return 0, writer.overflow
	}
	written, err := writer.target.Write(payload)
	writer.remaining -= int64(written)
	writer.written += int64(written)
	return written, err
}

func newComfyVideoID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return hex.EncodeToString([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(buf)
}
