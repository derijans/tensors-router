package proxy

import (
	"context"
	"path"
	"strings"
	"sync"
	"time"

	"encoding/json"
	"io"
	"net/http"
	"tensors-router/internal/downloader"
	"tensors-router/internal/modelassets"
	"tensors-router/internal/openai"
	"tensors-router/internal/siteapi"
)

func (service *Service) handleSiteModelAssetCandidates(w http.ResponseWriter, r *http.Request) {
	request, ok := decodeModelAssetCandidateRequest(w, r)
	if !ok {
		return
	}
	target, err := service.configNodeTarget(request.NodeID, request.NodeURL)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid config node")
		return
	}
	if !target.local {
		var candidates []hfAssetCandidate
		if err := service.clusterClient.JSON(r.Context(), http.MethodPost, target.nodeURL, "/router/v1/node/site/model-assets/candidates", request, &candidates); err != nil {
			openai.WriteError(w, http.StatusBadGateway, "cluster_error", "candidate search failed")
			return
		}
		openai.WriteJSON(w, http.StatusOK, candidates)
		return
	}
	service.writeLocalModelAssetCandidates(w, r, request)
}

func (service *Service) handleNodeModelAssetCandidates(w http.ResponseWriter, r *http.Request) {
	request, ok := decodeModelAssetCandidateRequest(w, r)
	if !ok {
		return
	}
	service.writeLocalModelAssetCandidates(w, r, request)
}

func (service *Service) writeLocalModelAssetCandidates(w http.ResponseWriter, r *http.Request, request siteapi.ModelAssetCandidateRequest) {
	reference := modelassets.Reference{Hash: request.SHA256, Filename: request.Filename}
	if !modelassets.ValidHash(reference.Hash) || !modelassets.SafeFilename(reference.Filename) {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid asset reference")
		return
	}
	openai.WriteJSON(w, http.StatusOK, service.findHFCandidates(r.Context(), reference, request.Token))
}

func decodeModelAssetCandidateRequest(w http.ResponseWriter, r *http.Request) (siteapi.ModelAssetCandidateRequest, bool) {
	defer r.Body.Close()
	var request siteapi.ModelAssetCandidateRequest
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid candidate request")
		return siteapi.ModelAssetCandidateRequest{}, false
	}
	return request, true
}

type hfAssetCandidate struct {
	Repository     string `json:"repository"`
	RepositoryPath string `json:"repository_path"`
	Commit         string `json:"commit"`
	SHA256         string `json:"sha256,omitempty"`
	State          string `json:"state"`
}

func (service *Service) findHFCandidates(ctx context.Context, reference modelassets.Reference, token string) []hfAssetCandidate {
	if service.downloader == nil || !modelassets.ValidHash(reference.Hash) || !modelassets.SafeFilename(reference.Filename) {
		return nil
	}
	stem := strings.TrimSuffix(reference.Filename, path.Ext(reference.Filename))
	results, err := service.downloader.Search(ctx, downloader.SearchRequest{Query: stem, Limit: 20}, token)
	if err != nil {
		return nil
	}
	workers := 4
	if len(results) < workers {
		workers = len(results)
	}
	if workers == 0 {
		return nil
	}
	jobs := make(chan downloader.SearchResult)
	candidates := make(chan hfAssetCandidate, len(results))
	var group sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for result := range jobs {
				details, err := service.downloader.Repository(ctx, downloader.RepositoryRequest{Repository: result.ID, Token: token})
				if err != nil {
					continue
				}
				for _, file := range details.Files {
					if path.Base(file.Path) != reference.Filename {
						continue
					}
					state := "unverifiable"
					if modelassets.ValidHash(strings.ToLower(file.LFSHash)) {
						state = "mismatched"
						if strings.EqualFold(file.LFSHash, reference.Hash) {
							state = "exact"
						}
					}
					select {
					case candidates <- hfAssetCandidate{Repository: details.Repository, RepositoryPath: file.Path, Commit: details.Commit, SHA256: strings.ToLower(file.LFSHash), State: state}:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, result := range results {
			select {
			case jobs <- result:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { group.Wait(); close(candidates) }()
	values := make([]hfAssetCandidate, 0)
	for candidate := range candidates {
		values = append(values, candidate)
	}
	return values
}

func (service *Service) findUniqueExactHFOrigin(reference modelassets.Reference) (modelassets.Origin, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var exact []hfAssetCandidate
	for _, candidate := range service.findHFCandidates(ctx, reference, "") {
		if candidate.State == "exact" {
			exact = append(exact, candidate)
		}
	}
	if len(exact) != 1 {
		return modelassets.Origin{}, false
	}
	return modelassets.Origin{Repository: exact[0].Repository, Commit: exact[0].Commit, Path: exact[0].RepositoryPath}, true
}
