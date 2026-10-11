package requestmedia

import (
	"net/http"
	"strings"
)

var noVisionPhrases = []string{
	"image input is not supported",
	"image(s) may be provided",
	"does not support multimodal",
	"not a multimodal model",
}

func RefusalStatus(status int) bool {
	return status == http.StatusBadRequest || status == http.StatusInternalServerError
}

func IsNoVisionRejection(status int, body []byte) bool {
	if !RefusalStatus(status) {
		return false
	}
	text := strings.ToLower(string(body))
	for _, phrase := range noVisionPhrases {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}
