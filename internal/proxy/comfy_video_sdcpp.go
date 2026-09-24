package proxy

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"tensors-router/internal/catalog"
	"tensors-router/internal/comfyvideo"
)

const (
	sdcppVideoSubmitPath       = "/sdcpp/v1/vid_gen"
	sdcppVideoJobPathPrefix    = "/sdcpp/v1/jobs/"
	sdcppUnsupportedMediaCode  = "unsupported_media"
	sdcppNoAudioInputMessage   = "sd-server has no HTTP audio input, so workflows with audio references (such as Wan2.2 S2V) need backend_mode kobold"
	comfyVideoPollInterval     = 1 * time.Second
	sdcppVideoOutputFormatMJPG = "avi"
)

// generateSDCPPVideo submits an sd-server /sdcpp/v1/vid_gen job and polls it
// to completion. sample_params is intentionally omitted: the server API
// documents it as a nested sampler override but does not fix its field
// names, so guessing them risks the whole request being rejected. Only the
// fields the API reference confirms are sent; sampler/scheduler/steps/cfg
// fall back to the loaded config's defaults for this path.
func (service *Service) generateSDCPPVideo(ctx context.Context, model catalog.Model, requestBody []byte) ([]byte, error) {
	submitResponse, finalizer, err := service.forwardWithFallbackObserved(ctx, syntheticImageRequest(http.MethodPost, sdcppVideoSubmitPath), requestBody, model.ImageID, model.Filename, true, readinessImage, BackendModeLlamaSDCPP)
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
	return service.pollSDCPPVideoJob(ctx, model, jobID)
}

func (service *Service) pollSDCPPVideoJob(ctx context.Context, model catalog.Model, jobID string) ([]byte, error) {
	ticker := time.NewTicker(comfyVideoPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
		pollResponse, pollFinalizer, err := service.forwardWithFallbackObserved(ctx, syntheticImageRequest(http.MethodGet, sdcppVideoJobPathPrefix+jobID), nil, model.ImageID, model.Filename, true, readinessImage, BackendModeLlamaSDCPP)
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

func buildSDCPPVideoRequest(params comfyvideo.Params, media comfyVideoMedia) ([]byte, error) {
	if len(media.referenceAudios) > 0 {
		return nil, &comfyVideoRequestError{code: sdcppUnsupportedMediaCode, message: sdcppNoAudioInputMessage}
	}
	body := map[string]any{
		"prompt":          params.Prompt,
		"negative_prompt": params.NegativePrompt,
		"width":           params.Width,
		"height":          params.Height,
		"video_frames":    params.Frames,
		"fps":             params.FPS,
		"seed":            params.Seed,
		"output_format":   sdcppVideoOutputFormatMJPG,
	}
	setBase64Field(body, "init_image", media.startFrame)
	setBase64Field(body, "end_image", media.endFrame)
	if len(media.referenceImages) > 0 {
		body["ref_images"] = base64Images(media.referenceImages)
	}
	if len(params.LoRAs) > 0 {
		body["lora"] = comfyVideoLoRARequests(params.LoRAs)
	}
	return json.Marshal(body)
}
