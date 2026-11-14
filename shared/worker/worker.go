package worker

import (
	"context"
	"log"
	"time"
)

// Func is the function signature a Worker will call on each tick.
type Func func(ctx context.Context) error

// Worker runs a function on a fixed interval until its context is canceled.
type Worker struct {
	name     string
	interval time.Duration
	fn       Func
}

// New creates a Worker that will run fn every interval.
func New(name string, interval time.Duration, fn Func) *Worker {
	return &Worker{
		name:     name,
		interval: interval,
		fn:       fn,
	}
}

// Start runs the worker in a goroutine. It executes fn immediately on start,
// then on each tick. It returns when the context is canceled.
func (w *Worker) Start(ctx context.Context) {
	go w.run(ctx)
}

func (w *Worker) run(ctx context.Context) {
	log.Printf("[worker:%s] started (interval=%v)", w.name, w.interval)

	// Run immediately on start.
	w.execute(ctx)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[worker:%s] stopped", w.name)
			return
		case <-ticker.C:
			w.execute(ctx)
		}
	}
}

func (w *Worker) execute(ctx context.Context) {
	if err := w.fn(ctx); err != nil {
		log.Printf("[worker:%s] error: %v", w.name, err)
	}
}
