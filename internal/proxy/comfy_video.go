package proxy

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
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
// finished-file download, and reference-image upload. A workflow that is not
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
	comfyVideoPollInterval           = 1 * time.Second
	comfyVideoResponseBodyLimit      = 64 << 10
	comfyVideoResultResponseCap      = 512 << 20
	comfyVideoOutputNodeIDForHistory = "1"
	maxComfyUploadRequestBytes       = maxComfyVideoUploadBytes + (1 << 20)
)

type comfyVideoRequest struct {
	params         comfyvideo.Params
	referenceImage []byte
}

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

	params := comfyvideo.ParseWorkflow(graph)
	request := comfyVideoRequest{params: params}
	if params.ReferenceImage != "" {
		if modelBackendMode != BackendModeKobold {
			openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "image-to-video from an uploaded reference image is only supported by the kobold backend; use the native /sdcpp/v1/vid_gen route on the split backend")
			return
		}
		reference, ok := service.comfyVideoJobs.uploadBytes(params.ReferenceImage)
		if !ok {
			openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", fmt.Sprintf("workflow references image %q, which was not uploaded to this router", params.ReferenceImage))
			return
		}
		request.referenceImage = reference
	}

	promptID, job, err := service.comfyVideoJobs.create()
	if err != nil {
		service.logger.Printf("comfyui video job could not be created error=%v", err)
		openai.WriteError(w, http.StatusInternalServerError, "backend_error", err.Error())
		return
	}
	go service.runComfyVideoJob(job, model, modelBackendMode, request)

	openai.WriteJSON(w, http.StatusOK, map[string]any{
		"prompt_id":   promptID,
		"number":      0,
		"node_errors": map[string]any{},
	})
}

// handleComfyUploadImage tees POST /upload/image: the upload still reaches the
// backend exactly as before, so KoboldCpp keeps serving image workflows that
// reference it, and the router additionally keeps its own copy keyed by the
// name the backend handed back. A later video workflow naming that image can
// then be satisfied locally, because the router generates video itself and
// never forwards the workflow.
//
// It reports false whenever it cannot tee cleanly, leaving the request to the
// ordinary image path untouched.
func (service *Service) handleComfyUploadImage(w http.ResponseWriter, r *http.Request) bool {
	if !service.ffmpeg.Available() || r.ContentLength > maxComfyUploadRequestBytes {
		return false
	}
	contentType := r.Header.Get("Content-Type")
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(contentType)), "multipart/form-data") {
		return false
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxComfyUploadRequestBytes+1))
	if err != nil || len(body) > maxComfyUploadRequestBytes {
		// Hand the untouched stream back rather than the prefix already read,
		// so an upload too large to copy still reaches the backend whole.
		r.Body = struct {
			io.Reader
			io.Closer
		}{Reader: io.MultiReader(bytes.NewReader(body), r.Body), Closer: r.Body}
		return false
	}
	_ = r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(body))

	image, filename, ok := comfyUploadImagePart(body, contentType)
	if !ok {
		return false
	}

	model, err := service.activeImageModel(r)
	if err != nil {
		writeImageModelError(service, w, r, "", err)
		return true
	}
	modelBackendMode, err := service.catalogModelBackendMode(model)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return true
	}
	response, finalizer, err := service.forwardWithFallbackObserved(r.Context(), r, body, model.ImageID, model.Filename, true, readinessImage, modelBackendMode)
	finishVRAMWork(finalizer)
	if err != nil {
		writeBackendFailure(w, err)
		return true
	}
	defer response.Body.Close()
	payload, err := readComfyJSONResponse(response)
	if err != nil {
		openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
		return true
	}
	storedName, _ := payload["name"].(string)
	if strings.TrimSpace(storedName) == "" {
		storedName = filename
	}
	if err := service.comfyVideoJobs.rememberUpload(storedName, image); err != nil {
		service.logger.Printf("comfyui upload copy failed name=%s error=%v", storedName, err)
	}
	openai.WriteJSON(w, http.StatusOK, payload)
	return true
}

// comfyUploadImagePart extracts the "image" file part a ComfyUI upload
// carries, without disturbing the bytes that are forwarded to the backend.
func comfyUploadImagePart(body []byte, contentType string) ([]byte, string, bool) {
	_, parameters, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, "", false
	}
	boundary := parameters["boundary"]
	if boundary == "" {
		return nil, "", false
	}
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	for {
		part, err := reader.NextPart()
		if err != nil {
			return nil, "", false
		}
		if part.FormName() != "image" {
			_ = part.Close()
			continue
		}
		content, err := io.ReadAll(io.LimitReader(part, maxComfyVideoUploadBytes))
		name := part.FileName()
		_ = part.Close()
		if err != nil || len(content) == 0 {
			return nil, "", false
		}
		return content, name, true
	}
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

func (service *Service) runComfyVideoJob(job *comfyVideoJob, model catalog.Model, backendMode string, request comfyVideoRequest) {
	job.setRunning()
	ctx, cancel := context.WithTimeout(context.Background(), comfyVideoGenerationTimeout)
	defer cancel()

	raw, err := service.generateBackendVideo(ctx, model, backendMode, request)
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

func (service *Service) generateBackendVideo(ctx context.Context, model catalog.Model, backendMode string, request comfyVideoRequest) ([]byte, error) {
	switch backendMode {
	case BackendModeLlamaSDCPP:
		return service.generateSDCPPVideo(ctx, model, request.params)
	case BackendModeKobold:
		return service.generateKoboldVideo(ctx, model, request)
	default:
		return nil, fmt.Errorf("backend mode %q does not support ComfyUI video generation", backendMode)
	}
}

// generateKoboldVideo issues a single synchronous generation call with
// KoboldCpp's video fields. video_output_type=1 selects the MJPG-AVI encoder,
// which is the only one of KoboldCpp's two video containers that carries a
// MiniMax-H3 audio track. A reference image switches the call to the A1111
// img2img route KoboldCpp implements, which is how image-to-video is driven.
func (service *Service) generateKoboldVideo(ctx context.Context, model catalog.Model, request comfyVideoRequest) ([]byte, error) {
	path := "/sdapi/v1/txt2img"
	if len(request.referenceImage) > 0 {
		path = "/sdapi/v1/img2img"
	}
	requestBody := buildKoboldVideoRequest(request)
	response, finalizer, err := service.forwardWithFallbackObserved(ctx, syntheticImageRequest(http.MethodPost, path), requestBody, model.ImageID, model.Filename, true, readinessImage, BackendModeKobold)
	finishVRAMWork(finalizer)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	payload, err := readComfyJSONResponse(response)
	if err != nil {
		return nil, err
	}
	extraData, _ := payload["extra_data"].(string)
	if extraData == "" {
		return nil, fmt.Errorf("koboldcpp did not return video data; is a video-capable model (for example MiniMax-H3) loaded?")
	}
	return base64.StdEncoding.DecodeString(extraData)
}

// generateSDCPPVideo submits an sd-server /sdcpp/v1/vid_gen job and polls it
// to completion. sample_params is intentionally omitted: the server API
// documents it as a nested sampler override but does not fix its field
// names, so guessing them risks the whole request being rejected. Only the
// fields the API reference confirms are sent; sampler/scheduler/steps/cfg
// fall back to the loaded config's defaults for this path.
func (service *Service) generateSDCPPVideo(ctx context.Context, model catalog.Model, params comfyvideo.Params) ([]byte, error) {
	requestBody := buildSDCPPVideoRequest(params)
	submitResponse, finalizer, err := service.forwardWithFallbackObserved(ctx, syntheticImageRequest(http.MethodPost, "/sdcpp/v1/vid_gen"), requestBody, model.ImageID, model.Filename, true, readinessImage, BackendModeLlamaSDCPP)
	finishVRAMWork(finalizer)
	if err != nil {
		return nil, err
	}
	submitted, err := readComfyJSONResponse(submitResponse)
	_ = submitResponse.Body.Close()
	if err != nil {
		return nil, err
	}
	jobID, _ := submitted["id"].(string)
	if jobID == "" {
		return nil, fmt.Errorf("sd-server did not return a job id for vid_gen")
	}

	ticker := time.NewTicker(comfyVideoPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
		pollResponse, pollFinalizer, err := service.forwardWithFallbackObserved(ctx, syntheticImageRequest(http.MethodGet, "/sdcpp/v1/jobs/"+jobID), nil, model.ImageID, model.Filename, true, readinessImage, BackendModeLlamaSDCPP)
		finishVRAMWork(pollFinalizer)
		if err != nil {
			return nil, err
		}
		status, err := readComfyJSONResponse(pollResponse)
		_ = pollResponse.Body.Close()
		if err != nil {
			return nil, err
		}
		switch fmt.Sprint(status["status"]) {
		case "completed":
			result, _ := status["result"].(map[string]any)
			b64, _ := result["b64_json"].(string)
			if b64 == "" {
				return nil, fmt.Errorf("sd-server vid_gen completed without a b64_json result")
			}
			return base64.StdEncoding.DecodeString(b64)
		case "failed", "cancelled":
			errMessage, _ := status["error"].(string)
			return nil, fmt.Errorf("sd-server vid_gen job %s: %s", fmt.Sprint(status["status"]), errMessage)
		}
	}
}

func buildKoboldVideoRequest(request comfyVideoRequest) []byte {
	params := request.params
	body := map[string]any{
		"prompt":            params.Prompt,
		"negative_prompt":   params.NegativePrompt,
		"width":             params.Width,
		"height":            params.Height,
		"steps":             params.Steps,
		"cfg_scale":         params.CFG,
		"seed":              params.Seed,
		"frames":            params.Frames,
		"fps":               params.FPS,
		"video_output_type": 1,
	}
	if params.SamplerName != "" {
		body["sampler_name"] = params.SamplerName
	}
	if len(request.referenceImage) > 0 {
		body["init_images"] = []string{base64.StdEncoding.EncodeToString(request.referenceImage)}
	}
	encoded, _ := json.Marshal(body)
	return encoded
}

func buildSDCPPVideoRequest(params comfyvideo.Params) []byte {
	body := map[string]any{
		"prompt":          params.Prompt,
		"negative_prompt": params.NegativePrompt,
		"width":           params.Width,
		"height":          params.Height,
		"video_frames":    params.Frames,
		"fps":             params.FPS,
		"seed":            params.Seed,
		"output_format":   "avi",
	}
	encoded, _ := json.Marshal(body)
	return encoded
}

func syntheticImageRequest(method string, path string) *http.Request {
	return &http.Request{
		Method: method,
		URL:    &url.URL{Path: path},
		Header: make(http.Header),
	}
}

func finishVRAMWork(finalizer analyticsEventFinalizer) {
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
