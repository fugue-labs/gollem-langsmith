package langsmith

import (
	"fmt"

	"github.com/fugue-labs/gollem/core"
)

// summarizeMessages extracts a simplified view of messages for LangSmith trace inputs.
func summarizeMessages(messages []core.ModelMessage) []map[string]any {
	var result []map[string]any
	for _, msg := range messages {
		switch m := msg.(type) {
		case core.ModelRequest:
			for _, part := range m.Parts {
				switch p := part.(type) {
				case core.SystemPromptPart:
					result = append(result, map[string]any{"role": "system", "content": p.Content})
				case core.UserPromptPart:
					result = append(result, map[string]any{"role": "user", "content": p.Content})
				case core.ToolReturnPart:
					result = append(result, map[string]any{"role": "tool", "tool": p.ToolName, "content": fmt.Sprintf("%v", p.Content)})
				}
			}
		case *core.ModelResponse:
			entry := map[string]any{"role": "assistant", "content": m.TextContent()}
			var tools []map[string]string
			for _, part := range m.Parts {
				if tc, ok := part.(core.ToolCallPart); ok {
					tools = append(tools, map[string]string{"tool": tc.ToolName, "args": tc.ArgsJSON})
				}
			}
			if len(tools) > 0 {
				entry["tool_calls"] = tools
			}
			result = append(result, entry)
		}
	}
	return result
}

// summarizeResponse extracts text + tool calls from a model response.
func summarizeResponse(resp *core.ModelResponse) map[string]any {
	out := map[string]any{}
	if text := resp.TextContent(); text != "" {
		out["text"] = text
	}
	var tools []map[string]string
	for _, part := range resp.Parts {
		if tc, ok := part.(core.ToolCallPart); ok {
			tools = append(tools, map[string]string{"tool": tc.ToolName, "args": tc.ArgsJSON})
		}
	}
	if len(tools) > 0 {
		out["tool_calls"] = tools
	}
	return out
}
