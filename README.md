# gollem-langsmith

LangSmith tracing adapter for [gollem](https://github.com/fugue-labs/gollem) agents. Provides hierarchical trace support for single agents, multi-agent delegation (AgentTool), and teams.

## Install

```
go get github.com/fugue-labs/gollem-langsmith
```

Requires `LANGSMITH_API_KEY` environment variable (or use `WithClient`/`WithClientOptions`).

## Usage

### Single Agent

```go
h := langsmith.New(
    langsmith.WithProjectName("my-project"),
    langsmith.WithTags("production"),
)
defer h.Close()

agent := core.NewAgent[string](model,
    core.WithHooks[string](h.Hook()),
)
result, err := agent.Run(ctx, "Hello")
```

### Nested Agents (TracedAgentTool)

For proper parent-child trace hierarchy in LangSmith:

```go
h := langsmith.New(langsmith.WithProjectName("my-project"))
defer h.Close()

inner := core.NewAgent[string](model,
    core.WithHooks[string](h.Hook()),
)
outer := core.NewAgent[string](model,
    core.WithHooks[string](h.Hook()),
    core.WithTools[string](
        langsmith.TracedAgentTool("delegate", "Delegate task", inner, h),
    ),
)
result, err := outer.Run(ctx, "Do something complex")
```

This produces a nested trace in LangSmith:

```
chain: "agent_run" (root)
  ├── llm: "claude-sonnet-4-20250514"
  ├── tool: "delegate"
  │   └── chain: "agent_run" (nested)
  │       ├── llm: "claude-sonnet-4-20250514"
  │       └── ...
  └── llm: "claude-sonnet-4-20250514"
```

### Teams

Use `WithTeammateHooks` to add tracing to spawned teammates:

```go
h := langsmith.New(langsmith.WithProjectName("my-project"))
defer h.Close()

t := team.NewTeam(team.TeamConfig{
    Model:       model,
    WorkerHooks: []core.Hook{h.Hook()},
})
```

Or per-teammate:

```go
t.SpawnTeammate(ctx, "worker", "task",
    team.WithTeammateHooks(h.Hook()),
)
```

## Options

| Option | Description |
|---|---|
| `WithClient(c)` | Use a pre-configured LangSmith client |
| `WithClientOptions(opts...)` | Configure the LangSmith client |
| `WithProjectName(name)` | Set the LangSmith project name |
| `WithTags(tags...)` | Add tags to every run |
| `WithMetadata(meta)` | Add metadata to every run |
| `WithFlushInterval(d)` | Set batch flush interval (default: 2s) |
| `WithBufferSize(n)` | Set batch buffer capacity (default: 100) |
| `WithLogger(l)` | Set a logger for diagnostics |
