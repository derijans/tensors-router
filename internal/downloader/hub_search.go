package downloader

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	defaultSearchLimit       = 20
	maximumSearchLimit       = 100
	maximumCachedSearches    = 256
	searchCacheLifetime      = 30 * time.Second
	maximumSearchFilterCount = 64
	maximumSearchFilterBytes = 256
)

var allowedSearchSorts = map[string]bool{"": true, "createdAt": true, "downloads": true, "lastModified": true, "likes": true, "trendingScore": true}

func (client *HubClient) SearchPage(ctx context.Context, request SearchRequest, token string) (SearchPage, error) {
	if err := validateSearchRequest(request); err != nil {
		return SearchPage{}, err
	}
	cacheKey := searchCacheKey(request, token)
	if page, found := client.cachedSearchPage(cacheKey); found {
		return page, nil
	}
	var response []hubModel
	header, err := client.getJSONWithHeaders(ctx, "/models?"+searchQuery(request).Encode(), token, &response)
	if err != nil {
		return SearchPage{}, err
	}
	result := make([]SearchResult, 0, len(response))
	for _, model := range response {
		result = append(result, SearchResult{ID: model.ID, Author: model.Author, Downloads: model.Downloads, Likes: model.Likes, Gated: string(model.Gated), Tags: model.Tags, UpdatedAt: model.LastModified})
	}
	page := SearchPage{Results: result, NextCursor: nextCursor(header.Values("Link"))}
	client.storeSearchPage(cacheKey, page)
	return page, nil
}

func searchQuery(request SearchRequest) url.Values {
	query := url.Values{}
	setTrimmedQuery(query, "search", request.Query)
	setTrimmedQuery(query, "author", request.Author)
	setTrimmedQuery(query, "sort", request.Sort)
	setTrimmedQuery(query, "direction", request.Direction)
	setTrimmedQuery(query, "cursor", request.Cursor)
	setTrimmedQuery(query, "pipeline_tag", request.PipelineTag)
	setTrimmedQuery(query, "num_parameters", request.NumParameters)
	setTrimmedQuery(query, "inference", request.Inference)
	query.Set("limit", fmt.Sprintf("%d", searchLimit(request.Limit)))
	for _, field := range searchExpandedFields {
		query.Add("expand", field)
	}
	addTrimmedQueryValues(query, "filter", append(append([]string{}, request.Filters...), request.Tags...))
	addTrimmedQueryValues(query, "apps", request.Apps)
	addTrimmedQueryValues(query, "inference_provider", request.InferenceProviders)
	addTrimmedQueryValues(query, "trained_dataset", request.TrainedDatasets)
	if request.Gated == "true" || request.Gated == "false" {
		query.Set("gated", request.Gated)
	}
	return query
}

func searchLimit(limit int) int {
	if limit < 1 {
		return defaultSearchLimit
	}
	return min(limit, maximumSearchLimit)
}

func setTrimmedQuery(query url.Values, key string, value string) {
	if value = strings.TrimSpace(value); value != "" {
		query.Set(key, value)
	}
}

func addTrimmedQueryValues(query url.Values, key string, values []string) {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			query.Add(key, value)
		}
	}
}

func (client *HubClient) cachedSearchPage(cacheKey string) (SearchPage, bool) {
	client.mu.Lock()
	cached, found := client.cache[cacheKey]
	client.mu.Unlock()
	if found && time.Now().Before(cached.expires) {
		return cached.page, true
	}
	return SearchPage{}, false
}

func (client *HubClient) storeSearchPage(cacheKey string, page SearchPage) {
	client.mu.Lock()
	defer client.mu.Unlock()
	for key, cached := range client.cache {
		if time.Now().After(cached.expires) {
			delete(client.cache, key)
		}
	}
	for len(client.cache) >= maximumCachedSearches {
		for key := range client.cache {
			delete(client.cache, key)
			break
		}
	}
	client.cache[cacheKey] = cachedSearch{page: page, expires: time.Now().Add(searchCacheLifetime)}
}

func validateSearchRequest(request SearchRequest) error {
	if len(request.Query) > 256 || len(request.Author) > 128 || len(request.PipelineTag) > 128 || len(request.NumParameters) > 128 || len(request.Cursor) > 2048 {
		return fmt.Errorf("Hugging Face search parameter is too long")
	}
	if !allowedSearchSorts[strings.TrimSpace(request.Sort)] || request.Direction != "" && request.Direction != "-1" && request.Direction != "1" {
		return fmt.Errorf("invalid Hugging Face search ordering")
	}
	if !optionalBooleanText(request.Gated) || !optionalBooleanText(request.Inference) {
		return fmt.Errorf("invalid Hugging Face boolean search filter")
	}
	for _, values := range [][]string{request.Filters, request.Tags, request.Apps, request.InferenceProviders, request.TrainedDatasets} {
		if err := validateSearchFilterGroup(values); err != nil {
			return err
		}
	}
	return nil
}

func optionalBooleanText(value string) bool {
	return value == "" || value == "true" || value == "false"
}

func validateSearchFilterGroup(values []string) error {
	if len(values) > maximumSearchFilterCount {
		return fmt.Errorf("too many Hugging Face search filters")
	}
	for _, value := range values {
		if len(value) > maximumSearchFilterBytes {
			return fmt.Errorf("Hugging Face search filter is too long")
		}
	}
	return nil
}
