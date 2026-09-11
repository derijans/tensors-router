package proxy

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"tensors-router/internal/cluster"
	"tensors-router/internal/openai"
	"tensors-router/internal/routinggroups"
	"tensors-router/internal/siteapi"
)

func (service *Service) handleSiteTextRoutingGroups(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
		service.writeTextRoutingGroups(w, r)
	case http.MethodPost:
		service.saveTextRoutingGroup(w, r)
	case http.MethodDelete:
		service.deleteTextRoutingGroup(w, r)
	default:
		openai.WriteError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
	}
}

func (service *Service) writeTextRoutingGroups(w http.ResponseWriter, r *http.Request) {
	if service.routingGroups == nil {
		openai.WriteJSON(w, http.StatusOK, siteapi.TextRoutingGroupsResponse{Groups: []siteapi.TextRoutingGroup{}})
		return
	}
	groups, err := service.routingGroups.TextGroups(r.Context())
	if err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	response := siteapi.TextRoutingGroupsResponse{Groups: siteTextRoutingGroups(groups)}

	anchor := siteapi.TextRoutingGroupMember{
		NodeID:  strings.TrimSpace(r.URL.Query().Get("node_id")),
		ModelID: strings.TrimSpace(r.URL.Query().Get("model_id")),
	}
	if anchor.NodeID == "" || anchor.ModelID == "" {
		openai.WriteJSON(w, http.StatusOK, response)
		return
	}
	response.Anchor = &anchor

	selected := map[siteapi.TextRoutingGroupMember]bool{}
	if group, found, err := service.routingGroups.TextGroup(r.Context(), routinggroups.TextMember{NodeID: anchor.NodeID, ModelID: anchor.ModelID}); err == nil && found {
		for _, member := range group.Members {
			selected[siteapi.TextRoutingGroupMember{NodeID: member.NodeID, ModelID: member.ModelID}] = true
		}
	}
	response.Candidates = service.textRoutingGroupCandidates(anchor, selected)
	openai.WriteJSON(w, http.StatusOK, response)
}

// textRoutingGroupCandidates lists every eligible LLM model on another node.
// Like its image counterpart it does not filter by name or config hash — the
// point is letting an operator group the same checkpoint configured
// differently on two nodes — but it does report, rather than silently omit,
// a model that fails eligibility, so the UI can explain why a row is disabled
// instead of it simply not appearing.
func (service *Service) textRoutingGroupCandidates(anchor siteapi.TextRoutingGroupMember, selected map[siteapi.TextRoutingGroupMember]bool) []siteapi.TextRoutingGroupCandidate {
	if service.registry == nil {
		return nil
	}
	models := service.registry.Models()
	anchorHash := ""
	anchorMultimodal := false
	for _, model := range models {
		if model.NodeID == anchor.NodeID && model.LocalID == anchor.ModelID {
			anchorHash = model.ModelHash
			anchorMultimodal = model.HasMultimodal
			break
		}
	}

	candidates := make([]siteapi.TextRoutingGroupCandidate, 0, len(models))
	for _, model := range models {
		if !model.HasLLM {
			continue
		}
		if model.NodeID == anchor.NodeID {
			continue
		}
		eligible, reason := textCandidateEligibility(model, anchorMultimodal)
		member := siteapi.TextRoutingGroupMember{NodeID: model.NodeID, ModelID: model.LocalID}
		candidates = append(candidates, siteapi.TextRoutingGroupCandidate{
			NodeID:           model.NodeID,
			ModelID:          model.LocalID,
			Filename:         model.Filename,
			ModelHash:        model.ModelHash,
			ConfigHash:       model.ConfigHash,
			ContextSize:      model.Capabilities.Context,
			Multimodal:       model.HasMultimodal,
			WeightsMatch:     anchorHash != "" && model.ModelHash == anchorHash,
			Selected:         selected[member],
			Eligible:         eligible,
			IneligibleReason: reason,
		})
	}
	sort.Slice(candidates, func(left, right int) bool {
		if candidates[left].NodeID != candidates[right].NodeID {
			return candidates[left].NodeID < candidates[right].NodeID
		}
		return candidates[left].ModelID < candidates[right].ModelID
	})
	return candidates
}

// textCandidateEligibility layers the pairwise modality-homogeneity rule over
// cluster.TextGroupEligible's per-model checks. Mixing a multimodal model with
// a text-only one would let a base64-heavy request drag a text-only model's
// measured bytes-per-token ratio in the unsafe direction — an
// under-estimate — so the two never share a group.
func textCandidateEligibility(model cluster.Model, anchorMultimodal bool) (bool, string) {
	if !cluster.TextGroupEligible(model) {
		if model.Capabilities.Context <= 0 {
			return false, "model does not report a context window"
		}
		return false, "model serves requests concurrently"
	}
	if model.HasMultimodal != anchorMultimodal {
		return false, "multimodal and text-only models cannot share a group"
	}
	return true, ""
}

func (service *Service) saveTextRoutingGroup(w http.ResponseWriter, r *http.Request) {
	if service.routingGroups == nil {
		openai.WriteError(w, http.StatusNotFound, "not_found", "routing groups are not configured")
		return
	}
	var request siteapi.TextRoutingGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	if service.registry != nil {
		anchorModel, anchorFound := service.registryModelFor(request.Anchor.NodeID, request.Anchor.ModelID)
		if anchorFound {
			if !cluster.TextGroupEligible(anchorModel) {
				openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "anchor model is not eligible for a text routing group")
				return
			}
			for _, member := range request.Members {
				model, found := service.registryModelFor(member.NodeID, member.ModelID)
				if !found {
					continue
				}
				if eligible, reason := textCandidateEligibility(model, anchorModel.HasMultimodal); !eligible {
					openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "member "+member.NodeID+"/"+member.ModelID+" is not eligible: "+reason)
					return
				}
			}
		}
	}
	members := make([]routinggroups.TextMember, 0, len(request.Members))
	for _, member := range request.Members {
		members = append(members, routinggroups.TextMember{NodeID: member.NodeID, ModelID: member.ModelID})
	}
	group, err := service.routingGroups.SetTextGroup(r.Context(),
		routinggroups.TextMember{NodeID: request.Anchor.NodeID, ModelID: request.Anchor.ModelID}, members)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	service.refreshRoutingGroupSource(r)
	openai.WriteJSON(w, http.StatusOK, siteTextRoutingGroup(group))
}

func (service *Service) registryModelFor(nodeID string, localID string) (cluster.Model, bool) {
	for _, model := range service.registry.Models() {
		if model.NodeID == nodeID && model.LocalID == localID {
			return model, true
		}
	}
	return cluster.Model{}, false
}

func (service *Service) deleteTextRoutingGroup(w http.ResponseWriter, r *http.Request) {
	if service.routingGroups == nil {
		openai.WriteError(w, http.StatusNotFound, "not_found", "routing groups are not configured")
		return
	}
	anchor := routinggroups.TextMember{
		NodeID:  strings.TrimSpace(r.URL.Query().Get("node_id")),
		ModelID: strings.TrimSpace(r.URL.Query().Get("model_id")),
	}
	if anchor.NodeID == "" || anchor.ModelID == "" {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "node_id and model_id are required")
		return
	}
	if err := service.routingGroups.DeleteTextGroup(r.Context(), anchor); err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	service.refreshRoutingGroupSource(r)
	openai.WriteJSON(w, http.StatusOK, siteapi.TextRoutingGroup{})
}

func siteTextRoutingGroups(groups []routinggroups.TextGroup) []siteapi.TextRoutingGroup {
	result := make([]siteapi.TextRoutingGroup, 0, len(groups))
	for _, group := range groups {
		result = append(result, siteTextRoutingGroup(group))
	}
	return result
}

func siteTextRoutingGroup(group routinggroups.TextGroup) siteapi.TextRoutingGroup {
	members := make([]siteapi.TextRoutingGroupMember, 0, len(group.Members))
	for _, member := range group.Members {
		members = append(members, siteapi.TextRoutingGroupMember{NodeID: member.NodeID, ModelID: member.ModelID})
	}
	return siteapi.TextRoutingGroup{ID: group.ID, Members: members}
}
