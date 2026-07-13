package transportbody

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestJSONTransformerMergesChatTemplateKwargsWithBothPrecedences(t *testing.T) {
	configured := json.RawMessage(`{"enable_thinking":true,"mode":"configured"}`)
	source := `{"model":"public","chat_template_kwargs":{"mode":"client","client_only":7}}`

	configWins := transformedChatTemplateRequest(t, source, configured, true)
	if configWins["model"] != "local" {
		t.Fatalf("selector was not rewritten: %#v", configWins)
	}
	configValues := configWins["chat_template_kwargs"].(map[string]any)
	if configValues["enable_thinking"] != true || configValues["mode"] != "configured" || configValues["client_only"] != float64(7) {
		t.Fatalf("config precedence merge failed: %#v", configValues)
	}

	clientWins := transformedChatTemplateRequest(t, source, configured, false)
	clientValues := clientWins["chat_template_kwargs"].(map[string]any)
	if clientValues["enable_thinking"] != true || clientValues["mode"] != "client" || clientValues["client_only"] != float64(7) {
		t.Fatalf("client precedence merge failed: %#v", clientValues)
	}
}

func TestJSONTransformerInjectsConfiguredKwargsForMissingAndNullValues(t *testing.T) {
	configured := json.RawMessage(`{"enable_thinking":false}`)
	for _, source := range []string{
		`{"model":"public","messages":[]}`,
		`{"model":"public","chat_template_kwargs":null}`,
	} {
		result := transformedChatTemplateRequest(t, source, configured, true)
		values, ok := result["chat_template_kwargs"].(map[string]any)
		if !ok || values["enable_thinking"] != false {
			t.Fatalf("configured kwargs were not injected for %s: %#v", source, result)
		}
	}
}

func TestJSONTransformerRejectsInvalidOrDuplicatedChatTemplateKwargs(t *testing.T) {
	configured := json.RawMessage(`{"enable_thinking":true}`)
	cases := []struct {
		name   string
		source string
		err    error
	}{
		{
			name:   "scalar",
			source: `{"model":"public","chat_template_kwargs":true}`,
			err:    ErrInvalidChatTemplateKwargs,
		},
		{
			name:   "duplicate root field",
			source: `{"model":"public","chat_template_kwargs":{},"chat_template_kwargs":{}}`,
			err:    ErrDuplicateChatTemplateKwargs,
		},
		{
			name:   "duplicate nested field",
			source: `{"model":"public","chat_template_kwargs":{"enabled":true,"enabled":false}}`,
			err:    ErrInvalidChatTemplateKwargs,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			reader := NewJSONTransformReadCloser(io.NopCloser(strings.NewReader(testCase.source)), JSONRewrite{
				ChatTemplateKwargs: &ChatTemplateKwargsRewrite{Configured: configured, ConfigWins: true},
			})
			_, err := io.ReadAll(reader)
			if !errors.Is(err, testCase.err) {
				t.Fatalf("unexpected error %v", err)
			}
		})
	}
}

func transformedChatTemplateRequest(t *testing.T, source string, configured json.RawMessage, configWins bool) map[string]any {
	t.Helper()
	reader := NewJSONTransformReadCloser(io.NopCloser(strings.NewReader(source)), JSONRewrite{
		Replacements: map[string]StringReplacement{
			PathModel: {To: "local"},
		},
		ChatTemplateKwargs: &ChatTemplateKwargsRewrite{Configured: configured, ConfigWins: configWins},
	})
	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(content, &result); err != nil {
		t.Fatal(err)
	}
	return result
}
