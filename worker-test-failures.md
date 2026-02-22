# Worker Test Suite — Honest Failures (Harness Fixed)

**Test site:** https://worker-tests.hostedat.ditto.moe
**Results after harness fix:** 253/264 (95.8%)
**Date:** 2026-02-22

The test harness was marking tests as `passed: true` even when the assertion returned `false` — it only checked "didn't throw." Fixed `passed` to require `result === true`. This exposed 11 real failures (8 were previously hidden as soft failures, 3 more in cache that had no `result` field).

---

## Fixed (hostedat bug)

### Cache API — 5 failures (FIXED)

**Category:** cache
**Root cause:** `BuildEnvFromDB` in `internal/workeradapter/env.go` never set `env.Cache`. The `GORMCacheStore` implementation existed but was never wired into the worker environment. All cache operations silently no-op'd because `getCacheStore()` returned nil.
**Fix:** Added `env.Cache = &GORMCacheStore{DB: opts.DB, SiteID: siteID}` to `BuildEnvFromDB`.

---

## Remaining — upstream issues (github.com/cryguy/worker)

See `worker-upstream-issues.md` for the full issue report.

### 1. Request constructor URL normalization — `result: false`

**Category:** web-apis
**File:** `webapi.go` — `Request` class + `parseURL()`

```js
const req = new Request('https://example.com');
return req.url === 'https://example.com/' && req.method === 'GET';
```

**Root cause:** `Request` constructor does `this.url = String(input)` instead of normalizing through the URL parser. Also, `parseURL()` returns empty `Pathname` instead of `"/"` for bare origins.

---

### 2. AbortController integration — `result: false`

**Category:** fetch
**File:** `fetch.go` lines 316–324

```js
const controller = new AbortController();
const promise = fetch('https://httpbin.org/delay/5', { signal: controller.signal });
setTimeout(() => controller.abort(), 100);
try {
  await promise;
  return false;
} catch (error) {
  return error.name === 'AbortError' || error.message.includes('abort');
}
```

**Root cause:** `result := <-resultCh` blocks the V8 thread synchronously. The `setTimeout` callback that calls `controller.abort()` can never fire because the event loop can't process timers while blocked. The Go-level cancellation mechanism is correctly implemented but unreachable.

---

### 3. AbortSignal.timeout() — `result: false`

**Category:** fetch
Same root cause as #2 — the timeout callback can't fire while fetch blocks the V8 thread.

---

### 4. Query parameter `+` decoding — `result: false`

**Category:** routing
**File:** `webapi.go` — `URLSearchParams` constructor

```js
const url = new URL('https://example.com/search?q=hello+world&page=2&limit=10');
return url.searchParams.get('q') === 'hello world';
```

**Root cause:** `URLSearchParams` uses `decodeURIComponent()` which doesn't decode `+` as space. WHATWG spec requires `+` → space in query parameters. Fix: `decodeURIComponent(x.replace(/\+/g, '%20'))`.

---

### 5. Empty string KV value — `result: false`

**Category:** edge-cases
**File:** `kv.go` lines 93–97 + `interfaces.go`

```js
await env.TEST_KV.put(key, '');
const val = await env.TEST_KV.get(key);
return val === '';
```

**Root cause:** The KV binding does `if val == "" { resolve(null) }` — treats empty string as "not found". The `KVStore.Get()` interface returns `(string, error)`, which can't distinguish between "not found" (returns `""`) and "stored empty string" (also returns `""`). Requires interface change to `(*string, error)`.

---

### 6. URL with all special components — `result: false`

**Category:** edge-cases
**File:** `webapi.go` — `urlParsed` struct + `parseURL()`

```js
const url = new URL('https://user:pass@example.com:8443/path/to/resource?key=val%20ue&arr=1&arr=2#section');
return url.username === 'user' && url.password === 'pass' && url.port === '8443';
```

**Root cause:** `urlParsed` struct and `parseURL()` don't include `username` or `password` fields. Go's `url.Parse()` correctly extracts them (`u.User.Username()`, `u.User.Password()`), but they're never exposed to JS. The `URL` class constructor also never assigns `this.username` or `this.password`.

---

## Priority for Fixing

| Priority | Tests | Location | Impact |
|----------|-------|----------|--------|
| ~~**High**~~ | ~~Cache API (5 tests)~~ | ~~hostedat~~ | ~~FIXED~~ |
| **High** | Empty string KV value | upstream | Real-world code stores empty strings |
| **Medium** | AbortController/timeout (2 tests) | upstream | Important for request cancellation |
| **Medium** | Query param `+` decoding | upstream | Affects form data handling |
| **Low** | Request constructor URL normalization | upstream | Minor spec compliance |
| **Low** | URL auth/fragment parsing | upstream | Rarely used components |
