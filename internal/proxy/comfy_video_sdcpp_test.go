package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"tensors-router/internal/comfyvideo"
	"tensors-router/internal/openai"
)

const sdcppSoundToVideoPromptBody = `{"prompt":{
	"1":{"class_type":"LoadImage","inputs":{"image":"speaker.png"}},
	"2":{"class_type":"LoadAudio","inputs":{"audio":"speech.wav"}},
	"3":{"class_type":"CLIPTextEncode","inputs":{"text":"a person talking"}},
	"4":{"class_type":"CLIPTextEncode","inputs":{"text":"blurry"}},
	"5":{"class_type":"WanSoundImageToVideo","inputs":{"ref_image":["1",0],"length":33}},
	"6":{"class_type":"KSampler","inputs":{"seed":1,"steps":4,"cfg":5.0,"positive":["3",0],"negative":["4",0]}}
}}`

func TestBuildSDCPPVideoRequestSendsFramesReferencesAndLoRAs(t *testing.T) {
	params := comfyvideo.Params{Frames: 33, LoRAs: []comfyvideo.LoRA{{Name: "motion.safetensors", Strength: 0.5}}}
	media := comfyVideoMedia{
		startFrame:      []byte("first"),
		endFrame:        []byte("last"),
		referenceImages: [][]byte{[]byte("subject"), []byte("style")},
	}
	encoded, err := buildSDCPPVideoRequest(params, media)
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		InitImage string           `json:"init_image"`
		EndImage  string           `json:"end_image"`
		RefImages []string         `json:"ref_images"`
		LoRA      []map[string]any `json:"lora"`
	}
	if err := json.Unmarshal(encoded, &body); err != nil {
		t.Fatal(err)
	}
	if body.InitImage != b64("first") || body.EndImage != b64("last") {
		t.Fatalf("frames = %q..%q", body.InitImage, body.EndImage)
	}
	if want := []string{b64("subject"), b64("style")}; !slices.Equal(body.RefImages, want) {
		t.Fatalf("ref_images = %v, want %v", body.RefImages, want)
	}
	if len(body.LoRA) != 1 || body.LoRA[0]["path"] != "motion.safetensors" || body.LoRA[0]["multiplier"] != 0.5 {
		t.Fatalf("lora = %v", body.LoRA)
	}
}

func TestComfyVideoSDCPPRejectsAudioReferencesAsUnsupportedMedia(t *testing.T) {
	tool := requireFFmpegTool(t)
	service, _, _ := newSplitTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("text backend should not receive a comfy request, got %s", r.URL.Path)
	}), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sdapi/v1/sd-models" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"model_name":"ready"}]`))
			return
		}
		t.Errorf("sd-server must not be contacted for an upload or an unsupported workflow, got %s", r.URL.Path)
	}), map[string]string{
		"image": `{"nomodel":true,"sddiffusionmodel":"C:\\models\\wan-s2v.safetensors"}`,
	})
	service.ffmpeg = tool
	warmUpActiveImageModel(t, service, "image-wan-s2v")

	for _, upload := range []struct{ filename, contentType string }{{"speaker.png", "image/png"}, {"speech.wav", "audio/wav"}} {
		recorder := httptest.NewRecorder()
		service.ServeHTTP(recorder, uploadMediaRequest(t, upload.filename, upload.contentType, []byte("media")))
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"name":"`+upload.filename+`"`) {
			t.Fatalf("upload %s on sd-server should be kept by the router: status=%d body=%s", upload.filename, recorder.Code, recorder.Body.String())
		}
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/prompt", strings.NewReader(sdcppSoundToVideoPromptBody))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 body=%s", recorder.Code, recorder.Body.String())
	}
	var response openai.ErrorBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Error.Code != sdcppUnsupportedMediaCode || !strings.Contains(response.Error.Message, "no HTTP audio input") {
		t.Fatalf("error = %+v, want an explicit unsupported_media rejection", response.Error)
	}
}
