package langsmith

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/core"
	ls "github.com/langchain-ai/langsmith-go"
	"github.com/langchain-ai/langsmith-go/option"
)

// captureServer creates an httptest server that captures all batch ingest requests.
func captureServer(t *testing.T) (*httptest.Server, *sync.Mutex, *[]map[string]any) {
	t.Helper()
	var mu sync.Mutex
	received := &[]map[string]any{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode error: %v", err)
			w.WriteHeader(500)
			return
		}
		mu.Lock()
		*received = append(*received, body)
		mu.Unlock()
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]any{})
	}))
	return srv, &mu, received
}

func newTestHandler(t *testing.T, baseURL string) *Handler {
	t.Helper()
	client := ls.NewClient(
		option.WithBaseURL(baseURL),
		option.WithAPIKey("test-key"),
	)
	return New(
		WithClient(client),
		WithProjectName("test-project"),
		WithTags("test"),
		WithFlushInterval(50*time.Millisecond),
	)
}

func TestHandlerHookLifecycle(t *testing.T) {
	srv, mu, received := captureServer(t)
	defer srv.Close()

	h := newTestHandler(t, srv.URL)
	hook := h.Hook()

	ctx := context.Background()
	rc := &core.RunContext{
		RunID:   "run-1",
		RunStep: 0,
	}

	// Simulate: OnRunStart → OnModelRequest → OnModelResponse → OnToolStart → OnToolEnd → OnRunEnd
	hook.OnRunStart(ctx, rc, "hello world")

	rc.RunStep = 1
	hook.OnModelRequest(ctx, rc, []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{
			core.UserPromptPart{Content: "hello world"},
		}},
	})

	hook.OnModelResponse(ctx, rc, &core.ModelResponse{
		Parts:     []core.ModelResponsePart{core.TextPart{Content: "hi there"}},
		ModelName: "claude-sonnet-4-20250514",
		Usage:     core.Usage{InputTokens: 10, OutputTokens: 20},
	})

	rc.ToolCallID = "tool-call-1"
	hook.OnToolStart(ctx, rc, "tool-call-1", "search", `{"query":"test"}`)
	hook.OnToolEnd(ctx, rc, "tool-call-1", "search", "result data", nil)

	hook.OnRunEnd(ctx, rc, nil, nil)

	h.Close()

	mu.Lock()
	defer mu.Unlock()

	if len(*received) == 0 {
		t.Fatal("expected batch requests")
	}

	// Count total POST and PATCH entries across all batches.
	var totalPosts, totalPatches int
	for _, batch := range *received {
		if posts, ok := batch["post"].([]any); ok {
			totalPosts += len(posts)
		}
		if patches, ok := batch["patch"].([]any); ok {
			totalPatches += len(patches)
		}
	}

	// Expected: 3 POSTs (chain, llm, tool) and 3 PATCHes (chain, llm, tool)
	if totalPosts != 3 {
		t.Errorf("expected 3 POST runs, got %d", totalPosts)
	}
	if totalPatches != 3 {
		t.Errorf("expected 3 PATCH runs, got %d", totalPatches)
	}
}

func TestHandlerNestedTrace(t *testing.T) {
	srv, mu, received := captureServer(t)
	defer srv.Close()

	h := newTestHandler(t, srv.URL)
	hook := h.Hook()

	ctx := context.Background()

	// Outer agent run.
	outerRC := &core.RunContext{RunID: "outer-run", RunStep: 0}
	hook.OnRunStart(ctx, outerRC, "outer prompt")

	// Simulate a tool call that will delegate to an inner agent.
	outerRC.ToolCallID = "delegate-call"
	hook.OnToolStart(ctx, outerRC, "delegate-call", "delegate", `{"prompt":"inner task"}`)

	// Get the tool run info to inject parent context.
	toolRI := h.GetToolRunInfo("outer-run", "delegate-call")
	if toolRI == nil {
		t.Fatal("expected tool run info")
	}

	// Inner agent with parent trace context.
	innerCtx := withParentTrace(ctx, &parentTraceInfo{
		TraceID:      toolRI.TraceID,
		ParentRunID:  toolRI.LangSmithID,
		ParentDotted: toolRI.DottedOrder,
	})
	innerRC := &core.RunContext{RunID: "inner-run", RunStep: 0}
	hook.OnRunStart(innerCtx, innerRC, "inner prompt")
	hook.OnRunEnd(innerCtx, innerRC, nil, nil)

	hook.OnToolEnd(ctx, outerRC, "delegate-call", "delegate", "done", nil)
	hook.OnRunEnd(ctx, outerRC, nil, nil)

	h.Close()

	mu.Lock()
	defer mu.Unlock()

	if len(*received) == 0 {
		t.Fatal("expected batch requests")
	}

	// Verify that all POSTs share the same trace_id.
	traceIDs := map[string]bool{}
	for _, batch := range *received {
		if posts, ok := batch["post"].([]any); ok {
			for _, p := range posts {
				post := p.(map[string]any)
				if tid, ok := post["trace_id"].(string); ok {
					traceIDs[tid] = true
				}
			}
		}
	}
	if len(traceIDs) != 1 {
		t.Errorf("expected all runs to share one trace_id, got %d distinct IDs", len(traceIDs))
	}
}

func TestHandlerWithMetadata(t *testing.T) {
	srv, mu, received := captureServer(t)
	defer srv.Close()

	client := ls.NewClient(
		option.WithBaseURL(srv.URL),
		option.WithAPIKey("test-key"),
	)
	h := New(
		WithClient(client),
		WithProjectName("meta-test"),
		WithMetadata(map[string]any{"env": "test"}),
		WithFlushInterval(50*time.Millisecond),
	)
	hook := h.Hook()

	ctx := context.Background()
	rc := &core.RunContext{RunID: "meta-run"}
	hook.OnRunStart(ctx, rc, "test")
	hook.OnRunEnd(ctx, rc, nil, nil)
	h.Close()

	mu.Lock()
	defer mu.Unlock()

	if len(*received) == 0 {
		t.Fatal("expected batch requests")
	}

	// Find the POST for the chain run and check it has extra.metadata.
	for _, batch := range *received {
		if posts, ok := batch["post"].([]any); ok {
			for _, p := range posts {
				post := p.(map[string]any)
				if extra, ok := post["extra"].(map[string]any); ok {
					if meta, ok := extra["metadata"].(map[string]any); ok {
						if meta["env"] != "test" {
							t.Errorf("expected metadata env=test, got %v", meta["env"])
						}
						return
					}
				}
			}
		}
	}
	t.Error("metadata not found in any POST run")
}
