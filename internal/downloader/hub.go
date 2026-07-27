package downloader

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

const hubAPIURL = "https://huggingface.co/api"

type HubClient struct {
	baseURL string
	client  *http.Client
}

func NewHubClient(timeout time.Duration) *HubClient {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &HubClient{baseURL: hubAPIURL, client: &http.Client{Timeout: timeout}}
}

func (client *HubClient) Search(ctx context.Context, request SearchRequest, token string) ([]SearchResult, error) {
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
	query.Set("limit", fmt.Sprintf("%d", limit))
	for _, tag := range request.Tags {
		if tag = strings.TrimSpace(tag); tag != "" {
			query.Add("filter", tag)
		}
	}
	if request.Gated == "true" || request.Gated == "false" {
		query.Set("gated", request.Gated)
	}
	var response []hubModel
	if err := client.getJSON(ctx, "/models?"+query.Encode(), token, &response); err != nil {
		return nil, err
	}
	result := make([]SearchResult, 0, len(response))
	for _, model := range response {
		result = append(result, SearchResult{ID: model.ID, Author: model.Author, Downloads: model.Downloads, Likes: model.Likes, Gated: string(model.Gated), Tags: model.Tags, UpdatedAt: model.LastModified})
	}
	return result, nil
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
			return RepositoryDetails{}, fmt.Errorf("Hugging Face returned unsafe path %q", sibling.Path)
		}
		details.Files = append(details.Files, File{Path: sibling.Path, Size: sibling.Size(), LFSHash: sibling.LFS.OID, GitOID: sibling.BlobID, XetHash: sibling.XetHash, Unsafe: jsonStatus(sibling.SecurityStatus)})
	}
	return details, nil
}

func (client *HubClient) getJSON(ctx context.Context, endpoint string, token string, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(client.baseURL, "/")+endpoint, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	if token = strings.TrimSpace(token); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	contentType := strings.ToLower(response.Header.Get("Content-Type"))
	if strings.Contains(contentType, "text/html") {
		return fmt.Errorf("Hugging Face returned HTML instead of its JSON API")
	}
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return fmt.Errorf("Hugging Face access was denied; approve the repository in the browser and provide an authorized token")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Hugging Face API returned status %d", response.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 32<<20)).Decode(target); err != nil {
		return err
	}
	return nil
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
	LFS            struct {
		OID  string `json:"oid"`
		Size int64  `json:"size"`
	} `json:"lfs"`
}

func (file hubFile) Size() int64 {
	if file.LFS.Size == 0 {
		return file.SizeValue
	}
	return file.LFS.Size
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

func fileNameMatches(candidate string, selected string) bool {
	if candidate == selected {
		return true
	}
	base := path.Base(selected)
	return path.Base(candidate) == base
}
