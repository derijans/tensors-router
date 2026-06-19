package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
)

type sdcppJobTarget struct {
	publicImageID  string
	configFilename string
	backendMode    string
}

type sdcppJobStore struct {
	mu   sync.Mutex
	jobs map[string]sdcppJobTarget
}

func newSdcppJobStore() *sdcppJobStore {
	return &sdcppJobStore{jobs: map[string]sdcppJobTarget{}}
}

func (store *sdcppJobStore) routeForPath(path string) (sdcppJobTarget, bool) {
	jobID, ok := sdcppJobIDFromPath(path)
	if !ok {
		return sdcppJobTarget{}, false
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	target, ok := store.jobs[jobID]
	return target, ok
}

func (store *sdcppJobStore) remember(jobID string, target sdcppJobTarget) {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return
	}
	store.mu.Lock()
	store.jobs[jobID] = target
	store.mu.Unlock()
}

func (service *Service) responseWithSdcppJobTracking(response *http.Response, target sdcppJobTarget) *http.Response {
	if response == nil || response.Body == nil || response.StatusCode < 200 || response.StatusCode >= 300 || !isJSONResponse(response.Header) {
		return response
	}
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		response.Body = io.NopCloser(bytes.NewReader(nil))
		response.ContentLength = 0
		return response
	}
	if jobID := sdcppJobIDFromResponse(body); jobID != "" {
		service.sdcppJobs.remember(jobID, target)
	}
	response.Body = io.NopCloser(bytes.NewReader(body))
	response.ContentLength = int64(len(body))
	return response
}

func isSdcppJobSubmissionPath(path string) bool {
	return path == "/sdcpp/v1/img_gen" || path == "/sdcpp/v1/vid_gen"
}

func sdcppJobIDFromPath(path string) (string, bool) {
	const prefix = "/sdcpp/v1/jobs/"
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	jobID := strings.TrimPrefix(path, prefix)
	if slash := strings.Index(jobID, "/"); slash >= 0 {
		jobID = jobID[:slash]
	}
	jobID = strings.TrimSpace(jobID)
	return jobID, jobID != ""
}

func sdcppJobIDFromResponse(body []byte) string {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return ""
	}
	return findSdcppJobID(value)
}

func findSdcppJobID(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range []string{"job_id", "jobId", "task_id", "taskId", "id"} {
			if jobID := jobIDString(typed[key]); jobID != "" {
				return jobID
			}
		}
		for _, item := range typed {
			if jobID := findSdcppJobID(item); jobID != "" {
				return jobID
			}
		}
	case []any:
		for _, item := range typed {
			if jobID := findSdcppJobID(item); jobID != "" {
				return jobID
			}
		}
	}
	return ""
}

func jobIDString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return strings.TrimSpace(typed.String())
	default:
		return ""
	}
}
