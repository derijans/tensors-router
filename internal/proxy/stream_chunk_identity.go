package proxy

import (
	"encoding/json"
	"strings"
)

type streamChunkIdentity struct {
	ID      string `json:"id,omitempty"`
	Object  string `json:"object,omitempty"`
	Created int64  `json:"created,omitempty"`
	Model   string `json:"model,omitempty"`
}

func streamChunkIdentityOf(line string) (streamChunkIdentity, bool) {
	payload, found := strings.CutPrefix(line, "data:")
	if !found {
		return streamChunkIdentity{}, false
	}
	payload = strings.TrimSpace(payload)
	if payload == "" || payload == "[DONE]" {
		return streamChunkIdentity{}, false
	}
	var identity streamChunkIdentity
	if json.Unmarshal([]byte(payload), &identity) != nil || identity.ID == "" {
		return streamChunkIdentity{}, false
	}
	return identity, true
}
