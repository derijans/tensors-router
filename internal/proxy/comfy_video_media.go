package proxy

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"

	"tensors-router/internal/comfyvideo"
	"tensors-router/internal/openai"
)

type comfyVideoRequestError struct {
	code    string
	message string
}

func (err *comfyVideoRequestError) Error() string {
	return err.message
}

func writeComfyVideoRequestError(w http.ResponseWriter, err error) {
	var requestErr *comfyVideoRequestError
	if errors.As(err, &requestErr) {
		openai.WriteErrorCode(w, http.StatusBadRequest, "invalid_request_error", requestErr.code, requestErr.message)
		return
	}
	openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
}

type comfyVideoMedia struct {
	startFrame      []byte
	endFrame        []byte
	referenceImages [][]byte
	referenceAudios []comfyUploadedMedia
}

type comfyVideoLoRARequest struct {
	Path       string  `json:"path"`
	Multiplier float64 `json:"multiplier"`
}

func comfyVideoLoRARequests(loras []comfyvideo.LoRA) []comfyVideoLoRARequest {
	requests := make([]comfyVideoLoRARequest, 0, len(loras))
	for _, lora := range loras {
		requests = append(requests, comfyVideoLoRARequest{Path: lora.Name, Multiplier: lora.Strength})
	}
	return requests
}

func setBase64Field(body map[string]any, key string, content []byte) {
	if len(content) > 0 {
		body[key] = base64.StdEncoding.EncodeToString(content)
	}
}

func base64Images(images [][]byte) []string {
	encoded := make([]string, 0, len(images))
	for _, image := range images {
		encoded = append(encoded, base64.StdEncoding.EncodeToString(image))
	}
	return encoded
}

func audioDataURL(audio comfyUploadedMedia) string {
	return "data:" + audio.contentType + ";base64," + base64.StdEncoding.EncodeToString(audio.content)
}

func (store *comfyVideoJobStore) resolveWorkflowMedia(params comfyvideo.Params) (comfyVideoMedia, error) {
	budget := store.promptMediaBudget()
	loader := &comfyVideoMediaLoader{store: store, budget: budget, remaining: budget}
	var media comfyVideoMedia
	var err error
	if media.startFrame, err = loader.image(params.StartFrame); err != nil {
		return comfyVideoMedia{}, err
	}
	if media.endFrame, err = loader.image(params.EndFrame); err != nil {
		return comfyVideoMedia{}, err
	}
	for _, name := range params.ReferenceImages {
		image, err := loader.image(name)
		if err != nil {
			return comfyVideoMedia{}, err
		}
		media.referenceImages = append(media.referenceImages, image)
	}
	for _, name := range params.ReferenceAudios {
		audio, err := loader.audio(name)
		if err != nil {
			return comfyVideoMedia{}, err
		}
		media.referenceAudios = append(media.referenceAudios, audio)
	}
	return media, nil
}

func (store *comfyVideoJobStore) promptMediaBudget() int64 {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.maxPromptMediaBytes
}

type comfyVideoMediaLoader struct {
	store     *comfyVideoJobStore
	budget    int64
	remaining int64
}

func (loader *comfyVideoMediaLoader) image(name string) ([]byte, error) {
	if name == "" {
		return nil, nil
	}
	media, err := loader.load(name, "image")
	if err != nil {
		return nil, err
	}
	if media.isAudio() {
		return nil, &comfyVideoRequestError{message: fmt.Sprintf("workflow loads %q as an image, but it was uploaded as audio", name)}
	}
	return media.content, nil
}

func (loader *comfyVideoMediaLoader) audio(name string) (comfyUploadedMedia, error) {
	media, err := loader.load(name, "audio")
	if err != nil {
		return comfyUploadedMedia{}, err
	}
	if !media.isAudio() {
		return comfyUploadedMedia{}, &comfyVideoRequestError{message: fmt.Sprintf("workflow loads %q as audio, but it was not uploaded as audio", name)}
	}
	return media, nil
}

func (loader *comfyVideoMediaLoader) load(name string, kind string) (comfyUploadedMedia, error) {
	media, err := loader.store.readUpload(name, loader.remaining)
	switch {
	case errors.Is(err, errComfyUploadOverBudget):
		return comfyUploadedMedia{}, &comfyVideoRequestError{code: "media_too_large", message: fmt.Sprintf("workflow media exceeds the router's %d byte per-prompt cap", loader.budget)}
	case err != nil:
		return comfyUploadedMedia{}, &comfyVideoRequestError{message: fmt.Sprintf("workflow references %s %q, which was not uploaded to this router", kind, name)}
	}
	loader.remaining -= int64(len(media.content))
	return media, nil
}
