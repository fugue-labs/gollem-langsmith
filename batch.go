package langsmith

import (
	"context"
	"sync"
	"time"

	ls "github.com/langchain-ai/langsmith-go"
)

// batchProcessor collects LangSmith run creates (POST) and updates (PATCH)
// and flushes them periodically via IngestBatch.
type batchProcessor struct {
	mu       sync.Mutex
	posts    []ls.RunParam
	patches  []ls.RunParam
	client   *ls.Client
	interval time.Duration
	done     chan struct{}
	wg       sync.WaitGroup
	logger   logger
}

type logger interface {
	Printf(format string, v ...any)
}

func newBatchProcessor(client *ls.Client, interval time.Duration, bufSize int, l logger) *batchProcessor {
	bp := &batchProcessor{
		posts:    make([]ls.RunParam, 0, bufSize),
		patches:  make([]ls.RunParam, 0, bufSize),
		client:   client,
		interval: interval,
		done:     make(chan struct{}),
		logger:   l,
	}
	bp.wg.Add(1)
	go bp.loop()
	return bp
}

func (bp *batchProcessor) loop() {
	defer bp.wg.Done()
	ticker := time.NewTicker(bp.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			bp.flush()
		case <-bp.done:
			bp.flush()
			return
		}
	}
}

// PostRun enqueues a new run creation.
func (bp *batchProcessor) PostRun(run ls.RunParam) {
	bp.mu.Lock()
	bp.posts = append(bp.posts, run)
	bp.mu.Unlock()
}

// PatchRun enqueues a run update.
func (bp *batchProcessor) PatchRun(run ls.RunParam) {
	bp.mu.Lock()
	bp.patches = append(bp.patches, run)
	bp.mu.Unlock()
}

func (bp *batchProcessor) flush() {
	bp.mu.Lock()
	posts := bp.posts
	patches := bp.patches
	bp.posts = make([]ls.RunParam, 0, cap(posts))
	bp.patches = make([]ls.RunParam, 0, cap(patches))
	bp.mu.Unlock()

	if len(posts) == 0 && len(patches) == 0 {
		return
	}

	params := ls.RunIngestBatchParams{}
	if len(posts) > 0 {
		params.Post = ls.F(posts)
	}
	if len(patches) > 0 {
		params.Patch = ls.F(patches)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := bp.client.Runs.IngestBatch(ctx, params); err != nil {
		if bp.logger != nil {
			bp.logger.Printf("langsmith: batch ingest error: %v", err)
		}
	}
}

// Close stops the background loop and performs a final flush.
func (bp *batchProcessor) Close() {
	close(bp.done)
	bp.wg.Wait()
}
