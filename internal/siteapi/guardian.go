package siteapi

import "tensors-router/internal/guardianmodels"

type GuardianCandidate struct {
	NodeID   string `json:"node_id"`
	ModelID  string `json:"model_id"`
	Filename string `json:"filename"`
}

type GuardianModelsResponse struct {
	Assignments []guardianmodels.Assignment `json:"assignments"`
	Candidates  []GuardianCandidate         `json:"candidates"`
}
