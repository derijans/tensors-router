package proxy

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"tensors-router/internal/comfyvideo"
)

const koboldMultiReferencePromptBody = `{"prompt":{
	"1":{"class_type":"CheckpointLoaderSimple","inputs":{"ckpt_name":"minimax-h3.safetensors"}},
	"2":{"class_type":"LoadImage","inputs":{"image":"subject.png"}},
	"3":{"class_type":"LoadImage","inputs":{"image":"style.png"}},
	"4":{"class_type":"LoadImage","inputs":{"image":"first.png"}},
	"5":{"class_type":"LoadImage","inputs":{"image":"last.png"}},
	"6":{"class_type":"LoadAudio","inputs":{"audio":"voice.wav"}},
	"7":{"class_type":"LoadAudio","inputs":{"audio":"music.mp3"}},
	"8":{"class_type":"LoraLoaderModelOnly","inputs":{"lora_name":"video/motion.safetensors","strength_model":0.8,"model":["1",0]}},
	"9":{"class_type":"CLIPTextEncode","inputs":{"text":"a singer on stage","clip":["1",1]}},
	"10":{"class_type":"CLIPTextEncode","inputs":{"text":"blurry","clip":["1",1]}},
	"11":{"class_type":"MiniMaxH3ImageToVideo","inputs":{"start_image":["4",0],"end_image":["5",0],"reference_1":["2",0],"reference_2":["3",0],"length":600,"fps":24}},
	"12":{"class_type":"KSampler","inputs":{"seed":3,"steps":4,"cfg":5.0,"sampler_name":"euler","scheduler":"simple","positive":["9",0],"negative":["10",0],"model":["8",0]}},
	"13":{"class_type":"SaveWEBM","inputs":{"images":["12",0]}}
}}`

func TestBuildKoboldVideoRequestSendsReferencesThenAudiosFramesAndLoRAs(t *testing.T) {
	params := comfyvideo.Params{Frames: 900, FPS: 24, LoRAs: []comfyvideo.LoRA{{Name: "motion.safetensors", Strength: 0.8}}}
	media := comfyVideoMedia{
		startFrame:      []byte("first"),
		endFrame:        []byte("last"),
		referenceImages: [][]byte{[]byte("subject"), []byte("style")},
		referenceAudios: []comfyUploadedMedia{{content: []byte("voice"), contentType: "audio/wav"}, {content: []byte("music"), contentType: "audio/mpeg"}},
	}
	encoded, err := buildKoboldVideoRequest(params, media)
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		ExtraImages     []string         `json:"extra_images"`
		VideoStartFrame string           `json:"video_start_frame"`
		VideoEndFrame   string           `json:"video_end_frame"`
		Frames          int              `json:"frames"`
		LoRA            []map[string]any `json:"lora"`
	}
	if err := json.Unmarshal(encoded, &body); err != nil {
		t.Fatal(err)
	}
	wantExtraImages := []string{
		b64("subject"), b64("style"),
		"data:audio/wav;base64," + b64("voice"), "data:audio/mpeg;base64," + b64("music"),
	}
	if !slices.Equal(body.ExtraImages, wantExtraImages) {
		t.Fatalf("extra_images = %v, want %v", body.ExtraImages, wantExtraImages)
	}
	if body.VideoStartFrame != b64("first") || body.VideoEndFrame != b64("last") {
		t.Fatalf("frames = %q..%q", body.VideoStartFrame, body.VideoEndFrame)
	}
	if body.Frames != koboldMaxVideoFrames {
		t.Fatalf("frames = %d, want KoboldCpp's cap %d", body.Frames, koboldMaxVideoFrames)
	}
	if len(body.LoRA) != 1 || body.LoRA[0]["path"] != "motion.safetensors" || body.LoRA[0]["multiplier"] != 0.8 {
		t.Fatalf("lora = %v", body.LoRA)
	}
	for _, removed := range []string{"reverse_refimg", "init_images"} {
		if strings.Contains(string(encoded), removed) {
			t.Fatalf("body must not carry %s: %s", removed, encoded)
		}
	}
}

func TestBuildKoboldVideoRequestRefusesMoreReferencesThanKoboldKeeps(t *testing.T) {
	media := comfyVideoMedia{referenceImages: make([][]byte, koboldMaxReferenceImages+1)}
	_, err := buildKoboldVideoRequest(comfyvideo.Params{}, media)
	requireComfyVideoRequestError(t, err, koboldTooManyReferencesCode)
}

func TestComfyVideoKoboldReceivesEveryUploadedReferenceAndAudio(t *testing.T) {
	tool := requireFFmpegTool(t)
	aviBase64 := base64.StdEncoding.EncodeToString(synthTestAVI(t))

	generations := make(chan map[string]any, 1)
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/upload/image":
			_, _ = w.Write([]byte(`{"name":"kcpp_img2img.jpg","subfolder":"","type":"input"}`))
		case koboldVideoGenerationPath:
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("invalid generation body: %v", err)
			}
			generations <- payload
			_, _ = w.Write([]byte(`{"images":[],"animated":true,"extra_data":"` + aviBase64 + `"}`))
		default:
			t.Errorf("unexpected backend path %s", r.URL.Path)
		}
	}), map[string]string{
		"image": `{"nomodel":true,"sdmodel":"C:\\models\\minimax.safetensors"}`,
	})
	service.ffmpeg = tool
	warmUpActiveImageModel(t, service, "image-minimax")

	uploads := []struct{ filename, contentType, content string }{
		{"subject.png", "image/png", "subject"},
		{"style.png", "image/png", "style"},
		{"first.png", "image/png", "first"},
		{"last.png", "image/png", "last"},
		{"voice.wav", "audio/wav", "voice"},
		{"music.mp3", "application/octet-stream", "music"},
	}
	for _, upload := range uploads {
		recorder := httptest.NewRecorder()
		service.ServeHTTP(recorder, uploadMediaRequest(t, upload.filename, upload.contentType, []byte(upload.content)))
		if recorder.Code != http.StatusOK {
			t.Fatalf("upload %s status=%d body=%s", upload.filename, recorder.Code, recorder.Body.String())
		}
	}

	promptID := submitComfyPrompt(t, service, koboldMultiReferencePromptBody)
	pollComfyHistoryUntilComplete(t, service, promptID)
	payload := <-generations

	wantExtraImages := []any{b64("subject"), b64("style"), "data:audio/wav;base64," + b64("voice"), "data:audio/mpeg;base64," + b64("music")}
	if extraImages, _ := payload["extra_images"].([]any); !slices.Equal(extraImages, wantExtraImages) {
		t.Fatalf("extra_images = %v, want %v", payload["extra_images"], wantExtraImages)
	}
	if payload["video_start_frame"] != b64("first") || payload["video_end_frame"] != b64("last") {
		t.Fatalf("frames = %v..%v", payload["video_start_frame"], payload["video_end_frame"])
	}
	if loras, _ := payload["lora"].([]any); len(loras) != 1 {
		t.Fatalf("lora = %v, want the workflow's single LoRA", payload["lora"])
	}
}

func submitComfyPrompt(t *testing.T, service *Service, body string) string {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/prompt", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected submit status %d body=%s", recorder.Code, recorder.Body.String())
	}
	var submitted struct {
		PromptID string `json:"prompt_id"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &submitted); err != nil || submitted.PromptID == "" {
		t.Fatalf("expected a prompt_id, got %s (err=%v)", recorder.Body.String(), err)
	}
	return submitted.PromptID
}

func b64(content string) string {
	return base64.StdEncoding.EncodeToString([]byte(content))
}
