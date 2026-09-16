package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"tensors-router/internal/cluster"
	"tensors-router/internal/openai"
	"tensors-router/internal/routinggroups"
	"tensors-router/internal/siteapi"
)

type routingLane struct {
	store         routinggroups.Lane
	modelIDOf     func(cluster.Model) string
	servesLane    func(cluster.Model) bool
	anchorAllowed func(cluster.Model) bool
	peerEligible  func(peer cluster.Model, anchor cluster.Model) (bool, string)
}

var imageRoutingLane = routingLane{
	store:         routinggroups.ImageLane,
	modelIDOf:     func(model cluster.Model) string { return model.ImageID },
	servesLane:    func(model cluster.Model) bool { return model.HasImage && strings.TrimSpace(model.ImageID) != "" },
	anchorAllowed: func(cluster.Model) bool { return true },
	peerEligible:  func(cluster.Model, cluster.Model) (bool, string) { return true, "" },
}

var textRoutingLane = routingLane{
	store:         routinggroups.TextLane,
	modelIDOf:     func(model cluster.Model) string { return model.LocalID },
	servesLane:    func(model cluster.Model) bool { return model.HasLLM },
	anchorAllowed: cluster.TextGroupEligible,
	peerEligible:  textPeerEligibility,
}

func textPeerEligibility(peer cluster.Model, anchor cluster.Model) (bool, string) {
	if reason, eligible := cluster.TextGroupIneligibility(peer); !eligible {
		return false, reason
	}
	if peer.HasMultimodal != anchor.HasMultimodal {
		return false, "multimodal and text-only models cannot lend work to each other"
	}
	return true, ""
}

func (service *Service) handleSiteRoutingLinks(w http.ResponseWriter, r *http.Request, lane routingLane) {
	if !service.siteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
		service.writeRoutingLinks(w, r, lane)
	case http.MethodPost:
		service.saveRoutingLinks(w, r, lane)
	case http.MethodDelete:
		service.deleteRoutingLinks(w, r, lane)
	default:
		openai.WriteError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
	}
}

func anchorFromQuery(r *http.Request) routinggroups.Endpoint {
	return routinggroups.Endpoint{
		NodeID:  strings.TrimSpace(r.URL.Query().Get("node_id")),
		ModelID: strings.TrimSpace(r.URL.Query().Get("model_id")),
	}
}

func (service *Service) writeRoutingLinks(w http.ResponseWriter, r *http.Request, lane routingLane) {
	response := siteapi.RoutingLinksResponse{Links: []routinggroups.Link{}}
	if service.routingGroups == nil {
		openai.WriteJSON(w, http.StatusOK, response)
		return
	}
	links, err := service.routingGroups.Links(r.Context(), lane.store)
	if err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	if links != nil {
		response.Links = links
	}
	anchor := anchorFromQuery(r)
	if anchor.NodeID != "" && anchor.ModelID != "" {
		response.Anchor = &anchor
		response.Candidates = service.routingCandidates(lane, anchor, links)
	}
	openai.WriteJSON(w, http.StatusOK, response)
}

// routingCandidates lists every model on another node. It deliberately does not
// filter by name or by config hash: the whole point is to let an operator link the
// same checkpoint that two nodes configured differently, which is exactly the case
// those filters would exclude.
func (service *Service) routingCandidates(lane routingLane, anchor routinggroups.Endpoint, links []routinggroups.Link) []siteapi.RoutingCandidate {
	if service.registry == nil {
		return nil
	}
	models := service.registry.Models()
	anchorModel, _ := service.laneModelFor(lane, anchor)
	linkStates := linkStatesAround(anchor, links)

	candidates := make([]siteapi.RoutingCandidate, 0, len(models))
	for _, model := range models {
		if !lane.servesLane(model) || model.NodeID == anchor.NodeID {
			continue
		}
		peer := routinggroups.Endpoint{NodeID: model.NodeID, ModelID: lane.modelIDOf(model)}
		eligible, reason := lane.peerEligible(model, anchorModel)
		states := linkStates[peer]
		candidates = append(candidates, siteapi.RoutingCandidate{
			NodeID:           peer.NodeID,
			ModelID:          peer.ModelID,
			Filename:         model.Filename,
			ModelHash:        model.ModelHash,
			ConfigHash:       model.ConfigHash,
			ContextSize:      model.Capabilities.Context,
			Multimodal:       model.HasMultimodal,
			WeightsMatch:     anchorModel.ModelHash != "" && model.ModelHash == anchorModel.ModelHash,
			Eligible:         eligible,
			IneligibleReason: reason,
			LendsTo:          states.lendsTo,
			BorrowsFrom:      states.borrowsFrom,
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

type peerLinkStates struct {
	lendsTo     siteapi.RoutingLinkState
	borrowsFrom siteapi.RoutingLinkState
}

func linkStatesAround(anchor routinggroups.Endpoint, links []routinggroups.Link) map[routinggroups.Endpoint]peerLinkStates {
	states := map[routinggroups.Endpoint]peerLinkStates{}
	for _, link := range links {
		selected := siteapi.RoutingLinkState{Selected: true, LoadIfUnloaded: link.LoadIfUnloaded, RestoreAfterBorrow: link.RestoreAfterBorrow}
		switch anchor {
		case link.Owner:
			peer := states[link.Helper]
			peer.lendsTo = selected
			states[link.Helper] = peer
		case link.Helper:
			peer := states[link.Owner]
			peer.borrowsFrom = selected
			states[link.Owner] = peer
		}
	}
	return states
}

func (service *Service) laneModelFor(lane routingLane, endpoint routinggroups.Endpoint) (cluster.Model, bool) {
	if service.registry == nil {
		return cluster.Model{}, false
	}
	for _, model := range service.registry.Models() {
		if model.NodeID == endpoint.NodeID && lane.servesLane(model) && lane.modelIDOf(model) == endpoint.ModelID {
			return model, true
		}
	}
	return cluster.Model{}, false
}

func (service *Service) saveRoutingLinks(w http.ResponseWriter, r *http.Request, lane routingLane) {
	if service.routingGroups == nil {
		openai.WriteError(w, http.StatusNotFound, "not_found", "routing links are not configured")
		return
	}
	var request siteapi.RoutingLinksRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	if err := service.validateRoutingLinkChoices(lane, request); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	if err := service.routingGroups.ReplaceLinksTouching(r.Context(), lane.store, request.Anchor, linksFromChoices(request)); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	service.installAndPublishStoredRoutingLinks(r.Context())
	service.writeRoutingLinks(w, r, lane)
}

func (service *Service) validateRoutingLinkChoices(lane routingLane, request siteapi.RoutingLinksRequest) error {
	anchorModel, anchorFound := service.laneModelFor(lane, request.Anchor)
	if !anchorFound {
		return nil
	}
	if !lane.anchorAllowed(anchorModel) {
		return errors.New("anchor model cannot lend or borrow work")
	}
	for _, choice := range append(append([]siteapi.RoutingLinkChoice{}, request.LendsTo...), request.BorrowsFrom...) {
		peer, found := service.laneModelFor(lane, routinggroups.Endpoint{NodeID: choice.NodeID, ModelID: choice.ModelID})
		if !found {
			continue
		}
		if eligible, reason := lane.peerEligible(peer, anchorModel); !eligible {
			return fmt.Errorf("%s/%s is not eligible: %s", choice.NodeID, choice.ModelID, reason)
		}
	}
	return nil
}

func linksFromChoices(request siteapi.RoutingLinksRequest) []routinggroups.Link {
	links := make([]routinggroups.Link, 0, len(request.LendsTo)+len(request.BorrowsFrom))
	for _, choice := range request.LendsTo {
		links = append(links, routinggroups.Link{
			Owner:              request.Anchor,
			Helper:             routinggroups.Endpoint{NodeID: choice.NodeID, ModelID: choice.ModelID},
			LoadIfUnloaded:     choice.LoadIfUnloaded,
			RestoreAfterBorrow: choice.RestoreAfterBorrow,
		})
	}
	for _, choice := range request.BorrowsFrom {
		links = append(links, routinggroups.Link{
			Owner:              routinggroups.Endpoint{NodeID: choice.NodeID, ModelID: choice.ModelID},
			Helper:             request.Anchor,
			LoadIfUnloaded:     choice.LoadIfUnloaded,
			RestoreAfterBorrow: choice.RestoreAfterBorrow,
		})
	}
	return links
}

func (service *Service) deleteRoutingLinks(w http.ResponseWriter, r *http.Request, lane routingLane) {
	if service.routingGroups == nil {
		openai.WriteError(w, http.StatusNotFound, "not_found", "routing links are not configured")
		return
	}
	anchor := anchorFromQuery(r)
	if anchor.NodeID == "" || anchor.ModelID == "" {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "node_id and model_id are required")
		return
	}
	if err := service.routingGroups.ReplaceLinksTouching(r.Context(), lane.store, anchor, nil); err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	service.installAndPublishStoredRoutingLinks(r.Context())
	openai.WriteJSON(w, http.StatusOK, siteapi.RoutingLinksResponse{Links: []routinggroups.Link{}})
}
