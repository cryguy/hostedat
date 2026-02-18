# hostedat Worker Runtime

The worker runtime executes JavaScript using V8 (via v8go, CGO bindings to the V8 engine). Workers are Cloudflare Workers-compatible in structure, but not all Cloudflare APIs are available. This document describes exactly what is available.

---

## Entry Point

A worker is a file named `_worker.js` in the root of your deployed zip.

```js
export default {
  // Handle HTTP requests
  async fetch(request, env, ctx) {
    return new Response("Hello world");
  },

  // Handle cron triggers
  async scheduled(event, env, ctx) {
    console.log("Cron fired:", event.cron);
  },
};
```

Both handlers are optional. A site can have a `fetch` handler only, a `scheduled` handler only, or both.

---

## `fetch(request, env, ctx)`

Called for every HTTP request to the site.

| Param | Type | Description |
|-------|------|-------------|
| `request` | `Request` | The incoming HTTP request |
| `env` | `object` | Environment bindings (see below) |
| `ctx` | `ExecutionContext` | Execution context (no-op methods) |

Must return a `Response` (or a Promise that resolves to one).

```js
export default {
  async fetch(request, env, ctx) {
    const url = new URL(request.url);

    if (url.pathname === "/api/hello") {
      return Response.json({ message: "Hello!" });
    }

    // Fall through to static assets
    return env.ASSETS.fetch(request);
  },
};
```

---

## `scheduled(event, env, ctx)`

Called when a cron trigger fires. Cron schedules are configured via the API or dashboard.

| Param | Type | Description |
|-------|------|-------------|
| `event` | `ScheduledEvent` | Cron event info |
| `env` | `object` | Environment bindings (same as fetch) |
| `ctx` | `ExecutionContext` | Execution context (no-op methods) |

```js
// event shape:
// { scheduledTime: number (Unix ms), cron: string (e.g. "0 * * * *") }
```

Cron expressions use standard 5-field format: `minute hour day-of-month month day-of-week`.

---

## Web APIs

### `Request`

```js
new Request(url: string | Request, init?: RequestInit)
```

| Property/Method | Type | Description |
|-----------------|------|-------------|
| `url` | `string` | Full request URL |
| `method` | `string` | HTTP method (uppercased) |
| `headers` | `Headers` | Request headers |
| `text()` | `Promise<string>` | Read body as string |
| `json()` | `Promise<any>` | Parse body as JSON |
| `arrayBuffer()` | `Promise<ArrayBuffer>` | Read body as ArrayBuffer |
| `clone()` | `Request` | Clone the request |

`RequestInit`: `{ method?, headers?, body? }`

---

### `Response`

```js
new Response(body?: string | null, init?: ResponseInit)
```

| Property/Method | Type | Description |
|-----------------|------|-------------|
| `status` | `number` | HTTP status code (default 200) |
| `statusText` | `string` | Status text |
| `headers` | `Headers` | Response headers |
| `ok` | `boolean` | `status >= 200 && status < 300` |
| `url` | `string` | Response URL |
| `text()` | `Promise<string>` | Read body as string |
| `json()` | `Promise<any>` | Parse body as JSON |
| `arrayBuffer()` | `Promise<ArrayBuffer>` | Read body as ArrayBuffer |
| `clone()` | `Response` | Clone the response |
| `Response.json(data, init?)` | `Response` | Static: create JSON response |
| `Response.redirect(url, status?)` | `Response` | Static: create redirect response |

`ResponseInit`: `{ status?, statusText?, headers? }`

```js
// Common patterns:
return new Response("plain text");
return new Response(JSON.stringify(data), { headers: { "Content-Type": "application/json" } });
return Response.json({ key: "value" });
return Response.json({ error: "not found" }, { status: 404 });
return Response.redirect("https://example.com", 301);
return new Response(null, { status: 204 });
```

---

### `Headers`

```js
new Headers(init?: HeadersInit)
```

`HeadersInit`: `Headers | string[][] | Record<string, string>`

| Method | Signature | Description |
|--------|-----------|-------------|
| `get` | `(name: string) => string \| null` | Get header value |
| `set` | `(name: string, value: string) => void` | Set header |
| `has` | `(name: string) => boolean` | Check existence |
| `delete` | `(name: string) => void` | Remove header |
| `append` | `(name: string, value: string) => void` | Append to existing |
| `forEach` | `(cb: (value, name, headers) => void) => void` | Iterate |
| `entries` | `() => Iterator` | `[name, value]` pairs |
| `keys` | `() => Iterator` | Header names |
| `values` | `() => Iterator` | Header values |

Header names are case-insensitive (stored lowercase).

---

### `URL`

```js
new URL(input: string, base?: string)
```

| Property | Type | Description |
|----------|------|-------------|
| `href` | `string` | Full URL string |
| `protocol` | `string` | e.g. `"https:"` |
| `hostname` | `string` | e.g. `"example.com"` |
| `port` | `string` | e.g. `"8080"` or `""` |
| `host` | `string` | `hostname:port` or just `hostname` |
| `pathname` | `string` | e.g. `"/path/to/resource"` |
| `search` | `string` | e.g. `"?key=value"` |
| `hash` | `string` | e.g. `"#section"` |
| `origin` | `string` | e.g. `"https://example.com"` |
| `searchParams` | `URLSearchParams` | Parsed query parameters |
| `toString()` | `string` | Same as `href` |

---

### `URLSearchParams`

```js
new URLSearchParams(init?: string)
```

| Method | Signature | Description |
|--------|-----------|-------------|
| `get` | `(name: string) => string \| null` | First value for name |
| `has` | `(name: string) => boolean` | Check existence |
| `toString` | `() => string` | Encode to query string |
| `forEach` | `(cb) => void` | Iterate entries |
| `entries` | `() => Iterator` | `[name, value]` pairs |
| `keys` | `() => Iterator` | Param names |
| `values` | `() => Iterator` | Param values |

Note: `URLSearchParams` is read-only in the current implementation (no `set`, `append`, or `delete`).

---

### `TextEncoder` / `TextDecoder`

```js
const enc = new TextEncoder();
const bytes = enc.encode("hello"); // Uint8Array (UTF-8)

const dec = new TextDecoder();
const str = dec.decode(bytes); // string
```

---

## `fetch()`

Global `fetch()` is available for making outbound HTTP requests.

```js
const response = await fetch("https://httpbin.org/json");
const data = await response.json();
```

```js
const response = await fetch("https://httpbin.org/json", {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({ key: "value" }),
});
```

**Restrictions and limits:**

| Constraint | Value | Notes |
|------------|-------|-------|
| Max requests per invocation | configurable (default 50) | Rejects with error when exceeded |
| Per-request timeout | configurable (default 10s) | |
| Max response body size | configurable (default 10 MB) | Body is truncated, not errored |
| Max redirects | 10 | Error thrown beyond 10 |
| Private IPs | blocked | Loopback, RFC-1918, link-local |

**SSRF protection:** Requests to `127.x.x.x`, `10.x.x.x`, `172.16-31.x.x`, `192.168.x.x`, `169.254.x.x`, `::1`, and other private ranges are rejected with an error. Redirect destinations are also checked.

**Difference from Cloudflare:** fetch is rate-limited per invocation. Cloudflare Workers have no such hard per-invocation limit (only aggregate limits).

---

## `console`

```js
console.log("message");
console.info("message");
console.warn("message");
console.error("message");
console.debug("message");
```

All console output is persisted to the `WorkerLog` table in the database. Logs are viewable via `GET /api/v1/sites/:id/worker/logs` (last 100 entries). Logs are automatically pruned after `max_log_retention` days (default 7).

**Difference from Cloudflare:** Cloudflare logs are ephemeral and accessed via tail workers or live logging. Here, logs are stored in the DB and queryable via the REST API.

---

## `crypto`

### `crypto.getRandomValues(typedArray)`

```js
const bytes = new Uint8Array(16);
crypto.getRandomValues(bytes);
```

Fills the typed array with cryptographically secure random bytes. Max 65536 bytes per call.

### `crypto.randomUUID()`

```js
const id = crypto.randomUUID(); // e.g. "f47ac10b-58cc-4372-a567-0e02b2c3d479"
```

Returns a UUID v4 string.

---

## `crypto.subtle`

All `crypto.subtle` operations are async (return Promises). Key material is scoped to the current request — keys cannot be shared across requests.

### `crypto.subtle.digest(algorithm, data)`

```js
const hashBuffer = await crypto.subtle.digest("SHA-256", data);
```

| Algorithm | Notes |
|-----------|-------|
| `"SHA-1"` | Also accepts `"sha1"`, `"SHA1"` |
| `"SHA-256"` | Also accepts `"sha256"`, `"SHA256"` |
| `"SHA-384"` | Also accepts `"sha384"`, `"SHA384"` |
| `"SHA-512"` | Also accepts `"sha512"`, `"SHA512"` |

`data`: `ArrayBuffer`, `TypedArray`, or any BufferSource.

Returns `Promise<ArrayBuffer>`.

```js
// Common pattern: hex digest
async function sha256hex(message) {
  const msgBuffer = new TextEncoder().encode(message);
  const hashBuffer = await crypto.subtle.digest("SHA-256", msgBuffer);
  const hashArray = Array.from(new Uint8Array(hashBuffer));
  return hashArray.map(b => b.toString(16).padStart(2, "0")).join("");
}
```

---

### `crypto.subtle.importKey(format, keyData, algorithm, extractable, usages)`

```js
const key = await crypto.subtle.importKey(
  "raw",
  keyBytes,
  { name: "HMAC", hash: "SHA-256" },
  false,
  ["sign", "verify"]
);
```

**Restriction:** Only `"raw"` format is supported. Other formats (`"pkcs8"`, `"spki"`, `"jwk"`) throw `TypeError`.

Supported algorithms for import: `HMAC` (with hash: `SHA-1`, `SHA-256`, `SHA-384`, `SHA-512`), `AES-GCM`.

Returns `Promise<CryptoKey>`.

---

### `crypto.subtle.exportKey(format, key)`

```js
const keyBytes = await crypto.subtle.exportKey("raw", key);
```

**Restriction:** Only `"raw"` format is supported. Key must have been imported with `extractable: true`.

Returns `Promise<ArrayBuffer>`.

---

### `crypto.subtle.sign(algorithm, key, data)` / `crypto.subtle.verify(algorithm, key, signature, data)`

```js
// Sign
const signature = await crypto.subtle.sign("HMAC", key, data);

// Verify
const valid = await crypto.subtle.verify("HMAC", key, signature, data);
```

**Supported algorithm:** `"HMAC"` only. RSA and ECDSA are not supported.

`data`: BufferSource. Returns `Promise<ArrayBuffer>` for sign, `Promise<boolean>` for verify.

```js
// HMAC-SHA256 example
async function hmacSign(secret, message) {
  const enc = new TextEncoder();
  const key = await crypto.subtle.importKey(
    "raw", enc.encode(secret),
    { name: "HMAC", hash: "SHA-256" },
    false, ["sign"]
  );
  const sig = await crypto.subtle.sign("HMAC", key, enc.encode(message));
  return btoa(String.fromCharCode(...new Uint8Array(sig)));
}
```

---

### `crypto.subtle.encrypt(algorithm, key, data)` / `crypto.subtle.decrypt(algorithm, key, data)`

```js
// Encrypt
const ciphertext = await crypto.subtle.encrypt(
  { name: "AES-GCM", iv: ivBytes },
  key,
  plaintext
);

// Decrypt
const plaintext = await crypto.subtle.decrypt(
  { name: "AES-GCM", iv: ivBytes },
  key,
  ciphertext
);
```

**Supported algorithm:** `"AES-GCM"` only. The `iv` must be exactly 12 bytes.

Returns `Promise<ArrayBuffer>`.

**Difference from Cloudflare:** Only `HMAC` sign/verify and `AES-GCM` encrypt/decrypt are supported. RSA-OAEP, ECDSA, ECDH, and AES-CBC are not available.

---

## Timers

```js
setTimeout(fn, delay)   // -> id
clearTimeout(id)
setInterval(fn, delay)  // -> id
clearInterval(id)
```

**Important:** Timers are microtask-based, not wall-clock. All timers (regardless of `delay`) fire on the next microtask tick during Promise resolution. There is no real wall-clock delay in the single-threaded WASM environment.

`setInterval` is capped at 1000 invocations to prevent infinite loops.

**Difference from Cloudflare:** Timer delays are not honored; all timers fire at the next microtask checkpoint.

---

## Encoding

### `atob(data)` / `btoa(data)`

Standard Base64 encode/decode for Latin-1 strings.

```js
const encoded = btoa("Hello World");     // "SGVsbG8gV29ybGQ="
const decoded = atob("SGVsbG8gV29ybGQ="); // "Hello World"
```

`btoa` throws if any character code is > 255. For arbitrary binary data, use `Uint8Array` with `crypto.subtle` helpers instead.

---

## Other Globals

### `structuredClone(value)`

Deep clones a value using JSON serialization. Does not support `Map`, `Set`, `WeakMap`, `WeakSet`, functions, or symbols (throws `DOMException`).

```js
const clone = structuredClone({ a: 1, b: [2, 3] });
```

### `queueMicrotask(fn)`

```js
queueMicrotask(() => console.log("next microtask"));
```

Schedules `fn` as a microtask (equivalent to `Promise.resolve().then(fn)`).

### `performance.now()`

```js
const start = performance.now();
// ... work ...
const elapsed = performance.now() - start; // milliseconds
```

Returns milliseconds since the runtime was initialized (high-resolution via Go's `time.Since`).

### `navigator.userAgent`

```js
console.log(navigator.userAgent); // "hostedat-worker/1.0"
```

### `AbortController` / `AbortSignal`

Available (registered in the runtime). Accepted by `fetch()` but abort behavior may be limited in the synchronous WASM execution model.

### `ReadableStream` / `WritableStream` / `TransformStream`

Available as stubs. Sufficient for constructing Response bodies from stream sources, but streaming chunked responses are not fully supported — the body is read to completion before being sent.

### `Blob` / `File`

Available. `Blob` can be constructed with string or ArrayBuffer parts. `File` extends `Blob` with a filename.

```js
const blob = new Blob(["hello world"], { type: "text/plain" });
const text = await blob.text();
```

### `FormData`

Available for constructing and parsing multipart form data.

```js
const form = new FormData();
form.append("field", "value");
```

### `Event` / `EventTarget` / `DOMException`

Base classes available. Used by the runtime internally (e.g., `AbortController` events, `DOMException` errors from `structuredClone`).

---

## Environment Bindings (`env`)

The `env` object passed to `fetch` and `scheduled` contains:

### Plain Environment Variables

Set via `POST /api/v1/sites/:id/worker/env` with `secret: false`.

```js
const value = env.MY_VAR; // string
```

### Secret Environment Variables

Set via `POST /api/v1/sites/:id/worker/env` with `secret: true`. Values are masked in API responses but available at full value inside the worker.

```js
const apiKey = env.API_SECRET; // full string, not masked
```

### KV Namespaces

Namespaces created via `POST /api/v1/sites/:id/worker/kv` appear as bindings under their `name`.

```js
// KV binding methods:
const value = await env.MY_KV.get("key");              // string | null
await env.MY_KV.put("key", "value");
await env.MY_KV.put("key", "value", { metadata: "{}", expirationTtl: 3600 });
await env.MY_KV.delete("key");
const list = await env.MY_KV.list({ prefix: "user:", limit: 100 });
```

**KV API:**

| Method | Signature | Returns |
|--------|-----------|---------|
| `get` | `(key: string) => Promise<string \| null>` | Value or null if missing/expired |
| `put` | `(key: string, value: string, opts?: KVPutOptions) => Promise<void>` | |
| `delete` | `(key: string) => Promise<void>` | |
| `list` | `(opts?: KVListOptions) => Promise<KVListResult>` | |

`KVPutOptions`: `{ metadata?: string, expirationTtl?: number }` (TTL in seconds)

`KVListOptions`: `{ prefix?: string, limit?: number }` (limit default/max 1000)

`KVListResult`:
```js
{
  keys: [{ name: string, metadata?: string, expiration?: number }],
  list_complete: boolean,
  cursor: string,
}
```

**Limits:**
- Max value size: 1 MB
- Expired entries are lazily deleted on read

**Difference from Cloudflare KV:** Values are stored in the application database (SQLite/Postgres/MySQL), not a distributed eventually-consistent store. Reads are always strongly consistent.

---

### Storage Buckets (R2-compatible)

Buckets created via `POST /api/v1/sites/:id/storage/buckets` appear as bindings under their `name` (e.g., `IMAGES`).

```js
// Get an object
const obj = await env.IMAGES.get("photo.jpg");
if (obj) {
  const text = await obj.text();
  const buffer = await obj.arrayBuffer();
  const json = await obj.json();
  // obj.key, obj.size, obj.etag, obj.httpMetadata.contentType
}

// Put an object
await env.IMAGES.put("photo.jpg", body, {
  httpMetadata: {
    contentType: "image/jpeg",
    contentEncoding: undefined,
    contentDisposition: undefined,
    contentLanguage: undefined,
    cacheControl: undefined,
  },
  customMetadata: { "x-uploader": "worker" },
});

// Delete an object (or array of keys)
await env.IMAGES.delete("photo.jpg");
await env.IMAGES.delete(["a.jpg", "b.jpg"]);

// Head (metadata only)
const meta = await env.IMAGES.head("photo.jpg");
// meta: { key, size, etag, httpMetadata, customMetadata } | null

// List objects
const list = await env.IMAGES.list({
  prefix: "photos/",
  limit: 1000,
  cursor: undefined,
  delimiter: "/",
});
// list: { objects: R2Object[], truncated: boolean, cursor?: string, delimitedPrefixes: string[] }

// Create a presigned GET URL
const url = await env.IMAGES.createSignedUrl("photo.jpg", { expiresIn: 3600 });

// Get a public URL (only available if bucket.public = true)
const url = env.IMAGES.publicUrl("photo.jpg");
```

**Storage binding API:**

| Method | Signature | Returns |
|--------|-----------|---------|
| `get` | `(key: string) => Promise<R2ObjectBody \| null>` | Object with body methods, or null |
| `put` | `(key: string, value: string \| ArrayBuffer \| Blob, opts?) => Promise<R2Object>` | Object metadata |
| `delete` | `(key: string \| string[]) => Promise<void>` | |
| `head` | `(key: string) => Promise<R2Object \| null>` | Metadata only, no body |
| `list` | `(opts?) => Promise<R2Objects>` | Paginated object list |
| `createSignedUrl` | `(key: string, opts?) => Promise<string>` | Presigned GET URL |
| `publicUrl` | `(key: string) => string` | Public URL (sync, bucket must be public) |

**Difference from Cloudflare R2:** Backed by MinIO/SeaweedFS instead of Cloudflare's edge network. The API is compatible but not all R2 features are implemented (e.g., no multipart upload API, no conditional operations).

---

### `ASSETS` Binding

Always available. Serves static files through the full site serving pipeline (respects `_redirects`, `_headers`, 404 handling, SPA mode).

```js
export default {
  async fetch(request, env, ctx) {
    const url = new URL(request.url);

    // Handle custom API route
    if (url.pathname.startsWith("/api/")) {
      return handleAPI(request, env);
    }

    // Fall back to static asset serving
    return env.ASSETS.fetch(request);
  },
};
```

`env.ASSETS.fetch(request: Request) => Promise<Response>`

---

## Execution Context (`ctx`)

The `ctx` object is passed to both `fetch` and `scheduled` handlers.

| Method | Behavior |
|--------|----------|
| `ctx.waitUntil(promise)` | Accepted but no-op (promise is not awaited after response) |
| `ctx.passThroughOnException()` | Accepted but no-op |

**Difference from Cloudflare:** `waitUntil` does not extend the worker lifetime. Any async work after the response is returned is not guaranteed to complete.

---

## Limits Summary

| Limit | Default | Configurable |
|-------|---------|-------------|
| Memory per runtime | 128 MB | `worker.memory_limit_mb` |
| Execution timeout | 30,000 ms | `worker.execution_timeout` |
| Max `fetch()` calls per request | 50 | `worker.max_fetch_requests` |
| Per-fetch timeout | 10 s | `worker.fetch_timeout_sec` |
| Max fetch response body | 10 MB | `worker.max_response_bytes` |
| Max `_worker.js` size | 1024 KB | `worker.max_script_size_kb` |
| KV value max size | 1 MB | hard-coded |
| KV list max limit | 1000 | hard-coded |
| `crypto.getRandomValues` max bytes | 65536 | hard-coded |
| `setInterval` max ticks | 1000 | hard-coded |

---

## Lifecycle

```
Deploy zip → extract files → compile _worker.js to bytecode → save bytecode.bin
                                                                      │
Request arrives → check pool for {siteID, deployID} → checkout runtime
     │                                                                 │
     └→ build env object (vars, KV, storage, ASSETS) ─────────────────┘
                              │
                         call fetch(request, env, ctx)
                              │
                    extract Response from JS
                              │
                     return runtime to pool
```

**Pool invalidation:** When a new deployment is pushed, the old pool (keyed by the previous `deployID`) is marked invalid. The next request creates a new pool for the new `deployID`.

**Server restart:** Bytecode is reloaded from disk (`bytecode.bin`). Pools are recreated lazily on first request.

---

## Differences from Cloudflare Workers

| Feature | Cloudflare | hostedat |
|---------|-----------|---------|
| JS engine | V8 | V8 (via v8go) |
| Module system | ESM (native V8) | ESM (wrapped via globalThis) |
| `crypto.subtle` algorithms | Full Web Crypto API | HMAC, AES-GCM, digest only |
| `importKey` formats | JWK, PKCS8, SPKI, raw | `raw` only |
| Timer accuracy | Wall-clock | Microtask-based (no real delay) |
| `waitUntil` | Extends lifetime | No-op |
| `fetch` rate limit | Aggregate billing | Per-invocation limit (default 50) |
| KV consistency | Eventually consistent | Strongly consistent (DB) |
| Storage | R2 (edge-replicated) | MinIO/SeaweedFS (single node) |
| `structuredClone` | Full structured clone | JSON-based (no Map/Set/circular) |
| `ReadableStream` | Fully streaming | Read-to-completion |
| `navigator.userAgent` | `"Cloudflare-Workers"` | `"hostedat-worker/1.0"` |

---

## Example Workers

### JSON API

```js
export default {
  async fetch(request, env, ctx) {
    const url = new URL(request.url);

    if (request.method === "GET" && url.pathname === "/api/items") {
      const items = await env.MY_KV.list({ prefix: "item:" });
      return Response.json({ items: items.keys });
    }

    if (request.method === "POST" && url.pathname === "/api/items") {
      const body = await request.json();
      await env.MY_KV.put(`item:${body.id}`, JSON.stringify(body));
      return Response.json({ ok: true }, { status: 201 });
    }

    return new Response("Not Found", { status: 404 });
  },
};
```

### Request Authentication

```js
export default {
  async fetch(request, env, ctx) {
    const authHeader = request.headers.get("Authorization");
    if (!authHeader || !authHeader.startsWith("Bearer ")) {
      return new Response("Unauthorized", { status: 401 });
    }

    const token = authHeader.slice(7);
    const expectedToken = env.API_TOKEN;

    // Constant-time comparison using HMAC
    const enc = new TextEncoder();
    const key = await crypto.subtle.importKey(
      "raw", enc.encode("comparison-key"),
      { name: "HMAC", hash: "SHA-256" },
      false, ["sign"]
    );
    const [a, b] = await Promise.all([
      crypto.subtle.sign("HMAC", key, enc.encode(token)),
      crypto.subtle.sign("HMAC", key, enc.encode(expectedToken)),
    ]);
    const aArr = new Uint8Array(a);
    const bArr = new Uint8Array(b);
    const match = aArr.length === bArr.length && aArr.every((v, i) => v === bArr[i]);

    if (!match) {
      return new Response("Unauthorized", { status: 401 });
    }

    return env.ASSETS.fetch(request);
  },
};
```

### Caching Proxy with KV

```js
export default {
  async fetch(request, env, ctx) {
    const url = new URL(request.url);
    const cacheKey = url.pathname;

    const cached = await env.CACHE.get(cacheKey);
    if (cached) {
      return new Response(cached, { headers: { "X-Cache": "HIT" } });
    }

    const upstream = await fetch(`https://api.upstream.com${url.pathname}`);
    const body = await upstream.text();

    await env.CACHE.put(cacheKey, body, { expirationTtl: 300 });

    return new Response(body, {
      headers: { "X-Cache": "MISS", "Content-Type": upstream.headers.get("Content-Type") ?? "text/plain" },
    });
  },
};
```

### SPA with Worker

```js
// Serve static SPA but intercept /api/* routes
export default {
  async fetch(request, env, ctx) {
    const url = new URL(request.url);

    if (url.pathname.startsWith("/api/")) {
      return handleAPI(request, env);
    }

    return env.ASSETS.fetch(request);
  },
};

async function handleAPI(request, env) {
  return Response.json({ timestamp: Date.now() });
}
```
