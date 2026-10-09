package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"tensors-router/internal/offloadsettings"
	"tensors-router/internal/openai"
	"tensors-router/internal/siteapi"
)

func (service *Service) handleSiteLendingSettings(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteEndpointNotFound(w)
		return
	}
	switch r.Method {
	case http.MethodGet:
		service.writeLendingSettings(w, service.lending.snapshot())
	case http.MethodPost:
		var request siteapi.LendingSettingsRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}
		service.changeLendingSettings(w, r.Context(), func(ctx context.Context) (offloadsettings.Resolution, error) {
			return service.lending.set(ctx, request.Values)
		})
	case http.MethodDelete:
		key := strings.TrimSpace(r.URL.Query().Get("key"))
		service.changeLendingSettings(w, r.Context(), func(ctx context.Context) (offloadsettings.Resolution, error) {
			return service.lending.clear(ctx, key)
		})
	default:
		openai.WriteError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
	}
}

func (service *Service) changeLendingSettings(w http.ResponseWriter, ctx context.Context, change func(context.Context) (offloadsettings.Resolution, error)) {
	resolution, err := change(ctx)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errLendingSettingsReadOnly) {
			status = http.StatusConflict
		}
		openai.WriteError(w, status, "invalid_request_error", err.Error())
		return
	}
	service.publishLendingSettings(ctx)
	service.writeLendingSettings(w, resolution)
}

func (service *Service) writeLendingSettings(w http.ResponseWriter, resolution offloadsettings.Resolution) {
	openai.WriteJSON(w, http.StatusOK, siteapi.LendingSettingsResponse{
		Entries:     resolution.Entries,
		Fingerprint: resolution.Settings.Fingerprint(),
		Editable:    service.lending.store != nil,
	})
}
