# Changelog

## v2.0.0 — V8 Worker Runtime Migration

**Released:** 2026-02-20
**Branch:** `migrate/v8go` (16 commits, 160 files changed, +54,971 / -2,563 lines)

### Breaking Changes

- **Runtime engine replaced:** QuickJS/Wazero WASM interpreter removed, replaced with native V8 via [`tommie/v8go`](https://github.com/tommie/v8go). Workers now execute on the V8 engine with significantly better performance and broader Web API compatibility.
- **Configuration:** `worker.wasm_path` replaced with `worker.data_dir` in `config.yaml`.
- **Go 1.25+ required:** The minimum Go version is now 1.25 (up from 1.22).
- **CGO required:** V8 bindings require CGO enabled at build time (Linux only; no Windows native build).

### New Features

#### Web APIs
- **Cache API** — `caches.default` and `caches.open(name)` with `match`, `put`, `delete` backed by SQLite storage with TTL expiry
- **D1 Database** — SQL database bindings with `prepare`, `bind`, `first`, `all`, `run`, `batch`, `exec`; site-prefixed file isolation and path traversal protection
- **Durable Objects** — Stateful coordination primitives with `DurableObjectNamespace.get(id)`, `DurableObjectStub.fetch()`, transactional storage, and alarm scheduling
- **HTMLRewriter** — Streaming HTML transformation engine with `element()`, `text()`, `comments()` handlers and full selector support
- **WebSockets** — Full `WebSocket` client with `open`, `message`, `close`, `error` events and binary frame support
- **TCP Sockets** — `connect(address, options)` for raw TCP/TLS connections with `readable`/`writable` stream pairs
- **EventSource** — Server-Sent Events client with auto-reconnect, `lastEventId` tracking, and custom headers
- **URLPattern** — URL pattern matching compatible with the URLPattern Web API spec
- **Queues** — `Queue.send()` and `Queue.sendBatch()` for asynchronous message passing between workers
- **Service Bindings** — Inter-worker RPC via `serviceBinding.fetch()` with request forwarding
- **MessageChannel / MessagePort** — Structured clone messaging between execution contexts
- **BYOB Readers** — `ReadableStreamBYOBReader` for zero-copy reading into caller-supplied buffers

#### Crypto
- **ECDH / X25519** — Key agreement via `deriveBits` and `deriveKey` for P-256, P-384, P-521, and X25519 curves
- **Ed25519** — EdDSA signing and verification
- **RSA** — RSA-OAEP encryption/decryption, RSA-PSS and RSASSA-PKCS1-v1_5 signing
- **AES Key Wrapping** — AES-KW and AES-GCM key wrap/unwrap
- **HKDF / PBKDF2** — Key derivation functions
- **DigestStream** — Streaming hash computation (`SHA-1`, `SHA-256`, `SHA-384`, `SHA-512`)

#### Compression
- **Brotli** — `CompressionStream('br')` and `DecompressionStream('br')`
- **Gzip / Deflate** — `CompressionStream('gzip'|'deflate')` and `DecompressionStream('gzip'|'deflate'|'deflate-raw')`

#### Streams
- **TextEncoder/DecoderStream** — Streaming UTF-8 encode/decode via TransformStream
- **ReadableStream** — Full implementation with `tee()`, `pipeTo()`, `pipeThrough()`, `cancel()`
- **WritableStream** — With `abort()` and backpressure support
- **FixedLengthStream** — Validates content length and signals early on mismatch

#### Worker Engine
- **Worker Pool** — Configurable pool with per-site isolate reuse, memory limits (`v8.WithResourceConstraints`), and automatic invalidation on timeout
- **Event Loop** — Microtask-aware event loop with `setTimeout`/`setInterval`, promise settling, and `waitUntil` lifecycle
- **ES Module Wrapping** — `export default { fetch }` syntax support via `wrapESModule()`
- **Unhandled Rejection Tracking** — `unhandledrejection` event for debugging promise errors
- **Bundle Support** — Multi-file worker bundles with automatic dependency resolution

### Improvements

- **Fetch API** — Private network protection (CIDR ranges parsed at init), redirect following, streaming bodies
- **KV Store** — List with prefix/cursor pagination, expiration TTL, metadata support
- **R2 Storage** — `head`, `list`, `delete`, `createMultipartUpload`, `uploadPart`, `completeMultipartUpload`
- **Console** — Structured logging with `console.log/warn/error/debug/info` piped to `WorkerLog` model
- **Cron Scheduling** — Cron-triggered `scheduled` event handlers with jitter and distributed locking
- **FormData** — Multipart form parsing with `File` and `Blob` support
- **Abort Signals** — `AbortController`/`AbortSignal` with timeout support

### Database

- **New models:** `D1Database`, `DurableObjectNamespace`, `S3Credential`, `RevokedToken`, `CacheEntry`
- **Auto-migration:** All new models included in `AutoMigrate` call
- **D1 isolation:** Database files prefixed with `{siteID}_` to prevent cross-site access
- **Durable Object isolation:** Namespace IDs prefixed with `{siteID}:` for tenant separation
- **Storage binding uniqueness:** Startup check prevents duplicate binding names per site

### Testing

- **Comprehensive coverage:** 80+ new test files across all packages
- **API test suite:** Full handler coverage for admin, auth, deploy, site, storage, worker endpoints
- **Worker runtime tests:** Every new API (Cache, D1, Durable Objects, WebSockets, TCP, HTMLRewriter, etc.) has dedicated test files
- **Cross-site isolation tests:** Verify D1 and Durable Object tenant boundaries
- **Path traversal tests:** Validate `ValidateDatabaseID()` rejects malicious inputs
- **Crypto regression tests:** Cover BigInt trap, key import/export round-trips, algorithm edge cases

### Documentation

- **worker-runtime.md** — 16 new API sections with usage examples and Cloudflare compatibility table
- **architecture.md** — Updated execution flow and new model references
- **workers.astro** — New interactive examples, environment binding subsections
- **README.md** — Updated feature highlights and Web API listing

### Dependencies

- **Added:** `github.com/tommie/v8go` (V8 JavaScript engine bindings)
- **Removed:** QuickJS, Wazero (WASM runtime)
- **Updated:** `go.mod` now requires Go 1.25+

### Migration Guide

1. Update `config.yaml`: replace `worker.wasm_path` with `worker.data_dir`
2. Ensure build environment has CGO enabled (Linux required for V8)
3. Run the binary — `AutoMigrate` will create new tables automatically
4. Existing KV namespaces, environment variables, and cron schedules are preserved
5. Worker scripts using `export default { fetch(request, env) {} }` will work as before
