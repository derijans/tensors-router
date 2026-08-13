package openai

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"tensors-router/internal/catalog"
)

type ModelObject struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type ModelsResponse struct {
	Object string        `json:"object"`
	Data   []ModelObject `json:"data"`
}

type ErrorBody struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Param   string `json:"param,omitempty"`
	Code    string `json:"code,omitempty"`
}

func ModelsResponseFromCatalog(models []catalog.Model) ModelsResponse {
	primaryIDs := make(map[string]struct{}, len(models))
	servedNameOwners := make(map[string]int)
	for _, model := range models {
		primaryIDs[model.ID] = struct{}{}
		seen := make(map[string]struct{}, len(model.ServedNames))
		for _, servedName := range model.ServedNames {
			servedName = strings.TrimSpace(servedName)
			if servedName == "" || servedName == model.ID {
				continue
			}
			if _, duplicate := seen[servedName]; duplicate {
				continue
			}
			seen[servedName] = struct{}{}
			servedNameOwners[servedName]++
		}
	}
	data := make([]ModelObject, 0, len(models)+len(servedNameOwners))
	for _, model := range models {
		primary := ModelObject{
			ID:      model.ID,
			Object:  "model",
			Created: model.Created,
			OwnedBy: "koboldcpp",
		}
		data = append(data, primary)
		for _, servedName := range model.ServedNames {
			servedName = strings.TrimSpace(servedName)
			if servedName == "" || servedName == model.ID || servedNameOwners[servedName] != 1 {
				continue
			}
			if _, conflicts := primaryIDs[servedName]; conflicts {
				continue
			}
			alias := primary
			alias.ID = servedName
			data = append(data, alias)
		}
	}
	return ModelsResponse{
		Object: "list",
		Data:   data,
	}
}

func WriteJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func WriteError(w http.ResponseWriter, status int, errorType string, message string) {
	WriteErrorCode(w, status, errorType, "", message)
}

func WriteErrorCode(w http.ResponseWriter, status int, errorType string, code string, message string) {
	WriteJSON(w, status, ErrorBody{
		Error: ErrorDetail{
			Message: message,
			Type:    errorType,
			Code:    code,
		},
	})
}

func ModelFromJSON(body []byte) (string, bool, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return "", false, err
	}

	value, ok := raw["model"]
	if !ok {
		return "", false, nil
	}

	var model string
	if err := json.Unmarshal(value, &model); err != nil {
		return "", true, fmt.Errorf("model must be a string")
	}
	return model, true, nil
}
