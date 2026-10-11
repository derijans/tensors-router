package analytics

import (
	"encoding/json"
	"net/http"
)

type upstreamErrorBody struct {
	Error   json.RawMessage `json:"error"`
	Message string          `json:"message"`
	Detail  string          `json:"detail"`
}

type upstreamErrorObject struct {
	Message string `json:"message"`
}

func applyUpstreamError(event *Event, body []byte) {
	if event.StatusCode < http.StatusBadRequest || event.ErrorMessage != "" {
		return
	}
	event.ErrorMessage = upstreamErrorText(body)
	if event.UpstreamStatus == 0 {
		event.UpstreamStatus = event.StatusCode
	}
}

func upstreamErrorText(body []byte) string {
	var parsed upstreamErrorBody
	if json.Unmarshal(body, &parsed) != nil {
		return string(body)
	}
	var text string
	if json.Unmarshal(parsed.Error, &text) == nil && text != "" {
		return text
	}
	var object upstreamErrorObject
	if json.Unmarshal(parsed.Error, &object) == nil && object.Message != "" {
		return object.Message
	}
	if parsed.Message != "" {
		return parsed.Message
	}
	if parsed.Detail != "" {
		return parsed.Detail
	}
	return string(body)
}
