package worker

import (
	"fmt"
	"regexp"
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
func newV8Pool(size int, source string, setupFns []setupFunc, memoryLimitMB int) (*v8Pool, error) {
	pool := &v8Pool{
		workers: make(chan *v8Worker, size),
		size:    size,
	}

	for i := 0; i < size; i++ {
		w, err := newV8Worker(source, setupFns, memoryLimitMB)
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
func newV8Worker(source string, setupFns []setupFunc, memoryLimitMB int) (*v8Worker, error) {
	var iso *v8.Isolate
	if memoryLimitMB > 0 {
		heapSize := uint64(memoryLimitMB) * 1024 * 1024
		iso = v8.NewIsolate(v8.WithResourceConstraints(heapSize/2, heapSize))
	} else {
		iso = v8.NewIsolate()
	}
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

// reExportBlock matches an export { ... } block at the end of a script,
// as produced by esbuild in ESM format.
var reExportBlock = regexp.MustCompile(`(?s)export\s*\{([^}]+)\}\s*;?\s*$`)

// reExportDefault matches "export default" at the start of a line,
// avoiding false positives inside string literals or comments.
var reExportDefault = regexp.MustCompile(`(?m)^export\s+default\s+`)

// reInline matches inline named exports (export function, export const, etc.).
var reInline = regexp.MustCompile(`export\s+(async\s+function|function|const|let|var|class)\s+(\w+)`)

// wrapESModule transforms an ES module source into a script that assigns
// exports to globalThis.__worker_module__. Handles multiple patterns:
//
//  1. export default { fetch(request, env, ctx) { ... } }
//  2. export { name as default }  (esbuild output)
//  3. export { fetch, scheduled }  (named exports)
//  4. export function fetch(...)   (inline named exports)
func wrapESModule(source string) string {
	// Pattern 1: direct "export default ..." at line start
	if loc := reExportDefault.FindStringIndex(source); loc != nil {
		return source[:loc[0]] + "globalThis.__worker_module__ = " + source[loc[1]:]
	}

	// Pattern 2 & 3: export { ... } block (esbuild output style)
	if m := reExportBlock.FindStringSubmatchIndex(source); m != nil {
		block := source[m[2]:m[3]]
		defaultName, namedExports := parseExportBlock(block)
		result := source[:m[0]]

		if defaultName != "" {
			result += "globalThis.__worker_module__ = " + defaultName + ";\n"
		} else if len(namedExports) > 0 {
			result += "globalThis.__worker_module__ = { " + strings.Join(namedExports, ", ") + " };\n"
		}
		return result
	}

	// Pattern 4: inline named exports (export function, export const, etc.)
	var exportedNames []string
	result := reInline.ReplaceAllStringFunc(source, func(match string) string {
		parts := reInline.FindStringSubmatch(match)
		exportedNames = append(exportedNames, parts[2])
		return parts[1] + " " + parts[2]
	})
	if len(exportedNames) > 0 {
		result += "\nglobalThis.__worker_module__ = { " + strings.Join(exportedNames, ", ") + " };\n"
		return result
	}

	// Fallback: return as-is.
	return source
}

// parseExportBlock parses the contents of an export { ... } block.
// Returns the default export name (if any) and a list of named exports.
func parseExportBlock(block string) (defaultName string, names []string) {
	for _, entry := range strings.Split(block, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.Fields(entry)
		switch {
		case len(parts) == 3 && parts[1] == "as" && parts[2] == "default":
			defaultName = parts[0]
		case len(parts) == 3 && parts[1] == "as":
			names = append(names, parts[2]+": "+parts[0])
		case len(parts) == 1:
			names = append(names, parts[0])
		}
	}
	return
}
