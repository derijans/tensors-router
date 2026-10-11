package requestmedia

import "encoding/json"

var imagePartTypes = map[string]bool{"image_url": true, "input_image": true, "image": true}

type requestShape struct {
	Messages []message         `json:"messages"`
	Input    json.RawMessage   `json:"input"`
	Images   []json.RawMessage `json:"images"`
}

type message struct {
	Content json.RawMessage   `json:"content"`
	Images  []json.RawMessage `json:"images"`
}

type contentPart struct {
	Type string `json:"type"`
}

func HasImageInput(body []byte) bool {
	var request requestShape
	if json.Unmarshal(body, &request) != nil {
		return false
	}
	if len(request.Images) > 0 {
		return true
	}
	for _, entry := range request.Messages {
		if len(entry.Images) > 0 || containsImagePart(entry.Content) {
			return true
		}
	}
	var inputs []message
	if json.Unmarshal(request.Input, &inputs) != nil {
		return false
	}
	for _, entry := range inputs {
		if containsImagePart(entry.Content) {
			return true
		}
	}
	return false
}

func containsImagePart(content json.RawMessage) bool {
	var parts []contentPart
	if json.Unmarshal(content, &parts) != nil {
		return false
	}
	for _, part := range parts {
		if imagePartTypes[part.Type] {
			return true
		}
	}
	return false
}
