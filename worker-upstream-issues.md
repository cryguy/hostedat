# Worker Runtime Bug Report

**Module:** `github.com/cryguy/worker` v1.0.0
**Date:** 2026-02-22

5 bugs identified across URL parsing, fetch/abort, KV, and URLSearchParams. All are in the worker module's JS polyfills or Go-backed bindings. Ordered by severity.

---

## Bug 1: KV.get() returns null for empty string values

**Severity:** High
**File:** `kv.go` — `buildKVBinding`, `get` handler (around line 93)
**Also affects:** `interfaces.go` — `KVStore` interface

### Reproduction

```js
await env.TEST_KV.put('key', '');
const val = await env.TEST_KV.get('key');
console.log(val); // null — expected ''
```

### Root Cause

The binding treats empty string as "not found":

```go
val, err := store.Get(key)
// ...
if val == "" {
    resolver.Resolve(v8.Null(iso))
    return resolver.GetPromise().Value
}
```

The `KVStore.Get()` interface returns `(string, error)`. Go's zero-value string `""` is used for both "key not found" and "key exists with empty value", so the binding can't distinguish them.

### Suggested Fix

Change the interface to use a pointer:

```go
// interfaces.go
type KVStore interface {
    Get(key string) (*string, error)  // nil = not found, non-nil = value (even if empty)
    // ...
}
```

Then in `kv.go`:

```go
val, err := store.Get(key)
if err != nil { /* reject */ }
if val == nil {
    resolver.Resolve(v8.Null(iso))
    return resolver.GetPromise().Value
}
// Use *val from here
```

**Note:** This is a breaking interface change. All `KVStore` implementations will need to return `nil` for not-found and `&value` for found entries.

---

## Bug 2: fetch() blocks V8 thread, preventing AbortSignal from working

**Severity:** High
**File:** `fetch.go` lines 316–324

### Reproduction

```js
// AbortController — never aborts
const controller = new AbortController();
const promise = fetch('https://httpbin.org/delay/5', { signal: controller.signal });
setTimeout(() => controller.abort(), 100);
await promise; // Resolves normally after 5s instead of aborting at 100ms

// AbortSignal.timeout — never times out
await fetch('https://httpbin.org/delay/5', { signal: AbortSignal.timeout(100) });
// Resolves normally after 5s
```

### Root Cause

The fetch Go callback blocks the V8 thread:

```go
resultCh := make(chan fetchResult, 1)
go func() {
    resp, err := client.Do(httpReq)
    resultCh <- fetchResult{resp: resp, err: err}
}()
result := <-resultCh   // ← Blocks the V8 thread
```

While blocked on `<-resultCh`:
- `setTimeout` callbacks cannot fire (event loop is blocked)
- `AbortSignal.timeout()` timer callbacks cannot fire
- The JS abort listener that calls `__fetchAbort` is never executed

The Go-level cancellation mechanism (`fetchCtx`, `callFetchCancel`) is correctly implemented — the issue is that the V8 event loop can't deliver the abort event to trigger it.

### Suggested Fix

Integrate the fetch with the event loop instead of blocking:

1. Start the goroutine as-is
2. Return the unresolved promise immediately
3. Have the goroutine notify the event loop (via channel/callback) when the HTTP response arrives
4. The event loop processes the result and resolves/rejects the promise

This way, `setTimeout` and abort listeners can fire while the HTTP request is in-flight.

---

## Bug 3: URLSearchParams doesn't decode `+` as space

**Severity:** Medium
**File:** `webapi.go` — `URLSearchParams` constructor in `webAPIsJS`

### Reproduction

```js
const url = new URL('https://example.com/search?q=hello+world');
console.log(url.searchParams.get('q')); // 'hello+world' — expected 'hello world'
```

### Root Cause

```js
class URLSearchParams {
    constructor(init) {
        // ...
        for (const pair of s.split('&')) {
            const [k, ...rest] = pair.split('=');
            this._entries.push([decodeURIComponent(k), decodeURIComponent(rest.join('='))]);
        }
    }
}
```

`decodeURIComponent('hello+world')` returns `'hello+world'` literally. The WHATWG URL spec requires that `URLSearchParams` decode `+` as space (U+0020) in query strings.

### Suggested Fix

Replace `decodeURIComponent(x)` with `decodeURIComponent(x.replace(/\+/g, '%20'))` for both key and value:

```js
this._entries.push([
    decodeURIComponent(k.replace(/\+/g, '%20')),
    decodeURIComponent(rest.join('=').replace(/\+/g, '%20'))
]);
```

---

## Bug 4: Request constructor doesn't normalize URL

**Severity:** Low
**File:** `webapi.go` — `Request` class in `webAPIsJS` + `parseURL()` function

### Reproduction

```js
const req = new Request('https://example.com');
console.log(req.url); // 'https://example.com' — expected 'https://example.com/'
```

### Root Cause

Two issues stack:

1. `Request` constructor just stores the raw string:
   ```js
   this.url = String(input);
   ```
   Per the Fetch spec, `new Request(url)` should parse and serialize the URL, normalizing it.

2. `parseURL()` returns empty `Pathname` for bare origins:
   ```go
   // url.Parse("https://example.com") gives u.Path == ""
   Pathname: u.Path,  // "" instead of "/"
   ```

### Suggested Fix

1. Normalize URL in `Request` constructor:
   ```js
   this.url = new URL(String(input)).href;
   ```

2. Default empty path to `/` in `parseURL()`:
   ```go
   pathname := u.Path
   if pathname == "" {
       pathname = "/"
   }
   ```

---

## Bug 5: URL class missing `username` and `password` properties

**Severity:** Low
**File:** `webapi.go` — `urlParsed` struct, `parseURL()` function, `URL` class in `webAPIsJS`

### Reproduction

```js
const url = new URL('https://user:pass@example.com:8443/path');
console.log(url.username); // undefined — expected 'user'
console.log(url.password); // undefined — expected 'pass'
```

### Root Cause

The `urlParsed` struct omits `username` and `password`:

```go
type urlParsed struct {
    Href     string `json:"href"`
    Protocol string `json:"protocol"`
    Hostname string `json:"hostname"`
    Port     string `json:"port"`
    Pathname string `json:"pathname"`
    Search   string `json:"search"`
    Hash     string `json:"hash"`
    Origin   string `json:"origin"`
    Host     string `json:"host"`
    // Missing: Username, Password
}
```

Go's `url.Parse()` correctly extracts these (`u.User.Username()`, `u.User.Password()`), but `parseURL()` doesn't include them, and the JS `URL` class never assigns `this.username` or `this.password`.

### Suggested Fix

Add to struct:

```go
type urlParsed struct {
    // ... existing fields ...
    Username string `json:"username"`
    Password string `json:"password"`
}
```

In `parseURL()`:

```go
var username, password string
if u.User != nil {
    username = u.User.Username()
    password, _ = u.User.Password()
}
```

In the JS `URL` class constructor:

```js
this.username = parsed.username || '';
this.password = parsed.password || '';
```

---

## Summary

| #   | Bug                                | Severity | Breaking Change |
| --- | ---------------------------------- | -------- | --------------- |
| 1   | KV.get() null for empty string     | High     | Yes (interface) |
| 2   | fetch blocks V8, abort never fires | High     | No              |
| 3   | URLSearchParams `+` not decoded    | Medium   | No              |
| 4   | Request URL not normalized         | Low      | No              |
| 5   | URL missing username/password      | Low      | No              |
