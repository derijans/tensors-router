package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	routeranalytics "tensors-router/internal/analytics"
	"tensors-router/internal/catalog"
	"tensors-router/internal/comfyvideo"
	"tensors-router/internal/openai"
)

// ComfyUI video support intercepts only the subset of the ComfyUI HTTP API
// a video-producing /prompt call touches: submission, status polling, the
// finished-file download, and reference-media upload. A workflow that is not
// video-shaped falls straight through to the existing KoboldCpp ComfyUI image
// emulation unchanged, so nothing here can regress the working image path.
//
// Neither backend answers a video generation request with an MP4 that a
// generic ComfyUI client checks for: KoboldCpp emits GIF or MJPG-AVI,
// stable-diffusion.cpp emits WebM, animated WebP, or MJPG-AVI. Every
// finished job is transcoded with ffmpeg before it is ever served, so
// ffmpeg is a hard requirement for this feature specifically (config.FFmpeg
// missing fails the request explicitly, it never produces an unplayable
// download).
//
// Model selection deliberately does not parse a checkpoint out of the
// workflow graph: like KoboldCpp's own ComfyUI emulation, a request always
// targets whatever image-lane model the router currently has active. The job
// runs on the node that accepted it and its output lives on that node, so
// /history and /view for a router-minted id are answered only by that node
// and never forwarded across the cluster.

const (
	comfyVideoGenerationTimeout      = 30 * time.Minute
	comfyVideoResponseBodyLimit      = 64 << 10
	comfyVideoResultResponseCap      = 512 << 20
	comfyVideoOutputNodeIDForHistory = "1"
)

// handleComfyPrompt answers POST /prompt. A workflow that is not video-shaped
// is handed to the ordinary image-request path unchanged.
func (service *Service) handleComfyPrompt(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		service.logger.Printf("comfyui prompt body read failed remote=%s error=%v", r.RemoteAddr, err)
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "request body could not be read")
		return
	}
	_ = r.Body.Close()

	graph, parseErr := comfyvideo.DecodeWorkflow(body)
	if parseErr != nil || !comfyvideo.IsVideoWorkflow(graph) {
		r.Body = io.NopCloser(bytes.NewReader(body))
		service.handleImageRequest(w, r)
		return
	}

	if !service.ffmpeg.Available() {
		openai.WriteError(w, http.StatusServiceUnavailable, "backend_error", "ComfyUI video generation requires ffmpeg, which is not available on this router")
		return
	}

	model, err := service.activeImageModel(r)
	if err != nil {
		writeImageModelError(service, w, r, "", err)
		return
	}
	modelBackendMode, err := service.catalogModelBackendMode(model)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	if modelBackendMode != BackendModeKobold && modelBackendMode != BackendModeLlamaSDCPP {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", fmt.Sprintf("backend mode %q does not support ComfyUI video generation", modelBackendMode))
		return
	}

	requestBody, err := service.comfyVideoBackendBody(graph, modelBackendMode)
	if err != nil {
		writeComfyVideoRequestError(w, err)
		return
	}

	promptID, job, err := service.comfyVideoJobs.create()
	if err != nil {
		service.logger.Printf("comfyui video job could not be created error=%v", err)
		openai.WriteError(w, http.StatusInternalServerError, "backend_error", err.Error())
		return
	}
	go service.runComfyVideoJob(job, model, modelBackendMode, requestBody)

	openai.WriteJSON(w, http.StatusOK, map[string]any{
		"prompt_id":   promptID,
		"number":      0,
		"node_errors": map[string]any{},
	})
}

func (service *Service) comfyVideoBackendBody(graph comfyvideo.Graph, backendMode string) ([]byte, error) {
	params, err := comfyvideo.ParseWorkflow(graph)
	if err != nil {
		return nil, err
	}
	media, err := service.comfyVideoJobs.resolveWorkflowMedia(params)
	if err != nil {
		return nil, err
	}
	if backendMode == BackendModeKobold {
		return buildKoboldVideoRequest(params, media)
	}
	return buildSDCPPVideoRequest(params, media)
}

// handleComfyHistory answers GET /history/{prompt_id} for a router-minted
// job. It reports false for any id it does not own so the caller can fall
// through to KoboldCpp's own aggregate /history.
func (service *Service) handleComfyHistory(w http.ResponseWriter, r *http.Request) bool {
	const prefix = "/history/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		return false
	}
	promptID := strings.TrimPrefix(r.URL.Path, prefix)
	if slash := strings.Index(promptID, "/"); slash >= 0 {
		promptID = promptID[:slash]
	}
	job, ok := service.comfyVideoJobs.get(promptID)
	if !ok {
		return false
	}
	snapshot := job.snapshot()
	entry := map[string]any{}
	switch snapshot.status {
	case comfyVideoCompleted:
		entry["status"] = map[string]any{"completed": true, "status_str": "success"}
		entry["outputs"] = map[string]any{
			comfyVideoOutputNodeIDForHistory: map[string]any{
				"gifs": []map[string]any{{
					"filename":  snapshot.filename,
					"subfolder": "",
					"type":      "output",
				}},
			},
		}
	case comfyVideoFailed:
		entry["status"] = map[string]any{"completed": false, "status_str": "error", "messages": snapshot.errMsg}
	default:
		entry["status"] = map[string]any{"completed": false, "status_str": string(snapshot.status)}
	}
	openai.WriteJSON(w, http.StatusOK, map[string]any{promptID: entry})
	return true
}

// handleComfyView answers GET /view?filename=...&subfolder=...&type=... for
// a router-minted job's output file. It reports false for any filename it
// does not own so the caller can fall through to KoboldCpp's own /view.
func (service *Service) handleComfyView(w http.ResponseWriter, r *http.Request) bool {
	filename := strings.TrimSpace(r.URL.Query().Get("filename"))
	if filename == "" {
		return false
	}
	job, ok := service.comfyVideoJobs.jobForFilename(filename)
	if !ok {
		return false
	}
	snapshot := job.snapshot()
	if snapshot.status != comfyVideoCompleted {
		http.NotFound(w, r)
		return true
	}
	file, err := os.Open(snapshot.path)
	if err != nil {
		service.logger.Printf("comfyui video file unreadable filename=%s error=%v", filename, err)
		http.NotFound(w, r)
		return true
	}
	defer file.Close()
	w.Header().Set("Content-Type", "video/mp4")
	// ServeContent answers Range requests, which is what a video player uses
	// to seek without downloading the whole file.
	http.ServeContent(w, r, snapshot.filename, time.Time{}, file)
	return true
}

func (service *Service) runComfyVideoJob(job *comfyVideoJob, model catalog.Model, backendMode string, requestBody []byte) {
	job.setRunning()
	ctx, cancel := context.WithTimeout(context.Background(), comfyVideoGenerationTimeout)
	defer cancel()

	raw, err := service.generateBackendVideo(ctx, model, backendMode, requestBody)
	if err != nil {
		service.logger.Printf("comfyui video generation failed model=%s backend=%s error=%v", model.ImageID, backendMode, err)
		job.fail(err.Error())
		return
	}

	size, err := service.comfyVideoJobs.writeVideo(job, func(target io.Writer) error {
		return service.ffmpeg.RemuxToMP4(ctx, bytes.NewReader(raw), target)
	})
	if err != nil {
		service.logger.Printf("comfyui video remux failed model=%s error=%v", model.ImageID, err)
		job.fail(fmt.Sprintf("ffmpeg remux failed: %v", err))
		return
	}
	job.complete(size)
}

func (service *Service) generateBackendVideo(ctx context.Context, model catalog.Model, backendMode string, requestBody []byte) ([]byte, error) {
	switch backendMode {
	case BackendModeLlamaSDCPP:
		return service.generateSDCPPVideo(ctx, model, requestBody)
	case BackendModeKobold:
		return service.generateKoboldVideo(ctx, model, requestBody)
	default:
		return nil, fmt.Errorf("backend mode %q does not support ComfyUI video generation", backendMode)
	}
}

func syntheticImageRequest(method string, path string) *http.Request {
	return &http.Request{
		Method: method,
		URL:    &url.URL{Path: path},
		Header: make(http.Header),
	}
}

func finishVRAMWork(finalizer routeranalytics.EventFinalizer) {
	if finalizer == nil {
		return
	}
	finalizer(&routeranalytics.Event{})
}

func readComfyJSONResponse(response *http.Response) (map[string]any, error) {
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, comfyVideoResponseBodyLimit))
		return nil, fmt.Errorf("backend returned status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, comfyVideoResultResponseCap))
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("invalid JSON from backend: %w", err)
	}
	return payload, nil
}
