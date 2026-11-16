package proxy

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync"
	"testing"
)

const minimalWAVHeader = "RIFF\x24\x00\x00\x00WAVEfmt \x10\x00\x00\x00"

type soundToVideoSpeech struct {
	filename    string
	contentType string
	content     []byte
}

func wavSpeech() soundToVideoSpeech {
	return soundToVideoSpeech{filename: "speech.wav", contentType: "audio/wav", content: []byte(minimalWAVHeader)}
}

func TestComfyVideoSDCPPSendsUploadedWAVAsReferenceAudio(t *testing.T) {
	refAudios := runSoundToVideoThroughSDCPP(t, wavSpeech())

	if want := "data:audio/wav;base64," + base64.StdEncoding.EncodeToString([]byte(minimalWAVHeader)); len(refAudios) != 1 || refAudios[0] != want {
		t.Fatalf("ref_audios = %v, want the uploaded WAV unchanged", refAudios)
	}
}

func TestComfyVideoSDCPPConvertsNonWAVReferenceAudioBeforeSendingIt(t *testing.T) {
	refAudios := runSoundToVideoThroughSDCPP(t, soundToVideoSpeech{filename: "speech.mp3", contentType: "audio/mpeg", content: synthMP3Tone(t)})

	if len(refAudios) != 1 || !strings.HasPrefix(refAudios[0], "data:audio/wav;base64,") {
		t.Fatalf("ref_audios = %.80v, want one WAV data URL", refAudios)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(refAudios[0], "data:audio/wav;base64,"))
	if err != nil || !isRIFFWave(decoded) {
		t.Fatalf("converted reference audio is not WAV, sd-server would reject it: err=%v", err)
	}
}

func runSoundToVideoThroughSDCPP(t *testing.T, speech soundToVideoSpeech) []string {
	t.Helper()
	tool := requireFFmpegTool(t)
	aviBase64 := base64.StdEncoding.EncodeToString(synthTestAVI(t))
	var mutex sync.Mutex
	var refAudios []string
	service, _, _ := newSplitTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("text backend should not receive a comfy request, got %s", r.URL.Path)
	}), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/sdapi/v1/sd-models":
			_, _ = w.Write([]byte(`[{"model_name":"ready"}]`))
		case r.URL.Path == sdcppVideoSubmitPath:
			var body struct {
				RefAudios []string `json:"ref_audios"`
			}
			raw, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Errorf("vid_gen body is not JSON: %v", err)
			}
			mutex.Lock()
			refAudios = body.RefAudios
			mutex.Unlock()
			_, _ = w.Write([]byte(`{"id":"sd-job-1","kind":"vid_gen","status":"queued"}`))
		case strings.HasPrefix(r.URL.Path, sdcppVideoJobPathPrefix):
			_, _ = w.Write([]byte(`{"id":"sd-job-1","status":"completed","result":{"b64_json":"` + aviBase64 + `"}}`))
		default:
			t.Errorf("unexpected image backend path %s", r.URL.Path)
		}
	}), map[string]string{
		"image": `{"backend_mode":"llama_sdcpp","nomodel":true,"sddiffusionmodel":"C:\\models\\wan-s2v.safetensors","sdaudioencoder":"C:\\models\\wav2vec2.gguf"}`,
	})
	service.ffmpeg = tool
	warmUpActiveImageModel(t, service, "image-wan-s2v")
	uploadSoundToVideoMedia(t, service, speech)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/prompt", strings.NewReader(strings.ReplaceAll(sdcppSoundToVideoPromptBody, "speech.wav", speech.filename)))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("submit status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var submitted struct {
		PromptID string `json:"prompt_id"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &submitted); err != nil || submitted.PromptID == "" {
		t.Fatalf("expected a prompt_id, got %s (err=%v)", recorder.Body.String(), err)
	}
	pollComfyHistoryUntilComplete(t, service, submitted.PromptID)
	mutex.Lock()
	defer mutex.Unlock()
	return refAudios
}

func synthMP3Tone(t *testing.T) []byte {
	t.Helper()
	output, err := exec.Command("ffmpeg", "-f", "lavfi", "-i", "sine=frequency=440:duration=1", "-c:a", "libmp3lame", "-f", "mp3", "pipe:1").Output()
	if err != nil {
		t.Fatalf("failed to synthesize test tone: %v", err)
	}
	return output
}
