package ollama

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

var llamaSpelling = SamplerSpelling{RepeatPenalty: "repeat_penalty"}

const pngHeader = "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"

func translatedBody(t *testing.T, path string, body string, spelling SamplerSpelling) (OpenAIRequest, map[string]any) {
	t.Helper()
	request, err := ToOpenAI(path, []byte(body), spelling)
	if err != nil {
		t.Fatalf("ToOpenAI(%s): %v", body, err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(request.Body, &decoded); err != nil {
		t.Fatalf("translated body %s is not JSON: %v", request.Body, err)
	}
	return request, decoded
}

func TestChatBecomesAnOpenAIChatCompletion(t *testing.T) {
	image := base64.StdEncoding.EncodeToString([]byte(pngHeader + strings.Repeat("x", 600)))
	body := `{"model":"m","keep_alive":"5m","temperature":0.4,"messages":[{"role":"user","content":"what is it?","images":["` + image + `"]}],` +
		`"options":{"num_predict":16,"num_ctx":4096,"top_k":5,"repeat_penalty":1.2,"stop":["\n"]},"format":"json","think":false}`

	request, translated := translatedBody(t, PathChat, body, SamplerSpelling{RepeatPenalty: "repetition_penalty"})

	if request.Path != openAIChatPath || !request.stream {
		t.Fatalf("request = %+v, want a streamed chat completion (Ollama streams by default)", request)
	}
	parts := translated["messages"].([]any)[0].(map[string]any)["content"].([]any)
	imageURL := parts[1].(map[string]any)["image_url"].(map[string]any)["url"].(string)
	if !strings.HasPrefix(imageURL, "data:image/png;base64,") {
		t.Fatalf("image url = %.40s, want a PNG data URL", imageURL)
	}
	if translated["max_tokens"] != float64(16) || translated["top_k"] != float64(5) || translated["repetition_penalty"] != 1.2 || translated["temperature"] != 0.4 {
		t.Fatalf("samplers = %v", translated)
	}
	for _, dropped := range []string{"num_ctx", "options", "keep_alive", "format", "think", "repeat_penalty"} {
		if _, present := translated[dropped]; present {
			t.Fatalf("translated request kept %s: %v", dropped, translated)
		}
	}
	if translated["response_format"].(map[string]any)["type"] != "json_object" {
		t.Fatalf("response_format = %v", translated["response_format"])
	}
	if translated["chat_template_kwargs"].(map[string]any)["enable_thinking"] != false {
		t.Fatalf("chat_template_kwargs = %v", translated["chat_template_kwargs"])
	}
	if translated["stream_options"].(map[string]any)["include_usage"] != true {
		t.Fatalf("stream_options = %v, want usage for the final Ollama record", translated["stream_options"])
	}
}

func TestThinkNeverOverridesTheConfigsJinjaKwargs(t *testing.T) {
	_, translated := translatedBody(t, PathChat, `{"model":"m","messages":[],"think":true,"chat_template_kwargs":{"enable_thinking":false,"mode":"x"}}`, llamaSpelling)

	kwargs := translated["chat_template_kwargs"].(map[string]any)
	if kwargs["enable_thinking"] != false || kwargs["mode"] != "x" {
		t.Fatalf("chat_template_kwargs = %v, want the configured value kept", kwargs)
	}
}

func TestChatToolCallsGetIDsThatToolAnswersReferTo(t *testing.T) {
	body := `{"model":"m","stream":false,"messages":[` +
		`{"role":"assistant","content":"","tool_calls":[{"function":{"name":"weather","arguments":{"city":"Riga"}}}]},` +
		`{"role":"tool","content":"sunny","tool_name":"weather"}]}`

	_, translated := translatedBody(t, PathChat, body, llamaSpelling)

	messages := translated["messages"].([]any)
	call := messages[0].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)
	if call["function"].(map[string]any)["arguments"] != `{"city":"Riga"}` {
		t.Fatalf("tool call = %v, want arguments as a JSON string", call)
	}
	if messages[1].(map[string]any)["tool_call_id"] != call["id"] {
		t.Fatalf("tool answer = %v, want it tied to call %v", messages[1], call["id"])
	}
}

func TestGenerateBecomesChatOrRawCompletion(t *testing.T) {
	request, chat := translatedBody(t, PathGenerate, `{"model":"m","prompt":"hi","system":"be brief","stream":false,"options":{"temperature":0.1}}`, llamaSpelling)
	messages := chat["messages"].([]any)
	if request.Path != openAIChatPath || len(messages) != 2 || messages[0].(map[string]any)["role"] != "system" || chat["temperature"] != 0.1 {
		t.Fatalf("generate translated to %s %v", request.Path, chat)
	}

	raw, completion := translatedBody(t, PathGenerate, `{"model":"m","prompt":"once upon","raw":true,"stream":false}`, llamaSpelling)
	if raw.Path != openAICompletionsPath || completion["prompt"] != "once upon" {
		t.Fatalf("raw generate translated to %s %v", raw.Path, completion)
	}
}

func TestInvalidOllamaRequestsAreRejected(t *testing.T) {
	for name, body := range map[string]string{
		"not an object":    `[1]`,
		"bad messages":     `{"model":"m","messages":"hi"}`,
		"bad options":      `{"model":"m","messages":[],"options":3}`,
		"unknown format":   `{"model":"m","messages":[],"format":"yaml"}`,
		"embed no input":   `{"model":"m"}`,
		"image not base64": `{"model":"m","messages":[{"role":"user","content":"x","images":["%%%"]}]}`,
	} {
		path := PathChat
		if name == "embed no input" {
			path = PathEmbed
		}
		if _, err := ToOpenAI(path, []byte(body), llamaSpelling); err == nil {
			t.Fatalf("%s: expected an error", name)
		}
	}
}

func backendResponse(contentType string, body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(body))}
}

func readRecords(t *testing.T, response *http.Response) []map[string]any {
	t.Helper()
	content, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(content)), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("line %q is not JSON: %v", line, err)
		}
		records = append(records, record)
	}
	return records
}

func TestChatCompletionBecomesOneOllamaRecord(t *testing.T) {
	request, _ := translatedBody(t, PathChat, `{"model":"m","stream":false,"messages":[]}`, llamaSpelling)
	response := request.TranslateResponse(backendResponse("application/json",
		`{"choices":[{"message":{"content":"hi","reasoning_content":"hmm","tool_calls":[{"index":0,"function":{"name":"f","arguments":"{\"a\":1}"}}]},"finish_reason":"length"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`))

	record := readRecords(t, response)[0]
	message := record["message"].(map[string]any)
	if message["content"] != "hi" || message["thinking"] != "hmm" || record["done"] != true || record["done_reason"] != "length" || record["eval_count"] != float64(2) || record["model"] != "m" {
		t.Fatalf("record = %v", record)
	}
	arguments := message["tool_calls"].([]any)[0].(map[string]any)["function"].(map[string]any)["arguments"]
	if arguments.(map[string]any)["a"] != float64(1) {
		t.Fatalf("tool arguments = %v, want an object", arguments)
	}
	if response.Header.Get("Content-Type") != mediaTypeJSON {
		t.Fatalf("content type = %q", response.Header.Get("Content-Type"))
	}
}

func TestEventStreamBecomesOllamaNDJSON(t *testing.T) {
	request, _ := translatedBody(t, PathGenerate, `{"model":"m","prompt":"hi"}`, llamaSpelling)
	stream := "data: {\"choices\":[{\"delta\":{\"content\":\"He\"}}]}\n\n" +
		": keep-alive\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"llo\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":2}}\n\n" +
		"data: [DONE]\n\n"

	response := request.TranslateResponse(backendResponse("text/event-stream", stream))
	records := readRecords(t, response)

	if len(records) != 3 || records[0]["response"] != "He" || records[1]["response"] != "llo" || records[0]["done"] != false {
		t.Fatalf("records = %v", records)
	}
	final := records[2]
	if final["done"] != true || final["done_reason"] != "stop" || final["prompt_eval_count"] != float64(4) || final["response"] != "" {
		t.Fatalf("final record = %v", final)
	}
	if response.Header.Get("Content-Type") != mediaTypeNDJSON {
		t.Fatalf("content type = %q", response.Header.Get("Content-Type"))
	}
}

func TestStreamedToolCallsArriveWhole(t *testing.T) {
	request, _ := translatedBody(t, PathChat, `{"model":"m","messages":[]}`, llamaSpelling)
	stream := "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"name\":\"wea\",\"arguments\":\"{\\\"ci\"}}]}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"name\":\"ther\",\"arguments\":\"ty\\\":1}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\n" +
		"data: [DONE]\n\n"

	records := readRecords(t, request.TranslateResponse(backendResponse("text/event-stream", stream)))

	call := records[len(records)-1]["message"].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)["function"].(map[string]any)
	if call["name"] != "weather" || call["arguments"].(map[string]any)["city"] != float64(1) {
		t.Fatalf("tool call = %v", call)
	}
}

func TestStreamErrorEndsTheStream(t *testing.T) {
	request, _ := translatedBody(t, PathChat, `{"model":"m","messages":[]}`, llamaSpelling)
	stream := "data: {\"choices\":[{\"delta\":{\"content\":\"a\"}}]}\n\ndata: {\"error\":{\"message\":\"context exceeded\"}}\n\n"

	records := readRecords(t, request.TranslateResponse(backendResponse("text/event-stream", stream)))

	if len(records) != 2 || records[1]["error"] != "context exceeded" {
		t.Fatalf("records = %v", records)
	}
}

func TestBackendErrorsPassThroughUntouched(t *testing.T) {
	request, _ := translatedBody(t, PathChat, `{"model":"m","messages":[]}`, llamaSpelling)
	failed := &http.Response{StatusCode: http.StatusBadRequest, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":"bad"}`))}

	translated := request.TranslateResponse(failed)
	content, _ := io.ReadAll(translated.Body)

	if string(content) != `{"error":"bad"}` {
		t.Fatalf("body = %s, want the backend error unchanged", content)
	}
}

func TestEmbedRoundTrip(t *testing.T) {
	request, translated := translatedBody(t, PathEmbed, `{"model":"m","input":["a","b"],"truncate":true}`, llamaSpelling)
	if request.Path != openAIEmbeddingsPath || len(translated["input"].([]any)) != 2 {
		t.Fatalf("embed translated to %s %v", request.Path, translated)
	}

	response := request.TranslateResponse(backendResponse("application/json", `{"data":[{"index":1,"embedding":[0.2]},{"index":0,"embedding":[0.1]}],"usage":{"prompt_tokens":2}}`))
	record := readRecords(t, response)[0]

	embeddings := record["embeddings"].([]any)
	if embeddings[0].([]any)[0] != 0.1 || embeddings[1].([]any)[0] != 0.2 || record["prompt_eval_count"] != float64(2) {
		t.Fatalf("record = %v, want vectors in input order", record)
	}
}
