package requestmedia

import "testing"

func TestImageInputIsFoundInEveryRequestShapeThatCarriesOne(t *testing.T) {
	cases := map[string]string{
		"openai chat image_url":   `{"model":"m","messages":[{"role":"user","content":[{"type":"text","text":"look"},{"type":"image_url","image_url":{"url":"data:image/png;base64,AAAA"}}]}]}`,
		"openai chat input_image": `{"messages":[{"role":"user","content":[{"type":"input_image","image_url":"https://example.test/a.png"}]}]}`,
		"responses input_image":   `{"input":[{"role":"user","content":[{"type":"input_text","text":"look"},{"type":"input_image","image_url":"https://example.test/a.png"}]}]}`,
		"anthropic image block":   `{"messages":[{"role":"user","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AAAA"}}]}]}`,
		"ollama chat images":      `{"model":"m","messages":[{"role":"user","content":"look","images":["AAAA"]}]}`,
		"ollama generate images":  `{"model":"m","prompt":"look","images":["AAAA"]}`,
		"image in older turn":     `{"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"x"}}]},{"role":"assistant","content":"a cat"},{"role":"user","content":"and now?"}]}`,
	}
	for name, body := range cases {
		if !HasImageInput([]byte(body)) {
			t.Errorf("%s: no image found, want one", name)
		}
	}
}

func TestPlainTextRequestsCarryNoImageInput(t *testing.T) {
	cases := map[string]string{
		"string content":               `{"messages":[{"role":"user","content":"hello"}]}`,
		"text parts only":              `{"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`,
		"responses string input":       `{"input":"hello"}`,
		"text that talks about images": `{"messages":[{"role":"user","content":"what does type image_url mean in {\"type\":\"image_url\"}?"}]}`,
		"empty ollama images":          `{"model":"m","prompt":"hello","images":[]}`,
		"text part naming the type":    `{"messages":[{"role":"user","content":[{"type":"text","text":"image_url"}]}]}`,
		"completion prompt":            `{"prompt":"describe an image_url"}`,
		"not json":                     `image_url`,
		"empty body":                   ``,
		"embeddings input":             `{"model":"m","input":["a","b"]}`,
	}
	for name, body := range cases {
		if HasImageInput([]byte(body)) {
			t.Errorf("%s: found an image, want none", name)
		}
	}
}
