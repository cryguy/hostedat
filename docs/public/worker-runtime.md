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
| `getAll` | `(name: string) => string[]` | All values for name |
| `has` | `(name: string) => boolean` | Check existence |
| `set` | `(name: string, value: string) => void` | Set parameter (replaces existing) |
| `append` | `(name: string, value: string) => void` | Append parameter |
| `delete` | `(name: string) => void` | Remove parameter |
| `toString` | `() => string` | Encode to query string |
| `forEach` | `(cb) => void` | Iterate entries |
| `entries` | `() => Iterator` | `[name, value]` pairs |
| `keys` | `() => Iterator` | Param names |
| `values` | `() => Iterator` | Param values |

Full read-write support: `get`, `set`, `has`, `append`, `delete`, `toString`, `forEach`, `entries`, `keys`, `values`.

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

Supported formats: `"raw"`, `"jwk"`. For RSA keys: `"jwk"`, `"spki"` (public), `"pkcs8"` (private). For Ed25519: `"raw"`, `"jwk"`.

Supported algorithms for import: `HMAC`, `AES-GCM`, `AES-CBC`, `AES-CTR`, `AES-KW`, `ECDSA` (P-256, P-384), `ECDH` (P-256, P-384, P-521), `X25519`, `RSASSA-PKCS1-v1_5`, `RSA-PSS`, `RSA-OAEP`, `Ed25519`.

Returns `Promise<CryptoKey>`.

---

### `crypto.subtle.exportKey(format, key)`

```js
const keyBytes = await crypto.subtle.exportKey("raw", key);
```

Supported formats: `"raw"`, `"jwk"`. For RSA keys: `"jwk"`, `"spki"` (public), `"pkcs8"` (private). For Ed25519: `"raw"`, `"jwk"`. Key must have been imported with `extractable: true`.

Returns `Promise<ArrayBuffer>`.

---

### `crypto.subtle.sign(algorithm, key, data)` / `crypto.subtle.verify(algorithm, key, signature, data)`

```js
// Sign
const signature = await crypto.subtle.sign("HMAC", key, data);

// Verify
const valid = await crypto.subtle.verify("HMAC", key, signature, data);
```

Supported algorithms: `HMAC`, `ECDSA` (P-256, P-384 with SHA-256/384/512), `RSASSA-PKCS1-v1_5`, `RSA-PSS`, `Ed25519`.

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

Supported algorithms: `AES-GCM` (iv must be 12 bytes), `AES-CBC` (iv must be 16 bytes), `AES-CTR` (counter must be 16 bytes), `RSA-OAEP`.

Returns `Promise<ArrayBuffer>`.

**Difference from Cloudflare:** Most common algorithms are supported (HMAC, ECDSA, ECDH, X25519, RSA, Ed25519, AES-GCM, AES-CBC, AES-CTR, RSA-OAEP).

---

### `crypto.subtle.generateKey(algorithm, extractable, usages)`

```js
const keyPair = await crypto.subtle.generateKey(
  { name: "ECDSA", namedCurve: "P-256" },
  true,
  ["sign", "verify"]
);
// keyPair.publicKey, keyPair.privateKey
```

Supported algorithms:
- `ECDSA` (P-256, P-384) — returns `CryptoKeyPair`
- `ECDH` (P-256, P-384, P-521) — returns `CryptoKeyPair`
- `X25519` — returns `CryptoKeyPair`
- `HMAC` (with hash) — returns `CryptoKey`
- `AES-GCM`, `AES-CBC`, `AES-CTR`, `AES-KW` (with length: 128, 192, 256) — returns `CryptoKey`
- `RSASSA-PKCS1-v1_5`, `RSA-PSS`, `RSA-OAEP` (with modulusLength, publicExponent, hash) — returns `CryptoKeyPair`
- `Ed25519` — returns `CryptoKeyPair`

---

### `crypto.subtle.deriveBits(algorithm, baseKey, length)` / `crypto.subtle.deriveKey(...)`

```js
// HKDF
const derived = await crypto.subtle.deriveBits(
  { name: "HKDF", hash: "SHA-256", salt: saltBytes, info: infoBytes },
  baseKey,
  256
);

// PBKDF2
const derived = await crypto.subtle.deriveBits(
  { name: "PBKDF2", hash: "SHA-256", salt: saltBytes, iterations: 100000 },
  baseKey,
  256
);
```

Supported algorithms: `HKDF` (with hash, salt, info), `PBKDF2` (with hash, salt, iterations), `ECDH` (with public key), `X25519` (with public key).

`deriveBits` returns `Promise<ArrayBuffer>`. `deriveKey` returns `Promise<CryptoKey>` with the derived key imported for the specified target algorithm.

---

### `crypto.subtle.wrapKey(format, key, wrappingKey, wrapAlgorithm)` / `crypto.subtle.unwrapKey(...)`

```js
// Wrap a key
const wrapped = await crypto.subtle.wrapKey("raw", keyToWrap, wrappingKey, { name: "AES-GCM", iv });

// Unwrap a key
const unwrapped = await crypto.subtle.unwrapKey(
  "raw", wrappedKeyData, wrappingKey,
  { name: "AES-GCM", iv },
  { name: "AES-GCM" },
  true, ["encrypt", "decrypt"]
);
```

Wraps/unwraps a key for secure transport. Combines `exportKey` + `encrypt` (wrap) or `decrypt` + `importKey` (unwrap). Supported wrap algorithms: `AES-GCM`, `AES-KW` (RFC 3394).

---

### ECDH / X25519 Key Agreement

Elliptic-curve Diffie-Hellman key agreement is fully supported for `ECDH` (P-256, P-384, P-521) and `X25519`.

```js
// ECDH key agreement
const aliceKeys = await crypto.subtle.generateKey(
  { name: "ECDH", namedCurve: "P-256" },
  true,
  ["deriveBits", "deriveKey"]
);
const bobKeys = await crypto.subtle.generateKey(
  { name: "ECDH", namedCurve: "P-256" },
  true,
  ["deriveBits", "deriveKey"]
);

// Derive shared secret
const sharedBits = await crypto.subtle.deriveBits(
  { name: "ECDH", public: bobKeys.publicKey },
  aliceKeys.privateKey,
  256
);

// Or derive an AES key directly
const aesKey = await crypto.subtle.deriveKey(
  { name: "ECDH", public: bobKeys.publicKey },
  aliceKeys.privateKey,
  { name: "AES-GCM", length: 256 },
  false,
  ["encrypt", "decrypt"]
);
```

```js
// X25519 key agreement
const aliceKeys = await crypto.subtle.generateKey(
  { name: "X25519" },
  true,
  ["deriveBits", "deriveKey"]
);
const bobKeys = await crypto.subtle.generateKey(
  { name: "X25519" },
  true,
  ["deriveBits", "deriveKey"]
);

const sharedSecret = await crypto.subtle.deriveBits(
  { name: "X25519", public: bobKeys.publicKey },
  aliceKeys.privateKey,
  256
);
```

| Feature | Details |
|---------|---------|
| ECDH curves | P-256, P-384, P-521 |
| X25519 | 32-byte keys, Curve25519 |
| Import formats | `raw` (public), `jwk` (ECDH only) |
| Export formats | `raw`, `jwk` (ECDH only) |
| Operations | `generateKey`, `deriveBits`, `deriveKey`, `importKey`, `exportKey` |

**Difference from Cloudflare:** Fully compatible. Both P-curve ECDH and X25519 are supported.

---

### AES-CTR

Counter-mode symmetric encryption. The counter must be exactly 16 bytes.

```js
const key = await crypto.subtle.generateKey(
  { name: "AES-CTR", length: 256 },
  true,
  ["encrypt", "decrypt"]
);

const counter = crypto.getRandomValues(new Uint8Array(16));
const ciphertext = await crypto.subtle.encrypt(
  { name: "AES-CTR", counter, length: 64 },
  key,
  new TextEncoder().encode("Hello")
);
```

---

### AES-KW (Key Wrapping)

RFC 3394 AES Key Wrap for securely transporting keys. The key to wrap must be a multiple of 8 bytes and at least 16 bytes.

```js
const wrappingKey = await crypto.subtle.generateKey(
  { name: "AES-KW", length: 256 },
  true,
  ["wrapKey", "unwrapKey"]
);

const keyToWrap = await crypto.subtle.generateKey(
  { name: "AES-GCM", length: 256 },
  true,
  ["encrypt", "decrypt"]
);

// Wrap
const wrapped = await crypto.subtle.wrapKey("raw", keyToWrap, wrappingKey, { name: "AES-KW" });

// Unwrap
const unwrapped = await crypto.subtle.unwrapKey(
  "raw", wrapped, wrappingKey,
  { name: "AES-KW" },
  { name: "AES-GCM" },
  true, ["encrypt", "decrypt"]
);
```

---

### `crypto.DigestStream`

A `WritableStream` that computes a hash digest as data is written to it. Cloudflare Workers-compatible.

```js
const ds = new crypto.DigestStream("SHA-256");
const writer = ds.writable.getWriter();
await writer.write(new TextEncoder().encode("hello "));
await writer.write(new TextEncoder().encode("world"));
await writer.close();

const digest = await ds.digest; // ArrayBuffer
const hex = Array.from(new Uint8Array(digest))
  .map(b => b.toString(16).padStart(2, "0")).join("");
```

Supported algorithms: `SHA-1`, `SHA-256`, `SHA-384`, `SHA-512`.

**Difference from Cloudflare:** Fully compatible. Available as both `crypto.DigestStream` and the global `DigestStream`.

---

## Timers

```js
setTimeout(fn, delay)   // -> id
clearTimeout(id)
setInterval(fn, delay)  // -> id
clearInterval(id)
```

Timers use Go-backed wall-clock delays via the event loop. Timer delays are honored — `setTimeout(fn, 100)` will wait approximately 100ms before firing. `setInterval` has a minimum interval of 10ms.

**Difference from Cloudflare:** Timer semantics are similar. Both use real wall-clock delays.

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

AbortController/AbortSignal are fully functional. Accepted by `fetch()` and integrated with the V8 event loop.

### `ReadableStream` / `WritableStream` / `TransformStream`

Full JS polyfill implementation. `ReadableStream`, `WritableStream`, and `TransformStream` support `start`/`pull`/`cancel` controllers, `getReader()`/`getWriter()`, piping, and `Symbol.asyncIterator`. Also includes `FixedLengthStream` and `ReadableStream.from()`. Streaming chunked responses are not fully supported — the body is read to completion before being sent.

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

## WebSocket

Client WebSocket connections from workers. Compatible with the Cloudflare Workers WebSocket API.

```js
export default {
  async fetch(request, env) {
    const resp = await fetch("https://echo.websocket.org", {
      headers: { Upgrade: "websocket" },
    });
    const ws = resp.webSocket;
    ws.accept();

    ws.addEventListener("message", (event) => {
      console.log("Received:", event.data);
    });

    ws.send("Hello from worker!");
    ws.close();

    return new Response("WebSocket session complete");
  },
};
```

`WebSocketPair` is also available for creating paired WebSocket connections:

```js
const [client, server] = Object.values(new WebSocketPair());
server.accept();
server.addEventListener("message", (event) => { /* ... */ });
return new Response(null, { status: 101, webSocket: client });
```

---

## HTMLRewriter

Streaming HTML transformation using a Cloudflare-compatible API. Allows you to modify HTML responses on the fly using CSS selectors.

```js
export default {
  async fetch(request, env) {
    const response = await env.ASSETS.fetch(request);

    return new HTMLRewriter()
      .on("h1", {
        element(el) {
          el.setInnerContent("Modified Title");
        },
      })
      .on("a[href]", {
        element(el) {
          const href = el.getAttribute("href");
          if (href.startsWith("http://")) {
            el.setAttribute("href", href.replace("http://", "https://"));
          }
        },
      })
      .transform(response);
  },
};
```

**Element handlers:** `element(el)`, `comments(comment)`, `text(text)`

**Element methods:** `getAttribute(name)`, `setAttribute(name, value)`, `removeAttribute(name)`, `hasAttribute(name)`, `setInnerContent(content)`, `prepend(content)`, `append(content)`, `remove()`, `tagName` (read-only)

**Document handlers:** `.onDocument({ doctype(dt), comments(c), text(t), end(end) })`

---

## CompressionStream / DecompressionStream

Streaming compression and decompression using standard Web APIs.

```js
// Compress
const compressed = new Response("Hello world").body
  .pipeThrough(new CompressionStream("gzip"));

// Decompress
const decompressed = compressedStream
  .pipeThrough(new DecompressionStream("gzip"));
```

Supported formats: `"gzip"`, `"deflate"`, `"deflate-raw"`, `"br"` (Brotli).

Go-backed streaming compression using real compressor/decompressor goroutines. Each chunk is processed incrementally rather than buffered to completion.

**Difference from Cloudflare:** Fully compatible. Brotli (`"br"`) is additionally supported beyond the standard Web API formats.

---

## `tail()` Handler

Receive log events from your worker for observability.

```js
export default {
  async fetch(request, env) {
    console.log("handling request");
    return new Response("OK");
  },

  async tail(events) {
    for (const event of events) {
      console.log(event.logs);
    }
  },
};
```

The `tail` handler is called with an array of trace events after the `fetch` or `scheduled` handler completes.

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

### D1 Database

D1 bindings provide a Cloudflare Workers-compatible SQL database API backed by isolated per-binding SQLite databases (WAL mode enabled).

```js
// D1 bindings are configured via the API and appear as env properties.
const stmt = env.MY_DB.prepare("SELECT * FROM users WHERE id = ?").bind(userId);
const { results } = await stmt.all();

// Insert with bindings
await env.MY_DB.prepare("INSERT INTO users (name, email) VALUES (?, ?)")
  .bind("Alice", "alice@example.com")
  .run();

// Get a single row
const user = await env.MY_DB.prepare("SELECT * FROM users WHERE id = ?")
  .bind(1)
  .first();

// Raw rows (arrays instead of objects)
const rows = await env.MY_DB.prepare("SELECT id, name FROM users")
  .raw({ columnNames: true });

// Batch multiple statements
const results = await env.MY_DB.batch([
  env.MY_DB.prepare("INSERT INTO users (name) VALUES (?)").bind("Bob"),
  env.MY_DB.prepare("INSERT INTO users (name) VALUES (?)").bind("Carol"),
]);

// Execute raw SQL (multiple semicolon-separated statements)
await env.MY_DB.exec("CREATE TABLE IF NOT EXISTS logs (id INTEGER PRIMARY KEY, msg TEXT)");
```

**D1 PreparedStatement API:**

| Method | Signature | Returns |
|--------|-----------|---------|
| `bind` | `(...values) => D1PreparedStatement` | New statement with bound parameters |
| `all` | `() => Promise<{ results, success, meta }>` | All rows as objects |
| `first` | `(column?) => Promise<object \| value \| null>` | First row or column value |
| `raw` | `(opts?) => Promise<any[][]>` | Rows as arrays |
| `run` | `() => Promise<{ success, meta }>` | Execute without returning rows |

**D1 Meta:**

```js
// meta shape:
{ changed_db: boolean, changes: number, last_row_id: number, rows_read: number, rows_written: number }
```

**Difference from Cloudflare D1:** Backed by a local SQLite database file per binding (`{dataDir}/d1/{databaseID}.sqlite3`), not Cloudflare's distributed SQLite. `dump()` is not supported.

---

### Durable Objects

Durable Objects provide globally unique, persistent storage objects. Each object has a unique ID and transactional key-value storage.

```js
// Get a Durable Object namespace from env
const id = env.MY_DO.idFromName("my-object");
const stub = env.MY_DO.get(id);

// Storage operations (on the stub)
await stub.storage.put("counter", 42);
const value = await stub.storage.get("counter"); // 42

// Bulk operations
await stub.storage.put({ key1: "val1", key2: "val2" });
const map = await stub.storage.get(["key1", "key2"]); // Map { "key1" => "val1", ... }

// List with options
const entries = await stub.storage.list({ prefix: "user:", limit: 10, reverse: false });

// Delete
await stub.storage.delete("counter"); // true
await stub.storage.delete(["key1", "key2"]); // count
await stub.storage.deleteAll();
```

**DurableObjectNamespace API:**

| Method | Signature | Returns |
|--------|-----------|---------|
| `idFromName` | `(name: string) => DurableObjectId` | Deterministic ID from name |
| `idFromString` | `(hex: string) => DurableObjectId` | ID from hex string |
| `newUniqueId` | `() => DurableObjectId` | Random unique ID |
| `get` | `(id: DurableObjectId) => DurableObjectStub` | Get stub for the object |

**DurableObjectStorage API:**

| Method | Signature | Returns |
|--------|-----------|---------|
| `get` | `(key: string) => Promise<any \| null>` | Single value |
| `get` | `(keys: string[]) => Promise<Map>` | Multiple values |
| `put` | `(key: string, value: any) => Promise<void>` | Store single value |
| `put` | `(entries: object) => Promise<void>` | Store multiple values |
| `delete` | `(key: string) => Promise<boolean>` | Delete single key |
| `delete` | `(keys: string[]) => Promise<number>` | Delete multiple keys |
| `deleteAll` | `() => Promise<void>` | Delete all entries |
| `list` | `(opts?) => Promise<Map>` | List entries (prefix, limit, reverse) |

**Difference from Cloudflare:** Storage is backed by the application database (GORM/SQLite), not a globally distributed coordination layer. `stub.fetch()` returns a placeholder response. Real Durable Object class instantiation and alarm scheduling are not yet implemented.

---

### Cache API

A Cloudflare Workers-compatible Cache API for storing and retrieving HTTP responses.

```js
// Use the default cache
const cache = caches.default;

// Or open a named cache
const myCache = await caches.open("my-cache");

// Store a response (TTL from Cache-Control: max-age)
const response = new Response("cached data", {
  headers: { "Cache-Control": "max-age=3600" },
});
await cache.put("https://example.com/data", response);

// Retrieve a cached response
const cached = await cache.match("https://example.com/data");
if (cached) {
  const text = await cached.text();
}

// Delete from cache
const deleted = await cache.delete("https://example.com/data"); // true/false
```

**Cache API:**

| Method | Signature | Returns |
|--------|-----------|---------|
| `match` | `(request: string \| Request) => Promise<Response \| undefined>` | Cached response or undefined |
| `put` | `(request: string \| Request, response: Response) => Promise<void>` | Store response |
| `delete` | `(request: string \| Request) => Promise<boolean>` | Delete entry |

**CacheStorage:**

| Method | Signature | Returns |
|--------|-----------|---------|
| `open` | `(name: string) => Promise<Cache>` | Open a named cache |
| `default` | `Cache` | The default cache instance |

**Difference from Cloudflare:** Cache is backed by the application database, not Cloudflare's edge cache. Expired entries (based on `Cache-Control: max-age`) are cleaned up on read.

---

### Queues

Queue bindings allow workers to send messages to named queues for asynchronous processing.

```js
// Send a single message
await env.MY_QUEUE.send({ action: "process", id: 123 });

// Send with content type
await env.MY_QUEUE.send("raw text", { contentType: "text" });

// Send a batch of messages
await env.MY_QUEUE.sendBatch([
  { body: JSON.stringify({ id: 1 }), contentType: "json" },
  { body: JSON.stringify({ id: 2 }), contentType: "json" },
]);
```

**Queue API:**

| Method | Signature | Returns |
|--------|-----------|---------|
| `send` | `(body: any, options?: { contentType?: string }) => Promise<void>` | Send single message |
| `sendBatch` | `(messages: { body, contentType? }[]) => Promise<void>` | Send batch |

**Difference from Cloudflare:** Messages are stored in the application database (SQLite). Queue consumers and pull-based consumption are managed server-side, not via a `queue()` handler in the worker.

---

### Service Bindings

Service bindings allow one worker to call another worker's `fetch` handler directly, without going through HTTP.

```js
// Call another worker via service binding
const response = await env.AUTH_SERVICE.fetch("https://auth/verify", {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({ token: "..." }),
});
const result = await response.json();
```

**Service Binding API:**

| Method | Signature | Returns |
|--------|-----------|---------|
| `fetch` | `(url: string \| Request, init?: RequestInit) => Promise<Response>` | Response from target worker |

**Difference from Cloudflare:** Calls are routed through the worker engine on the same server. No edge network routing or zone-based binding configuration.

---

### TCP Sockets (`connect()`)

The `connect()` global creates outbound TCP connections, compatible with the Cloudflare Workers TCP Socket API.

```js
const socket = connect("example.com:8080");

// Or with options
const socket = connect({ hostname: "example.com", port: 443 }, {
  secureTransport: "on",  // "on" for TLS, "off" for plain TCP
  allowHalfOpen: false,
});

// Read from the socket
const reader = socket.readable.getReader();
const { value, done } = await reader.read();

// Write to the socket
const writer = socket.writable.getWriter();
await writer.write(new TextEncoder().encode("GET / HTTP/1.0\r\n\r\n"));

// Upgrade to TLS (STARTTLS)
const tlsSocket = socket.startTls();

// Close
await socket.close();

// Wait for close
await socket.closed;
```

**Socket properties:**

| Property | Type | Description |
|----------|------|-------------|
| `readable` | `ReadableStream` | Read data from the socket |
| `writable` | `WritableStream` | Write data to the socket |
| `closed` | `Promise<void>` | Resolves when the socket closes |
| `opened` | `Promise<{ remoteAddress, localAddress }>` | Resolves when connected |
| `close()` | `() => Promise<void>` | Close the socket |
| `startTls()` | `() => Socket` | Upgrade to TLS (returns new socket) |

**SSRF protection:** Connections to private/loopback IP addresses (127.x.x.x, 10.x.x.x, 172.16-31.x.x, 192.168.x.x, localhost) are blocked.

**Difference from Cloudflare:** Compatible API. Connections are made from the server process directly. No Cloudflare Spectrum or regional restrictions.

---

### EventSource (Server-Sent Events)

`EventSource` provides client-side SSE support for workers to consume server-sent event streams.

```js
const es = new EventSource("https://api.example.com/events");

es.onopen = () => console.log("Connected");

es.onmessage = (event) => {
  console.log("Message:", event.data);
};

es.addEventListener("custom-event", (event) => {
  console.log("Custom:", event.data, event.lastEventId);
});

es.onerror = (event) => {
  console.error("Error:", event.message);
  es.close();
};

// Close the connection
es.close();
```

**EventSource properties:**

| Property/Method | Type | Description |
|-----------------|------|-------------|
| `url` | `string` | The SSE endpoint URL |
| `readyState` | `number` | `0` (CONNECTING), `1` (OPEN), `2` (CLOSED) |
| `withCredentials` | `boolean` | Credentials flag |
| `onopen` | `EventHandler` | Fired on connection open |
| `onmessage` | `EventHandler` | Fired on `message` events |
| `onerror` | `EventHandler` | Fired on errors |
| `close()` | `() => void` | Close the connection |

**SSRF protection:** Connections to private IP addresses are blocked.

**Difference from Cloudflare:** Cloudflare Workers do not provide a built-in `EventSource` class. This is an extension.

---

### URLPattern

`URLPattern` provides URL pattern matching, compatible with the URLPattern Web API.

```js
const pattern = new URLPattern({ pathname: "/users/:id" });

// Test if a URL matches
pattern.test("https://example.com/users/123"); // true
pattern.test("https://example.com/posts/123"); // false

// Extract matched groups
const result = pattern.exec("https://example.com/users/123");
// result.pathname.groups.id === "123"
```

**Constructor forms:**

```js
// Object with individual components
new URLPattern({ protocol: "https", hostname: "*.example.com", pathname: "/api/*" });

// String pattern with base URL
new URLPattern("/users/:id", "https://example.com");

// Full URL string pattern
new URLPattern("https://example.com/users/:id");
```

**URLPattern API:**

| Method | Signature | Returns |
|--------|-----------|---------|
| `test` | `(input: string \| URL \| object, baseURL?: string) => boolean` | Whether the input matches |
| `exec` | `(input: string \| URL \| object, baseURL?: string) => URLPatternResult \| null` | Match result or null |

**Difference from Cloudflare:** Compatible API. Supports `:param` named groups and `*` wildcards. Does not support the full URLPattern regex syntax.

---

### TextEncoderStream / TextDecoderStream

Streaming text encoding/decoding as `TransformStream` subclasses.

```js
// Encode string chunks to UTF-8 bytes
const encoder = new TextEncoderStream();
const writer = encoder.writable.getWriter();
const reader = encoder.readable.getReader();
writer.write("Hello world");
const { value } = await reader.read(); // Uint8Array

// Decode UTF-8 bytes to string chunks
const decoder = new TextDecoderStream();
const dWriter = decoder.writable.getWriter();
const dReader = decoder.readable.getReader();
dWriter.write(new Uint8Array([72, 101, 108, 108, 111]));
const { value: text } = await dReader.read(); // "Hello"
```

`IdentityTransformStream` is also available as a pass-through `TransformStream`.

---

### MessageChannel / MessagePort

`MessageChannel` creates a pair of connected `MessagePort` objects for structured message passing.

```js
const channel = new MessageChannel();

channel.port1.onmessage = (event) => {
  console.log("Port 1 received:", event.data);
};

channel.port2.postMessage({ hello: "world" });
```

**MessagePort API:**

| Method | Signature | Description |
|--------|-----------|-------------|
| `postMessage` | `(data: any) => void` | Send a structured-cloned message to the remote port |
| `start` | `() => void` | Start receiving queued messages (auto-started) |
| `close` | `() => void` | Close the port |

Ports auto-start (Cloudflare Workers behavior). Messages are cloned via `structuredClone`.

---

### ReadableStream BYOB Reader

Byte-oriented readable streams with "bring your own buffer" readers for zero-copy reading.

```js
const stream = new ReadableStream({
  type: "bytes",
  start(controller) {
    controller.enqueue(new Uint8Array([1, 2, 3, 4]));
    controller.close();
  },
});

const reader = stream.getReader({ mode: "byob" });
const buffer = new Uint8Array(4);
const { value, done } = await reader.read(buffer);
// value is a Uint8Array view into the buffer with the read data
```

Adds `ReadableStreamBYOBReader` and `ReadableByteStreamController`. Existing `ReadableStream` is monkey-patched to support `{ type: "bytes" }` underlying sources and `getReader({ mode: "byob" })`.

---

### Unhandled Rejection Tracking

`PromiseRejectionEvent` and best-effort `unhandledrejection` event tracking.

```js
globalThis.addEventListener("unhandledrejection", (event) => {
  console.error("Unhandled rejection:", event.reason);
  // event.promise is the rejected promise
});

// This will fire the event:
Promise.reject(new Error("oops"));
```

Uses microtask-based detection: if a rejected promise is not handled before the next microtask, an `unhandledrejection` event is dispatched on `globalThis`.

**Difference from Cloudflare:** Compatible event shape. Detection is best-effort via microtask timing rather than V8 engine-level hooks.

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

---

## Lifecycle

```
Deploy zip → extract files → parse _worker.js → create V8 context
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

**Server restart:** V8 isolates are recreated lazily on first request. Source code is re-parsed from disk.

---

## Differences from Cloudflare Workers

| Feature | Cloudflare | hostedat |
|---------|-----------|---------|
| JS engine | V8 | V8 (via v8go) |
| Module system | ESM (native V8) | ESM (wrapped via globalThis) |
| `crypto.subtle` algorithms | Full Web Crypto API | HMAC, ECDSA, ECDH, X25519, RSA (PKCS1v15, PSS, OAEP), Ed25519, AES-GCM, AES-CBC, AES-CTR, AES-KW, HKDF, PBKDF2, digest |
| `importKey` formats | JWK, PKCS8, SPKI, raw | `raw`, `jwk`, `pkcs8`, `spki` |
| `DigestStream` | Available | Available |
| Timer accuracy | Wall-clock | Wall-clock (Go event loop) |
| `waitUntil` | Extends lifetime | No-op |
| `fetch` rate limit | Aggregate billing | Per-invocation limit (default 50) |
| KV consistency | Eventually consistent | Strongly consistent (DB) |
| Storage | R2 (edge-replicated) | MinIO/SeaweedFS (single node) |
| D1 database | Distributed SQLite | Local SQLite per binding |
| Durable Objects | Full DO runtime | Storage API only (no class instantiation/alarms) |
| Cache API | Edge cache | Database-backed cache |
| Queues | Queue producers + consumers | Queue producer (send/sendBatch) |
| Service Bindings | Edge routing | Same-server worker-to-worker calls |
| TCP Sockets (`connect()`) | Via Cloudflare network | Direct from server (SSRF-protected) |
| EventSource | Not built-in | Available (SSE client) |
| URLPattern | Available | Available (`:param` and `*` wildcards) |
| CompressionStream | gzip, deflate, deflate-raw | gzip, deflate, deflate-raw, br (Brotli) |
| TextEncoderStream / TextDecoderStream | Available | Available |
| MessageChannel / MessagePort | Available | Available (auto-started ports) |
| BYOB Reader | Available | Available (`ReadableStream` type: "bytes") |
| `unhandledrejection` event | Engine-level tracking | Best-effort microtask-based tracking |
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
