package ollama

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

type openAIUsage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
}

type openAIToolCall struct {
	Index    int `json:"index"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openAIMessage struct {
	Content          string           `json:"content"`
	ReasoningContent string           `json:"reasoning_content"`
	ToolCalls        []openAIToolCall `json:"tool_calls"`
}

type openAIChoice struct {
	Message      openAIMessage `json:"message"`
	Delta        openAIMessage `json:"delta"`
	Text         string        `json:"text"`
	FinishReason string        `json:"finish_reason"`
}

type openAIGeneration struct {
	Choices []openAIChoice  `json:"choices"`
	Usage   *openAIUsage    `json:"usage"`
	Error   json.RawMessage `json:"error"`
}

type openAIEmbeddings struct {
	Data []struct {
		Index     int             `json:"index"`
		Embedding json.RawMessage `json:"embedding"`
	} `json:"data"`
	Usage *openAIUsage `json:"usage"`
}

func (request OpenAIRequest) TranslateResponse(response *http.Response) *http.Response {
	if response == nil || response.Body == nil || response.StatusCode < 200 || response.StatusCode >= 300 {
		return response
	}
	mediaType := mediaTypeJSON
	if strings.HasPrefix(strings.ToLower(response.Header.Get(headerContentType)), "text/event-stream") {
		mediaType = mediaTypeNDJSON
		response.Body = newEventStreamTranslator(response.Body, request)
	} else {
		response.Body = &wholeBodyTranslator{source: response.Body, translate: request.translateWholeBody}
	}
	response.Header.Set(headerContentType, mediaType)
	response.Header.Del("Content-Length")
	response.ContentLength = -1
	return response
}

func (request OpenAIRequest) translateWholeBody(content []byte) ([]byte, error) {
	if request.kind == embedResponse {
		return request.embeddingsRecord(content)
	}
	var generation openAIGeneration
	if err := json.Unmarshal(content, &generation); err != nil {
		return nil, err
	}
	accumulated := generationText{}
	finishReason := ""
	if len(generation.Choices) > 0 {
		choice := generation.Choices[0]
		accumulated.add(choice.Message, choice.Text)
		finishReason = choice.FinishReason
	}
	return json.Marshal(request.doneRecord(accumulated, finishReason, generation.Usage))
}

func (request OpenAIRequest) embeddingsRecord(content []byte) ([]byte, error) {
	var embeddings openAIEmbeddings
	if err := json.Unmarshal(content, &embeddings); err != nil {
		return nil, err
	}
	sort.SliceStable(embeddings.Data, func(left, right int) bool { return embeddings.Data[left].Index < embeddings.Data[right].Index })
	vectors := make([]json.RawMessage, 0, len(embeddings.Data))
	for _, item := range embeddings.Data {
		vectors = append(vectors, item.Embedding)
	}
	record := map[string]any{"model": request.model, "embeddings": vectors}
	if embeddings.Usage != nil {
		record["prompt_eval_count"] = embeddings.Usage.PromptTokens
	}
	return json.Marshal(record)
}

type generationText struct {
	content   strings.Builder
	thinking  strings.Builder
	toolCalls map[int]*openAIToolCall
}

func (text *generationText) add(message openAIMessage, completionText string) {
	text.content.WriteString(message.Content)
	text.content.WriteString(completionText)
	text.thinking.WriteString(message.ReasoningContent)
	for _, call := range message.ToolCalls {
		if text.toolCalls == nil {
			text.toolCalls = map[int]*openAIToolCall{}
		}
		existing, found := text.toolCalls[call.Index]
		if !found {
			copied := call
			text.toolCalls[call.Index] = &copied
			continue
		}
		existing.Function.Name += call.Function.Name
		existing.Function.Arguments += call.Function.Arguments
	}
}

func (text *generationText) ollamaToolCalls() []ollamaToolCall {
	indexes := make([]int, 0, len(text.toolCalls))
	for index := range text.toolCalls {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	calls := make([]ollamaToolCall, 0, len(indexes))
	for _, index := range indexes {
		call := text.toolCalls[index]
		arguments := json.RawMessage(call.Function.Arguments)
		if !json.Valid(arguments) {
			arguments = mustMarshal(call.Function.Arguments)
		}
		calls = append(calls, ollamaToolCall{Function: ollamaFunction{Name: call.Function.Name, Arguments: arguments}})
	}
	return calls
}

func (request OpenAIRequest) partialRecord(content string, thinking string) map[string]any {
	return request.record(content, thinking, nil)
}

func (request OpenAIRequest) doneRecord(text generationText, finishReason string, usage *openAIUsage) map[string]any {
	record := request.record(text.content.String(), text.thinking.String(), text.ollamaToolCalls())
	record["done"] = true
	record["done_reason"] = ollamaDoneReason(finishReason)
	if usage != nil {
		record["prompt_eval_count"] = usage.PromptTokens
		record["eval_count"] = usage.CompletionTokens
	}
	return record
}

func (request OpenAIRequest) record(content string, thinking string, toolCalls []ollamaToolCall) map[string]any {
	record := map[string]any{"model": request.model, "created_at": time.Now().UTC().Format(time.RFC3339Nano), "done": false}
	if request.kind == generateResponse {
		record["response"] = content
		if thinking != "" {
			record["thinking"] = thinking
		}
		return record
	}
	message := map[string]any{"role": "assistant", "content": content}
	if thinking != "" {
		message["thinking"] = thinking
	}
	if len(toolCalls) > 0 {
		message["tool_calls"] = toolCalls
	}
	record["message"] = message
	return record
}

func ollamaDoneReason(finishReason string) string {
	if finishReason == "length" {
		return "length"
	}
	return "stop"
}

type wholeBodyTranslator struct {
	source     io.ReadCloser
	translate  func([]byte) ([]byte, error)
	translated *bytes.Reader
}

func (body *wholeBodyTranslator) Read(buffer []byte) (int, error) {
	if body.translated == nil {
		content, err := io.ReadAll(body.source)
		if err != nil {
			return 0, err
		}
		translated, err := body.translate(content)
		if err != nil {
			return 0, err
		}
		body.translated = bytes.NewReader(translated)
	}
	return body.translated.Read(buffer)
}

func (body *wholeBodyTranslator) Close() error {
	return body.source.Close()
}
