package analytics

import (
	"bytes"
	"mime"
	"strings"
	"time"
)

func RouteClass(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "unknown"
	}
	if strings.HasPrefix(path, "/v1/chat/") {
		return "/v1/chat/*"
	}
	if strings.HasPrefix(path, "/v1/images/") {
		return "/v1/images/*"
	}
	if strings.HasPrefix(path, "/v1/audio/") {
		return "/v1/audio/*"
	}
	if strings.HasPrefix(path, "/sdapi/v1/") {
		return "/sdapi/v1/*"
	}
	if strings.HasPrefix(path, "/sdcpp/v1/") {
		return "/sdcpp/v1/*"
	}
	if strings.HasPrefix(path, "/api/extra/music/") {
		return "/api/extra/music/*"
	}
	if strings.HasPrefix(path, "/api/extra/") {
		return "/api/extra/*"
	}
	if strings.HasPrefix(path, "/api/") {
		return "/api/*"
	}
	return path
}

func ImageType(path string) string {
	lower := strings.ToLower(path)
	switch {
	case strings.Contains(lower, "img2img"):
		return "img2img"
	case strings.Contains(lower, "txt2img"):
		return "txt2img"
	case strings.Contains(lower, "/images/edits"):
		return "img2img"
	case strings.Contains(lower, "/images/generations"):
		return "txt2img"
	case strings.Contains(lower, "img_gen"):
		return "txt2img"
	case strings.Contains(lower, "vid_gen"):
		return "video"
	default:
		return ""
	}
}

func ApplyRequest(event *Event, path string, body []byte, contentType string) {
	if event == nil {
		return
	}
	if event.Route == "" {
		event.Route = RouteClass(path)
	}
	if event.Section == SectionImage && event.ImageType == "" {
		event.ImageType = ImageType(path)
	}
	if len(bytes.TrimSpace(body)) == 0 || !looksJSON(body, contentType) {
		return
	}
	root, ok := decodeObject(body)
	if !ok {
		return
	}
	if event.Section == SectionImage {
		applyImageRequest(event, root)
	}
}

func ApplyResponse(event *Event, contentType string, body []byte) {
	if event == nil || len(bytes.TrimSpace(body)) == 0 || !looksJSON(body, contentType) {
		return
	}
	root, ok := decodeObject(body)
	if !ok {
		return
	}
	applyTokenResponse(event, root)
	applyFinishReason(event, root)
	if event.Section == SectionImage {
		applyImageResponse(event, root)
	}
	if event.Section == SectionVoice || event.Section == SectionMusic {
		applyAudioResponse(event, root)
	}
}

func ApplyEventStreamData(event *Event, data []byte) {
	if event == nil {
		return
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("[DONE]")) {
		return
	}
	root, ok := decodeObject(trimmed)
	if !ok {
		return
	}
	applyTokenResponse(event, root)
	applyFinishReason(event, root)
}

func StreamPayloadCarriesContent(payload []byte) bool {
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("[DONE]")) {
		return false
	}
	root, ok := decodeObject(trimmed)
	if !ok {
		return false
	}
	return streamContentLength(root) > 0
}

func streamContentLength(root map[string]any) int {
	for _, path := range [][]string{
		{"choices", "0", "delta", "content"},
		{"choices", "0", "delta", "reasoning_content"},
		{"choices", "0", "text"},
		{"response"},
		{"token"},
		{"message", "content"},
		{"delta", "text"},
		{"delta", "thinking"},
	} {
		if value, ok := resolvePath(root, path); ok {
			if text, ok := value.(string); ok && text != "" {
				return len(text)
			}
		}
	}
	return 0
}

func applyFinishReason(event *Event, root map[string]any) {
	if event.FinishReason != "" {
		return
	}
	event.FinishReason = firstString(root,
		[]string{"choices", "0", "finish_reason"},
		[]string{"finish_reason"},
		[]string{"results", "0", "finish_reason"},
		[]string{"done_reason"},
		[]string{"stop_reason"},
		[]string{"delta", "stop_reason"},
		[]string{"response", "status"},
	)
}

func applyImageRequest(event *Event, root map[string]any) {
	if event.ImageWidth == 0 {
		event.ImageWidth = int64(firstNumber(root,
			[]string{"width"},
			[]string{"image_width"},
			[]string{"W"},
		))
	}
	if event.ImageHeight == 0 {
		event.ImageHeight = int64(firstNumber(root,
			[]string{"height"},
			[]string{"image_height"},
			[]string{"H"},
		))
	}
	if event.ImageCount == 0 {
		event.ImageCount = imageRequestCount(root)
	}
	if event.ImageSteps == 0 {
		event.ImageSteps = int64(firstNumber(root,
			[]string{"steps"},
			[]string{"num_inference_steps"},
			[]string{"sample_steps"},
		))
	}
}

func applyImageResponse(event *Event, root map[string]any) {
	if event.ImageCount > 0 {
		return
	}
	for _, path := range [][]string{{"data"}, {"images"}, {"output"}, {"artifacts"}} {
		if values, ok := nestedArray(root, path); ok && len(values) > 0 {
			event.ImageCount = int64(len(values))
			return
		}
	}
}

func applyAudioResponse(event *Event, root map[string]any) {
	if event.AudioSeconds == 0 {
		event.AudioSeconds = firstNumber(root,
			[]string{"duration_seconds"},
			[]string{"audio_duration_seconds"},
			[]string{"audio_length_seconds"},
			[]string{"duration"},
			[]string{"audio_duration"},
			[]string{"audio_length"},
		)
	}
	if event.AudioTokens == 0 {
		event.AudioTokens = int64(firstNumber(root,
			[]string{"audio_tokens"},
			[]string{"usage", "audio_tokens"},
			[]string{"usage", "input_audio_tokens"},
			[]string{"tokens"},
		))
	}
	if event.AudioLanguage == "" {
		event.AudioLanguage = firstString(root, []string{"language"}, []string{"detected_language"})
	}
	if event.AudioTask == "" {
		event.AudioTask = firstString(root, []string{"task"})
	}
}

func applyTokenResponse(event *Event, root map[string]any) {
	if event.InputTokens == 0 {
		event.InputTokens = int64(firstNumber(root,
			[]string{"usage", "prompt_tokens"},
			[]string{"prompt_tokens"},
			[]string{"timings", "prompt_n"},
			[]string{"prompt_eval_count"},
			[]string{"results", "0", "prompt_tokens"},
			[]string{"usage", "input_tokens"},
			[]string{"response", "usage", "input_tokens"},
			[]string{"message", "usage", "input_tokens"},
		))
	}
	if event.OutputTokens == 0 {
		event.OutputTokens = int64(firstNumber(root,
			[]string{"usage", "completion_tokens"},
			[]string{"completion_tokens"},
			[]string{"timings", "predicted_n"},
			[]string{"eval_count"},
			[]string{"results", "0", "completion_tokens"},
			[]string{"usage", "output_tokens"},
			[]string{"response", "usage", "output_tokens"},
			[]string{"message", "usage", "output_tokens"},
		))
	}
	if event.TotalTokens == 0 {
		event.TotalTokens = int64(firstNumber(root,
			[]string{"usage", "total_tokens"},
			[]string{"total_tokens"},
			[]string{"response", "usage", "total_tokens"},
		))
	}
	if event.TokensPerSecond == 0 {
		event.TokensPerSecond = firstNumber(root,
			[]string{"usage", "tokens_per_second"},
			[]string{"tokens_per_second"},
			[]string{"timings", "predicted_per_second"},
			[]string{"predicted_per_second"},
		)
	}
	if event.TokensPerSecond == 0 {
		evalCount := firstNumber(root, []string{"eval_count"})
		evalDuration := firstNumber(root, []string{"eval_duration"})
		if evalCount > 0 && evalDuration >= float64(shortestCredibleEvalDuration) {
			event.TokensPerSecond = evalCount / (evalDuration / float64(time.Second))
		}
	}
}

const shortestCredibleEvalDuration = time.Millisecond

func DeriveTotals(event *Event) {
	deriveTokenTotals(event)
}

func deriveTokenTotals(event *Event) {
	if event.TotalTokens == 0 && (event.InputTokens > 0 || event.OutputTokens > 0) {
		event.TotalTokens = event.InputTokens + event.OutputTokens
	}
	if event.TokensPerSecond == 0 && event.OutputTokens > 0 && event.DurationMS > 0 {
		event.TokensPerSecond = float64(event.OutputTokens) / (float64(event.DurationMS) / 1000)
	}
}

func imageRequestCount(root map[string]any) int64 {
	n := int64(firstNumber(root, []string{"n"}))
	if n > 0 {
		return n
	}
	batchSize := int64(firstNumber(root, []string{"batch_size"}))
	batchCount := int64(firstNumber(root, []string{"n_iter"}, []string{"batch_count"}))
	switch {
	case batchSize > 0 && batchCount > 0:
		return batchSize * batchCount
	case batchSize > 0:
		return batchSize
	default:
		return 0
	}
}

func looksJSON(body []byte, contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err == nil && strings.Contains(strings.ToLower(mediaType), "json") {
		return true
	}
	trimmed := bytes.TrimSpace(body)
	return len(trimmed) > 0 && trimmed[0] == '{'
}


