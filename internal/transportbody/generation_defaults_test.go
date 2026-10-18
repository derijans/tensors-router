package transportbody

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"
)

var samplerDefaults = []JSONField{
	{Key: "temperature", Value: json.RawMessage(`0.7`)},
	{Key: "top_p", Value: json.RawMessage(`0.8`)},
}

func transformWithDefaults(source io.Reader, rewrite JSONRewrite) ([]byte, error) {
	return io.ReadAll(NewJSONTransformReadCloser(io.NopCloser(source), rewrite))
}

func decodedWithDefaults(t *testing.T, source string, defaults GenerationDefaultsRewrite) map[string]any {
	t.Helper()
	content, err := transformWithDefaults(strings.NewReader(source), JSONRewrite{GenerationDefaults: &defaults})
	if err != nil {
		t.Fatalf("transform %s: %v", source, err)
	}
	var result map[string]any
	if err := json.Unmarshal(content, &result); err != nil {
		t.Fatalf("transformed body %s is not JSON: %v", content, err)
	}
	return result
}

func TestGenerationDefaultsFillOnlyTheFieldsTheClientLeftOut(t *testing.T) {
	result := decodedWithDefaults(t, `{"model":"m","temperature":0.1}`, GenerationDefaultsRewrite{Defaults: samplerDefaults})

	if result["temperature"] != 0.1 || result["top_p"] != 0.8 || result["model"] != "m" {
		t.Fatalf("result = %v, want the client's temperature kept and top_p added", result)
	}
}

func TestGenerationDefaultsOverwriteReplacesTheClientsValue(t *testing.T) {
	result := decodedWithDefaults(t, `{"temperature":{"nested":[1,2]},"stream":true}`, GenerationDefaultsRewrite{Defaults: samplerDefaults, Overwrite: true})

	if result["temperature"] != 0.7 || result["top_p"] != 0.8 || result["stream"] != true {
		t.Fatalf("result = %v, want the config to win", result)
	}
}

func TestGenerationDefaultsFillAnEmptyRequest(t *testing.T) {
	result := decodedWithDefaults(t, `{}`, GenerationDefaultsRewrite{Defaults: samplerDefaults})

	if result["temperature"] != 0.7 || result["top_p"] != 0.8 {
		t.Fatalf("result = %v", result)
	}
}

func TestGenerationDefaultsSitBesideInjectedChatTemplateKwargs(t *testing.T) {
	content, err := transformWithDefaults(strings.NewReader(`{}`), JSONRewrite{
		ChatTemplateKwargs: &ChatTemplateKwargsRewrite{Configured: json.RawMessage(`{"enable_thinking":true}`), ConfigWins: true},
		GenerationDefaults: &GenerationDefaultsRewrite{Defaults: samplerDefaults},
	})
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(content, &result); err != nil {
		t.Fatalf("body %s is not JSON: %v", content, err)
	}
	if result["temperature"] != 0.7 || result["chat_template_kwargs"] == nil {
		t.Fatalf("result = %v", result)
	}
}

func TestGenerationDefaultsGoIntoOllamaOptions(t *testing.T) {
	rewrite := GenerationDefaultsRewrite{Defaults: samplerDefaults, IntoOllamaOptions: true}
	for name, source := range map[string]string{
		"existing options": `{"model":"m","options":{"temperature":0.1,"num_ctx":4096}}`,
		"no options":       `{"model":"m","temperature":0.1}`,
		"null options":     `{"model":"m","options":null}`,
		"empty options":    `{"options":{}}`,
	} {
		result := decodedWithDefaults(t, source, rewrite)
		options, ok := result["options"].(map[string]any)
		if !ok || options["top_p"] != 0.8 {
			t.Fatalf("%s: result = %v, want top_p inside options", name, result)
		}
		if _, leaked := result["top_p"]; leaked {
			t.Fatalf("%s: result = %v, want nothing added at the root", name, result)
		}
	}
	existing := decodedWithDefaults(t, `{"options":{"temperature":0.1,"num_ctx":4096}}`, rewrite)["options"].(map[string]any)
	if existing["temperature"] != 0.1 || existing["num_ctx"] != float64(4096) {
		t.Fatalf("options = %v, want the client's options kept", existing)
	}
}

func TestGenerationDefaultsLeaveNestedFieldsOfTheSameNameAlone(t *testing.T) {
	result := decodedWithDefaults(t, `{"messages":[{"temperature":"text"}],"extra":{"top_p":1}}`, GenerationDefaultsRewrite{Defaults: samplerDefaults, Overwrite: true})

	if result["extra"].(map[string]any)["top_p"] != float64(1) || result["top_p"] != 0.8 {
		t.Fatalf("result = %v, want only root fields touched", result)
	}
}

func TestGenerationDefaultsSurviveAByteAtATimeStream(t *testing.T) {
	source := `{"model":"m","messages":[{"role":"user","content":"a \"quoted\" } brace"}],"options":{"seed":1}}`
	content, err := transformWithDefaults(iotest.OneByteReader(strings.NewReader(source)), JSONRewrite{GenerationDefaults: &GenerationDefaultsRewrite{Defaults: samplerDefaults, IntoOllamaOptions: true}})
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(content, &result); err != nil {
		t.Fatalf("body %s is not JSON: %v", content, err)
	}
	if result["options"].(map[string]any)["temperature"] != 0.7 {
		t.Fatalf("result = %v", result)
	}
}

func TestGenerationDefaultsRejectAmbiguousOrMalformedInput(t *testing.T) {
	cases := []struct {
		name     string
		source   string
		defaults GenerationDefaultsRewrite
		err      error
	}{
		{"duplicate client field", `{"temperature":0.1,"temperature":0.2}`, GenerationDefaultsRewrite{Defaults: samplerDefaults}, ErrDuplicateGenerationDefault},
		{"duplicate options", `{"options":{},"options":{}}`, GenerationDefaultsRewrite{Defaults: samplerDefaults, IntoOllamaOptions: true}, ErrDuplicateGenerationDefault},
		{"scalar options", `{"options":3}`, GenerationDefaultsRewrite{Defaults: samplerDefaults, IntoOllamaOptions: true}, ErrInvalidGenerationOptions},
		{"malformed configured value", `{}`, GenerationDefaultsRewrite{Defaults: []JSONField{{Key: "temperature", Value: json.RawMessage(`0.7,`)}}}, ErrInvalidGenerationDefault},
		{"unnamed configured value", `{}`, GenerationDefaultsRewrite{Defaults: []JSONField{{Value: json.RawMessage(`1`)}}}, ErrInvalidGenerationDefault},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			defaults := testCase.defaults
			_, err := transformWithDefaults(strings.NewReader(testCase.source), JSONRewrite{GenerationDefaults: &defaults})
			if !errors.Is(err, testCase.err) {
				t.Fatalf("err = %v, want %v", err, testCase.err)
			}
		})
	}
}

func TestOverwrittenClientValueObeysTheTransformationLimit(t *testing.T) {
	oversized := `{"temperature":"` + strings.Repeat("x", int(maxChatTemplateKwargsBytes)) + `"}`

	_, err := transformWithDefaults(strings.NewReader(oversized), JSONRewrite{GenerationDefaults: &GenerationDefaultsRewrite{Defaults: samplerDefaults, Overwrite: true}})

	if !errors.Is(err, ErrChatTemplateKwargsTooLarge) {
		t.Fatalf("err = %v, want the transformation limit", err)
	}
}

func TestClientValueAtTheTransformationLimitIsReplaced(t *testing.T) {
	quotes := 2
	atLimit := `{"temperature":"` + strings.Repeat("x", int(maxChatTemplateKwargsBytes)-quotes) + `"}`

	result := decodedWithDefaults(t, atLimit, GenerationDefaultsRewrite{Defaults: samplerDefaults, Overwrite: true})

	if result["temperature"] != 0.7 {
		t.Fatalf("temperature = %v, want the configured value", result["temperature"])
	}
}
