package proxy

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"tensors-router/internal/cluster"
	"tensors-router/internal/cook"
	"tensors-router/internal/recipes"
	"tensors-router/internal/siteapi"
)

type cookGroup struct {
	nodeID     string
	nodeURL    string
	local      bool
	components []cook.Component
}

func (service *Service) writeCookGroup(ctx context.Context, group cookGroup, request cook.NodeConfigRequest) (cook.ConfigResult, error) {
	if group.local {
		return service.writeLocalCookConfig(ctx, request)
	}
	var result cook.ConfigResult
	if err := service.clusterClient.JSON(ctx, http.MethodPost, group.nodeURL, "/router/v1/node/site/configs", request, &result); err != nil {
		return cook.ConfigResult{}, err
	}
	return result, nil
}

func (service *Service) writeLocalCookConfig(ctx context.Context, request cook.NodeConfigRequest) (cook.ConfigResult, error) {
	components, err := cook.NormalizedComponents(request.Components)
	if err != nil {
		return cook.ConfigResult{}, err
	}
	options, err := cook.NormalizedOptions(request.Options)
	if err != nil {
		return cook.ConfigResult{}, err
	}
	request.Components = components
	request.Options = options
	group := cookGroup{
		nodeID:     service.nodeID,
		nodeURL:    service.nodeURL,
		local:      true,
		components: components,
	}
	if _, err := service.validateCookGroups(ctx, []cookGroup{group}, request.Options); err != nil {
		return cook.ConfigResult{}, err
	}
	writer := cook.Writer{
		ConfigDir: service.configDir,
		FileRoots: service.assets.fileRoots,
		Catalog:   service.catalog,
		NodeID:    service.nodeID,
		NodeURL:   service.nodeURL,
	}
	if request.DryRun {
		return writer.Preview(request)
	}
	return writer.Apply(request)
}

func (service *Service) refreshAfterCook(ctx context.Context, groups []cookGroup) error {
	if err := service.refreshLocalRegistry(); err != nil {
		return err
	}
	if service.registry == nil {
		return nil
	}
	for _, group := range groups {
		if group.local {
			continue
		}
		snapshot, err := service.clusterClient.FetchSnapshot(ctx, group.nodeURL)
		if err != nil {
			service.registry.MarkNodeURLHealth(group.nodeURL, false)
			continue
		}
		if snapshot.NodeURL == "" {
			snapshot.NodeURL = group.nodeURL
		}
		if err := service.registry.UpdateNode(snapshot); err != nil {
			return err
		}
	}
	return nil
}

func (service *Service) nodeURLByID() map[string]string {
	result := map[string]string{}
	if service.nodeID != "" {
		result[service.nodeID] = service.nodeURL
	}
	if service.registry != nil {
		for nodeID, nodeURL := range service.registry.NodeURLsByID() {
			result[nodeID] = nodeURL
		}
		for _, model := range service.registry.Models() {
			if strings.TrimSpace(model.NodeID) != "" && strings.TrimSpace(model.NodeURL) != "" {
				result[model.NodeID] = model.NodeURL
			}
		}
	}
	for _, rawURL := range service.slaveURLs {
		parsed, err := url.Parse(rawURL)
		if err != nil {
			continue
		}
		nodeID := strings.TrimSpace(parsed.Hostname())
		if nodeID != "" {
			result[nodeID] = rawURL
		}
	}
	return result
}

func buildRecipe(id string, groups []cookGroup, results []cook.ConfigResult) recipes.Recipe {
	resultByNode := map[string]cook.ConfigResult{}
	for _, result := range results {
		resultByNode[result.NodeID] = result
	}
	recipe := recipes.Recipe{
		ID:       id,
		PublicID: id,
		Created:  time.Now().Unix(),
	}
	for _, group := range groups {
		result := resultByNode[group.nodeID]
		for _, component := range group.components {
			recipeComponent := recipes.Component{
				Kind:           component.Kind,
				NodeID:         result.NodeID,
				NodeURL:        result.NodeURL,
				ModelID:        result.ModelID,
				ImageID:        result.ImageID,
				ConfigFilename: result.Filename,
			}
			switch component.Kind {
			case cook.KindText:
				recipe.Text = &recipeComponent
			case cook.KindEmbeddings:
				recipe.Embeddings = &recipeComponent
			case cook.KindImage:
				recipe.Image = &recipeComponent
				if result.ImageID != "" {
					recipe.PublicImageID = id + "-" + imageSuffix(result.ModelID, result.ImageID)
				}
			case cook.KindVoice:
				recipe.Voice = &recipeComponent
			case cook.KindMusic:
				recipe.Music = &recipeComponent
			}
		}
	}
	return recipe
}

func imageSuffix(modelID string, imageID string) string {
	prefix := modelID + "-"
	if strings.HasPrefix(imageID, prefix) {
		return strings.TrimPrefix(imageID, prefix)
	}
	return imageID
}

func componentKinds(components []cook.Component) []string {
	kinds := make([]string, 0, len(components))
	for _, component := range components {
		kinds = append(kinds, component.Kind)
	}
	sort.Strings(kinds)
	return kinds
}

func (service *Service) planCook(ctx context.Context, request siteapi.CookRequest, dryRun bool) (siteapi.CookResponse, error) {
	id, err := cook.SanitizedID(request.ID)
	if err != nil {
		return siteapi.CookResponse{}, err
	}
	if request.Options, err = cook.NormalizedOptions(request.Options); err != nil {
		return siteapi.CookResponse{}, err
	}
	groups, err := service.cookGroups(request.Components)
	if err != nil {
		return siteapi.CookResponse{}, err
	}
	validation, err := service.validateCookGroups(ctx, groups, request.Options)
	if err != nil {
		return siteapi.CookResponse{}, err
	}
	results, err := service.writeCookConfigs(ctx, id, groups, request, dryRun)
	if err != nil {
		return siteapi.CookResponse{}, err
	}
	plan, recipe, err := service.cookPlan(id, groups, results, request.Overwrite, dryRun)
	if err != nil {
		return siteapi.CookResponse{}, err
	}
	if !dryRun {
		if err := service.refreshAfterCook(ctx, groups); err != nil {
			return siteapi.CookResponse{}, err
		}
	}
	return siteapi.CookResponse{Plan: plan, Recipe: recipe, Validation: validation}, nil
}

func (service *Service) writeCookConfigs(ctx context.Context, id string, groups []cookGroup, request siteapi.CookRequest, dryRun bool) ([]cook.ConfigResult, error) {
	multiNode := len(groups) > 1
	results := make([]cook.ConfigResult, 0, len(groups))
	for _, group := range groups {
		configID, err := cookGroupConfigID(id, group, multiNode)
		if err != nil {
			return nil, err
		}
		result, err := service.writeCookGroup(ctx, group, cook.NodeConfigRequest{
			ID:         configID,
			Overwrite:  request.Overwrite,
			DryRun:     dryRun,
			Components: group.components,
			Options:    cook.FilterOptionsForKinds(request.Options, group.components),
		})
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func cookGroupConfigID(id string, group cookGroup, multiNode bool) (string, error) {
	if !multiNode {
		return id, nil
	}
	return cook.SanitizedID(id + "-" + group.nodeID + "-" + strings.Join(componentKinds(group.components), "-"))
}

func (service *Service) cookPlan(id string, groups []cookGroup, results []cook.ConfigResult, overwrite bool, dryRun bool) (cook.Plan, *recipes.Recipe, error) {
	plan := cook.Plan{ID: id, PublicID: id, RequiresMasterRecipe: len(groups) > 1, Configs: results}
	if len(groups) <= 1 {
		if len(results) == 1 {
			plan.PublicID = results[0].ModelID
			plan.PublicImageID = results[0].ImageID
		}
		return plan, nil, nil
	}
	built := buildRecipe(id, groups, results)
	plan.PublicImageID = built.PublicImageID
	if dryRun {
		return plan, &built, nil
	}
	if service.recipeStore == nil {
		return cook.Plan{}, nil, fmt.Errorf("recipe store is not configured")
	}
	if err := service.recipeStore.Save(built, overwrite); err != nil {
		return cook.Plan{}, nil, err
	}
	return plan, &built, nil
}

func (service *Service) cookGroups(components []cook.Component) ([]cookGroup, error) {
	normalized, err := cook.NormalizedComponents(components)
	if err != nil {
		return nil, err
	}
	nodeURLs := service.nodeURLByID()
	groupsByNode := map[string]*cookGroup{}
	for _, component := range normalized {
		component.NodeID = strings.TrimSpace(component.NodeID)
		if component.NodeID == "" {
			component.NodeID = service.nodeID
		}
		if component.NodeURL, err = service.cookComponentNodeURL(component, nodeURLs); err != nil {
			return nil, err
		}
		group := groupsByNode[component.NodeID]
		if group == nil {
			group = &cookGroup{nodeID: component.NodeID, nodeURL: component.NodeURL, local: component.NodeID == service.nodeID}
			groupsByNode[component.NodeID] = group
		}
		group.components = append(group.components, component)
	}
	groups := make([]cookGroup, 0, len(groupsByNode))
	for _, group := range groupsByNode {
		groups = append(groups, *group)
	}
	sort.Slice(groups, func(left, right int) bool {
		return groups[left].nodeID < groups[right].nodeID
	})
	return groups, nil
}

func (service *Service) cookComponentNodeURL(component cook.Component, nodeURLs map[string]string) (string, error) {
	requestedNodeURL := strings.TrimSpace(component.NodeURL)
	if component.NodeID == service.nodeID {
		if requestedNodeURL != "" && service.nodeURL != "" && !cluster.BaseURLEqual(requestedNodeURL, service.nodeURL) {
			return "", fmt.Errorf("node url for %q does not match the registered node", component.NodeID)
		}
		return service.nodeURL, nil
	}
	resolvedNodeURL := nodeURLs[component.NodeID]
	if resolvedNodeURL == "" {
		return "", fmt.Errorf("node url for %q is required", component.NodeID)
	}
	if requestedNodeURL != "" && !cluster.BaseURLEqual(requestedNodeURL, resolvedNodeURL) {
		return "", fmt.Errorf("node url for %q does not match the registered node", component.NodeID)
	}
	return resolvedNodeURL, nil
}
