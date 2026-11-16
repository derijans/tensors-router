package proxy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"tensors-router/internal/catalog"
	"tensors-router/internal/ffmpeg"
)

const (
	sdcppAudioComponentRequiredMessage = "sd-server reads reference audio (such as Wan2.2 S2V speech) only when the image config loads sdaudioencoder or sdaudiovae"
	sdcppAudioNeedsWAVMessage          = "sd-server reads reference audio only as WAV; upload a WAV file or give the router a working ffmpeg to convert it"
	wavContentType                     = "audio/wav"
)

func (service *Service) sdcppReferenceAudios(ctx context.Context, model catalog.Model, audios []comfyUploadedMedia) ([]comfyUploadedMedia, error) {
	if len(audios) == 0 {
		return nil, nil
	}
	if !modelLoadsSDCPPAudioComponent(model) {
		return nil, &comfyVideoRequestError{code: sdcppUnsupportedMediaCode, message: sdcppAudioComponentRequiredMessage}
	}
	prepared := make([]comfyUploadedMedia, 0, len(audios))
	for _, audio := range audios {
		wav, err := service.referenceAudioAsWAV(ctx, audio)
		if err != nil {
			return nil, err
		}
		prepared = append(prepared, wav)
	}
	return prepared, nil
}

func modelLoadsSDCPPAudioComponent(model catalog.Model) bool {
	image := model.Capabilities.Image
	return image != nil && (strings.TrimSpace(image.AudioEncoder) != "" || strings.TrimSpace(image.AudioVAE) != "")
}

func (service *Service) referenceAudioAsWAV(ctx context.Context, audio comfyUploadedMedia) (comfyUploadedMedia, error) {
	if isRIFFWave(audio.content) {
		return comfyUploadedMedia{content: audio.content, contentType: wavContentType}, nil
	}
	var converted bytes.Buffer
	budget := service.comfyVideoJobs.promptMediaBudget()
	writer := &cappedWriter{target: &converted, remaining: budget, overflow: errComfyUploadOverBudget}
	err := service.ffmpeg.ConvertToFullRateWAV(ctx, bytes.NewReader(audio.content), writer)
	switch {
	case errors.Is(err, ffmpeg.ErrNotAvailable):
		return comfyUploadedMedia{}, &comfyVideoRequestError{code: sdcppUnsupportedMediaCode, message: sdcppAudioNeedsWAVMessage}
	case writer.exceeded:
		return comfyUploadedMedia{}, &comfyVideoRequestError{code: "media_too_large", message: fmt.Sprintf("reference audio converted to WAV exceeds the router's %d byte per-prompt cap", budget)}
	case err != nil:
		return comfyUploadedMedia{}, &comfyVideoRequestError{code: sdcppUnsupportedMediaCode, message: "reference audio could not be converted to WAV for sd-server: " + err.Error()}
	}
	return comfyUploadedMedia{content: converted.Bytes(), contentType: wavContentType}, nil
}

func isRIFFWave(content []byte) bool {
	return len(content) >= 12 && string(content[0:4]) == "RIFF" && string(content[8:12]) == "WAVE"
}
