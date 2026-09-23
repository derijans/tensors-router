package transportbody

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

const fuzzInputLimit = 64 * 1024

var jsonProcessorSeeds = []string{
	`{"model":"public","messages":[{"role":"user","content":"hello <b>&</b>"}],"stream":true,"max_tokens":256,"temperature":0.7}`,
	`{"model":"public","prompt":"once upon","n":2,"logprobs":null,"stop":["\n","END"]}`,
	`{"model":"llama3","messages":[{"role":"user","content":"hi","images":["aGVsbG8="]}],"options":{"num_ctx":8192}}`,
	`{"model":"embed","input":["first","second"],"encoding_format":"float"}`,
	`{"sd_model_checkpoint":"cc11-ff","prompt":"cat","width":512,"height":768,"batch_size":1,"n_iter":2}`,
	`{"prompt":"cat","override_settings":{"sd_model_checkpoint":"public-image","CLIP_stop_at_last_layers":2}}`,
	`{"input":"hello"}`,
	`{"model":"a","model":"b"}`,
	`{"object":"model","model":"local"}`,
	`{"mod\u0065l":"escaped-key","model":"x\"y\\z\u00e9"}`,
	`{"model":"<script>&amp;</script>","choices":[{"message":{"content":"a<b>c&d"}}]}`,
	` { "model" : "spaced" , "a" : [ 1 , -2.5e10 , true , false , null ] } `,
	`{"id":"chatcmpl-1","object":"chat.completion","created":1700000000,"model":"backend-local","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4}}`,
	`{"model":null}`,
	`{"model":5}`,
	`[{"model":"array-root"}]`,
	`{"model":"unterminated`,
	`{"model":"trailing"} {}`,
	`{"a":"bad\q"}`,
	`{"a":1,}`,
	`{"a":[1,]}`,
	`{"a":"\ud800"}`,
	`{"\ud83d\ude00":"emoji-key","model":"\ud83d\ude00"}`,
	"{\"a\":\"\xff\xfe\"}",
}

func FuzzProcessJSONMatchesEncodingJSON(f *testing.F) {
	for _, seed := range jsonProcessorSeeds {
		f.Add([]byte(seed), "public-model")
	}
	f.Fuzz(func(t *testing.T, input []byte, replacement string) {
		if len(input) > fuzzInputLimit || len(replacement) > selectorValueLimit/6 {
			return
		}
		checkInspection(t, input)
		checkResponseRewrite(t, input, replacement)
	})
}

func checkInspection(t *testing.T, input []byte) {
	t.Helper()
	var output bytes.Buffer
	fields, err := processJSON(bytes.NewReader(input), &output, JSONRewrite{})
	root, accepted := decodeObjectRoot(input)
	if !accepted {
		if err == nil {
			t.Fatalf("processor accepted input encoding/json rejects: %q", input)
		}
		return
	}
	if err != nil {
		requireDocumentedRejection(t, input, err)
		return
	}
	if !bytes.Equal(output.Bytes(), input) {
		t.Fatalf("inspection changed the body:\ninput  %q\noutput %q", input, output.Bytes())
	}
	model, present := root[PathModel].(string)
	if fields.ModelSet != present || fields.Model != strings.TrimSpace(model) {
		t.Fatalf("selector = %q (set %v), encoding/json sees %q (present %v) in %q", fields.Model, fields.ModelSet, model, present, input)
	}
}

func checkResponseRewrite(t *testing.T, input []byte, replacement string) {
	t.Helper()
	var output bytes.Buffer
	_, err := processJSON(bytes.NewReader(input), &output, JSONRewrite{
		Replacements: map[string]StringReplacement{PathModel: {To: replacement}},
		EscapeHTML:   true,
	})
	root, accepted := decodeObjectRoot(input)
	if !accepted {
		if err == nil {
			t.Fatalf("processor accepted input encoding/json rejects: %q", input)
		}
		return
	}
	if err != nil {
		requireDocumentedRejection(t, input, err)
		return
	}
	rewritten, ok := decodeObjectRoot(output.Bytes())
	if !ok {
		t.Fatalf("rewrite produced invalid JSON:\ninput  %q\noutput %q", input, output.Bytes())
	}
	if _, present := root[PathModel]; present {
		root[PathModel] = jsonRoundTrip(t, replacement)
	}
	if !reflect.DeepEqual(rewritten, root) {
		t.Fatalf("rewrite changed more than the model:\ninput  %q\noutput %q", input, output.Bytes())
	}
	if bytes.ContainsAny(output.Bytes(), "<>&") {
		t.Fatalf("HTML-escaping rewrite left a raw HTML character: %q", output.Bytes())
	}
}

func jsonRoundTrip(t *testing.T, value string) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var decoded string
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}

func decodeObjectRoot(data []byte) (map[string]any, bool) {
	if !json.Valid(data) {
		return nil, false
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var root map[string]any
	if err := decoder.Decode(&root); err != nil || root == nil {
		return nil, false
	}
	return root, true
}

func requireDocumentedRejection(t *testing.T, input []byte, err error) {
	t.Helper()
	if errors.Is(err, ErrSelectorTooLarge) && len(input) > selectorValueLimit {
		return
	}
	if limits := scanLimitsAcrossDuplicateKeys(input); errors.Is(err, ErrInvalidJSON) && (limits.nonStringSelector || limits.depth > maxJSONNesting || limits.longestNumber > 129) {
		return
	}
	t.Fatalf("processor rejected valid JSON within its limits (%v): %q", err, input)
}

type processorLimits struct {
	depth             int
	longestNumber     int
	nonStringSelector bool
}

type scannedContainer struct {
	object       bool
	expectingKey bool
	key          string
	selectorKeys map[string]bool
}

var rootSelectorKeys = map[string]bool{PathModel: true, PathImageModel: true}
var overrideSelectorKeys = map[string]bool{PathImageModel: true}

func scanLimitsAcrossDuplicateKeys(input []byte) processorLimits {
	var limits processorLimits
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.UseNumber()
	var stack []scannedContainer
	for {
		token, err := decoder.Token()
		if err != nil {
			return limits
		}
		var parent *scannedContainer
		if len(stack) > 0 {
			parent = &stack[len(stack)-1]
		}
		if delimiter, isDelimiter := token.(json.Delim); isDelimiter && (delimiter == '}' || delimiter == ']') {
			stack = stack[:len(stack)-1]
			continue
		}
		if parent != nil && parent.object && parent.expectingKey {
			parent.key, _ = token.(string)
			parent.expectingKey = false
			continue
		}
		if parent != nil && parent.object {
			if _, isString := token.(string); parent.selectorKeys[parent.key] && !isString {
				limits.nonStringSelector = true
			}
			parent.expectingKey = true
		}
		switch typed := token.(type) {
		case json.Delim:
			child := scannedContainer{object: typed == '{', expectingKey: typed == '{'}
			if child.object && len(stack) == 0 {
				child.selectorKeys = rootSelectorKeys
			} else if child.object && len(stack) == 1 && parent.object && parent.key == "override_settings" {
				child.selectorKeys = overrideSelectorKeys
			}
			stack = append(stack, child)
			limits.depth = max(limits.depth, len(stack))
		case json.Number:
			limits.longestNumber = max(limits.longestNumber, len(typed))
		}
	}
}
