package proxy

import (
	"errors"
	"testing"

	"tensors-router/internal/comfyvideo"
)

func rememberTestUpload(t *testing.T, store *comfyVideoJobStore, name string, contentType string, content string) {
	t.Helper()
	if err := store.rememberUpload(name, comfyUploadedMedia{content: []byte(content), contentType: contentType}); err != nil {
		t.Fatal(err)
	}
}

func requireComfyVideoRequestError(t *testing.T, err error, code string) {
	t.Helper()
	var requestErr *comfyVideoRequestError
	if !errors.As(err, &requestErr) {
		t.Fatalf("error = %v, want a comfyVideoRequestError", err)
	}
	if requestErr.code != code {
		t.Fatalf("error code = %q, want %q (message %q)", requestErr.code, code, requestErr.message)
	}
}

func TestComfyVideoMediaIsCappedPerPrompt(t *testing.T) {
	store := newTestComfyVideoStore(t)
	store.maxPromptMediaBytes = 10
	rememberTestUpload(t, store, "a.png", "image/png", "123456")
	rememberTestUpload(t, store, "b.png", "image/png", "123456")

	if _, err := store.resolveWorkflowMedia(comfyvideo.Params{ReferenceImages: []string{"a.png"}}); err != nil {
		t.Fatalf("media within the per-prompt cap was refused: %v", err)
	}
	_, err := store.resolveWorkflowMedia(comfyvideo.Params{ReferenceImages: []string{"a.png", "b.png"}})
	requireComfyVideoRequestError(t, err, "media_too_large")
}

func TestComfyVideoMediaRefusesAnUploadLoadedAsTheWrongKind(t *testing.T) {
	store := newTestComfyVideoStore(t)
	rememberTestUpload(t, store, "voice.wav", "audio/wav", "RIFF")
	rememberTestUpload(t, store, "still.png", "image/png", "PNG")

	if _, err := store.resolveWorkflowMedia(comfyvideo.Params{ReferenceImages: []string{"voice.wav"}}); err == nil {
		t.Fatal("an audio upload must not be sent as a reference image")
	}
	if _, err := store.resolveWorkflowMedia(comfyvideo.Params{ReferenceAudios: []string{"still.png"}}); err == nil {
		t.Fatal("an image upload must not be sent as a reference audio")
	}
}
