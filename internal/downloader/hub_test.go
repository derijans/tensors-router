package downloader

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRepositoryUsesJSONAPIForNFAAAndBooleanGatedStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models/owner/model/revision/main" || r.Header.Get("Authorization") != "Bearer operation-token" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sha":"0123456789abcdef0123456789abcdef01234567","gated":false,"tags":["not-for-all-audiences"],"securityStatus":"safe","siblings":[{"rfilename":"model.gguf","lfs":{"oid":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","size":4}}]}`))
	}))
	defer server.Close()
	client := NewHubClient(0)
	client.baseURL = server.URL
	details, err := client.Repository(context.Background(), "owner/model", "main", "operation-token")
	if err != nil {
		t.Fatal(err)
	}
	if details.Gated != "false" || len(details.Files) != 1 || details.Files[0].Path != "model.gguf" {
		t.Fatalf("unexpected repository details %#v", details)
	}
}

func TestHubClientRejectsHTMLResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>interstitial</html>"))
	}))
	defer server.Close()
	client := NewHubClient(0)
	client.baseURL = server.URL
	if _, err := client.Search(context.Background(), SearchRequest{}, ""); err == nil {
		t.Fatal("expected HTML protocol rejection")
	}
}

func TestSearchSerializesDocumentedFilterParameters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("search") != "llama" || query.Get("author") != "owner" || query.Get("pipeline_tag") != "text-generation" || query.Get("num_parameters") != "min:7B,max:70B" || query.Get("gated") != "false" || query.Get("inference") != "true" || query.Get("sort") != "likes" || query.Get("direction") != "-1" {
			http.Error(w, "missing scalar filters", http.StatusBadRequest)
			return
		}
		if len(query["filter"]) != 2 || len(query["apps"]) != 1 || len(query["inference_provider"]) != 1 || len(query["trained_dataset"]) != 1 {
			http.Error(w, "missing repeated filters", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()
	client := NewHubClient(0)
	client.baseURL = server.URL
	_, err := client.Search(context.Background(), SearchRequest{Query: "llama", Author: "owner", Filters: []string{"gguf", "transformers"}, PipelineTag: "text-generation", NumParameters: "min:7B,max:70B", Apps: []string{"llama.cpp"}, Gated: "false", Inference: "true", InferenceProviders: []string{"hf-inference"}, TrainedDatasets: []string{"allenai/c4"}, Sort: "likes", Direction: "-1"}, "")
	if err != nil {
		t.Fatal(err)
	}
}

func TestSearchPageCachesAndReturnsNextCursor(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Link", `<https://huggingface.co/api/models?cursor=next-token>; rel="next"`)
		_, _ = w.Write([]byte(`[{"id":"owner/model","tags":["gguf"]}]`))
	}))
	defer server.Close()
	client := NewHubClient(0)
	client.baseURL = server.URL
	request := SearchRequest{Query: "model", Limit: 20}
	first, err := client.SearchPage(context.Background(), request, "token")
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.SearchPage(context.Background(), request, "token")
	if err != nil {
		t.Fatal(err)
	}
	if first.NextCursor != "next-token" || second.NextCursor != "next-token" || requests.Load() != 1 {
		t.Fatalf("pagination cache failed first=%#v second=%#v requests=%d", first, second, requests.Load())
	}
}

func TestSearchRateLimitAndInputBounds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "12")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	client := NewHubClient(0)
	client.baseURL = server.URL
	if _, err := client.Search(context.Background(), SearchRequest{}, ""); err == nil || !strings.Contains(err.Error(), "12") {
		t.Fatalf("unexpected rate-limit error %v", err)
	}
	if _, err := client.Search(context.Background(), SearchRequest{Query: strings.Repeat("x", 257)}, ""); err == nil {
		t.Fatal("oversized search query was accepted")
	}
}
