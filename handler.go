package langsmith

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/fugue-labs/gollem/core"
	"github.com/google/uuid"
	ls "github.com/langchain-ai/langsmith-go"
)

// runInfo tracks the LangSmith run state for an in-flight gollem event.
type runInfo struct {
	LangSmithID string
	TraceID     string
	DottedOrder string
	StartTime   time.Time
}

// Handler is the LangSmith tracing handler shared across all agents in a trace.
type Handler struct {
	mu        sync.Mutex
	agentRuns map[string]*runInfo // gollem RunID → LangSmith chain run
	llmRuns   map[string]*runInfo // "runID:step" → in-flight LLM run
	toolRuns  map[string]*runInfo // "runID:toolCallID" → in-flight tool run
	batch     *batchProcessor
	cfg       config
}

// New creates a new LangSmith Handler with the given options.
func New(opts ...Option) *Handler {
	cfg := defaultConfig()
	for _, o := range opts {
		o(&cfg)
	}
	client := cfg.client
	if client == nil {
		client = ls.NewClient(cfg.clientOpts...)
	}
	var l logger
	if cfg.logger != nil {
		l = cfg.logger
	}
	return &Handler{
		agentRuns: make(map[string]*runInfo),
		llmRuns:   make(map[string]*runInfo),
		toolRuns:  make(map[string]*runInfo),
		batch:     newBatchProcessor(client, cfg.flushInterval, cfg.bufferSize, l),
		cfg:       cfg,
	}
}

// Hook returns a gollem Hook that sends trace data to LangSmith.
func (h *Handler) Hook() core.Hook {
	return core.Hook{
		OnRunStart:     h.onRunStart,
		OnRunEnd:       h.onRunEnd,
		OnModelRequest: h.onModelRequest,
		OnModelResponse: h.onModelResponse,
		OnToolStart:    h.onToolStart,
		OnToolEnd:      h.onToolEnd,
	}
}

// Close flushes any remaining trace data. Call this when the root agent is done.
func (h *Handler) Close() error {
	h.batch.Close()
	return nil
}

func (h *Handler) onRunStart(ctx context.Context, rc *core.RunContext, prompt string) {
	now := time.Now().UTC()
	id := uuid.New().String()

	var traceID, parentRunID, dottedOrder string

	if parent := parentTraceFromContext(ctx); parent != nil {
		// Nested agent: inherit trace, parent from the tool run that delegated.
		traceID = parent.TraceID
		parentRunID = parent.ParentRunID
		dottedOrder = parent.ParentDotted + "." + formatDottedOrder(now, id)
	} else {
		// Root agent: start a new trace.
		// Use pre-set trace ID if configured (allows caller to know the ID
		// before the run starts, e.g. for logging). Only used once.
		h.mu.Lock()
		if h.cfg.traceID != "" {
			id = h.cfg.traceID
			h.cfg.traceID = "" // consume: subsequent root runs get fresh IDs
		}
		h.mu.Unlock()
		traceID = id
		dottedOrder = formatDottedOrder(now, id)
	}

	ri := &runInfo{
		LangSmithID: id,
		TraceID:     traceID,
		DottedOrder: dottedOrder,
		StartTime:   now,
	}

	h.mu.Lock()
	h.agentRuns[rc.RunID] = ri
	h.mu.Unlock()

	run := ls.RunParam{
		ID:          ls.F(id),
		Name:        ls.F("agent_run"),
		RunType:     ls.F(ls.RunRunTypeChain),
		StartTime:   ls.F(formatTime(now)),
		TraceID:     ls.F(traceID),
		DottedOrder: ls.F(dottedOrder),
		SessionName: ls.F(h.cfg.projectName),
		Inputs:      ls.F(map[string]any{"prompt": prompt}),
	}
	if parentRunID != "" {
		run.ParentRunID = ls.F(parentRunID)
	}
	if len(h.cfg.tags) > 0 {
		run.Tags = ls.F(h.cfg.tags)
	}
	if len(h.cfg.metadata) > 0 {
		run.Extra = ls.F(map[string]any{"metadata": h.cfg.metadata})
	}
	h.batch.PostRun(run)
}

func (h *Handler) onRunEnd(ctx context.Context, rc *core.RunContext, messages []core.ModelMessage, err error) {
	now := time.Now().UTC()

	h.mu.Lock()
	ri, ok := h.agentRuns[rc.RunID]
	if ok {
		delete(h.agentRuns, rc.RunID)
	}
	h.mu.Unlock()
	if !ok {
		return
	}

	patch := ls.RunParam{
		ID:          ls.F(ri.LangSmithID),
		EndTime:     ls.F(formatTime(now)),
		TraceID:     ls.F(ri.TraceID),
		DottedOrder: ls.F(ri.DottedOrder),
	}
	if err != nil {
		patch.Error = ls.F(err.Error())
	} else if len(messages) > 0 {
		patch.Outputs = ls.F(map[string]any{"messages": summarizeMessages(messages)})
	}
	h.batch.PatchRun(patch)
}

func (h *Handler) onModelRequest(ctx context.Context, rc *core.RunContext, messages []core.ModelMessage) {
	now := time.Now().UTC()
	id := uuid.New().String()

	h.mu.Lock()
	parent, ok := h.agentRuns[rc.RunID]
	h.mu.Unlock()
	if !ok {
		return
	}

	key := fmt.Sprintf("%s:%d", rc.RunID, rc.RunStep)
	dottedOrder := parent.DottedOrder + "." + formatDottedOrder(now, id)

	ri := &runInfo{
		LangSmithID: id,
		TraceID:     parent.TraceID,
		DottedOrder: dottedOrder,
		StartTime:   now,
	}

	h.mu.Lock()
	h.llmRuns[key] = ri
	h.mu.Unlock()

	run := ls.RunParam{
		ID:          ls.F(id),
		Name:        ls.F("llm"),
		RunType:     ls.F(ls.RunRunTypeLlm),
		StartTime:   ls.F(formatTime(now)),
		TraceID:     ls.F(parent.TraceID),
		DottedOrder: ls.F(dottedOrder),
		ParentRunID: ls.F(parent.LangSmithID),
		SessionName: ls.F(h.cfg.projectName),
		Inputs:      ls.F(map[string]any{"messages": summarizeMessages(messages)}),
	}
	if len(h.cfg.tags) > 0 {
		run.Tags = ls.F(h.cfg.tags)
	}
	h.batch.PostRun(run)
}

func (h *Handler) onModelResponse(ctx context.Context, rc *core.RunContext, response *core.ModelResponse) {
	now := time.Now().UTC()
	key := fmt.Sprintf("%s:%d", rc.RunID, rc.RunStep)

	h.mu.Lock()
	ri, ok := h.llmRuns[key]
	if ok {
		delete(h.llmRuns, key)
	}
	h.mu.Unlock()
	if !ok {
		return
	}

	patch := ls.RunParam{
		ID:          ls.F(ri.LangSmithID),
		EndTime:     ls.F(formatTime(now)),
		TraceID:     ls.F(ri.TraceID),
		DottedOrder: ls.F(ri.DottedOrder),
		Outputs:     ls.F(map[string]any{"response": summarizeResponse(response)}),
	}
	if response.ModelName != "" {
		patch.Name = ls.F(response.ModelName)
	}
	if response.Usage.TotalTokens() > 0 {
		patch.Extra = ls.F(map[string]any{
			"usage": map[string]any{
				"input_tokens":  response.Usage.InputTokens,
				"output_tokens": response.Usage.OutputTokens,
				"total_tokens":  response.Usage.TotalTokens(),
			},
		})
	}
	h.batch.PatchRun(patch)
}

func (h *Handler) onToolStart(ctx context.Context, rc *core.RunContext, toolName string, argsJSON string) {
	now := time.Now().UTC()
	id := uuid.New().String()

	h.mu.Lock()
	parent, ok := h.agentRuns[rc.RunID]
	h.mu.Unlock()
	if !ok {
		return
	}

	key := fmt.Sprintf("%s:%s", rc.RunID, rc.ToolCallID)
	dottedOrder := parent.DottedOrder + "." + formatDottedOrder(now, id)

	ri := &runInfo{
		LangSmithID: id,
		TraceID:     parent.TraceID,
		DottedOrder: dottedOrder,
		StartTime:   now,
	}

	h.mu.Lock()
	h.toolRuns[key] = ri
	h.mu.Unlock()

	run := ls.RunParam{
		ID:          ls.F(id),
		Name:        ls.F(toolName),
		RunType:     ls.F(ls.RunRunTypeTool),
		StartTime:   ls.F(formatTime(now)),
		TraceID:     ls.F(parent.TraceID),
		DottedOrder: ls.F(dottedOrder),
		ParentRunID: ls.F(parent.LangSmithID),
		SessionName: ls.F(h.cfg.projectName),
		Inputs:      ls.F(map[string]any{"tool": toolName, "args": argsJSON}),
	}
	if len(h.cfg.tags) > 0 {
		run.Tags = ls.F(h.cfg.tags)
	}
	h.batch.PostRun(run)
}

func (h *Handler) onToolEnd(ctx context.Context, rc *core.RunContext, toolName string, result string, err error) {
	now := time.Now().UTC()
	key := fmt.Sprintf("%s:%s", rc.RunID, rc.ToolCallID)

	h.mu.Lock()
	ri, ok := h.toolRuns[key]
	if ok {
		delete(h.toolRuns, key)
	}
	h.mu.Unlock()
	if !ok {
		return
	}

	patch := ls.RunParam{
		ID:          ls.F(ri.LangSmithID),
		EndTime:     ls.F(formatTime(now)),
		TraceID:     ls.F(ri.TraceID),
		DottedOrder: ls.F(ri.DottedOrder),
	}
	if err != nil {
		patch.Error = ls.F(err.Error())
	} else {
		patch.Outputs = ls.F(map[string]any{"result": result})
	}
	h.batch.PatchRun(patch)
}

// GetToolRunInfo returns the LangSmith run info for an in-flight tool call.
// This is used by TracedAgentTool to inject parent context.
func (h *Handler) GetToolRunInfo(runID, toolCallID string) *runInfo {
	key := fmt.Sprintf("%s:%s", runID, toolCallID)
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.toolRuns[key]
}
