package proxy

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"slices"
	"strings"

	"tensors-router/internal/catalog"
	"tensors-router/internal/openai"
)

const (
	maxComfyUploadRequestBytes = maxComfyVideoUploadBytes + (1 << 20)
	comfyUploadFormField       = "image"
	unknownComfyUploadType     = "application/octet-stream"
)

var comfyAudioTypesByExtension = map[string]string{
	".wav":  "audio/wav",
	".mp3":  "audio/mpeg",
	".flac": "audio/flac",
	".ogg":  "audio/ogg",
	".opus": "audio/opus",
	".m4a":  "audio/mp4",
	".aac":  "audio/aac",
}

type comfyUploadPart struct {
	media    comfyUploadedMedia
	filename string
}

// handleComfyUploadImage tees POST /upload/image: the upload still reaches the
// backend exactly as before, so KoboldCpp keeps serving image workflows that
// reference it, and the router additionally keeps its own copy keyed by the
// name the backend handed back and by the uploaded file name. KoboldCpp names
// every upload kcpp_img2img.jpg, so only the uploaded file name tells several
// references apart. sd-server has no ComfyUI upload endpoint, so on that
// backend the router keeps the upload without forwarding it.
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
	body, ok := readComfyUploadBody(r)
	if !ok {
		return false
	}
	upload, ok := comfyUploadMediaPart(body, contentType)
	if !ok {
		return false
	}
	if upload.filename != "" && !isPlainComfyUploadName(upload.filename) {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", fmt.Sprintf("upload filename %q is not a plain file name", upload.filename))
		return true
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
	if modelBackendMode == BackendModeLlamaSDCPP {
		service.keepComfyUploadWithoutBackend(w, upload)
		return true
	}
	service.teeComfyUploadToBackend(w, r, body, model, modelBackendMode, upload)
	return true
}

func readComfyUploadBody(r *http.Request) ([]byte, bool) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxComfyUploadRequestBytes+1))
	if err != nil || len(body) > maxComfyUploadRequestBytes {
		// Hand the untouched stream back rather than the prefix already read,
		// so an upload too large to copy still reaches the backend whole.
		r.Body = struct {
			io.Reader
			io.Closer
		}{Reader: io.MultiReader(bytes.NewReader(body), r.Body), Closer: r.Body}
		return nil, false
	}
	_ = r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(body))
	return body, true
}

func (service *Service) keepComfyUploadWithoutBackend(w http.ResponseWriter, upload comfyUploadPart) {
	if upload.filename == "" {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "upload needs a file name")
		return
	}
	if err := service.comfyVideoJobs.rememberUpload(upload.filename, upload.media); err != nil {
		service.logger.Printf("comfyui upload copy failed name=%s error=%v", upload.filename, err)
		openai.WriteError(w, http.StatusInternalServerError, "backend_error", "router could not keep the upload")
		return
	}
	openai.WriteJSON(w, http.StatusOK, map[string]any{"name": upload.filename, "subfolder": "", "type": "input"})
}

func (service *Service) teeComfyUploadToBackend(w http.ResponseWriter, r *http.Request, body []byte, model catalog.Model, backendMode string, upload comfyUploadPart) {
	response, finalizer, err := service.forwardWithFallbackObserved(r.Context(), r, body, model.ImageID, model.Filename, true, readinessImage, backendMode)
	finishVRAMWork(finalizer)
	if err != nil {
		writeBackendFailure(w, err)
		return
	}
	defer response.Body.Close()
	payload, err := readComfyJSONResponse(response)
	if err != nil {
		openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
		return
	}
	backendName, _ := payload["name"].(string)
	backendName = strings.TrimSpace(backendName)
	for _, name := range distinctNonEmpty(backendName, upload.filename) {
		if err := service.comfyVideoJobs.rememberUpload(name, upload.media); err != nil {
			service.logger.Printf("comfyui upload copy failed name=%s error=%v", name, err)
		}
	}
	openai.WriteJSON(w, http.StatusOK, payload)
}

func distinctNonEmpty(names ...string) []string {
	var distinct []string
	for _, name := range names {
		if name != "" && !slices.Contains(distinct, name) {
			distinct = append(distinct, name)
		}
	}
	return distinct
}

func comfyUploadMediaPart(body []byte, contentType string) (comfyUploadPart, bool) {
	_, parameters, err := mime.ParseMediaType(contentType)
	if err != nil || parameters["boundary"] == "" {
		return comfyUploadPart{}, false
	}
	reader := multipart.NewReader(bytes.NewReader(body), parameters["boundary"])
	for {
		part, err := reader.NextPart()
		if err != nil {
			return comfyUploadPart{}, false
		}
		if part.FormName() != comfyUploadFormField {
			_ = part.Close()
			continue
		}
		content, err := io.ReadAll(io.LimitReader(part, maxComfyVideoUploadBytes+1))
		filename := strings.TrimSpace(part.FileName())
		partType := comfyUploadContentType(part.Header.Get("Content-Type"), filename)
		_ = part.Close()
		if err != nil || len(content) == 0 || len(content) > maxComfyVideoUploadBytes {
			return comfyUploadPart{}, false
		}
		return comfyUploadPart{media: comfyUploadedMedia{content: content, contentType: partType}, filename: filename}, true
	}
}

func comfyUploadContentType(declaredType string, filename string) string {
	if mediaType, _, err := mime.ParseMediaType(declaredType); err == nil && (strings.HasPrefix(mediaType, "audio/") || strings.HasPrefix(mediaType, "image/")) {
		return mediaType
	}
	if audioType, ok := comfyAudioTypesByExtension[strings.ToLower(filepath.Ext(filename))]; ok {
		return audioType
	}
	return unknownComfyUploadType
}
