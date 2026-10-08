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
	applyLatestTokenReport(event, root)
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
	applyLatestTokenReport(event, root)
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

func applyLatestTokenReport(event *Event, root map[string]any) {
	if input := reportedInputTokens(root); input > 0 {
		event.InputTokens = input
	}
	if output := reportedOutputTokens(root); output > 0 {
		event.OutputTokens = output
	}
	if total := int64(firstNumber(root,
		[]string{"usage", "total_tokens"},
		[]string{"total_tokens"},
		[]string{"response", "usage", "total_tokens"},
	)); total > 0 {
		event.TotalTokens = total
	}
	if tokensPerSecond := reportedTokensPerSecond(root); tokensPerSecond > 0 {
		event.TokensPerSecond = tokensPerSecond
	}
	if promptTokensPerSecond := reportedPromptTokensPerSecond(root); promptTokensPerSecond > 0 {
		event.PromptTokensPS = promptTokensPerSecond
	}
}

func reportedInputTokens(root map[string]any) int64 {
	if usage := int64(firstNumber(root, []string{"usage", "prompt_tokens"}, []string{"prompt_tokens"})); usage > 0 {
		return usage
	}
	if evaluated, ok := numberAt(root, []string{"timings", "prompt_n"}); ok {
		cached := firstNumber(root, []string{"timings", "cache_n"})
		if evaluated+cached > 0 {
			return int64(evaluated + cached)
		}
	}
	return int64(firstNumber(root,
		[]string{"prompt_eval_count"},
		[]string{"results", "0", "prompt_tokens"},
		[]string{"usage", "input_tokens"},
		[]string{"response", "usage", "input_tokens"},
		[]string{"message", "usage", "input_tokens"},
	))
}

func reportedOutputTokens(root map[string]any) int64 {
	return int64(firstNumber(root,
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

type throughputReport struct {
	reportedRate      [][]string
	llamaTimingMS     []string
	llamaTimingRate   []string
	bareRate          []string
	ollamaCount       []string
	ollamaNanoseconds []string
}

var generationThroughput = throughputReport{
	reportedRate:      [][]string{{"usage", "tokens_per_second"}, {"tokens_per_second"}},
	llamaTimingMS:     []string{"timings", "predicted_ms"},
	llamaTimingRate:   []string{"timings", "predicted_per_second"},
	bareRate:          []string{"predicted_per_second"},
	ollamaCount:       []string{"eval_count"},
	ollamaNanoseconds: []string{"eval_duration"},
}

var promptThroughput = throughputReport{
	reportedRate:      [][]string{{"usage", "prompt_tokens_per_second"}},
	llamaTimingMS:     []string{"timings", "prompt_ms"},
	llamaTimingRate:   []string{"timings", "prompt_per_second"},
	bareRate:          []string{"prompt_per_second"},
	ollamaCount:       []string{"prompt_eval_count"},
	ollamaNanoseconds: []string{"prompt_eval_duration"},
}

func reportedTokensPerSecond(root map[string]any) float64 {
	return generationThroughput.tokensPerSecond(root)
}

func reportedPromptTokensPerSecond(root map[string]any) float64 {
	return promptThroughput.tokensPerSecond(root)
}

func (report throughputReport) tokensPerSecond(root map[string]any) float64 {
	if reported := firstNumber(root, report.reportedRate...); reported > 0 {
		return reported
	}
	timingMS, timingMSReported := numberAt(root, report.llamaTimingMS)
	if !timingMSReported || time.Duration(timingMS*float64(time.Millisecond)) >= shortestCredibleEvalDuration {
		if reported := firstNumber(root, report.llamaTimingRate); reported > 0 {
			return reported
		}
	}
	if reported := firstNumber(root, report.bareRate); reported > 0 {
		return reported
	}
	count := firstNumber(root, report.ollamaCount)
	nanoseconds := firstNumber(root, report.ollamaNanoseconds)
	if count > 0 && nanoseconds >= float64(shortestCredibleEvalDuration) {
		return count / (nanoseconds / float64(time.Second))
	}
	return 0
}

const shortestCredibleEvalDuration = time.Millisecond

func DeriveTotals(event *Event) {
	deriveTokenTotals(event)
}

func deriveTokenTotals(event *Event) {
	if event.TotalTokens == 0 && (event.InputTokens > 0 || event.OutputTokens > 0) {
		event.TotalTokens = event.InputTokens + event.OutputTokens
	}
	if event.TokensPerSecond == 0 && event.OutputTokens > 0 {
		event.TokensPerSecond = derivedTokensPerSecond(event)
	}
	if event.PromptTokensPS == 0 && event.InputTokens > 0 {
		event.PromptTokensPS = estimatedPromptTokensPerSecond(event)
	}
}

func derivedTokensPerSecond(event *Event) float64 {
	generationMS := workDurationMS(event)
	if event.DecodeMS > 0 {
		generationMS = event.DecodeMS
	}
	if generationMS <= 0 {
		return 0
	}
	return float64(event.OutputTokens) / (float64(generationMS) / 1000)
}

func workDurationMS(event *Event) int64 {
	if event.WorkStartedAt.IsZero() || event.FinishedAt.IsZero() || !event.FinishedAt.After(event.WorkStartedAt) {
		return event.DurationMS
	}
	return event.FinishedAt.Sub(event.WorkStartedAt).Milliseconds()
}

func estimatedPromptTokensPerSecond(event *Event) float64 {
	if event.WorkStartedAt.IsZero() || event.TTFTMS <= 0 {
		return 0
	}
	firstTokenAt := event.StartedAt.Add(time.Duration(event.TTFTMS) * time.Millisecond)
	prefill := firstTokenAt.Sub(event.WorkStartedAt)
	if prefill < shortestCredibleEvalDuration {
		return 0
	}
	return float64(event.InputTokens) / prefill.Seconds()
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
