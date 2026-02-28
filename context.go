// Package langsmith provides a LangSmith tracing adapter for gollem agents.
package langsmith

import "context"

// parentTraceKey is the private context key for trace propagation.
type parentTraceKey struct{}

// parentTraceInfo carries trace context from a parent agent to a child agent.
type parentTraceInfo struct {
	TraceID      string
	ParentRunID  string // the LangSmith run ID of the tool run that triggered delegation
	ParentDotted string // dotted order prefix for child ordering
}

// withParentTrace injects parent trace info into a context.
func withParentTrace(ctx context.Context, info *parentTraceInfo) context.Context {
	return context.WithValue(ctx, parentTraceKey{}, info)
}

// parentTraceFromContext extracts parent trace info from context, or nil if absent.
func parentTraceFromContext(ctx context.Context) *parentTraceInfo {
	if v, ok := ctx.Value(parentTraceKey{}).(*parentTraceInfo); ok {
		return v
	}
	return nil
}
