# V8 Migration Plan: QuickJS/Wazero → V8 Worker Runtime

## Executive Summary

Replace the QuickJS/Wazero (pure Go, WASM-sandboxed) worker runtime with V8 via
`tommie/v8go`. This eliminates the Promise double-free class of bugs, enables
proper timer support (setTimeout/setInterval), and provides 10-100x better JS
performance through V8's JIT compiler.

**Target platforms:** Linux amd64/arm64, macOS amd64/arm64
**Windows server support:** Dropped (V8 has no Windows prebuilts; dev can cross-compile or use WSL)
**Branch:** `migrate/v8go`
**Library:** `github.com/nicholasgasior/gost-dom/v8go` (tracks tommie/v8go upstream)

---

## Why Migrate

| Problem | QuickJS/Wazero | V8 |
|---------|---------------|-----|
| Promise double-free | Manual Free, WASM crashes | GC handles lifecycle |
| Timers (setTimeout) | Microtask hack, no real delays | Go-side event loop with real timers |
| JS performance | Bytecode interpreter only | JIT compiler (TurboFan) |
| ES module support | Compiled bytecode, fragile | Native ESM via CompileUnboundScript |
| Constructor globals | Bytecode resolves at load time | Standard prototype chain |
| Value lifecycle | Must manually Free, leak-prone | GC handles everything |

### What We Keep (Unchanged)

- `types.go` — WorkerRequest, WorkerResponse, WorkerResult, Env, AssetsFetcher
- `runtime.go` — requestState, cryptoKeyEntry, per-request state management
- `cron.go` — CronRunner (only calls `engine.ExecuteScheduled`, engine-agnostic)
- All Go-side bridge logic (KVBridge, StorageBridge, SSRF protection, crypto ops)
- All JS polyfill strings (Headers, URL, Request, Response, streams, etc.)
- Test structure and test cases (adapted for v8go API)

### What Gets Removed

- `github.com/fastschema/qjs` dependency
- `github.com/tetratelabs/wazero` dependency (indirect)
- All `qjs.*` type references across 15 files
- Manual `val.Free()` calls and Promise double-free workarounds
- The `CloseOnContextDone` / context-cancellation watchdog pattern

---

## V8 Library Choice: `tommie/v8go`

Import path: `github.com/nicholasgasior/gost-dom/v8go` (or directly `github.com/tommie/v8go`)

| Feature | Detail |
|---------|--------|
| V8 version | Tracks Chrome releases |
| Platforms | Linux amd64/arm64, macOS amd64/arm64 |
| Callback sig | `func(info *v8go.FunctionCallbackInfo) (*v8go.Value, error)` |
| Code caching | `iso.CompileUnboundScript()` + `CompileOptions{CachedData}` |
| Termination | `iso.TerminateExecution()` (thread-safe) |
| Memory | `iso.GetHeapStatistics()` for monitoring |

---

## Architecture

### QuickJS → V8 Concept Map

| Current (QuickJS) | V8 Equivalent | Notes |
|-------------------|---------------|-------|
| `qjs.Pool` | `v8Pool` (custom, chan-based) | Buffered channel of `*v8Worker` |
| `qjs.Runtime` | `*v8go.Isolate` | One isolate per pool slot |
| `qjs.Context` | `*v8go.Context` | One context per isolate |
| `qjs.Value` | `*v8go.Value` | GC'd, no manual Free needed |
| `*qjs.This` | `*v8go.FunctionCallbackInfo` | Args, context access |
| `this.Promise()` | `v8go.NewPromiseResolver(ctx)` | Explicit resolver creation |
| `promise.Resolve(val)` | `resolver.Resolve(val)` | Same pattern |
| `promise.Reject(err)` | `resolver.Reject(val)` | Must wrap error as Value |
| `ctx.Function(fn, async)` | `v8go.NewFunctionTemplate(iso, fn)` | Then `tmpl.GetFunction(ctx)` |
| `ctx.NewObject()` | `v8go.NewObjectTemplate(iso)` or RunScript | ObjectTemplate for structure |
| `ctx.NewString(s)` | `v8go.NewValue(iso, s)` | Generic value constructor |
| `ctx.NewInt32(n)` | `v8go.NewValue(iso, int32(n))` | Generic value constructor |
| `ctx.NewFloat64(f)` | `v8go.NewValue(iso, f)` | Generic value constructor |
| `ctx.NewNull()` | `v8go.Null(iso)` | Singleton |
| `ctx.NewUndefined()` | `v8go.Undefined(iso)` | Singleton |
| `ctx.NewError(err)` | `v8go.NewValue(iso, err.Error())` | For reject; or RunScript throw |
| `ctx.ParseJSON(s)` | `v8go.JSONParse(ctx, s)` | Built-in JSON support |
| `val.String()` | `val.String()` | Same API |
| `val.Int32()` | `val.Int32()` | Same API |
| `val.Int64()` | `val.Integer()` | Returns int64 |
| `val.IsObject()` | `val.IsObject()` | Same API |
| `val.IsPromise()` | `val.IsPromise()` | Same API |
| `val.IsNull()` | `val.IsNull()` | Same API |
| `val.IsUndefined()` | `val.IsUndefined()` | Same API |
| `val.GetPropertyStr(k)` | `obj.Get(k)` | After `val.AsObject()` |
| `val.SetPropertyStr(k,v)` | `obj.Set(k, v)` | After `val.AsObject()` |
| `val.GetOwnPropertyNames()` | `obj.GetPropertyNames()` | Returns `[]string` |
| `val.InvokeJS(method, args...)` | `fn.Call(recv, args...)` | Get fn, then Call |
| `val.CallConstructor(args...)` | RunScript `new Cls(...)` | Or use ObjectTemplate |
| `val.Free()` | *(nothing)* | GC handles it |
| `val.Await()` | microtask checkpoint loop | See Promise handling below |
| `rt.Eval(name, Code(src))` | `ctx.RunScript(src, name)` | Direct execution |
| `rt.Eval(name, Code(src), TypeModule())` | `iso.CompileUnboundScript(src, name, opts)` then `script.Run(ctx)` | Module compilation |
| `rt.Compile(name, Code(src), TypeModule())` | `iso.CompileUnboundScript(src, name, opts)` + code cache | Returns UnboundScript |
| `rt.Load(name, Bytecode(bc))` | `iso.CompileUnboundScript(src, name, CompileOptions{CachedData: cd})` | Cached compilation |
| `rt.Context().Context = reqCtx` | `iso.TerminateExecution()` from watchdog | Different timeout mechanism |
| `rt.Close()` | `ctx.Close()` then `iso.Dispose()` | Explicit resource cleanup |

### Pool Model

```go
type v8Pool struct {
    workers chan *v8Worker  // buffered channel as pool
    size    int
}

type v8Worker struct {
    iso       *v8go.Isolate
    ctx       *v8go.Context
    eventLoop *eventLoop
}

func (p *v8Pool) Get() (*v8Worker, error) {
    select {
    case w := <-p.workers:
        return w, nil
    default:
        return nil, fmt.Errorf("pool exhausted")
    }
}

func (p *v8Pool) Put(w *v8Worker) {
    w.eventLoop.Reset()
    p.workers <- w
}

func (p *v8Pool) Dispose() {
    close(p.workers)
    for w := range p.workers {
        w.ctx.Close()
        w.iso.Dispose()
    }
}
```

### Promise Handling (replaces Await + double-free workaround)

```go
// drainMicrotasks pumps the V8 microtask queue until the promise settles
// or the deadline is reached. This replaces qjs val.Await().
func drainMicrotasks(ctx *v8go.Context, iso *v8go.Isolate, val *v8go.Value, deadline time.Time) (*v8go.Value, error) {
    if !val.IsPromise() {
        return val, nil
    }
    promise, _ := val.AsPromise()

    for promise.State() == v8go.Pending {
        ctx.PerformMicrotaskCheckpoint()
        if time.Now().After(deadline) {
            iso.TerminateExecution()
            return nil, fmt.Errorf("promise resolution timed out")
        }
        runtime.Gosched() // yield to other goroutines
    }

    if promise.State() == v8go.Rejected {
        return nil, fmt.Errorf("promise rejected: %s", promise.Result().String())
    }
    return promise.Result(), nil
}
```

### Event Loop (Real Timers)

```go
type eventLoop struct {
    mu      sync.Mutex
    timers  map[int]*timerEntry
    nextID  int
    pending int32 // atomic count of pending timers
}

type timerEntry struct {
    callback *v8go.Function
    deadline time.Time
    interval time.Duration // 0 for setTimeout
    id       int
    cleared  bool
}

// Drain processes all pending timers until none remain or deadline is hit.
// Must be called on the isolate's goroutine.
func (el *eventLoop) Drain(ctx *v8go.Context, deadline time.Time) {
    for {
        el.mu.Lock()
        if len(el.timers) == 0 {
            el.mu.Unlock()
            return
        }

        // Find the next timer to fire.
        var next *timerEntry
        for _, t := range el.timers {
            if t.cleared {
                continue
            }
            if next == nil || t.deadline.Before(next.deadline) {
                next = t
            }
        }
        el.mu.Unlock()

        if next == nil {
            return
        }

        // Wait until timer fires or deadline.
        now := time.Now()
        if next.deadline.After(now) {
            wait := next.deadline.Sub(now)
            if now.Add(wait).After(deadline) {
                return // would exceed execution timeout
            }
            time.Sleep(wait)
        }

        if time.Now().After(deadline) {
            return
        }

        el.mu.Lock()
        if next.cleared {
            el.mu.Unlock()
            continue
        }
        if next.interval > 0 {
            next.deadline = time.Now().Add(next.interval)
        } else {
            delete(el.timers, next.id)
        }
        cb := next.callback
        el.mu.Unlock()

        // Fire callback on isolate goroutine.
        cb.Call(v8go.Undefined(cb.GetIsolate()))
        ctx.PerformMicrotaskCheckpoint()
    }
}
```

### Timeout Mechanism

```go
// Watchdog: iso.TerminateExecution() is the ONE thread-safe V8 call.
// Replaces the context.WithCancel + CloseOnContextDone pattern.
func executeWithWatchdog(iso *v8go.Isolate, timeout time.Duration, fn func() error) error {
    var timedOut atomic.Bool
    watchdog := time.AfterFunc(timeout, func() {
        timedOut.Store(true)
        iso.TerminateExecution() // thread-safe
    })

    err := fn()
    watchdog.Stop()

    if timedOut.Load() {
        return fmt.Errorf("worker execution timed out (limit: %v)", timeout)
    }
    return err
}
```

---

## Migration Phases

Since we're dropping Windows support, there are **no build tags** and **no QJS
fallback files**. Every file is rewritten in-place to use v8go.

### Phase 1: Engine Core + Pool
**Files:** `engine.go` (rewrite), `pool.go` (new), `eventloop.go` (new)

The core engine struct stays the same shape but internals change:

```go
// engine.go changes:
// - import: remove "github.com/fastschema/qjs", add v8go
// - sitePool.pool: *qjs.Pool → *v8Pool
// - CompileAndCache: qjs.New() + rt.Compile() → iso.CompileUnboundScript()
//   Store source string instead of bytecode (V8 code cache is optional optimization)
// - GetOrCreatePool: qjs.NewPool() → newV8Pool() with setup functions
// - Execute: pool.Get() returns *v8Worker instead of *qjs.Runtime
//   Watchdog: time.AfterFunc + iso.TerminateExecution() replaces context cancel
//   Promise: drainMicrotasks() replaces val.Await()
//   Response: jsResponseToGo reads obj.Get("_body") instead of val.GetPropertyStr()
//   No val.Free() calls anywhere
// - ExecuteScheduled: same changes as Execute
// - Shutdown: pool.Dispose() calls ctx.Close() + iso.Dispose()
```

**Key structural decisions:**
1. **One isolate per pool slot** (not shared). V8 isolates are thread-constrained;
   sharing one isolate across the pool would serialize all requests.
2. **Code cache as optimization, not requirement.** Store original JS source in
   `Engine.sources` sync.Map. On first compile, generate code cache. On pool
   creation, use `CompileOptions{CachedData}` for fast startup.
3. **Pool Get is non-blocking with fallback.** If channel is empty, return error
   (same as current behavior — pool size bounds concurrency).

### Phase 2: Web API Bindings
**Files:** `webapi.go` (rewrite), `globals.go` (rewrite), `encoding.go` (rewrite)

Most of the JS polyfill strings (`webAPIsJS`, `urlSearchParamsExtJS`, `timersJS`,
`globalsJS`, `encodingJS`, etc.) are **kept as-is** — they're pure JS that works
in any engine. The Go registration code changes:

```go
// BEFORE (QuickJS):
func setupWebAPIs(rt *qjs.Runtime) error {
    ctx := rt.Context()
    ctx.SetFunc("__parseURL", func(this *qjs.This) (*qjs.Value, error) {
        args := this.Args()
        rawURL := args[0].String()
        // ...
        return c.NewString(result), nil
    })
    _, err := rt.Eval("webapi.js", qjs.Code(webAPIsJS))
    return err
}

// AFTER (V8):
func setupWebAPIs(iso *v8go.Isolate, ctx *v8go.Context) error {
    // Register Go-backed URL parser as global function.
    parseURLTmpl := v8go.NewFunctionTemplate(iso, func(info *v8go.FunctionCallbackInfo) (*v8go.Value, error) {
        args := info.Args()
        if len(args) < 1 {
            return nil, fmt.Errorf("URL constructor requires at least 1 argument")
        }
        rawURL := args[0].String()
        var base string
        if len(args) > 1 {
            base = args[1].String()
        }
        parsed, err := parseURL(rawURL, base)
        if err != nil {
            errJSON := fmt.Sprintf(`{"error":%q}`, err.Error())
            return v8go.NewValue(iso, errJSON)
        }
        data, _ := json.Marshal(parsed)
        return v8go.NewValue(iso, string(data))
    })
    parseURLFn, _ := parseURLTmpl.GetFunction(ctx)
    ctx.Global().Set("__parseURL", parseURLFn)

    // Evaluate the same JS polyfills.
    _, err := ctx.RunScript(webAPIsJS, "webapi.js")
    return err
}
```

**What changes per file:**
- `webapi.go`: `setupWebAPIs(rt *qjs.Runtime)` → `setupWebAPIs(iso, ctx)`. JS strings unchanged. `goRequestToJS` and `jsResponseToGo` rewritten to use `obj.Get()`/`obj.Set()` instead of `GetPropertyStr`/`SetPropertyStr`/`Free`.
- `globals.go`: `setupGlobals(rt *qjs.Runtime)` → `setupGlobals(iso, ctx)`. structuredClone polyfill JS unchanged. `performance.now()` and `navigator` use FunctionTemplate.
- `encoding.go`: `setupEncoding(rt *qjs.Runtime)` → `setupEncoding(iso, ctx)`. JS string unchanged, just `ctx.RunScript()` instead of `rt.Eval()`.

**What we can remove (V8 built-ins):**
- TextEncoder/TextDecoder polyfill JS (V8 7.8+ has native, **but V8 ≠ browser** — verify at runtime, keep polyfill as fallback)
- structuredClone polyfill JS (V8 9.8+ — verify, may keep)

### Phase 3: Async Bindings (KV, Storage, Fetch, Assets)
**Files:** `kv.go`, `storage.go`, `fetch.go`, `assets.go` (all rewritten)

The async pattern changes from `ctx.Function(fn, true)` + `this.Promise()` to
`v8go.NewFunctionTemplate` + `v8go.NewPromiseResolver`:

```go
// BEFORE (QuickJS):
getFn := ctx.Function(func(this *qjs.This) (*qjs.Value, error) {
    c := this.Context()
    promise := this.Promise()
    key := this.Args()[0].String()
    val, err := bridge.Get(key)
    if err != nil {
        promise.Reject(c.NewError(err))
        return c.NewUndefined(), nil
    }
    promise.Resolve(c.NewString(val))
    return c.NewUndefined(), nil
}, true)

// AFTER (V8):
getFnTmpl := v8go.NewFunctionTemplate(iso, func(info *v8go.FunctionCallbackInfo) (*v8go.Value, error) {
    args := info.Args()
    if len(args) == 0 {
        return nil, fmt.Errorf("KV.get requires a key argument")
    }
    key := args[0].String()
    resolver, promise, _ := v8go.NewPromiseResolver(info.Context())
    val, err := bridge.Get(key)
    if err != nil {
        errVal, _ := v8go.NewValue(iso, err.Error())
        resolver.Reject(errVal)
    } else if val == "" {
        resolver.Resolve(v8go.Null(iso))
    } else {
        strVal, _ := v8go.NewValue(iso, val)
        resolver.Resolve(strVal)
    }
    return promise.Value, nil
})
```

**Per-file changes:**
- `kv.go`: `buildKVBinding(ctx *qjs.Context, bridge)` → `buildKVBinding(iso, ctx, bridge)`. KVBridge struct unchanged. 4 methods (get/put/delete/list) rewritten with PromiseResolver pattern. No more `val.Free()` calls.
- `storage.go`: `buildStorageBinding(ctx, bridge)` → `buildStorageBinding(iso, ctx, bridge)`. StorageBridge unchanged. 5 methods rewritten. `buildR2Object`/`buildR2ObjectBody` use `obj.Set()` instead of `SetPropertyStr`. `coerceStoragePutBody` simplified (no WASM boundary issues).
- `fetch.go`: `setupFetch(rt, cfg)` → `setupFetch(iso, ctx, cfg)`. SSRF protection code (`isPrivateHostname`, `ssrfSafeDialContext`, `isPrivateIP`, `privateRanges`) **unchanged**. HTTP client code unchanged. Only the JS↔Go value conversion changes.
- `assets.go`: `buildAssetsBinding(ctx, fetcher)` → `buildAssetsBinding(iso, ctx, fetcher)`. `buildEnvObject` and `buildExecContext` signatures change to take `(iso, ctx, ...)`.

### Phase 4: Console, Crypto, Abort, Streams, FormData, BodyTypes
**Files:** `console.go`, `crypto.go`, `crypto_ext.go`, `abort.go`, `streams.go`, `formdata.go`, `bodytypes.go` (all rewritten)

**console.go**: Simple — 5 FunctionTemplates for log/info/warn/error/debug. Read `__requestID` from global, call `addLog()`.

**crypto.go** + **crypto_ext.go**: Go crypto logic unchanged (SHA, HMAC, ECDSA, AES, JWK). Only the JS shim registration changes from `ctx.SetFunc`/`ctx.SetAsyncFunc` to FunctionTemplate pattern. ~40 functions to port.

**abort.go, streams.go, formdata.go, bodytypes.go**: These are almost entirely JS polyfill strings evaluated via `rt.Eval()`. Change to `ctx.RunScript()`. The bodytypes.go prototype patching pattern works identically in V8.

### Phase 5: Timers (Real Event Loop)
**Files:** `timers.go` (rewrite), `eventloop.go` (new)

Replace the microtask-hack JS timer implementation with Go-backed real timers:

```go
func setupTimers(iso *v8go.Isolate, ctx *v8go.Context, el *eventLoop) error {
    // setTimeout(fn, delay) -> timerID
    setTimeoutTmpl := v8go.NewFunctionTemplate(iso, func(info *v8go.FunctionCallbackInfo) (*v8go.Value, error) {
        args := info.Args()
        if len(args) == 0 || !args[0].IsFunction() {
            return v8go.NewValue(iso, int32(0))
        }
        fn, _ := args[0].AsFunction()
        delay := 0
        if len(args) > 1 {
            delay = int(args[1].Integer())
        }
        id := el.SetTimeout(fn, time.Duration(delay)*time.Millisecond)
        return v8go.NewValue(iso, int32(id))
    })
    setTimeoutFn, _ := setTimeoutTmpl.GetFunction(ctx)
    ctx.Global().Set("setTimeout", setTimeoutFn)

    // clearTimeout, setInterval, clearInterval follow same pattern...
    return nil
}
```

The event loop is drained **after** the fetch handler returns (in `Execute`),
giving timers a chance to fire within the execution timeout window.

### Phase 6: Tests
**Files:** All `*_test.go` files updated

Test changes are mechanical — same test logic, different setup:
- Replace `qjs.New()` / `qjs.NewPool()` with `v8go.NewIsolate()` + pool creation
- Replace `rt.Eval(name, qjs.Code(src))` with `ctx.RunScript(src, name)`
- Remove all `defer val.Free()` / `val.Free()` calls
- Replace `val.Await()` with microtask drain loop
- Replace property access (`GetPropertyStr`/`SetPropertyStr`) with `Get`/`Set`

Existing test files:
- `engine_test.go` — Core Execute/ExecuteScheduled tests
- `worker_test.go` — Integration tests
- `integration_test.go` — Full pipeline tests
- `webapi_test.go` — URL, Headers, Request, Response
- `urlsearchparams_test.go` — URLSearchParams mutations
- `kv_test.go` — KV operations
- `storage_test.go` — R2 storage operations
- `fetch_test.go` — Fetch with SSRF
- `crypto_test.go` + `crypto_ext_test.go` — Crypto operations
- `encoding_test.go` — atob/btoa
- `timers_test.go` — Timer behavior
- `globals_test.go` — structuredClone, performance.now
- `streams_test.go` — ReadableStream/WritableStream
- `formdata_test.go` — FormData/Blob/File
- `bodytypes_test.go` — Body type coercion
- `abort_test.go` — AbortController
- `assets_test.go` — ASSETS.fetch
- `cron_test.go` — Cron scheduling
- `runtime_test.go` — Request state management (no changes needed)

**New tests to add:**
- Timer tests with real delays (setTimeout 100ms actually waits)
- Promise rejection propagation (no double-free possible)
- `iso.TerminateExecution()` timeout behavior
- Code cache hit/miss validation
- Event loop drain with multiple concurrent timers

---

## File-by-File Change Map

| File | Action | Scope |
|------|--------|-------|
| `engine.go` | **Rewrite** | Replace qjs.Pool/Runtime with v8Pool, change compile/execute flow, new watchdog |
| `pool.go` | **New** | v8Pool, v8Worker, channel-based pool with Dispose |
| `eventloop.go` | **New** | Go-side timer event loop for setTimeout/setInterval |
| `webapi.go` | **Rewrite** | FunctionTemplate for __parseURL, RunScript for JS polyfills, rewrite goRequestToJS/jsResponseToGo |
| `kv.go` | **Rewrite** | FunctionTemplate + PromiseResolver for get/put/delete/list |
| `storage.go` | **Rewrite** | FunctionTemplate + PromiseResolver for get/put/delete/head/list |
| `fetch.go` | **Rewrite** | FunctionTemplate + PromiseResolver for fetch(). SSRF code unchanged |
| `assets.go` | **Rewrite** | buildAssetsBinding, buildEnvObject, buildExecContext use v8go API |
| `console.go` | **Rewrite** | 5 FunctionTemplates for log levels |
| `crypto.go` | **Rewrite** | FunctionTemplate registration. Go crypto logic unchanged |
| `crypto_ext.go` | **Rewrite** | FunctionTemplate registration. Go crypto logic unchanged |
| `timers.go` | **Rewrite** | Go-backed real timers via eventLoop, replaces JS microtask hack |
| `globals.go` | **Rewrite** | FunctionTemplate for performance.now, RunScript for JS polyfills |
| `encoding.go` | **Rewrite** | RunScript instead of rt.Eval (trivial) |
| `abort.go` | **Rewrite** | RunScript instead of rt.Eval (trivial) |
| `streams.go` | **Rewrite** | RunScript instead of rt.Eval (trivial) |
| `formdata.go` | **Rewrite** | RunScript instead of rt.Eval (trivial) |
| `bodytypes.go` | **Rewrite** | RunScript instead of rt.Eval (trivial) |
| `types.go` | **Unchanged** | No qjs dependency |
| `runtime.go` | **Unchanged** | No qjs dependency |
| `cron.go` | **Minimal** | Only if Engine method signatures change |
| All `*_test.go` | **Rewrite** | Mechanical: qjs setup → v8go setup, remove Free calls |
| `go.mod` | **Update** | Remove fastschema/qjs + wazero, add tommie/v8go |

**Total: 17 files rewritten, 2 new files, 2 unchanged, ~18 test files updated**

---

## Execution Order (Dependency Graph)

```
Phase 1: engine.go + pool.go + eventloop.go
    │     (core compiles, "hello world" worker runs)
    │
    ├── Phase 2: webapi.go + globals.go + encoding.go
    │     (basic Request/Response/URL works)
    │
    ├── Phase 3: fetch.go + kv.go + storage.go + assets.go
    │     (all async bindings work)
    │
    ├── Phase 4: console.go + crypto.go + crypto_ext.go +
    │            abort.go + streams.go + formdata.go + bodytypes.go
    │     (full API surface)
    │
    └── Phase 5: timers.go (uses eventloop.go from Phase 1)
          (real timer support)

Phase 6: Tests (can start after each phase)
```

Phases 2-5 are parallelizable once Phase 1 is done — they only depend on the
engine core and pool being functional.

---

## Setup Function Signature Change

All setup functions change from:
```go
func setupX(rt *qjs.Runtime) error
```
to:
```go
func setupX(iso *v8go.Isolate, ctx *v8go.Context) error
```

The pool creation in `GetOrCreatePool` changes from:
```go
pool := qjs.NewPool(cfg.PoolSize, option,
    setupWebAPIs,
    setupGlobals,
    // ... 13 setup functions
)
```
to a loop that creates N isolates, each with a fresh context, and runs all setup
functions + loads the worker script:
```go
func newV8Pool(size int, source string, codeCache []byte, setupFns ...setupFunc) (*v8Pool, error) {
    pool := &v8Pool{workers: make(chan *v8Worker, size), size: size}
    for i := 0; i < size; i++ {
        iso := v8go.NewIsolate()
        ctx := v8go.NewContext(iso)
        el := newEventLoop()

        for _, setup := range setupFns {
            if err := setup(iso, ctx, el); err != nil {
                iso.Dispose()
                return nil, fmt.Errorf("setup failed: %w", err)
            }
        }

        // Load worker script (with code cache if available).
        opts := v8go.CompileOptions{}
        if codeCache != nil {
            opts.CachedData = &v8go.CompilerCachedData{Bytes: codeCache}
        }
        script, err := iso.CompileUnboundScript(source, "worker.js", opts)
        if err != nil {
            iso.Dispose()
            return nil, fmt.Errorf("compiling worker: %w", err)
        }
        if _, err := script.Run(ctx); err != nil {
            iso.Dispose()
            return nil, fmt.Errorf("running worker: %w", err)
        }

        // Extract default export.
        mod, err := ctx.RunScript("globalThis.__worker_module__", "extract.js")
        if err != nil {
            iso.Dispose()
            return nil, fmt.Errorf("extracting worker module: %w", err)
        }
        _ = mod // verify it's not undefined

        pool.workers <- &v8Worker{iso: iso, ctx: ctx, eventLoop: el}
    }
    return pool, nil
}
```

---

## Module Loading Strategy

QuickJS uses compiled bytecode + ES module import:
```go
rt.Load("worker.js", qjs.Bytecode(bytecode))
rt.Eval("__worker_import__.js", Code(`import mod from 'worker.js'; export default mod;`), TypeModule())
ctx.Global().SetPropertyStr("__worker_module__", moduleVal)
```

V8 via v8go doesn't expose ES module loader hooks. Options:

**Option A: Bundle to IIFE (recommended)**
Require workers to be bundled (esbuild) into a self-executing script that assigns
to `globalThis.__worker_module__`:
```js
// Bundled worker output:
globalThis.__worker_module__ = (function() {
    // ... bundled worker code ...
    return { fetch: handleFetch };
})();
```
This works with `ctx.RunScript()` directly. The deploy pipeline already bundles
with esbuild — just change the output format.

**Option B: Wrapper script**
Wrap the user's ES module source in a script that captures the exports:
```go
wrapped := fmt.Sprintf(`
    const __exports = {};
    (function(exports) { %s })(/* ... */);
    globalThis.__worker_module__ = __exports.default;
`, source)
ctx.RunScript(wrapped, "worker.js")
```

**Option C: Use V8's Module API if v8go exposes it**
Check if tommie/v8go has `v8go.CompileModule()` or similar. If not, Option A.

**Decision: Option A.** The deploy pipeline already uses esbuild. We change the
esbuild config to output `iife` format with `globalName: "__worker_module__"`.
This is the cleanest approach and avoids any module loading complexity.

---

## Go Module Changes

```go
// go.mod
require (
    github.com/nicholasgasior/gost-dom/v8go v0.x.x  // or github.com/tommie/v8go
)

// Remove:
// github.com/fastschema/qjs v0.0.6
// github.com/tetratelabs/wazero v1.9.0 (indirect, pulled by qjs)
```

---

## Risk Assessment

| Risk | Impact | Likelihood | Mitigation |
|------|--------|------------|------------|
| Binary size +30-50MB | Low | Certain | Acceptable for server binary; strip debug symbols |
| CGO required | Medium | Certain | Linux/Mac only targets; CI uses cgo-enabled images |
| V8 fork goes unmaintained | Medium | Low | Two active forks; V8 Go API is stable |
| No Windows builds | Low | Certain | Documented; dev uses WSL or cross-compile for testing |
| Thread safety mistakes | High | Medium | Strict one-goroutine-per-isolate; TerminateExecution is the only cross-goroutine call |
| Event loop bugs | High | Medium | Extensive timer tests; bounded by execution timeout |
| Module loading without ESM | Medium | Low | esbuild IIFE bundling eliminates the problem |
| V8 cold start slower than QuickJS | Low | Low | Code caching; isolate reuse in pool |

---

## Milestones

1. **M1:** Engine core + pool compiles, "hello world" worker returns Response
2. **M2:** Web APIs (Headers, URL, Request, Response) work, basic fetch handler passes
3. **M3:** Async bindings (KV, Storage, Fetch) work with PromiseResolver
4. **M4:** Full API surface (crypto, streams, formdata, abort, console)
5. **M5:** Real timer event loop (setTimeout with actual delays)
6. **M6:** All existing tests pass
7. **M7:** Code cache optimization, cold start benchmarks

---

## Appendix: QJS API Surface Audit

Every `qjs.*` usage across the codebase, grouped by file:

**engine.go** (heaviest user):
- `qjs.New(Option{...})` — create runtime for compilation
- `qjs.NewPool(size, option, ...setupFns)` — create pool
- `qjs.Option{Context, CloseOnContextDone, MemoryLimit, MaxStackSize, MaxExecutionTime, GCThreshold}`
- `rt.Compile("worker.js", Code(src), TypeModule())`
- `rt.Load("worker.js", Bytecode(bc))`
- `rt.Eval("name.js", Code(src), TypeModule())`
- `rt.Close()`
- `rt.Context()` — get context from runtime
- `pool.Get()` / `pool.Put(rt)`
- `ctx.Global()` — get global object
- `ctx.NewObject()` / `ctx.NewString()` / `ctx.NewInt64()` / `ctx.NewFloat64()` / `ctx.NewInt32()`
- `val.GetPropertyStr()` / `val.SetPropertyStr()`
- `val.InvokeJS()` — call JS method
- `val.IsPromise()` / `val.Await()`
- `val.IsUndefined()` / `val.IsNull()`
- `val.Free()`

**webapi.go**:
- `ctx.SetFunc("__parseURL", fn)` — register sync function
- `ctx.NewString()` / `ctx.NewObject()`
- `val.GetPropertyStr()` / `val.SetPropertyStr()`
- `val.CallConstructor()` — call `new Request(...)`
- `val.GetOwnPropertyNames()`
- `val.String()` / `val.Int32()` / `val.IsObject()` / `val.IsError()`
- `val.Free()`

**kv.go, storage.go**:
- `ctx.Function(fn, isAsync)` — create JS function
- `this.Context()` / `this.Args()` / `this.Promise()`
- `promise.Resolve()` / `promise.Reject()`
- `ctx.NewObject()` / `ctx.NewString()` / `ctx.NewError()` / `ctx.NewNull()` / `ctx.NewUndefined()`
- `ctx.ParseJSON()`
- `val.GetPropertyStr()` / `val.SetPropertyStr()`
- `val.Free()`

**fetch.go**:
- `ctx.SetAsyncFunc("fetch", fn)` — register async global
- Same value manipulation as above

**console.go**:
- `ctx.Function(fn, false)` — sync functions
- `ctx.Global()` / `ctx.NewObject()`
- `val.GetPropertyStr()` / `val.Int64()` / `val.String()` / `val.Free()`

**crypto.go, crypto_ext.go**:
- `ctx.SetFunc()` / `ctx.SetAsyncFunc()`
- Same value patterns

**All JS polyfill files** (abort.go, streams.go, formdata.go, bodytypes.go, encoding.go, globals.go, timers.go):
- `rt.Eval("name.js", qjs.Code(jsString))` — only this one call per file
