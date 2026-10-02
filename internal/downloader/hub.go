package downloader

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const metadataAttempts = 3

var searchExpandedFields = []string{"author", "downloads", "likes", "gated", "tags", "lastModified"}

type HubClient struct {
	baseURL        string
	client         *http.Client
	downloadClient *http.Client
	wait           retryWaiter
	mu             sync.Mutex
	cache          map[string]cachedSearch
}

type cachedSearch struct {
	page    SearchPage
	expires time.Time
}

func NewHubClient(endpoint string, timeout time.Duration) *HubClient {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if strings.TrimSpace(endpoint) == "" {
		endpoint = defaultHubEndpoint
	}
	transport := newHubTransport(timeout)
	downloadClient := &http.Client{Transport: transport}
	downloadClient.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) > 0 && !strings.EqualFold(request.URL.Host, via[0].URL.Host) {
			request.Header.Del("Authorization")
		}
		if len(via) >= 10 {
			return fmt.Errorf("too many download redirects")
		}
		return nil
	}
	return &HubClient{baseURL: strings.TrimRight(endpoint, "/") + "/api", client: &http.Client{Transport: transport, Timeout: timeout}, downloadClient: downloadClient, wait: waitWithContext, cache: map[string]cachedSearch{}}
}

func (client *HubClient) FileURL(repository string, commit string, repositoryPath string) string {
	owner, name, _ := splitRepository(repository)
	apiURL, _ := url.Parse(client.baseURL)
	apiURL.Path = strings.TrimSuffix(apiURL.Path, "/api") + "/" + owner + "/" + name + "/resolve/" + commit + "/" + repositoryPath
	apiURL.RawQuery = "download=true"
	return apiURL.String()
}

func (client *HubClient) Search(ctx context.Context, request SearchRequest, token string) ([]SearchResult, error) {
	page, err := client.SearchPage(ctx, request, token)
	return page.Results, err
}

func (client *HubClient) SearchPage(ctx context.Context, request SearchRequest, token string) (SearchPage, error) {
	if err := validateSearchRequest(request); err != nil {
		return SearchPage{}, err
	}
	cacheKey := searchCacheKey(request, token)
	client.mu.Lock()
	cached, found := client.cache[cacheKey]
	client.mu.Unlock()
	if found && time.Now().Before(cached.expires) {
		return cached.page, nil
	}
	limit := request.Limit
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	query := url.Values{}
	if request.Query = strings.TrimSpace(request.Query); request.Query != "" {
		query.Set("search", request.Query)
	}
	if request.Author = strings.TrimSpace(request.Author); request.Author != "" {
		query.Set("author", request.Author)
	}
	if request.Sort = strings.TrimSpace(request.Sort); request.Sort != "" {
		query.Set("sort", request.Sort)
	}
	if request.Direction = strings.TrimSpace(request.Direction); request.Direction != "" {
		query.Set("direction", request.Direction)
	}
	if request.Cursor = strings.TrimSpace(request.Cursor); request.Cursor != "" {
		query.Set("cursor", request.Cursor)
	}
	if request.PipelineTag = strings.TrimSpace(request.PipelineTag); request.PipelineTag != "" {
		query.Set("pipeline_tag", request.PipelineTag)
	}
	if request.NumParameters = strings.TrimSpace(request.NumParameters); request.NumParameters != "" {
		query.Set("num_parameters", request.NumParameters)
	}
	if request.Inference = strings.TrimSpace(request.Inference); request.Inference != "" {
		query.Set("inference", request.Inference)
	}
	query.Set("limit", fmt.Sprintf("%d", limit))
	for _, field := range searchExpandedFields {
		query.Add("expand", field)
	}
	for _, tag := range append(append([]string{}, request.Filters...), request.Tags...) {
		if tag = strings.TrimSpace(tag); tag != "" {
			query.Add("filter", tag)
		}
	}
	for _, app := range request.Apps {
		if app = strings.TrimSpace(app); app != "" {
			query.Add("apps", app)
		}
	}
	for _, provider := range request.InferenceProviders {
		if provider = strings.TrimSpace(provider); provider != "" {
			query.Add("inference_provider", provider)
		}
	}
	for _, dataset := range request.TrainedDatasets {
		if dataset = strings.TrimSpace(dataset); dataset != "" {
			query.Add("trained_dataset", dataset)
		}
	}
	if request.Gated == "true" || request.Gated == "false" {
		query.Set("gated", request.Gated)
	}
	var response []hubModel
	header, err := client.getJSONWithHeaders(ctx, "/models?"+query.Encode(), token, &response)
	if err != nil {
		return SearchPage{}, err
	}
	result := make([]SearchResult, 0, len(response))
	for _, model := range response {
		result = append(result, SearchResult{ID: model.ID, Author: model.Author, Downloads: model.Downloads, Likes: model.Likes, Gated: string(model.Gated), Tags: model.Tags, UpdatedAt: model.LastModified})
	}
	client.mu.Lock()
	page := SearchPage{Results: result, NextCursor: nextCursor(header.Values("Link"))}
	for key, cached := range client.cache {
		if time.Now().After(cached.expires) {
			delete(client.cache, key)
		}
	}
	for len(client.cache) >= 256 {
		for key := range client.cache {
			delete(client.cache, key)
			break
		}
	}
	client.cache[cacheKey] = cachedSearch{page: page, expires: time.Now().Add(30 * time.Second)}
	client.mu.Unlock()
	return page, nil
}

func validateSearchRequest(request SearchRequest) error {
	if len(request.Query) > 256 || len(request.Author) > 128 || len(request.PipelineTag) > 128 || len(request.NumParameters) > 128 || len(request.Cursor) > 2048 {
		return fmt.Errorf("Hugging Face search parameter is too long")
	}
	allowedSort := map[string]bool{"": true, "createdAt": true, "downloads": true, "lastModified": true, "likes": true, "trendingScore": true}
	if !allowedSort[strings.TrimSpace(request.Sort)] || request.Direction != "" && request.Direction != "-1" && request.Direction != "1" {
		return fmt.Errorf("invalid Hugging Face search ordering")
	}
	if request.Gated != "" && request.Gated != "true" && request.Gated != "false" || request.Inference != "" && request.Inference != "true" && request.Inference != "false" {
		return fmt.Errorf("invalid Hugging Face boolean search filter")
	}
	groups := [][]string{request.Filters, request.Tags, request.Apps, request.InferenceProviders, request.TrainedDatasets}
	for _, values := range groups {
		if len(values) > 64 {
			return fmt.Errorf("too many Hugging Face search filters")
		}
		for _, value := range values {
			if len(value) > 256 {
				return fmt.Errorf("Hugging Face search filter is too long")
			}
		}
	}
	return nil
}

func searchCacheKey(request SearchRequest, token string) string {
	content, _ := json.Marshal(request)
	digest := sha256.Sum256(append(content, []byte(token)...))
	return hex.EncodeToString(digest[:])
}

func (client *HubClient) Repository(ctx context.Context, repository string, revision string, token string) (RepositoryDetails, error) {
	if err := ValidateRepository(repository); err != nil {
		return RepositoryDetails{}, err
	}
	revision = strings.TrimSpace(revision)
	if revision == "" {
		revision = "main"
	}
	owner, name, _ := splitRepository(repository)
	endpoint := "/models/" + url.PathEscape(owner) + "/" + url.PathEscape(name) + "/revision/" + url.PathEscape(revision) + "?blobs=true"
	var response hubModel
	if err := client.getJSON(ctx, endpoint, token, &response); err != nil {
		return RepositoryDetails{}, err
	}
	commit := strings.TrimSpace(response.SHA)
	if commit == "" {
		return RepositoryDetails{}, fmt.Errorf("Hugging Face response did not contain a resolved commit")
	}
	details := RepositoryDetails{Repository: repository, Revision: revision, Commit: commit, License: response.CardData.License, Gated: string(response.Gated), Security: jsonStatus(response.SecurityStatus), Files: make([]File, 0, len(response.Siblings))}
	for _, sibling := range response.Siblings {
		if err := ValidateRepositoryPath(sibling.Path); err != nil {
			details.Skipped = append(details.Skipped, SkippedFile{Path: sibling.Path, Reason: err.Error()})
			continue
		}
		details.Files = append(details.Files, File{Path: sibling.Path, Size: sibling.Size(), LFSHash: sibling.contentSHA256(), GitOID: sibling.gitBlobOfContent(), XetHash: sibling.XetHash, Unsafe: jsonStatus(sibling.SecurityStatus)})
	}
	return details, nil
}

func (client *HubClient) getJSON(ctx context.Context, endpoint string, token string, target any) error {
	_, err := client.getJSONWithHeaders(ctx, endpoint, token, target)
	return err
}

func (client *HubClient) getJSONWithHeaders(ctx context.Context, endpoint string, token string, target any) (http.Header, error) {
	var lastError error
	for attempt := 1; attempt <= metadataAttempts; attempt++ {
		if attempt > 1 {
			if err := client.wait(ctx, backoffDelay(attempt-1, lastError)); err != nil {
				return nil, err
			}
		}
		header, err := client.getJSONOnce(ctx, endpoint, token, target)
		if err == nil || !retryableMetadataError(ctx, err) {
			return header, err
		}
		lastError = err
	}
	return nil, lastError
}

func retryableMetadataError(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return false
	}
	var status upstreamStatusError
	if errors.As(err, &status) {
		return retryableStatus(status.status)
	}
	return ErrorCodeOf(err) == ErrorUpstreamUnavailable
}

func (client *HubClient) getJSONOnce(ctx context.Context, endpoint string, token string, target any) (http.Header, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(client.baseURL, "/")+endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", userAgent())
	if token = strings.TrimSpace(token); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, err
		}
		return nil, &Error{Code: ErrorUpstreamUnavailable, Message: fmt.Sprintf("Hugging Face is unreachable: %v", err)}
	}
	defer response.Body.Close()
	contentType := strings.ToLower(response.Header.Get("Content-Type"))
	switch {
	case response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden:
		return nil, newUpstreamStatusError(response, "Hugging Face denied access; check the repository name, approve gated repositories in the browser, and provide an authorized token")
	case response.StatusCode == http.StatusNotFound:
		return nil, newUpstreamStatusError(response, "Hugging Face repository or revision was not found")
	case response.StatusCode == http.StatusTooManyRequests:
		return nil, newUpstreamStatusError(response, fmt.Sprintf("Hugging Face rate limit reached; retry after %s", sanitizedRetryAfter(response.Header.Get("Retry-After"))))
	case response.StatusCode < 200 || response.StatusCode >= 300:
		return nil, newUpstreamStatusError(response, fmt.Sprintf("Hugging Face API returned status %d", response.StatusCode))
	case strings.Contains(contentType, "text/html"):
		return nil, &Error{Code: ErrorUpstreamUnavailable, Message: "Hugging Face returned HTML instead of its JSON API; check huggingface.endpoint and any intercepting proxy"}
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 32<<20)).Decode(target); err != nil {
		return nil, &Error{Code: ErrorUpstreamUnavailable, Message: fmt.Sprintf("decode Hugging Face response: %v", err)}
	}
	return response.Header.Clone(), nil
}

func nextCursor(links []string) string {
	for _, value := range links {
		for _, part := range strings.Split(value, ",") {
			if !strings.Contains(part, `rel="next"`) {
				continue
			}
			left, right := strings.Index(part, "<"), strings.Index(part, ">")
			if left < 0 || right <= left {
				continue
			}
			parsed, err := url.Parse(part[left+1 : right])
			if err == nil {
				return parsed.Query().Get("cursor")
			}
		}
	}
	return ""
}

func sanitizedRetryAfter(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 64 {
		return "the server-provided interval"
	}
	for _, character := range value {
		if character < 0x20 || character > 0x7e {
			return "the server-provided interval"
		}
	}
	return value
}

type hubModel struct {
	ID             string          `json:"id"`
	Author         string          `json:"author"`
	Downloads      int64           `json:"downloads"`
	Likes          int64           `json:"likes"`
	Gated          flexibleString  `json:"gated"`
	Tags           []string        `json:"tags"`
	LastModified   time.Time       `json:"lastModified"`
	SHA            string          `json:"sha"`
	SecurityStatus json.RawMessage `json:"securityStatus"`
	CardData       hubCardData     `json:"cardData"`
	Siblings       []hubFile       `json:"siblings"`
}

type hubCardData struct {
	License string `json:"license"`
}

type hubFile struct {
	Path           string          `json:"rfilename"`
	BlobID         string          `json:"blobId"`
	XetHash        string          `json:"xetHash"`
	SecurityStatus json.RawMessage `json:"securityStatus"`
	SizeValue      int64           `json:"size"`
	LFS            *struct {
		SHA256 string `json:"sha256"`
		OID    string `json:"oid"`
		Size   int64  `json:"size"`
	} `json:"lfs"`
}

func (file hubFile) Size() int64 {
	if file.LFS == nil || file.LFS.Size == 0 {
		return file.SizeValue
	}
	return file.LFS.Size
}

func (file hubFile) contentSHA256() string {
	if file.LFS == nil {
		return ""
	}
	if file.LFS.SHA256 != "" {
		return file.LFS.SHA256
	}
	return strings.TrimPrefix(file.LFS.OID, "sha256:")
}

func (file hubFile) gitBlobOfContent() string {
	if file.LFS != nil {
		return ""
	}
	return file.BlobID
}

type flexibleString string

func (value *flexibleString) UnmarshalJSON(content []byte) error {
	if string(content) == "null" {
		*value = ""
		return nil
	}
	var stringValue string
	if err := json.Unmarshal(content, &stringValue); err == nil {
		*value = flexibleString(stringValue)
		return nil
	}
	var booleanValue bool
	if err := json.Unmarshal(content, &booleanValue); err == nil {
		if booleanValue {
			*value = "true"
		} else {
			*value = "false"
		}
		return nil
	}
	return fmt.Errorf("expected string or boolean")
}

func jsonStatus(value json.RawMessage) string {
	if len(value) == 0 || string(value) == "null" {
		return ""
	}
	var stringValue string
	if err := json.Unmarshal(value, &stringValue); err == nil {
		return stringValue
	}
	return "pending"
}
