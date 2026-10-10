package proxy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"tensors-router/internal/openai"
)

const (
	nodeStoreDownloadPath  = "/router/v1/node/store/download"
	storeDownloadMediaType = "application/vnd.sqlite3"
)

type storeSnapshotter interface {
	Snapshot(ctx context.Context) (string, func(), error)
}

type storeDownload struct {
	snapshotter storeSnapshotter
	flush       func(ctx context.Context) error
	nodeID      string
}

func (download *storeDownload) serve(w http.ResponseWriter, r *http.Request) {
	if download == nil || download.snapshotter == nil {
		openai.WriteError(w, http.StatusNotFound, "not_found", "router database is not available on this node")
		return
	}
	if err := download.flush(r.Context()); err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	path, cleanup, err := download.snapshotter.Snapshot(r.Context())
	if err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	defer cleanup()
	file, err := os.Open(path)
	if err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	w.Header().Set(headerContentType, storeDownloadMediaType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", download.filename(time.Now())))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, "", info.ModTime(), file)
}

func (download *storeDownload) filename(now time.Time) string {
	return fmt.Sprintf("analytics-%s-%s.sqlite", safeFilenamePart(download.nodeID), now.UTC().Format("20060102T150405Z"))
}

func safeFilenamePart(value string) string {
	var cleaned strings.Builder
	for _, character := range value {
		switch {
		case character >= 'a' && character <= 'z', character >= 'A' && character <= 'Z', character >= '0' && character <= '9', character == '-', character == '_', character == '.':
			cleaned.WriteRune(character)
		default:
			cleaned.WriteRune('_')
		}
	}
	if cleaned.Len() == 0 {
		return "node"
	}
	return cleaned.String()
}

func (service *Service) handleNodeStoreDownload(w http.ResponseWriter, r *http.Request) {
	service.storeDownload.serve(w, r)
}

func (service *Service) handleSiteStoreDownload(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteEndpointNotFound(w)
		return
	}
	nodeID := strings.TrimSpace(r.URL.Query().Get("node_id"))
	if nodeID == "" {
		nodeID = service.nodeID
	}
	nodes, err := service.selectedLoadCaptureNodes([]string{nodeID})
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	if nodes[0].NodeID == service.nodeID {
		service.storeDownload.serve(w, r)
		return
	}
	response, err := service.clusterClient.Stream(r.Context(), http.MethodGet, nodes[0].URL, nodeStoreDownloadPath)
	if err != nil {
		openai.WriteError(w, http.StatusBadGateway, "node_error", err.Error())
		return
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		openai.WriteError(w, http.StatusBadGateway, "node_error", fmt.Sprintf("node %s answered %d", nodes[0].NodeID, response.StatusCode))
		return
	}
	relayDownloadHeaders(w.Header(), response.Header)
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, response.Body)
}

func relayDownloadHeaders(destination http.Header, source http.Header) {
	for _, name := range []string{headerContentType, "Content-Disposition", "Content-Length", "X-Content-Type-Options"} {
		if value := source.Get(name); value != "" {
			destination.Set(name, value)
		}
	}
}
