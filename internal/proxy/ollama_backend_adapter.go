package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"

	"tensors-router/internal/ollama"
)

func backendLacksOllamaAPI(mode string, path string) bool {
	return mode != BackendModeKobold && ollama.TranslatesPath(path)
}

func samplerSpellingFor(mode string) ollama.SamplerSpelling {
	if mode == BackendModeVLLM {
		return ollama.SamplerSpelling{RepeatPenalty: "repetition_penalty"}
	}
	return ollama.SamplerSpelling{RepeatPenalty: "repeat_penalty"}
}

func (service *Service) forwardOllamaAsOpenAI(runtime *backendRuntime, ctx context.Context, original *http.Request, body []byte) (*http.Response, error) {
	translated, err := ollama.ToOpenAI(original.URL.Path, body, samplerSpellingFor(runtime.mode))
	if err != nil {
		return invalidOllamaRequestResponse(err), nil
	}
	response, err := service.forwardToPath(runtime, ctx, original, translated.Path, translated.Body)
	if err != nil {
		return nil, err
	}
	return translated.TranslateResponse(response), nil
}

func invalidOllamaRequestResponse(err error) *http.Response {
	body, _ := json.Marshal(map[string]string{"error": err.Error()})
	return &http.Response{
		StatusCode:    http.StatusBadRequest,
		Header:        http.Header{headerContentType: []string{"application/json"}},
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
	}
}
