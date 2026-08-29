package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"tensors-router/internal/cluster"
	"tensors-router/internal/openai"
	"tensors-router/internal/routinggroups"
	"tensors-router/internal/siteapi"
)

func (service *Service) handleSiteRoutingGroups(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
		service.writeRoutingGroups(w, r)
	case http.MethodPost:
		service.saveRoutingGroup(w, r)
	case http.MethodDelete:
		service.deleteRoutingGroup(w, r)
	default:
		openai.WriteError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
	}
}

func (service *Service) writeRoutingGroups(w http.ResponseWriter, r *http.Request) {
	if service.routingGroups == nil {
		openai.WriteJSON(w, http.StatusOK, siteapi.RoutingGroupsResponse{Groups: []siteapi.RoutingGroup{}})
		return
	}
	groups, err := service.routingGroups.Groups(r.Context())
	if err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	response := siteapi.RoutingGroupsResponse{Groups: siteRoutingGroups(groups)}

	anchor := siteapi.RoutingGroupMember{
		NodeID:  strings.TrimSpace(r.URL.Query().Get("node_id")),
		ImageID: strings.TrimSpace(r.URL.Query().Get("image_id")),
	}
	if anchor.NodeID == "" || anchor.ImageID == "" {
		openai.WriteJSON(w, http.StatusOK, response)
		return
	}
	response.Anchor = &anchor

	selected := map[siteapi.RoutingGroupMember]bool{}
	if group, found, err := service.routingGroups.Group(r.Context(), routinggroups.Member{NodeID: anchor.NodeID, ImageID: anchor.ImageID}); err == nil && found {
		for _, member := range group.Members {
			selected[siteapi.RoutingGroupMember{NodeID: member.NodeID, ImageID: member.ImageID}] = true
		}
	}
	response.Candidates = service.routingGroupCandidates(anchor, selected)
	openai.WriteJSON(w, http.StatusOK, response)
}

// routingGroupCandidates lists every image model on another node. It deliberately
// does not filter by name or by config hash: the whole point is to let an operator
// group the same checkpoint that two nodes configured differently, which is
// exactly the case those filters would exclude.
func (service *Service) routingGroupCandidates(anchor siteapi.RoutingGroupMember, selected map[siteapi.RoutingGroupMember]bool) []siteapi.RoutingGroupCandidate {
	if service.registry == nil {
		return nil
	}
	models := service.registry.Models()
	anchorHash := ""
	for _, model := range models {
		if model.NodeID == anchor.NodeID && model.ImageID == anchor.ImageID {
			anchorHash = model.ModelHash
			break
		}
	}

	candidates := make([]siteapi.RoutingGroupCandidate, 0, len(models))
	for _, model := range models {
		if !model.HasImage || strings.TrimSpace(model.ImageID) == "" {
			continue
		}
		if model.NodeID == anchor.NodeID && model.ImageID == anchor.ImageID {
			continue
		}
		if model.NodeID == anchor.NodeID {
			continue
		}
		member := siteapi.RoutingGroupMember{NodeID: model.NodeID, ImageID: model.ImageID}
		candidates = append(candidates, siteapi.RoutingGroupCandidate{
			NodeID:       model.NodeID,
			ImageID:      model.ImageID,
			Filename:     model.Filename,
			ModelHash:    model.ModelHash,
			ConfigHash:   model.ConfigHash,
			WeightsMatch: anchorHash != "" && model.ModelHash == anchorHash,
			Selected:     selected[member],
		})
	}
	sort.Slice(candidates, func(left, right int) bool {
		if candidates[left].NodeID != candidates[right].NodeID {
			return candidates[left].NodeID < candidates[right].NodeID
		}
		return candidates[left].ImageID < candidates[right].ImageID
	})
	return candidates
}

func (service *Service) saveRoutingGroup(w http.ResponseWriter, r *http.Request) {
	if service.routingGroups == nil {
		openai.WriteError(w, http.StatusNotFound, "not_found", "routing groups are not configured")
		return
	}
	var request siteapi.RoutingGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	members := make([]routinggroups.Member, 0, len(request.Members))
	for _, member := range request.Members {
		members = append(members, routinggroups.Member{NodeID: member.NodeID, ImageID: member.ImageID})
	}
	group, err := service.routingGroups.SetGroup(r.Context(),
		routinggroups.Member{NodeID: request.Anchor.NodeID, ImageID: request.Anchor.ImageID}, members)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	service.refreshRoutingGroupSource(r)
	openai.WriteJSON(w, http.StatusOK, siteRoutingGroup(group))
}

func (service *Service) deleteRoutingGroup(w http.ResponseWriter, r *http.Request) {
	if service.routingGroups == nil {
		openai.WriteError(w, http.StatusNotFound, "not_found", "routing groups are not configured")
		return
	}
	anchor := routinggroups.Member{
		NodeID:  strings.TrimSpace(r.URL.Query().Get("node_id")),
		ImageID: strings.TrimSpace(r.URL.Query().Get("image_id")),
	}
	if anchor.NodeID == "" || anchor.ImageID == "" {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "node_id and image_id are required")
		return
	}
	if err := service.routingGroups.DeleteGroup(r.Context(), anchor); err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	service.refreshRoutingGroupSource(r)
	openai.WriteJSON(w, http.StatusOK, siteapi.RoutingGroup{})
}

func siteRoutingGroups(groups []routinggroups.Group) []siteapi.RoutingGroup {
	result := make([]siteapi.RoutingGroup, 0, len(groups))
	for _, group := range groups {
		result = append(result, siteRoutingGroup(group))
	}
	return result
}

func siteRoutingGroup(group routinggroups.Group) siteapi.RoutingGroup {
	members := make([]siteapi.RoutingGroupMember, 0, len(group.Members))
	for _, member := range group.Members {
		members = append(members, siteapi.RoutingGroupMember{NodeID: member.NodeID, ImageID: member.ImageID})
	}
	return siteapi.RoutingGroup{ID: group.ID, Members: members}
}

// routingGroupLookup answers the registry from the store. It is rebuilt after every
// edit so a saved group takes effect on the next request rather than at the next
// refresh tick.
type routingGroupLookup struct {
	byMember map[cluster.GroupMember]routingGroupEntry
}

type routingGroupEntry struct {
	groupID string
	members []cluster.GroupMember
}

func (lookup *routingGroupLookup) GroupMembers(member cluster.GroupMember) (string, []cluster.GroupMember, bool) {
	if lookup == nil {
		return "", nil, false
	}
	entry, ok := lookup.byMember[member]
	if !ok {
		return "", nil, false
	}
	return entry.groupID, entry.members, true
}

func newRoutingGroupLookup(groups []routinggroups.Group) *routingGroupLookup {
	lookup := &routingGroupLookup{byMember: map[cluster.GroupMember]routingGroupEntry{}}
	for _, group := range groups {
		members := make([]cluster.GroupMember, 0, len(group.Members))
		for _, member := range group.Members {
			members = append(members, cluster.GroupMember{NodeID: member.NodeID, ImageID: member.ImageID})
		}
		for _, member := range members {
			lookup.byMember[member] = routingGroupEntry{groupID: group.ID, members: members}
		}
	}
	return lookup
}

func (service *Service) refreshRoutingGroupSource(r *http.Request) {
	service.applyRoutingGroupSource(r.Context())
}

// loadRoutingGroupSource seeds routing from the store at startup, so groups saved
// in a previous run are in force before the first request rather than after the
// first edit.
func (service *Service) loadRoutingGroupSource() {
	service.applyRoutingGroupSource(context.Background())
}

func (service *Service) applyRoutingGroupSource(ctx context.Context) {
	if service.routingGroups == nil || service.registry == nil {
		return
	}
	groups, err := service.routingGroups.Groups(ctx)
	if err != nil {
		service.logger.Printf("routing group load failed: %v", err)
		return
	}
	service.registry.SetGroupSource(newRoutingGroupLookup(groups))
}
