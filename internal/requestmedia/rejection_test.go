package requestmedia

import "testing"

func TestBackendRefusalsOfImagesAreRecognised(t *testing.T) {
	cases := map[string]string{
		"llama-server": `{"error":{"code":400,"message":"image input is not supported - hint: if this is unexpected, you may need to provide the mmproj","type":"invalid_request_error"}}`,
		"vllm limit":   `{"error":{"message":"At most 0 image(s) may be provided in one prompt.","type":"BadRequestError","code":400}}`,
		"vllm text":    `{"object":"error","message":"This model does not support multimodal inputs","type":"BadRequestError","code":400}`,
	}
	for name, body := range cases {
		if !IsNoVisionRejection(400, []byte(body)) {
			t.Errorf("%s: refusal not recognised", name)
		}
	}
}

func TestOtherFailuresAreNotTakenForAnImageRefusal(t *testing.T) {
	if IsNoVisionRejection(400, []byte(`{"error":{"message":"context length exceeded"}}`)) {
		t.Error("a context overflow was taken for an image refusal")
	}
	if IsNoVisionRejection(200, []byte(`{"choices":[{"message":{"content":"image input is not supported"}}]}`)) {
		t.Error("a model sentence in a successful answer was taken for an image refusal")
	}
	if IsNoVisionRejection(502, []byte(`image input is not supported`)) {
		t.Error("a gateway failure was taken for an image refusal")
	}
}

func TestServerErrorRefusalOfImagesIsRecognised(t *testing.T) {
	if !IsNoVisionRejection(500, []byte(`{"error":{"code":500,"message":"image input is not supported - hint: provide the mmproj"}}`)) {
		t.Error("llama-server's 500 refusal was not recognised")
	}
}
