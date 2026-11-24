package siteapi

import "time"

type APICertificateState struct {
	Present     bool      `json:"present"`
	Name        string    `json:"name,omitempty"`
	Source      string    `json:"source,omitempty"`
	NotAfter    time.Time `json:"not_after,omitzero"`
	Fingerprint string    `json:"fingerprint,omitempty"`
}
