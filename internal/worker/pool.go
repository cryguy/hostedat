package worker

import (
	"fmt"
	"strings"
	"sync"

	v8 "github.com/tommie/v8go"
)

// v8Worker is a single V8 isolate+context pair in the pool.
type v8Worker struct {
	iso       *v8.Isolate
	ctx       *v8.Context
	eventLoop *eventLoop
}

// v8Pool manages a fixed-size pool of pre-warmed V8 workers.
type v8Pool struct {
	workers chan *v8Worker
	size    int
	mu      sync.Mutex
}

// setupFunc configures a V8 context with Web APIs, crypto, console, etc.
type setupFunc func(iso *v8.Isolate, ctx *v8.Context, el *eventLoop) error

// newV8Pool creates a pool of V8 isolates, each configured with the given
// setup functions and loaded with the worker script.
func newV8Pool(size int, source string, setupFns []setupFunc) (*v8Pool, error) {
	pool := &v8Pool{
		workers: make(chan *v8Worker, size),
		size:    size,
	}

	for i := 0; i < size; i++ {
		w, err := newV8Worker(source, setupFns)
		if err != nil {
			pool.dispose()
			return nil, fmt.Errorf("creating pool worker %d: %w", i, err)
		}
		pool.workers <- w
	}

	return pool, nil
}

// newV8Worker creates a single V8 isolate+context, runs all setup functions,
// and loads the worker script.
func newV8Worker(source string, setupFns []setupFunc) (*v8Worker, error) {
	iso := v8.NewIsolate()
	ctx := v8.NewContext(iso)
	el := newEventLoop()

	// Run all setup functions (Web APIs, crypto, console, fetch, etc.).
	for _, setup := range setupFns {
		if err := setup(iso, ctx, el); err != nil {
			ctx.Close()
			iso.Dispose()
			return nil, fmt.Errorf("setup: %w", err)
		}
	}

	// Compile and run the worker script.
	wrapped := wrapESModule(source)
	script, err := iso.CompileUnboundScript(wrapped, "worker.js", v8.CompileOptions{})
	if err != nil {
		ctx.Close()
		iso.Dispose()
		return nil, fmt.Errorf("compiling worker script: %w", err)
	}

	if _, err := script.Run(ctx); err != nil {
		ctx.Close()
		iso.Dispose()
		return nil, fmt.Errorf("running worker script: %w", err)
	}

	// Verify __worker_module__ was set.
	check, err := ctx.RunScript("typeof globalThis.__worker_module__ !== 'undefined'", "check.js")
	if err != nil || !check.Boolean() {
		ctx.Close()
		iso.Dispose()
		return nil, fmt.Errorf("worker script did not export a default module")
	}

	return &v8Worker{iso: iso, ctx: ctx, eventLoop: el}, nil
}

// get acquires a worker from the pool. Blocks until one is available.
func (p *v8Pool) get() (*v8Worker, error) {
	w, ok := <-p.workers
	if !ok {
		return nil, fmt.Errorf("worker pool is closed")
	}
	return w, nil
}

// put returns a worker to the pool after resetting its event loop.
func (p *v8Pool) put(w *v8Worker) {
	w.eventLoop.reset()
	select {
	case p.workers <- w:
	default:
		// Pool full (shouldn't happen), dispose the worker.
		w.ctx.Close()
		w.iso.Dispose()
	}
}

// dispose closes all workers in the pool.
func (p *v8Pool) dispose() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for {
		select {
		case w := <-p.workers:
			w.ctx.Close()
			w.iso.Dispose()
		default:
			return
		}
	}
}

// wrapESModule transforms an ES module source into a script that assigns the
// default export to globalThis.__worker_module__. Handles the common pattern:
//
//	export default { fetch(request, env, ctx) { ... } }
//
// For complex modules with imports, the deploy pipeline should bundle with
// esbuild in IIFE format before storing.
func wrapESModule(source string) string {
	return strings.Replace(source, "export default", "globalThis.__worker_module__ =", 1)
}
