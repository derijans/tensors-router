package cluster

import (
	"fmt"
	"sort"
)

type identityGroup struct {
	publicID string
	models   []Model
}

func assignPublicIDs(models []Model) {
	byBaseID := map[string][]Model{}
	for _, model := range models {
		byBaseID[model.LocalID] = append(byBaseID[model.LocalID], model)
	}

	baseIDs := make([]string, 0, len(byBaseID))
	for baseID := range byBaseID {
		baseIDs = append(baseIDs, baseID)
	}
	sort.Strings(baseIDs)

	publicIDByRoute := map[string]string{}
	for _, baseID := range baseIDs {
		groups := identityGroups(baseID, byBaseID[baseID])
		for index, group := range groups {
			publicID := baseID
			if index > 0 {
				publicID = fmt.Sprintf("%s-%d", baseID, index+1)
			}
			for _, model := range group.models {
				publicIDByRoute[modelRouteKey(model)] = publicID
			}
		}
	}

	for index := range models {
		if publicID, ok := publicIDByRoute[modelRouteKey(models[index])]; ok {
			models[index].PublicID = publicID
		}
	}
}

func identityGroups(baseID string, models []Model) []identityGroup {
	sort.Slice(models, func(left, right int) bool {
		return identitySortKey(models[left]) < identitySortKey(models[right])
	})

	groups := make([]identityGroup, 0)
	for _, model := range models {
		identity := modelIdentity(model)
		match := -1
		for index, group := range groups {
			if modelIdentity(group.models[0]) == identity {
				match = index
				break
			}
		}
		if match >= 0 {
			groups[match].models = append(groups[match].models, model)
			continue
		}
		groups = append(groups, identityGroup{
			publicID: baseID,
			models:   []Model{model},
		})
	}
	return groups
}

func modelIdentity(model Model) string {
	return model.ModelHash + ":" + model.ConfigHash
}

func identitySortKey(model Model) string {
	prefix := "2:"
	if model.Source == SourceMaster || model.Source == SourceLocal {
		prefix = "0:"
	}
	return prefix + routeSortKey(model)
}

func routeSortKey(model Model) string {
	return model.NodeID + ":" + model.LocalID
}

func modelRouteKey(model Model) string {
	return model.NodeID + ":" + model.LocalID + ":" + modelIdentity(model)
}

func routeKey(route Route) string {
	return route.NodeID + ":" + route.LocalID
}

func routeFromModel(model Model, remote bool) Route {
	return Route{
		PublicID: model.PublicID,
		LocalID:  model.LocalID,
		Filename: model.Filename,
		NodeID:   model.NodeID,
		NodeURL:  model.NodeURL,
		Remote:   remote,
	}
}
