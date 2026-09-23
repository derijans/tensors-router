package downloads

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"tensors-router/internal/downloader"
	"tensors-router/internal/openai"
)

func (handlers *Handlers) SiteEvents(w http.ResponseWriter, r *http.Request) {
	if !handlers.deps.SiteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	jobID, ok := downloadJobID(r.URL.Path, "/events", "")
	if !ok {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	if remoteURL, remote, err := handlers.downloadTarget(r.URL.Query().Get("node_id")); err != nil {
		writeDownloadError(w, err)
		return
	} else if remote {
		handlers.streamRemoteDownloadEvents(w, r, remoteURL, jobID)
		return
	}
	handlers.writeDownloadEvents(w, r, jobID)
}

func (handlers *Handlers) NodeEvents(w http.ResponseWriter, r *http.Request) {
	jobID, ok := downloadJobID(r.URL.Path, "/events", "")
	if !ok {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	handlers.writeDownloadEvents(w, r, jobID)
}

func (handlers *Handlers) streamRemoteDownloadEvents(w http.ResponseWriter, r *http.Request, nodeURL string, jobID string) {
	response, err := handlers.deps.ClusterClient().Stream(r.Context(), http.MethodGet, nodeURL, "/router/v1/node/site/download/jobs/"+jobID+"/events")
	if err != nil {
		writeDownloadError(w, err)
		return
	}
	defer response.Body.Close()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if response.Header.Get("Content-Encoding") != "" {
		w.Header().Set("Content-Encoding", response.Header.Get("Content-Encoding"))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(flushingWriter{ResponseWriter: w}, response.Body)
}

type flushingWriter struct {
	http.ResponseWriter
}

func (writer flushingWriter) Write(content []byte) (int, error) {
	written, err := writer.ResponseWriter.Write(content)
	if flusher, ok := writer.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
	return written, err
}

func (handlers *Handlers) writeDownloadEvents(w http.ResponseWriter, r *http.Request, jobID string) {
	if handlers.downloader == nil {
		writeDownloadUnavailable(w, handlers.capability)
		return
	}
	if _, found, err := handlers.downloader.Job(jobID); err != nil {
		writeDownloadError(w, err)
		return
	} else if !found {
		openai.WriteError(w, http.StatusNotFound, "not_found", "download job was not found")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		openai.WriteError(w, http.StatusInternalServerError, "download_error", "streaming is unavailable")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	events, unsubscribe := handlers.downloader.Subscribe(jobID)
	defer unsubscribe()
	for {
		select {
		case <-r.Context().Done():
			return
		case job := <-events:
			content, err := json.Marshal(job)
			if err != nil {
				return
			}
			_, _ = fmt.Fprintf(w, "event: progress\ndata: %s\n\n", content)
			flusher.Flush()
			if job.State == downloader.JobCompleted || job.State == downloader.JobFailed || job.State == downloader.JobCancelled {
				return
			}
		}
	}
}
