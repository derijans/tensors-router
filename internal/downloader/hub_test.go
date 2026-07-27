package downloader

import (
	"context"
	"net/http"
	"net/http/httptest"
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
