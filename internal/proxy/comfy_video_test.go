package proxy

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"tensors-router/internal/ffmpeg"
)

const wanVideoPromptBody = `{"prompt":{
	"1":{"class_type":"CheckpointLoaderSimple","inputs":{"ckpt_name":"wan2.2.safetensors"}},
	"2":{"class_type":"CLIPTextEncode","inputs":{"text":"a cat riding a skateboard","clip":["1",1]}},
	"3":{"class_type":"CLIPTextEncode","inputs":{"text":"blurry","clip":["1",1]}},
	"4":{"class_type":"EmptyHunyuanLatentVideo","inputs":{"width":64,"height":64,"length":9}},
	"5":{"class_type":"KSampler","inputs":{"seed":1,"steps":4,"cfg":5.0,"sampler_name":"euler","scheduler":"simple","positive":["2",0],"negative":["3",0],"model":["1",0],"latent_image":["4",0]}},
	"6":{"class_type":"WanImageToVideo","inputs":{"fps":8,"samples":["5",0]}},
	"7":{"class_type":"SaveWEBM","inputs":{"images":["6",0]}}
}}`

const stillImagePromptBody = `{"prompt":{
	"1":{"class_type":"CheckpointLoaderSimple","inputs":{"ckpt_name":"sdxl.safetensors"}},
	"2":{"class_type":"CLIPTextEncode","inputs":{"text":"a mountain","clip":["1",1]}},
	"3":{"class_type":"CLIPTextEncode","inputs":{"text":"","clip":["1",1]}},
	"4":{"class_type":"EmptyLatentImage","inputs":{"width":512,"height":512,"batch_size":1}},
	"5":{"class_type":"KSampler","inputs":{"seed":1,"steps":20,"cfg":7.0,"sampler_name":"euler","scheduler":"normal","positive":["2",0],"negative":["3",0],"model":["1",0],"latent_image":["4",0]}},
	"6":{"class_type":"SaveImage","inputs":{"images":["5",0]}}
}}`

func requireFFmpegTool(t *testing.T) ffmpeg.Tool {
	t.Helper()
	tool, err := ffmpeg.Locate("")
	if err != nil {
		t.Skip("ffmpeg not installed on this machine")
	}
	return tool
}

// synthTestAVI produces a tiny real MJPG-AVI with a silent audio track, the
// same shape KoboldCpp's video_output_type=1 and sd-server's "avi" output
// format both emit, so RemuxToMP4 has real bytes to transcode.
func synthTestAVI(t *testing.T) []byte {
	t.Helper()
	cmd := exec.Command("ffmpeg",
		"-f", "lavfi", "-i", "testsrc=size=64x64:rate=8:duration=1",
		"-f", "lavfi", "-i", "anullsrc=r=8000:cl=mono",
		"-shortest", "-c:v", "mjpeg", "-c:a", "pcm_s16le", "-f", "avi", "pipe:1")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to synthesize test AVI: %v", err)
	}
	return output
}

// warmUpActiveImageModel makes imageModelID the router's active image model,
// the same way a client selecting a model in the WebUI would, since a
// ComfyUI /prompt request (like KoboldCpp's own emulation) never names a
// model explicitly and only operates on whatever is already active.
func warmUpActiveImageModel(t *testing.T, service *Service, imageModelID string) {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/sdapi/v1/options", strings.NewReader(`{"sd_model_checkpoint":"`+imageModelID+`"}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("failed to warm up active image model %q: status=%d body=%s", imageModelID, recorder.Code, recorder.Body.String())
	}
}

func pollComfyHistoryUntilComplete(t *testing.T, service *Service, promptID string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		recorder := httptest.NewRecorder()
		service.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/history/"+promptID, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("history poll failed status=%d body=%s", recorder.Code, recorder.Body.String())
		}
		var payload map[string]map[string]any
		if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
			t.Fatalf("invalid history JSON: %v", err)
		}
		entry := payload[promptID]
		status, _ := entry["status"].(map[string]any)
		switch statusStr, _ := status["status_str"].(string); statusStr {
		case "success":
			return entry
		case "error":
			t.Fatalf("job failed: %v", status["messages"])
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("job did not complete before deadline")
	return nil
}

func TestComfyVideoWorkflowGeneratesMP4ViaKobold(t *testing.T) {
	tool := requireFFmpegTool(t)
	avi := synthTestAVI(t)
	aviBase64 := base64.StdEncoding.EncodeToString(avi)

	var sawPlainPrompt atomic.Bool
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sdapi/v1/txt2img":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"images":[],"animated":true,"extra_data":"` + aviBase64 + `"}`))
		case "/prompt":
			sawPlainPrompt.Store(true)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"prompt_id":"12345678-0000-0000-0000-000000000001","number":0,"node_errors":{}}`))
		default:
			t.Fatalf("unexpected backend path %s", r.URL.Path)
		}
	}), map[string]string{
		"image": `{"nomodel":true,"sdmodel":"C:\\models\\dream.safetensors"}`,
	})
	service.ffmpeg = tool
	warmUpActiveImageModel(t, service, "image-dream")

	submitRecorder := httptest.NewRecorder()
	submitRequest := httptest.NewRequest(http.MethodPost, "/prompt", strings.NewReader(wanVideoPromptBody))
	submitRequest.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(submitRecorder, submitRequest)
	if submitRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected submit status %d body=%s", submitRecorder.Code, submitRecorder.Body.String())
	}
	var submitted struct {
		PromptID string `json:"prompt_id"`
	}
	if err := json.Unmarshal(submitRecorder.Body.Bytes(), &submitted); err != nil || submitted.PromptID == "" {
		t.Fatalf("expected a prompt_id, got %s (err=%v)", submitRecorder.Body.String(), err)
	}

	entry := pollComfyHistoryUntilComplete(t, service, submitted.PromptID)
	outputs, _ := entry["outputs"].(map[string]any)
	firstNode, _ := outputs["1"].(map[string]any)
	gifs, _ := firstNode["gifs"].([]any)
	if len(gifs) != 1 {
		t.Fatalf("expected one gifs entry, got %#v", outputs)
	}
	first, _ := gifs[0].(map[string]any)
	filename, _ := first["filename"].(string)
	if !strings.HasSuffix(filename, ".mp4") {
		t.Fatalf("expected an .mp4 filename, got %q", filename)
	}

	viewRecorder := httptest.NewRecorder()
	service.ServeHTTP(viewRecorder, httptest.NewRequest(http.MethodGet, "/view?filename="+filename+"&subfolder=&type=output", nil))
	if viewRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected view status %d", viewRecorder.Code)
	}
	if viewRecorder.Header().Get("Content-Type") != "video/mp4" {
		t.Fatalf("unexpected content type %q", viewRecorder.Header().Get("Content-Type"))
	}
	video := viewRecorder.Body.Bytes()
	if len(video) < 12 || string(video[4:8]) != "ftyp" {
		t.Fatalf("view response is not a valid MP4: %d bytes", len(video))
	}

	// A plain image workflow must still reach KoboldCpp's own ComfyUI
	// emulation unchanged.
	imagePromptRecorder := httptest.NewRecorder()
	imagePromptRequest := httptest.NewRequest(http.MethodPost, "/prompt", strings.NewReader(stillImagePromptBody))
	imagePromptRequest.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(imagePromptRecorder, imagePromptRequest)
	if imagePromptRecorder.Code != http.StatusOK || !sawPlainPrompt.Load() {
		t.Fatalf("expected the plain image workflow to reach the backend /prompt, status=%d saw=%t", imagePromptRecorder.Code, sawPlainPrompt.Load())
	}
}

func TestComfyVideoWorkflowGeneratesMP4ViaSDCPP(t *testing.T) {
	tool := requireFFmpegTool(t)
	avi := synthTestAVI(t)
	aviBase64 := base64.StdEncoding.EncodeToString(avi)

	var pollCount atomic.Int32
	service, _, imageBackend := newSplitTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("text backend should not receive a comfy video request, got %s", r.URL.Path)
	}), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/sdapi/v1/sd-models":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"model_name":"ready"}]`))
		case r.URL.Path == "/sdcpp/v1/vid_gen":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"sd-job-1","kind":"vid_gen","status":"queued"}`))
		case strings.HasPrefix(r.URL.Path, "/sdcpp/v1/jobs/"):
			w.Header().Set("Content-Type", "application/json")
			if pollCount.Add(1) < 2 {
				_, _ = w.Write([]byte(`{"id":"sd-job-1","status":"generating"}`))
				return
			}
			_, _ = w.Write([]byte(`{"id":"sd-job-1","status":"completed","result":{"output_format":"avi","mime_type":"video/avi","b64_json":"` + aviBase64 + `"}}`))
		default:
			t.Fatalf("unexpected image backend path %s", r.URL.Path)
		}
	}), map[string]string{
		"image": `{"nomodel":true,"sddiffusionmodel":"C:\\models\\wan.safetensors"}`,
	})
	service.ffmpeg = tool
	_ = imageBackend
	warmUpActiveImageModel(t, service, "image-wan")

	submitRecorder := httptest.NewRecorder()
	submitRequest := httptest.NewRequest(http.MethodPost, "/prompt", strings.NewReader(wanVideoPromptBody))
	submitRequest.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(submitRecorder, submitRequest)
	if submitRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected submit status %d body=%s", submitRecorder.Code, submitRecorder.Body.String())
	}
	var submitted struct {
		PromptID string `json:"prompt_id"`
	}
	if err := json.Unmarshal(submitRecorder.Body.Bytes(), &submitted); err != nil || submitted.PromptID == "" {
		t.Fatalf("expected a prompt_id, got %s (err=%v)", submitRecorder.Body.String(), err)
	}

	entry := pollComfyHistoryUntilComplete(t, service, submitted.PromptID)
	if entry["outputs"] == nil {
		t.Fatalf("expected outputs once completed, got %#v", entry)
	}
	if pollCount.Load() < 2 {
		t.Fatalf("expected the router to poll sd-server's job at least twice, got %d", pollCount.Load())
	}
}

func TestComfyVideoWorkflowFailsExplicitlyWithoutFFmpeg(t *testing.T) {
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("backend should not be contacted when ffmpeg is unavailable, got %s", r.URL.Path)
	}), map[string]string{
		"image": `{"nomodel":true,"sdmodel":"C:\\models\\dream.safetensors"}`,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/prompt", strings.NewReader(wanVideoPromptBody))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 without ffmpeg, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}

const wanImageToVideoPromptBody = `{"prompt":{
	"1":{"class_type":"CheckpointLoaderSimple","inputs":{"ckpt_name":"wan2.2.safetensors"}},
	"2":{"class_type":"LoadImage","inputs":{"image":"REFERENCE_NAME"}},
	"3":{"class_type":"CLIPTextEncode","inputs":{"text":"make it move","clip":["1",1]}},
	"4":{"class_type":"CLIPTextEncode","inputs":{"text":"blurry","clip":["1",1]}},
	"5":{"class_type":"KSampler","inputs":{"seed":7,"steps":4,"cfg":5.0,"sampler_name":"euler","scheduler":"simple","positive":["3",0],"negative":["4",0],"model":["1",0]}},
	"6":{"class_type":"WanImageToVideo","inputs":{"fps":8,"length":9,"start_image":["2",0]}},
	"7":{"class_type":"SaveWEBM","inputs":{"images":["6",0]}}
}}`

// An upload too large for the router to copy must still reach the backend
// intact: handing back only the prefix already read would silently truncate a
// legitimate image workflow's upload. The declared length is cleared so the
// router cannot bail out before reading, which is what a chunked upload looks
// like and is the only way into the truncating path.
func TestComfyUploadImageTooLargeToCopyStillReachesTheBackendWhole(t *testing.T) {
	tool := requireFFmpegTool(t)
	oversized := bytes.Repeat([]byte("A"), maxComfyUploadRequestBytes+(1<<20))

	var received atomic.Int64
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/upload/image" {
			forwarded, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("backend could not read the forwarded upload: %v", err)
			}
			received.Store(int64(len(forwarded)))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"huge.png","subfolder":"","type":"input"}`))
			return
		}
		t.Fatalf("unexpected backend path %s", r.URL.Path)
	}), map[string]string{
		"image": `{"nomodel":true,"sdmodel":"C:\\models\\dream.safetensors"}`,
	})
	service.ffmpeg = tool
	warmUpActiveImageModel(t, service, "image-dream")

	request := uploadImageRequest(t, oversized)
	sent := request.ContentLength
	// A chunked upload declares no length, which is the only way past the
	// cheap up-front rejection and into the path that reads before deciding.
	request.ContentLength = -1
	recorder := httptest.NewRecorder()

	if service.handleComfyUploadImage(recorder, request) {
		t.Fatalf("an upload past the copy limit must not be intercepted, status=%d", recorder.Code)
	}
	if _, ok := service.comfyVideoJobs.uploadBytes("huge.png"); ok {
		t.Fatal("an upload past the copy limit must not be kept by the router")
	}

	// Whatever the router declined to copy must still be readable in full by
	// the handler it fell through to.
	fellThrough, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatalf("body left unreadable after declining to copy: %v", err)
	}
	if int64(len(fellThrough)) != sent {
		t.Fatalf("fall-through body is %d bytes, want the whole %d byte upload", len(fellThrough), sent)
	}
	if received.Load() != 0 {
		t.Fatal("the backend should not have been called by the interceptor itself")
	}
}

func uploadImageRequest(t *testing.T, content []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("image", "reference.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/upload/image", bytes.NewReader(body.Bytes()))
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

// The router must not divert /upload/image away from the backend: KoboldCpp
// still serves image workflows that reference the uploaded name, so the
// upload is teed, and the router's copy is keyed by the name the backend
// assigned rather than one of its own.
func TestComfyUploadImageIsForwardedToTheBackendAndCopiedLocally(t *testing.T) {
	tool := requireFFmpegTool(t)
	reference := []byte("\x89PNG\r\n\x1a\nreference-image-bytes")

	var sawUpload atomic.Bool
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/upload/image" {
			sawUpload.Store(true)
			forwarded, err := io.ReadAll(r.Body)
			if err != nil || !bytes.Contains(forwarded, reference) {
				t.Fatalf("backend did not receive the uploaded bytes (err=%v)", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"backend-assigned.png","subfolder":"","type":"input"}`))
			return
		}
		t.Fatalf("unexpected backend path %s", r.URL.Path)
	}), map[string]string{
		"image": `{"nomodel":true,"sdmodel":"C:\\models\\dream.safetensors"}`,
	})
	service.ffmpeg = tool
	warmUpActiveImageModel(t, service, "image-dream")

	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, uploadImageRequest(t, reference))
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected upload status %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !sawUpload.Load() {
		t.Fatal("upload never reached the backend; image workflows would break")
	}
	var response struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Name != "backend-assigned.png" {
		t.Fatalf("router returned name %q, want the backend's own name", response.Name)
	}
	stored, ok := service.comfyVideoJobs.uploadBytes("backend-assigned.png")
	if !ok || !bytes.Equal(stored, reference) {
		t.Fatalf("router did not keep a copy under the backend's name: ok=%t", ok)
	}
}

// An image-to-video workflow naming an uploaded reference must reach the
// backend's img2img route carrying that image, since the router generates the
// video itself and never forwards the workflow.
func TestComfyVideoImageToVideoSendsTheUploadedReference(t *testing.T) {
	tool := requireFFmpegTool(t)
	avi := synthTestAVI(t)
	aviBase64 := base64.StdEncoding.EncodeToString(avi)
	reference := []byte("\x89PNG\r\n\x1a\nreference-image-bytes")

	var sawInitImage atomic.Bool
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/upload/image":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"ref.png","subfolder":"","type":"input"}`))
		case "/sdapi/v1/img2img":
			var payload struct {
				InitImages []string `json:"init_images"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("invalid img2img body: %v", err)
			}
			if len(payload.InitImages) != 1 || payload.InitImages[0] != base64.StdEncoding.EncodeToString(reference) {
				t.Fatalf("img2img did not carry the uploaded reference: %#v", payload.InitImages)
			}
			sawInitImage.Store(true)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"images":[],"animated":true,"extra_data":"` + aviBase64 + `"}`))
		case "/sdapi/v1/txt2img":
			t.Fatal("an image-to-video workflow must not fall back to txt2img")
		default:
			t.Fatalf("unexpected backend path %s", r.URL.Path)
		}
	}), map[string]string{
		"image": `{"nomodel":true,"sdmodel":"C:\\models\\dream.safetensors"}`,
	})
	service.ffmpeg = tool
	warmUpActiveImageModel(t, service, "image-dream")

	uploadRecorder := httptest.NewRecorder()
	service.ServeHTTP(uploadRecorder, uploadImageRequest(t, reference))
	if uploadRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected upload status %d", uploadRecorder.Code)
	}

	submitRecorder := httptest.NewRecorder()
	submitBody := strings.Replace(wanImageToVideoPromptBody, "REFERENCE_NAME", "ref.png", 1)
	submitRequest := httptest.NewRequest(http.MethodPost, "/prompt", strings.NewReader(submitBody))
	submitRequest.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(submitRecorder, submitRequest)
	if submitRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected submit status %d body=%s", submitRecorder.Code, submitRecorder.Body.String())
	}
	var submitted struct {
		PromptID string `json:"prompt_id"`
	}
	if err := json.Unmarshal(submitRecorder.Body.Bytes(), &submitted); err != nil {
		t.Fatal(err)
	}

	pollComfyHistoryUntilComplete(t, service, submitted.PromptID)
	if !sawInitImage.Load() {
		t.Fatal("img2img never received the reference image")
	}
}

// A workflow naming an image this router never stored must be refused at
// submission rather than silently generating text-to-video instead.
func TestComfyVideoRejectsAnUnknownReferenceImage(t *testing.T) {
	tool := requireFFmpegTool(t)
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("backend must not be contacted for an unresolvable reference, got %s", r.URL.Path)
	}), map[string]string{
		"image": `{"nomodel":true,"sdmodel":"C:\\models\\dream.safetensors"}`,
	})
	service.ffmpeg = tool
	warmUpActiveImageModel(t, service, "image-dream")

	recorder := httptest.NewRecorder()
	body := strings.Replace(wanImageToVideoPromptBody, "REFERENCE_NAME", "never-uploaded.png", 1)
	request := httptest.NewRequest(http.MethodPost, "/prompt", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for an unknown reference image, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "not uploaded to this router") {
		t.Fatalf("rejection did not explain the missing upload: %s", recorder.Body.String())
	}
}
