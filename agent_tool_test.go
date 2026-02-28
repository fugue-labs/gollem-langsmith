package langsmith

import (
	"context"
	"testing"
)

func TestContextPropagation(t *testing.T) {
	ctx := context.Background()

	// No parent trace initially.
	if got := parentTraceFromContext(ctx); got != nil {
		t.Errorf("expected nil parent trace, got %+v", got)
	}

	info := &parentTraceInfo{
		TraceID:      "trace-1",
		ParentRunID:  "parent-run-1",
		ParentDotted: "20250101T000000000000Ztrace-1",
	}
	ctx = withParentTrace(ctx, info)

	got := parentTraceFromContext(ctx)
	if got == nil {
		t.Fatal("expected parent trace info")
	}
	if got.TraceID != "trace-1" {
		t.Errorf("TraceID = %q, want %q", got.TraceID, "trace-1")
	}
	if got.ParentRunID != "parent-run-1" {
		t.Errorf("ParentRunID = %q, want %q", got.ParentRunID, "parent-run-1")
	}
	if got.ParentDotted != "20250101T000000000000Ztrace-1" {
		t.Errorf("ParentDotted = %q, want %q", got.ParentDotted, "20250101T000000000000Ztrace-1")
	}
}
