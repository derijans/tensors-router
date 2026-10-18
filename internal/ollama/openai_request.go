package ollama

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	PathChat     = "/api/chat"
	PathGenerate = "/api/generate"
	PathEmbed    = "/api/embed"

	openAIChatPath        = "/v1/chat/completions"
	openAICompletionsPath = "/v1/completions"
	openAIEmbeddingsPath  = "/v1/embeddings"
)

type responseKind int

const (
	chatResponse responseKind = iota
	generateResponse
	embedResponse
)

type OpenAIRequest struct {
	Path       string
	Body       []byte
	stream     bool
	kind       responseKind
	model      string
	completion bool
}

type SamplerSpelling struct {
	RepeatPenalty string
}

var ollamaOnlyRootFields = []string{"model", "messages", "format", "options", "stream", "keep_alive", "think", "prompt", "system", "images", "raw", "suffix", "template", "context", "input", "truncate"}

var samplerOptionNames = []string{"temperature", "top_p", "top_k", "min_p", "typical_p", "presence_penalty", "frequency_penalty", "repeat_last_n", "seed", "stop"}

func TranslatesPath(path string) bool {
	return path == PathChat || path == PathGenerate || path == PathEmbed
}

func ToOpenAI(path string, body []byte, spelling SamplerSpelling) (OpenAIRequest, error) {
	root, err := decodeObject(body)
	if err != nil {
		return OpenAIRequest{}, fmt.Errorf("ollama request is not a JSON object: %w", err)
	}
	switch path {
	case PathChat:
		return chatToOpenAI(root, spelling)
	case PathGenerate:
		return generateToOpenAI(root, spelling)
	case PathEmbed:
		return embedToOpenAI(root)
	default:
		return OpenAIRequest{}, fmt.Errorf("ollama path %q has no OpenAI equivalent", path)
	}
}

func chatToOpenAI(root map[string]json.RawMessage, spelling SamplerSpelling) (OpenAIRequest, error) {
	var messages []ollamaMessage
	if err := json.Unmarshal(root["messages"], &messages); err != nil {
		return OpenAIRequest{}, fmt.Errorf("ollama messages are invalid: %w", err)
	}
	converted, err := openAIMessages(messages)
	if err != nil {
		return OpenAIRequest{}, err
	}
	translated := passThroughFields(root)
	translated["messages"] = mustMarshal(converted)
	return finishGenerationRequest(root, translated, spelling, chatResponse, openAIChatPath)
}

func generateToOpenAI(root map[string]json.RawMessage, spelling SamplerSpelling) (OpenAIRequest, error) {
	var prompt, system string
	_ = json.Unmarshal(root["prompt"], &prompt)
	_ = json.Unmarshal(root["system"], &system)
	var images []string
	_ = json.Unmarshal(root["images"], &images)
	translated := passThroughFields(root)
	if jsonTrue(root["raw"]) {
		translated["prompt"] = mustMarshal(prompt)
		request, err := finishGenerationRequest(root, translated, spelling, generateResponse, openAICompletionsPath)
		request.completion = true
		return request, err
	}
	messages := []ollamaMessage{}
	if system != "" {
		messages = append(messages, ollamaMessage{Role: "system", Content: system})
	}
	messages = append(messages, ollamaMessage{Role: "user", Content: prompt, Images: images})
	converted, err := openAIMessages(messages)
	if err != nil {
		return OpenAIRequest{}, err
	}
	translated["messages"] = mustMarshal(converted)
	return finishGenerationRequest(root, translated, spelling, generateResponse, openAIChatPath)
}

func embedToOpenAI(root map[string]json.RawMessage) (OpenAIRequest, error) {
	if len(root["input"]) == 0 {
		return OpenAIRequest{}, fmt.Errorf("ollama embed input is required")
	}
	translated := passThroughFields(root)
	translated["model"] = root["model"]
	translated["input"] = root["input"]
	return OpenAIRequest{Path: openAIEmbeddingsPath, Body: mustMarshal(translated), kind: embedResponse, model: modelOf(root)}, nil
}

func finishGenerationRequest(root map[string]json.RawMessage, translated map[string]json.RawMessage, spelling SamplerSpelling, kind responseKind, path string) (OpenAIRequest, error) {
	translated["model"] = root["model"]
	if err := applyOptions(root["options"], translated, spelling); err != nil {
		return OpenAIRequest{}, err
	}
	if err := applyFormat(root["format"], translated); err != nil {
		return OpenAIRequest{}, err
	}
	applyThink(root["think"], translated)
	stream := root["stream"] == nil || jsonTrue(root["stream"])
	translated["stream"] = mustMarshal(stream)
	if stream {
		translated["stream_options"] = json.RawMessage(`{"include_usage":true}`)
	}
	return OpenAIRequest{Path: path, Body: mustMarshal(translated), stream: stream, kind: kind, model: modelOf(root)}, nil
}

func passThroughFields(root map[string]json.RawMessage) map[string]json.RawMessage {
	translated := make(map[string]json.RawMessage, len(root))
	for key, value := range root {
		translated[key] = value
	}
	for _, key := range ollamaOnlyRootFields {
		delete(translated, key)
	}
	return translated
}

func applyOptions(raw json.RawMessage, translated map[string]json.RawMessage, spelling SamplerSpelling) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	options, err := decodeObject(raw)
	if err != nil {
		return fmt.Errorf("ollama options must be an object: %w", err)
	}
	for _, name := range samplerOptionNames {
		if value, present := options[name]; present {
			translated[name] = value
		}
	}
	if value, present := options["repeat_penalty"]; present && spelling.RepeatPenalty != "" {
		translated[spelling.RepeatPenalty] = value
	}
	if value, present := options["num_predict"]; present {
		var limit int64
		if json.Unmarshal(value, &limit) == nil && limit > 0 {
			translated["max_tokens"] = value
		}
	}
	return nil
}

func applyFormat(raw json.RawMessage, translated map[string]json.RawMessage) error {
	if len(raw) == 0 || string(raw) == "null" || string(raw) == `""` {
		return nil
	}
	var named string
	if json.Unmarshal(raw, &named) == nil {
		if named != "json" {
			return fmt.Errorf("ollama format %q is not supported", named)
		}
		translated["response_format"] = json.RawMessage(`{"type":"json_object"}`)
		return nil
	}
	if _, err := decodeObject(raw); err != nil {
		return fmt.Errorf("ollama format must be \"json\" or a JSON schema object")
	}
	translated["response_format"] = mustMarshal(map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "response", "schema": raw}})
	return nil
}

func applyThink(raw json.RawMessage, translated map[string]json.RawMessage) {
	if len(raw) == 0 || string(raw) == "null" {
		return
	}
	var enabled bool
	if json.Unmarshal(raw, &enabled) == nil {
		fillEnableThinking(translated, enabled)
		return
	}
	var effort string
	if json.Unmarshal(raw, &effort) == nil && effort != "" {
		fillEnableThinking(translated, true)
		translated["reasoning_effort"] = mustMarshal(effort)
	}
}

func fillEnableThinking(translated map[string]json.RawMessage, enabled bool) {
	kwargs, err := decodeObject(translated["chat_template_kwargs"])
	if err != nil {
		kwargs = map[string]json.RawMessage{}
	}
	if _, configured := kwargs["enable_thinking"]; configured {
		return
	}
	kwargs["enable_thinking"] = mustMarshal(enabled)
	translated["chat_template_kwargs"] = mustMarshal(kwargs)
}

type ollamaMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	Images    []string         `json:"images,omitempty"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
	ToolName  string           `json:"tool_name,omitempty"`
}

type ollamaToolCall struct {
	Function ollamaFunction `json:"function"`
}

type ollamaFunction struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func openAIMessages(messages []ollamaMessage) ([]map[string]any, error) {
	converted := make([]map[string]any, 0, len(messages))
	var pendingToolCallIDs []string
	for index, message := range messages {
		item := map[string]any{"role": message.Role, "content": message.Content}
		if len(message.Images) > 0 {
			parts, err := contentWithImages(message.Content, message.Images)
			if err != nil {
				return nil, err
			}
			item["content"] = parts
		}
		if len(message.ToolCalls) > 0 {
			calls := make([]map[string]any, 0, len(message.ToolCalls))
			for callIndex, call := range message.ToolCalls {
				id := fmt.Sprintf("call_%d_%d", index, callIndex)
				pendingToolCallIDs = append(pendingToolCallIDs, id)
				calls = append(calls, map[string]any{"id": id, "type": "function", "function": map[string]any{"name": call.Function.Name, "arguments": argumentsString(call.Function.Arguments)}})
			}
			item["tool_calls"] = calls
		}
		if message.Role == "tool" && len(pendingToolCallIDs) > 0 {
			item["tool_call_id"] = pendingToolCallIDs[0]
			pendingToolCallIDs = pendingToolCallIDs[1:]
		}
		if message.ToolName != "" {
			item["name"] = message.ToolName
		}
		converted = append(converted, item)
	}
	return converted, nil
}

func contentWithImages(text string, images []string) ([]map[string]any, error) {
	parts := []map[string]any{}
	if text != "" {
		parts = append(parts, map[string]any{"type": "text", "text": text})
	}
	for _, image := range images {
		mediaType, err := imageMediaType(image)
		if err != nil {
			return nil, err
		}
		parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]string{"url": "data:" + mediaType + ";base64," + image}})
	}
	return parts, nil
}

const sniffedImageBytes = 512

func imageMediaType(encoded string) (string, error) {
	decoder := base64.NewDecoder(base64.StdEncoding, strings.NewReader(encoded))
	head := make([]byte, sniffedImageBytes)
	headLength, err := io.ReadFull(decoder, head)
	if err == nil {
		_, err = io.Copy(io.Discard, decoder)
	}
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) || headLength == 0 {
		return "", fmt.Errorf("ollama image is not base64: %v", err)
	}
	mediaType := http.DetectContentType(head[:headLength])
	if !strings.HasPrefix(mediaType, "image/") {
		return "image/png", nil
	}
	return mediaType, nil
}

func argumentsString(arguments json.RawMessage) string {
	var text string
	if json.Unmarshal(arguments, &text) == nil {
		return text
	}
	if len(arguments) == 0 {
		return "{}"
	}
	return string(arguments)
}

func modelOf(root map[string]json.RawMessage) string {
	var model string
	_ = json.Unmarshal(root["model"], &model)
	return model
}

func jsonTrue(raw json.RawMessage) bool {
	var value bool
	return json.Unmarshal(raw, &value) == nil && value
}

func decodeObject(raw []byte) (map[string]json.RawMessage, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, err
	}
	if object == nil {
		return nil, fmt.Errorf("value is not an object")
	}
	return object, nil
}

func mustMarshal(value any) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return encoded
}
