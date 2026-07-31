package proxy

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
)

func TestAdaptBufferedWhisperRequestForcesVerboseTranslationAndWAV(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	file, err := writer.CreateFormFile("file", "sample.wav")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.Write(testWAVBytes())
	_ = writer.WriteField("model", "voice")
	_ = writer.WriteField("response_format", "srt")
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, "/v1/audio/translations", bytes.NewReader(body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	adapted, err := adaptBufferedWhisperRequest(request, body.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if request.Header.Get("X-Tensors-Whisper-Response-Format") != "srt" {
		t.Fatalf("response format was not retained")
	}
	values := readMultipartValues(t, adapted, request.Header.Get("Content-Type"))
	if values["response_format"] != "verbose_json" || values["translate"] != "true" {
		t.Fatalf("unexpected forced fields %#v", values)
	}
	if _, found := values["model"]; found {
		t.Fatalf("router model selector leaked to whisper-server")
	}
}

func TestAdaptKoboldTranscriptionStreamsBase64AndRejectsInvalidAudio(t *testing.T) {
	audio := base64.StdEncoding.EncodeToString(testWAVBytes())
	request, _ := http.NewRequest(http.MethodPost, "/api/extra/transcribe", nil)
	request.Header.Set("Content-Type", "application/json")
	adapted, err := adaptBufferedWhisperRequest(request, []byte(`{"audio_data":"`+audio+`","prompt":"hello","langcode":"lv","suppress_non_speech":true}`))
	if err != nil {
		t.Fatal(err)
	}
	values := readMultipartValues(t, adapted, request.Header.Get("Content-Type"))
	if values["language"] != "lv" || values["prompt"] != "hello" || values["suppress_nst"] != "true" {
		t.Fatalf("unexpected Kobold fields %#v", values)
	}
	invalidRequest, _ := http.NewRequest(http.MethodPost, "/api/extra/transcribe", nil)
	invalidRequest.Header.Set("Content-Type", "application/json")
	if _, err := adaptBufferedWhisperRequest(invalidRequest, []byte(`{"audio_data":"not-base64!"}`)); err == nil {
		t.Fatal("expected malformed base64 rejection")
	}
}

func TestAdaptWhisperResponseFormats(t *testing.T) {
	payload := `{"task":"transcribe","language":"lv","duration":1.25,"text":" hello ","segments":[{"start":0,"end":1.25,"text":" hello "}]}`
	for _, format := range []string{"json", "verbose_json", "text", "srt", "vtt"} {
		t.Run(format, func(t *testing.T) {
			response := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(payload))}
			adapted, err := adaptWhisperResponse(response, format)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(adapted.Body)
			if err != nil {
				t.Fatal(err)
			}
			if len(body) == 0 || adapted.Header.Get("X-Tensors-Audio-Language") != "lv" || adapted.Header.Get("X-Tensors-Audio-Task") != "transcribe" {
				t.Fatalf("unexpected adapted response format=%s body=%q headers=%v", format, body, adapted.Header)
			}
		})
	}
}

func readMultipartValues(t *testing.T, body []byte, contentType string) map[string]string {
	t.Helper()
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatal(err)
	}
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	values := map[string]string{}
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			return values
		}
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(part)
		if err != nil {
			t.Fatal(err)
		}
		values[part.FormName()] = string(content)
	}
}

func testWAVBytes() []byte {
	return []byte{'R', 'I', 'F', 'F', 4, 0, 0, 0, 'W', 'A', 'V', 'E', 'd', 'a', 't', 'a'}
}
