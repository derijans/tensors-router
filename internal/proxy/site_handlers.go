package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"tensors-router/internal/cluster"
	"tensors-router/internal/cook"
	"tensors-router/internal/inventory"
	"tensors-router/internal/openai"
	"tensors-router/internal/recipes"
	"tensors-router/internal/siteapi"
)

func (service *Service) handleSiteInventory(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	response, err := service.siteInventory(r.Context())
	if err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "site_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, response)
}

func (service *Service) handleNodeSiteInventory(w http.ResponseWriter, r *http.Request) {
	node, err := service.localNodeInventory(r.Context())
	if err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "site_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, node)
}

func (service *Service) handleSiteCookPreview(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	var request siteapi.CookRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	response, err := service.planCook(r.Context(), request, true)
	if err != nil {
		if issues, ok := validationIssues(err); ok {
			openai.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error(), "validation": issues})
			return
		}
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, response)
}

func (service *Service) handleSiteCookApply(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	var request siteapi.CookRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	response, err := service.planCook(r.Context(), request, false)
	if err != nil {
		if issues, ok := validationIssues(err); ok {
			openai.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error(), "validation": issues})
			return
		}
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, response)
}

func (service *Service) handleSiteCookDelete(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	if service.recipeStore == nil {
		openai.WriteError(w, http.StatusBadRequest, "site_error", "recipe store is not configured")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/router/v1/site/cook/")
	if err := service.recipeStore.Delete(id); err != nil {
		openai.WriteError(w, http.StatusNotFound, "site_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (service *Service) handleNodeSiteConfigs(w http.ResponseWriter, r *http.Request) {
	var request cook.NodeConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	result, err := service.writeLocalCookConfig(r.Context(), request)
	if err != nil {
		if issues, ok := validationIssues(err); ok {
			openai.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error(), "validation": issues})
			return
		}
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	if !request.DryRun {
		if err := service.refreshLocalRegistry(); err != nil {
			openai.WriteError(w, http.StatusInternalServerError, "site_error", err.Error())
			return
		}
	}
	openai.WriteJSON(w, http.StatusOK, result)
}

func (service *Service) siteInventory(ctx context.Context) (siteapi.InventoryResponse, error) {
	localNode, err := service.localNodeInventory(ctx)
	if err != nil {
		return siteapi.InventoryResponse{}, err
	}
	nodes := []siteapi.NodeInventory{localNode}
	if service.clusterRole == cluster.RoleMaster {
		for _, nodeURL := range service.remoteInventoryURLs() {
			remoteNode := siteapi.NodeInventory{
				NodeURL:   nodeURL,
				Source:    cluster.SourceSlave,
				Role:      cluster.RoleSlave,
				Available: false,
			}
			err := service.clusterClient.JSON(ctx, http.MethodGet, nodeURL, "/router/v1/node/site/inventory", nil, &remoteNode)
			if err != nil {
				remoteNode.Error = err.Error()
				nodes = append(nodes, remoteNode)
				continue
			}
			nodes = append(nodes, remoteNode)
		}
	}
	models := service.siteModels()
	recipesList := []recipes.Recipe{}
	if service.recipeStore != nil {
		var err error
		recipesList, err = service.recipeStore.List()
		if err != nil {
			return siteapi.InventoryResponse{}, err
		}
	}
	return siteapi.InventoryResponse{
		Role:            service.clusterRole,
		NodeID:          service.nodeID,
		NodeURL:         service.nodeURL,
		Nodes:           nodes,
		Models:          models,
		Recipes:         recipesList,
		OptionCatalog:   cook.OptionCatalog(),
		ObservedOptions: observedOptions(nodes, models),
	}, nil
}

func (service *Service) localNodeInventory(ctx context.Context) (siteapi.NodeInventory, error) {
	models, err := service.localClusterModels()
	if err != nil {
		return siteapi.NodeInventory{}, err
	}
	files, err := inventory.Scan(service.fileRoots, models, service.nodeID)
	if err != nil {
		return siteapi.NodeInventory{}, err
	}
	return siteapi.NodeInventory{
		NodeID:      service.nodeID,
		NodeURL:     service.nodeURL,
		Source:      service.localSource(),
		Role:        service.clusterRole,
		BackendMode: service.backendMode,
		Available:   true,
		Hardware:    service.hardware.Info(ctx),
		Models:      models,
		Files:       files,
	}, nil
}

func (service *Service) siteModels() []cluster.Model {
	if service.registry != nil {
		return service.registry.Models()
	}
	models, err := service.localClusterModels()
	if err != nil {
		return []cluster.Model{}
	}
	return models
}

func (service *Service) localClusterModels() ([]cluster.Model, error) {
	models, err := service.catalog.List()
	if err != nil {
		return nil, err
	}
	return cluster.LocalModelsWithBackendMode(models, service.nodeID, service.nodeURL, service.localSource(), service.backendMode), nil
}

func (service *Service) localSource() string {
	if service.clusterRole == cluster.RoleMaster {
		return cluster.SourceMaster
	}
	return cluster.SourceLocal
}

func (service *Service) refreshLocalRegistry() error {
	if service.registry == nil {
		return nil
	}
	models, err := service.localClusterModels()
	if err != nil {
		return err
	}
	return service.registry.UpdateLocal(models)
}

func (service *Service) remoteInventoryURLs() []string {
	values := append([]string{}, service.slaveURLs...)
	if service.registry != nil {
		for _, model := range service.registry.Models() {
			if model.NodeID == service.nodeID || strings.TrimSpace(model.NodeURL) == "" {
				continue
			}
			values = append(values, model.NodeURL)
		}
	}
	return uniqueSortedStrings(values)
}

func (service *Service) siteControlAllowed() bool {
	return service.clusterRole == cluster.RoleStandalone || service.clusterRole == cluster.RoleMaster
}

func (service *Service) planCook(ctx context.Context, request siteapi.CookRequest, dryRun bool) (siteapi.CookResponse, error) {
	id, err := cook.SanitizedID(request.ID)
	if err != nil {
		return siteapi.CookResponse{}, err
	}
	options, err := cook.NormalizedOptions(request.Options)
	if err != nil {
		return siteapi.CookResponse{}, err
	}
	request.Options = options
	groups, err := service.cookGroups(request.Components)
	if err != nil {
		return siteapi.CookResponse{}, err
	}
	validation, err := service.validateCookGroups(ctx, groups, request.Options)
	if err != nil {
		return siteapi.CookResponse{}, err
	}
	multiNode := len(groups) > 1
	results := make([]cook.ConfigResult, 0, len(groups))
	for _, group := range groups {
		configID := id
		if multiNode {
			groupID, err := cook.SanitizedID(id + "-" + group.nodeID + "-" + strings.Join(componentKinds(group.components), "-"))
			if err != nil {
				return siteapi.CookResponse{}, err
			}
			configID = groupID
		}
		nodeRequest := cook.NodeConfigRequest{
			ID:         configID,
			Overwrite:  request.Overwrite,
			DryRun:     dryRun,
			Components: group.components,
			Options:    cook.FilterOptionsForKinds(request.Options, group.components),
		}
		result, err := service.writeCookGroup(ctx, group, nodeRequest)
		if err != nil {
			return siteapi.CookResponse{}, err
		}
		results = append(results, result)
	}
	plan := cook.Plan{
		ID:                   id,
		PublicID:             id,
		RequiresMasterRecipe: multiNode,
		Configs:              results,
	}
	var recipe *recipes.Recipe
	if multiNode {
		built := buildRecipe(id, groups, results)
		plan.PublicImageID = built.PublicImageID
		recipe = &built
		if !dryRun {
			if service.recipeStore == nil {
				return siteapi.CookResponse{}, fmt.Errorf("recipe store is not configured")
			}
			if err := service.recipeStore.Save(built, request.Overwrite); err != nil {
				return siteapi.CookResponse{}, err
			}
		}
	} else if len(results) == 1 {
		plan.PublicID = results[0].ModelID
		plan.PublicImageID = results[0].ImageID
	}
	if !dryRun {
		if err := service.refreshAfterCook(ctx, groups); err != nil {
			return siteapi.CookResponse{}, err
		}
	}
	return siteapi.CookResponse{Plan: plan, Recipe: recipe, Validation: validation}, nil
}

type cookGroup struct {
	nodeID     string
	nodeURL    string
	local      bool
	components []cook.Component
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
		requestedNodeURL := strings.TrimSpace(component.NodeURL)
		if component.NodeID == service.nodeID {
			if requestedNodeURL != "" && service.nodeURL != "" && !cluster.BaseURLEqual(requestedNodeURL, service.nodeURL) {
				return nil, fmt.Errorf("node url for %q does not match the registered node", component.NodeID)
			}
			component.NodeURL = service.nodeURL
		} else {
			resolvedNodeURL := nodeURLs[component.NodeID]
			if resolvedNodeURL == "" {
				return nil, fmt.Errorf("node url for %q is required", component.NodeID)
			}
			if requestedNodeURL != "" && !cluster.BaseURLEqual(requestedNodeURL, resolvedNodeURL) {
				return nil, fmt.Errorf("node url for %q does not match the registered node", component.NodeID)
			}
			component.NodeURL = resolvedNodeURL
		}
		group := groupsByNode[component.NodeID]
		if group == nil {
			group = &cookGroup{
				nodeID:  component.NodeID,
				nodeURL: component.NodeURL,
				local:   component.NodeID == service.nodeID,
			}
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
		FileRoots: service.fileRoots,
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

func uniqueSortedStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
