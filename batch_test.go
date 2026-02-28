package langsmith

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	ls "github.com/langchain-ai/langsmith-go"
	"github.com/langchain-ai/langsmith-go/option"
)

func TestBatchProcessor(t *testing.T) {
	var mu sync.Mutex
	var received []map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode error: %v", err)
			w.WriteHeader(500)
			return
		}
		mu.Lock()
		received = append(received, body)
		mu.Unlock()
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer srv.Close()

	client := ls.NewClient(
		option.WithBaseURL(srv.URL),
		option.WithAPIKey("test-key"),
	)

	bp := newBatchProcessor(client, 50*time.Millisecond, 10, nil)

	bp.PostRun(ls.RunParam{
		ID:      ls.F("run-1"),
		Name:    ls.F("test"),
		RunType: ls.F(ls.RunRunTypeChain),
	})
	bp.PatchRun(ls.RunParam{
		ID:      ls.F("run-1"),
		EndTime: ls.F("2025-01-01T00:00:00.000000Z"),
	})

	// Wait for at least one flush cycle.
	time.Sleep(150 * time.Millisecond)
	bp.Close()

	mu.Lock()
	defer mu.Unlock()

	if len(received) == 0 {
		t.Fatal("expected at least one batch request")
	}

	// Check the first batch has both post and patch.
	batch := received[0]
	if posts, ok := batch["post"]; !ok || posts == nil {
		t.Error("expected post array in batch")
	}
	if patches, ok := batch["patch"]; !ok || patches == nil {
		t.Error("expected patch array in batch")
	}
}

func TestBatchProcessorCloseFlushes(t *testing.T) {
	var mu sync.Mutex
	var received []map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		received = append(received, body)
		mu.Unlock()
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer srv.Close()

	client := ls.NewClient(
		option.WithBaseURL(srv.URL),
		option.WithAPIKey("test-key"),
	)

	// Very long interval so only Close triggers flush.
	bp := newBatchProcessor(client, 10*time.Minute, 10, nil)

	bp.PostRun(ls.RunParam{
		ID:   ls.F("run-close"),
		Name: ls.F("test"),
	})

	bp.Close()

	mu.Lock()
	defer mu.Unlock()

	if len(received) == 0 {
		t.Fatal("Close should have flushed pending runs")
	}
}
