package proxy

import (
	"bytes"
	"fmt"
	"io"
	"net/http"

	"tensors-router/internal/guardianmodels"
	"tensors-router/internal/requestmedia"
)

const guardianRefusalPeekLimit = 64 << 10

func (service *Service) guardianPublicID(guardian guardianmodels.Endpoint) (string, bool) {
	for _, model := range service.registry.Models() {
		if model.NodeID == guardian.NodeID && model.LocalID == guardian.ModelID && guardianEligible(model) {
			return model.PublicID, true
		}
	}
	return "", false
}

func (service *Service) guardiansMayAnswer(r *http.Request) bool {
	return service.registry != nil && !requestIsBorrowed(r) && !contextIsGuardianRedirect(r.Context())
}

func (service *Service) guardianForTextOnlyModel(publicID string) (string, bool) {
	var guardian string
	for _, model := range service.registry.Models() {
		if model.PublicID != publicID || !model.HasLLM {
			continue
		}
		if model.HasMultimodal {
			return "", false
		}
		if endpoint, assigned := service.guardians.guardianOf(model.NodeID, model.LocalID); assigned && guardian == "" {
			guardian, _ = service.guardianPublicID(endpoint)
		}
	}
	return guardian, guardian != ""
}

func (service *Service) guardianBeforeSend(r *http.Request, body []byte, modelID string) (string, bool) {
	if !service.guardiansMayAnswer(r) {
		return "", false
	}
	guardian, assigned := service.guardianForTextOnlyModel(modelID)
	if !assigned || !requestmedia.HasImageInput(body) {
		return "", false
	}
	return guardian, true
}

func (service *Service) handleGuardianTakeover(w http.ResponseWriter, r *http.Request, body []byte, guardianPublicID string) {
	taken := r.WithContext(markGuardianRedirect(r.Context()))
	service.handleRegistryModelRequest(w, taken, rewriteRequestModel(body, guardianPublicID), guardianPublicID)
}

func (service *Service) guardianAfterRefusal(r *http.Request, body []byte, acquired acquiredRegistryRoute, forwarded *registryForward) (string, bool) {
	response := forwarded.response
	if response == nil || !requestmedia.RefusalStatus(response.StatusCode) || acquired.route.Remote || !service.guardiansMayAnswer(r) {
		return "", false
	}
	endpoint, assigned := service.guardians.guardianOf(acquired.route.NodeID, acquired.route.LocalID)
	if !assigned || !requestmedia.HasImageInput(body) {
		return "", false
	}
	guardian, eligible := service.guardianPublicID(endpoint)
	if !eligible {
		return "", false
	}
	refusal := peekResponseBody(response, guardianRefusalPeekLimit)
	if !requestmedia.IsNoVisionRejection(response.StatusCode, refusal) {
		return "", false
	}
	_ = response.Body.Close()
	forwarded.tracking.recordFailure(r.Context(), fmt.Errorf("model cannot read images, handed to guardian %s", guardian))
	return guardian, true
}

func peekResponseBody(response *http.Response, limit int64) []byte {
	head, _ := io.ReadAll(io.LimitReader(response.Body, limit))
	response.Body = readCloser{Reader: io.MultiReader(bytes.NewReader(head), response.Body), Closer: response.Body}
	return head
}

type readCloser struct {
	io.Reader
	io.Closer
}
