package proxy

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tensors-router/internal/catalog"
	"tensors-router/internal/cluster"
	"tensors-router/internal/recipes"
)

type recipeNodeProbe struct {
	expectedPath     string
	localModel       string
	response         string
	sawAuthorization bool
	sawLocalModel    bool
}

func (probe *recipeNodeProbe) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != probe.expectedPath {
			t.Fatalf("unexpected path %s, want %s", r.URL.Path, probe.expectedPath)
		}
		probe.sawAuthorization = r.Header.Get("Authorization") == "Bearer secret"
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		probe.sawLocalModel = strings.Contains(string(body), `"model":"`+probe.localModel+`"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(probe.response))
	}
}

func TestSplitRecipeRoutesTextAndImageToDifferentNodes(t *testing.T) {
	textProbe := &recipeNodeProbe{
		expectedPath: "/router/v1/node/inference/v1/chat/completions",
		localModel:   "llm-local",
		response:     `{"model":"llm-local","choices":[]}`,
	}
	imageProbe := &recipeNodeProbe{
		expectedPath: "/router/v1/node/inference/v1/images/generations",
		localModel:   "image-local-dream",
		response:     `{"model":"image-local-dream","data":[]}`,
	}
	textNode := httptest.NewServer(textProbe.handler(t))
	defer textNode.Close()
	imageNode := httptest.NewServer(imageProbe.handler(t))
	defer imageNode.Close()

	service := newMixedRecipeService(t, textNode.URL, imageNode.URL)

	assertRecipeLaneRouted(t, service, textProbe, "/v1/chat/completions", `{"model":"mixed","messages":[]}`, "mixed")
	assertRecipeLaneRouted(t, service, imageProbe, "/v1/images/generations", `{"model":"mixed-dream","prompt":"cat"}`, "mixed-dream")
}

func newMixedRecipeService(t *testing.T, textNodeURL string, imageNodeURL string) *Service {
	store, err := recipes.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(recipes.Recipe{
		ID:            "mixed",
		PublicID:      "mixed",
		PublicImageID: "mixed-dream",
		Text: &recipes.Component{
			Kind:           recipes.KindText,
			NodeID:         "text-node",
			NodeURL:        textNodeURL,
			ModelID:        "llm-local",
			ConfigFilename: "llm-local.kcpps",
		},
		Image: &recipes.Component{
			Kind:           recipes.KindImage,
			NodeID:         "image-node",
			NodeURL:        imageNodeURL,
			ModelID:        "image-local",
			ImageID:        "image-local-dream",
			ConfigFilename: "image-local.kcpps",
		},
	}, false); err != nil {
		t.Fatal(err)
	}
	registry := cluster.NewRegistry(cluster.RoleMaster, "master", "http://master")
	for nodeID, nodeURL := range map[string]string{"text-node": textNodeURL, "image-node": imageNodeURL} {
		if err := registry.UpdateNode(cluster.Snapshot{ProtocolVersion: cluster.ProtocolVersion, NodeID: nodeID, NodeURL: nodeURL}); err != nil {
			t.Fatal(err)
		}
	}
	return NewService(ServiceConfig{
		Backend:      &fakeBackend{url: mustParseURL(t, "http://127.0.0.1:1"), healthy: true},
		Registry:     registry,
		ClusterRole:  cluster.RoleMaster,
		Catalog:      catalog.New(t.TempDir()),
		ClusterToken: "secret",
		NodeID:       "master",
		SlaveURLs:    []string{textNodeURL, imageNodeURL},
		RecipeStore:  store,
		Logger:       log.New(io.Discard, "", 0),
	})
}

func assertRecipeLaneRouted(t *testing.T, service *Service, probe *recipeNodeProbe, path string, body string, publicModel string) {
	t.Helper()
	recorder := expectProxyStatus(t, service, modelJSONRequest(path, body, ""), http.StatusOK, path)
	if !probe.sawAuthorization || !probe.sawLocalModel {
		t.Fatalf("%s route failed auth=%t model=%t", path, probe.sawAuthorization, probe.sawLocalModel)
	}
	if !strings.Contains(recorder.Body.String(), `"model":"`+publicModel+`"`) {
		t.Fatalf("%s response was not rewritten: %s", path, recorder.Body.String())
	}
}
