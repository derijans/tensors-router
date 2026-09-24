package proxy

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"

	"tensors-router/internal/catalog"
	"tensors-router/internal/comfyvideo"
)

const (
	koboldVideoGenerationPath   = "/sdapi/v1/txt2img"
	koboldMJPGAVIVideoOutput    = 1
	koboldMaxVideoFrames        = 480
	koboldMaxReferenceImages    = 4
	koboldTooManyReferencesCode = "too_many_reference_images"
)

// generateKoboldVideo issues a single synchronous generation call with
// KoboldCpp's video fields. The MJPG-AVI encoder is the only one of
// KoboldCpp's two video containers that carries a MiniMax-H3 audio track.
func (service *Service) generateKoboldVideo(ctx context.Context, model catalog.Model, requestBody []byte) ([]byte, error) {
	response, finalizer, err := service.forwardWithFallbackObserved(ctx, syntheticImageRequest(http.MethodPost, koboldVideoGenerationPath), requestBody, model.ImageID, model.Filename, true, readinessImage, BackendModeKobold)
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

func buildKoboldVideoRequest(params comfyvideo.Params, media comfyVideoMedia) ([]byte, error) {
	if len(media.referenceImages) > koboldMaxReferenceImages {
		return nil, &comfyVideoRequestError{
			code:    koboldTooManyReferencesCode,
			message: fmt.Sprintf("koboldcpp accepts at most %d reference images per video; the workflow loads %d", koboldMaxReferenceImages, len(media.referenceImages)),
		}
	}
	body := map[string]any{
		"prompt":            params.Prompt,
		"negative_prompt":   params.NegativePrompt,
		"width":             params.Width,
		"height":            params.Height,
		"steps":             params.Steps,
		"cfg_scale":         params.CFG,
		"seed":              params.Seed,
		"frames":            min(max(params.Frames, 1), koboldMaxVideoFrames),
		"fps":               params.FPS,
		"video_output_type": koboldMJPGAVIVideoOutput,
	}
	if params.SamplerName != "" {
		body["sampler_name"] = params.SamplerName
	}
	if extraImages := koboldExtraImages(media); len(extraImages) > 0 {
		body["extra_images"] = extraImages
	}
	setBase64Field(body, "video_start_frame", media.startFrame)
	setBase64Field(body, "video_end_frame", media.endFrame)
	if len(params.LoRAs) > 0 {
		body["lora"] = comfyVideoLoRARequests(params.LoRAs)
	}
	return json.Marshal(body)
}

func koboldExtraImages(media comfyVideoMedia) []string {
	extraImages := base64Images(media.referenceImages)
	for _, audio := range media.referenceAudios {
		extraImages = append(extraImages, audioDataURL(audio))
	}
	return extraImages
}
