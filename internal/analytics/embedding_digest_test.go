package analytics

import (
	"fmt"
	"io"
	"strings"
	"testing"
)

func observeEmbeddingResponse(t *testing.T, body string) Event {
	t.Helper()
	sink := &recordingEventSink{}
	observer := NewResponseObserver(sink, Event{Section: SectionEmbed, DurationMS: 100}, "application/json", io.NopCloser(strings.NewReader(body)))
	if _, err := io.ReadAll(observer); err != nil {
		t.Fatal(err)
	}
	if len(sink.events) != 1 {
		t.Fatalf("unexpected recorded events %#v", sink.events)
	}
	return sink.events[0]
}

func openAIEmbeddingData(vectors int, dimensions int) string {
	values := strings.TrimSuffix(strings.Repeat("-0.016362652182579041,", dimensions), ",")
	items := make([]string, vectors)
	for index := range items {
		items[index] = fmt.Sprintf(`{"embedding":[%s],"index":%d,"object":"embedding"}`, values, index)
	}
	return "[" + strings.Join(items, ",") + "]"
}

func TestResponseObserverReadsUsageAfterVectorsBeyondBufferLimit(t *testing.T) {
	data := openAIEmbeddingData(150, 1024)
	if len(data) <= observedBodyLimit {
		t.Fatalf("fixture must exceed the observed body limit, got %d bytes", len(data))
	}
	recorded := observeEmbeddingResponse(t, `{"data":`+data+`,"model":"embed-llama","object":"list","usage":{"prompt_tokens":1276,"total_tokens":1276}}`)

	if recorded.EmbeddingCount != 150 || recorded.InputTokens != 1276 || recorded.TotalTokens != 1276 {
		t.Fatalf("unexpected embedding analytics %#v", recorded)
	}
}

func TestResponseObserverCountsVectorsWhenUsageLeadsTheBody(t *testing.T) {
	recorded := observeEmbeddingResponse(t, `{"model":"embed-llama","object":"list","usage":{"prompt_tokens":6,"total_tokens":6},"data":`+openAIEmbeddingData(2, 8)+`}`)

	if recorded.EmbeddingCount != 2 || recorded.InputTokens != 6 {
		t.Fatalf("unexpected embedding analytics %#v", recorded)
	}
}

func TestResponseObserverCountsOllamaEmbedVectors(t *testing.T) {
	recorded := observeEmbeddingResponse(t, `{"model":"m","embeddings":[[0.1,0.2,[0.3]],[0.4,0.5,0.6],[0.7,0.8,0.9]],"total_duration":14143917,"prompt_eval_count":8}`)

	if recorded.EmbeddingCount != 3 || recorded.InputTokens != 8 {
		t.Fatalf("unexpected embedding analytics %#v", recorded)
	}
}

func TestResponseObserverCountsSingleEmbeddingAsOneVector(t *testing.T) {
	recorded := observeEmbeddingResponse(t, `{"embedding":[0.5670403838157654,0.009260174818336964,0.23178744316101074]}`)

	if recorded.EmbeddingCount != 1 {
		t.Fatalf("unexpected embedding analytics %#v", recorded)
	}
}

func TestResponseObserverIgnoresVectorKeysInsideStringsAndNestedObjects(t *testing.T) {
	recorded := observeEmbeddingResponse(t, `{"model":"a \"data\":[1,2] ]} name","meta":{"data":[1,2,3]},"data":[{"embedding":[1],"object":"[embedding]"}],"usage":{"prompt_tokens":4}}`)

	if recorded.EmbeddingCount != 1 || recorded.InputTokens != 4 {
		t.Fatalf("unexpected embedding analytics %#v", recorded)
	}
}
