package proxy

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newKoboldUploadTestService(t *testing.T, backendName string) *Service {
	t.Helper()
	tool := requireFFmpegTool(t)
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/upload/image" {
			t.Errorf("unexpected backend path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"` + backendName + `","subfolder":"","type":"input"}`))
	}), map[string]string{
		"image": `{"nomodel":true,"sdmodel":"C:\\models\\dream.safetensors"}`,
	})
	service.ffmpeg = tool
	warmUpActiveImageModel(t, service, "image-dream")
	return service
}

func TestComfyUploadKeepsAudioUnderItsUploadedNameAndType(t *testing.T) {
	service := newKoboldUploadTestService(t, "kcpp_img2img.jpg")
	speech := []byte("RIFF....WAVEfmt ")

	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, uploadMediaRequest(t, "speech.wav", "application/octet-stream", speech))
	if recorder.Code != http.StatusOK {
		t.Fatalf("upload status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	stored, err := service.comfyVideoJobs.readUpload("speech.wav", maxComfyVideoUploadBytes)
	if err != nil || !bytes.Equal(stored.content, speech) {
		t.Fatalf("audio upload not kept under its uploaded name: err=%v", err)
	}
	if !stored.isAudio() || stored.contentType != "audio/wav" {
		t.Fatalf("content type = %q, want audio/wav detected from the extension", stored.contentType)
	}
}

func TestComfyUploadRefusesATraversalShapedFileName(t *testing.T) {
	tool := requireFFmpegTool(t)
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("a traversal-shaped upload must not reach the backend, got %s", r.URL.Path)
	}), map[string]string{
		"image": `{"nomodel":true,"sdmodel":"C:\\models\\dream.safetensors"}`,
	})
	service.ffmpeg = tool

	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, uploadMediaRequest(t, "..", "image/png", []byte("PNG")))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 body=%s", recorder.Code, recorder.Body.String())
	}
	if len(service.comfyVideoJobs.uploads) != 0 {
		t.Fatal("a refused upload must not be kept")
	}
}

func TestComfyUploadPartOverThePerFileCapIsNotKeptTruncated(t *testing.T) {
	service := newKoboldUploadTestService(t, "oversized.png")
	request := uploadMediaRequest(t, "oversized.png", "image/png", bytes.Repeat([]byte("A"), maxComfyVideoUploadBytes+1))
	if request.ContentLength > maxComfyUploadRequestBytes {
		t.Fatalf("test upload is %d bytes, it must fit the request cap to reach the per-file check", request.ContentLength)
	}

	if service.handleComfyUploadImage(httptest.NewRecorder(), request) {
		t.Fatal("an upload part over the per-file cap must fall through instead of being teed")
	}
	if _, err := service.comfyVideoJobs.readUpload("oversized.png", maxComfyVideoUploadBytes); err == nil {
		t.Fatal("an upload part over the per-file cap must not be kept truncated")
	}
}
