package transportbody

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

var modelRewrite = JSONRewrite{
	Replacements: map[string]StringReplacement{PathModel: {To: "backend-local"}},
	EscapeHTML:   true,
}

func chatCompletionBody(promptBytes int) []byte {
	return []byte(`{"model":"public","stream":true,"messages":[{"role":"system","content":"You are terse."},{"role":"user","content":"` +
		strings.Repeat("word ", promptBytes/5) + `"}],"temperature":0.7,"max_tokens":512}`)
}

func imagePayloadBody(payloadBytes int) []byte {
	return []byte(`{"model":"public","prompt":"describe","images":["data:image/png;base64,` + strings.Repeat("A", payloadBytes) + `"]}`)
}

func streamedChunkBody() []byte {
	return []byte(`{"id":"chatcmpl-1","object":"chat.completion.chunk","created":1700000000,"model":"backend-local","choices":[{"index":0,"delta":{"content":"token"},"finish_reason":null}]}`)
}

func BenchmarkStreamRewriteChatCompletion4KiB(b *testing.B) {
	benchmarkStreamRewrite(b, chatCompletionBody(4*1024))
}

func BenchmarkStreamRewriteImagePayload64MiB(b *testing.B) {
	benchmarkStreamRewrite(b, imagePayloadBody(int(64*MiB)))
}

func BenchmarkEncodingJSONRewriteImagePayload64MiB(b *testing.B) {
	body := imagePayloadBody(int(64 * MiB))
	b.SetBytes(int64(len(body)))
	b.ReportAllocs()
	for b.Loop() {
		var decoded map[string]any
		if err := json.Unmarshal(body, &decoded); err != nil {
			b.Fatal(err)
		}
		decoded[PathModel] = "backend-local"
		if _, err := json.Marshal(decoded); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRewriteJSONStreamedChunk(b *testing.B) {
	body := streamedChunkBody()
	b.SetBytes(int64(len(body)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := RewriteJSON(body, modelRewrite); err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkStreamRewrite(b *testing.B, body []byte) {
	b.SetBytes(int64(len(body)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := processJSON(bytes.NewReader(body), io.Discard, modelRewrite); err != nil {
			b.Fatal(err)
		}
	}
}
