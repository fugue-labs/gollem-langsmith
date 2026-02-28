package langsmith

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/middleware"
)

// agentToolParams is the input schema for an agent-as-tool.
type agentToolParams struct {
	Prompt string `json:"prompt" jsonschema:"description=The prompt to send to the inner agent"`
}

// TracedAgentTool wraps an agent as a tool (like orchestration.AgentTool) but
// injects parent trace context so that the inner agent's runs appear nested
// under the outer agent's trace in LangSmith.
func TracedAgentTool[T any](name, description string, agent *core.Agent[T], h *Handler) core.Tool {
	return core.FuncTool[agentToolParams](
		name,
		description,
		func(ctx context.Context, rc *core.RunContext, params agentToolParams) (any, error) {
			// Look up the current tool run to get its LangSmith ID for parenting.
			toolRI := h.GetToolRunInfo(rc.RunID, rc.ToolCallID)
			if toolRI != nil {
				ctx = withParentTrace(ctx, &parentTraceInfo{
					TraceID:      toolRI.TraceID,
					ParentRunID:  toolRI.LangSmithID,
					ParentDotted: toolRI.DottedOrder,
				})
			}

			result, err := agent.Run(ctx, params.Prompt)
			if err != nil {
				return nil, fmt.Errorf("inner agent %q failed: %w", name, err)
			}

			output, marshalErr := json.Marshal(result.Output)
			if marshalErr != nil {
				return result.Output, nil //nolint:nilerr // graceful fallback
			}
			return json.RawMessage(output), nil
		},
	)
}

// TracedModel wraps a model with middleware that propagates trace context
// for team-based usage. When a team leader's trace context is present,
// all model calls through this wrapped model will appear under the leader's
// trace in LangSmith.
func TracedModel(model core.Model, h *Handler) core.Model {
	mw := middleware.Func(func(next middleware.RequestFunc) middleware.RequestFunc {
		return func(ctx context.Context, messages []core.ModelMessage, settings *core.ModelSettings, params *core.ModelRequestParameters) (*core.ModelResponse, error) {
			// The trace context is already propagated via context.Context.
			// This middleware is a pass-through that ensures the wrapped model
			// preserves context values through the middleware chain.
			return next(ctx, messages, settings, params)
		}
	})
	return middleware.Wrap(model, mw)
}
