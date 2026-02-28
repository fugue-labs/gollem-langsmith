package langsmith

import (
	"log"
	"time"

	ls "github.com/langchain-ai/langsmith-go"
	"github.com/langchain-ai/langsmith-go/option"
)

type config struct {
	client        *ls.Client
	clientOpts    []option.RequestOption
	projectName   string
	tags          []string
	metadata      map[string]any
	flushInterval time.Duration
	bufferSize    int
	logger        *log.Logger
	traceID       string // pre-set trace ID for root runs
}

func defaultConfig() config {
	return config{
		projectName:   "default",
		flushInterval: 2 * time.Second,
		bufferSize:    100,
	}
}

// Option configures a Handler.
type Option func(*config)

// WithClient sets a pre-configured LangSmith client.
func WithClient(c *ls.Client) Option {
	return func(cfg *config) { cfg.client = c }
}

// WithClientOptions sets LangSmith client options (e.g. option.WithBaseURL).
// Ignored if WithClient is also set.
func WithClientOptions(opts ...option.RequestOption) Option {
	return func(cfg *config) { cfg.clientOpts = opts }
}

// WithProjectName sets the LangSmith project name (session_name).
func WithProjectName(name string) Option {
	return func(cfg *config) { cfg.projectName = name }
}

// WithTags adds tags to every run.
func WithTags(tags ...string) Option {
	return func(cfg *config) { cfg.tags = append(cfg.tags, tags...) }
}

// WithMetadata adds metadata to every run.
func WithMetadata(meta map[string]any) Option {
	return func(cfg *config) { cfg.metadata = meta }
}

// WithFlushInterval sets how often the batch processor flushes.
func WithFlushInterval(d time.Duration) Option {
	return func(cfg *config) { cfg.flushInterval = d }
}

// WithBufferSize sets the batch buffer capacity.
func WithBufferSize(n int) Option {
	return func(cfg *config) { cfg.bufferSize = n }
}

// WithLogger sets a logger for trace diagnostics.
func WithLogger(l *log.Logger) Option {
	return func(cfg *config) { cfg.logger = l }
}

// WithTraceID sets a pre-determined trace ID for root agent runs.
// This allows the caller to know the trace ID before the agent starts,
// e.g. for logging or post-run score injection.
func WithTraceID(id string) Option {
	return func(cfg *config) { cfg.traceID = id }
}
