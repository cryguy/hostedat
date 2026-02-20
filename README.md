# hostedat

Self-hosted static site hosting platform with server-side JavaScript workers. Upload a zip of your built site and get a live subdomain — like Cloudflare Pages, but on your own server with single-command deploys.

## Features

- **Subdomain routing** — each site gets `<name>.yourdomain.com`, served instantly
- **Single-command deploys** — `hostedat deploy my-site ./dist` from your terminal or CI
- **Server-side workers** — deploy `_worker.js` for dynamic request handling (Cloudflare Workers-compatible API)
- **KV storage** — per-site key-value namespaces accessible from workers
- **D1 database** — per-site SQLite databases accessible from workers (Cloudflare D1-compatible API)
- **Durable Objects** — persistent, transactional key-value storage with atomic operations
- **Cache API** — `caches.default` and `caches.open()` for HTTP response caching
- **Cron triggers** — schedule worker execution with standard cron expressions
- **Netlify-compatible `_redirects`** — redirects, rewrites, and SPA fallback rules
- **Custom `_headers`** — per-path response headers
- **Custom `404.html`** — drop one in your upload and it just works
- **SPA mode** — auto-detected on deploy or toggled per site
- **Automatic HTTPS** — wildcard certs via Let's Encrypt + CertMagic (DNS-01 with Cloudflare)
- **User management** — roles (superadmin, admin, user), invite system, API keys
- **Dashboard** — React frontend for managing sites, deployments, users, and settings
- **CLI client** — deploy from anywhere, integrates with CI/CD
- **Reproducible builds** — deterministic binaries with `-trimpath` and zero build IDs
- **Portable** — single server binary (requires CGO for V8), pure-Go CLI, SQLite by default, swap to Postgres/MySQL via config

## Quick Start

### 1. Configure

```bash
cp config.example.yaml config.yaml
```

Edit `config.yaml` — at minimum set `domain`, `jwt_secret`, and `cloudflare.api_token` for production:

```yaml
domain: hostedat.example.com
listen: ":443"
jwt_secret: "your-random-secret"    # openssl rand -hex 32
storage_path: ./data/sites
database:
  driver: sqlite
  dsn: ./data/hostedat.db
cloudflare:
  api_token: "your-cloudflare-token"
```

For local development, leave `cloudflare.api_token` empty and use `listen: ":8080"`.

### 2. Run the Server

```bash
go run ./cmd/server
```

Or build and run:

```bash
go build -o hostedat-server ./cmd/server
./hostedat-server
```

The first user to register becomes the **superadmin**.

### 3. Deploy a Site

Install the CLI:

```bash
go install github.com/cryguy/hostedat/cmd/hostedat@latest
```

Then:

```bash
hostedat login
hostedat sites create my-site
hostedat deploy my-site ./dist
```

Your site is live at `my-site.yourdomain.com`.

## CLI Usage

```
hostedat login                     # Authenticate via browser
hostedat sites list                # List your sites
hostedat sites create <name>       # Create a new site
hostedat sites delete <name>       # Delete a site
hostedat deploy <site> <dir>       # Deploy a directory
hostedat version                   # Print version info
```

The CLI auto-detects SPA projects (single `index.html` with scripts, few HTML files) and suggests enabling SPA mode. Override with `--spa`.

## Static Site Features

### `_redirects`

Place a `_redirects` file in the root of your upload:

```
/old-path    /new-path    301
/blog/*      /blog/:splat 200
/*           /index.html  200    # SPA fallback
```

Static files always take precedence. First matching rule wins.

### `_headers`

Custom response headers per path pattern:

```
/*
  X-Frame-Options: DENY
  X-Content-Type-Options: nosniff
/*.js
  Cache-Control: public, max-age=31536000, immutable
```

### `404.html`

Include a `404.html` in your upload and it will be served for requests that don't match any file or rewrite rule.

## Workers

Workers let you run server-side JavaScript on your site. Include a `_worker.js` file in your upload to handle requests dynamically — the API is compatible with Cloudflare Workers.

Workers run in a sandboxed V8 JavaScript engine via [tommie/v8go](https://github.com/tommie/v8go). The server binary requires `CGO_ENABLED=1` (the default on Linux/macOS). V8 prebuilt libraries are available for Linux (amd64, arm64), macOS (amd64, arm64), and Android — Windows is not supported for the server binary. The CLI remains pure Go and works on all platforms including Windows.

### Basic Worker

```js
export default {
  async fetch(request, env, ctx) {
    const url = new URL(request.url);

    if (url.pathname === "/api/hello") {
      return Response.json({ message: "Hello from the edge!" });
    }

    // Fall through to static assets
    return env.ASSETS.fetch(request);
  },
};
```

### Available Web APIs

Workers have access to standard Web APIs:

- `fetch()` — outbound HTTP requests
- `Request`, `Response`, `Headers`, `URL`, `URLSearchParams`
- `URLPattern` — URL pattern matching
- `crypto.getRandomValues()`, `crypto.subtle` (ECDSA, ECDH, X25519, RSA, Ed25519, HKDF, PBKDF2, AES), `crypto.randomUUID()`
- `ReadableStream`, `WritableStream`, `TransformStream`
- `TextEncoderStream`, `TextDecoderStream`
- `CompressionStream`, `DecompressionStream` (gzip, deflate, deflate-raw, brotli)
- `DigestStream` — streaming hash computation
- `FormData`, `Blob`, `File`
- `AbortController`, `AbortSignal`
- `WebSocket` — client WebSocket connections from workers
- `EventSource` — Server-Sent Events (SSE) client
- `HTMLRewriter` — streaming HTML transformation (Cloudflare-compatible API)
- `MessageChannel`, `MessagePort` — structured message passing
- `Cache API` — `caches.default` and `caches.open()` for HTTP response caching
- `D1 Database` — per-site SQLite databases (Cloudflare D1-compatible)
- `Durable Objects` — persistent, transactional key-value storage
- `Queues` — asynchronous message queues
- `Service Bindings` — invoke other workers directly
- `TCP Sockets` — `connect()` for outbound TCP/TLS connections
- `setTimeout`, `setInterval`, `clearTimeout`, `clearInterval`
- `atob`, `btoa`
- `structuredClone`
- `console.log/warn/error` (captured to worker logs)

### Environment Variables & Secrets

Set environment variables and secrets via the API. They're available on the `env` object in your worker, alongside bindings for KV namespaces, D1 databases, Durable Objects, Queues, Service Bindings, and storage buckets:

```js
export default {
  async fetch(request, env) {
    const apiKey = env.MY_SECRET;
    // ...
  },
};
```

### KV Storage

KV namespaces provide per-site key-value storage accessible from workers:

```js
export default {
  async fetch(request, env) {
    // Read
    const value = await env.MY_KV.get("key");

    // Write (with optional TTL in seconds)
    await env.MY_KV.put("key", "value", { expirationTtl: 3600 });

    // Delete
    await env.MY_KV.delete("key");

    // List keys (with optional prefix filter)
    const { keys } = await env.MY_KV.list({ prefix: "user:" });

    return new Response("OK");
  },
};
```

### Cron Triggers

Schedule periodic worker execution with standard 5-field cron expressions:

```js
export default {
  async fetch(request, env) {
    return new Response("OK");
  },

  async scheduled(event, env, ctx) {
    console.log(`Cron fired: ${event.cron} at ${event.scheduledTime}`);
    // Run background tasks, cleanup, etc.
  },
};
```

### Tail Handler

Receive log events from your worker for observability:

```js
export default {
  async fetch(request, env) {
    console.log("handling request");
    return new Response("OK");
  },

  async tail(events) {
    // Process log events (e.g., send to external logging service)
    for (const event of events) {
      console.log(event.logs);
    }
  },
};
```

### Worker Configuration

Configure worker resource limits in `config.yaml`:

```yaml
worker:
  pool_size: 4               # Pre-warmed JS runtimes per site
  memory_limit_mb: 128       # Max memory per runtime
  execution_timeout: 30000   # Max execution time in ms
  max_fetch_requests: 50     # Max outbound fetch() calls per invocation
  fetch_timeout_sec: 10      # Timeout per fetch() call
  max_response_bytes: 10485760  # 10 MB max response body
  max_log_retention: 7       # Days to keep worker logs
  max_script_size_kb: 1024   # Max _worker.js file size
```

## Building

```bash
make all          # Build frontend + server + CLI
make build-all    # Cross-compile for all platforms
make test         # Run tests
make release      # Full release (binaries + checksums + docs)
```

### Prerequisites

- Go 1.25+
- Node.js 18+ (for frontend)
- C/C++ toolchain with `CGO_ENABLED=1` (for server binary — V8 engine requires CGO)
- **Supported server platforms:** Linux (amd64, arm64), macOS (amd64, arm64)
- **CLI works on all platforms** including Windows (pure Go, no CGO)

## Project Structure

```
cmd/server/        Server entry point
cmd/hostedat/      CLI entry point
internal/api/      HTTP handlers and routing
internal/models/   Database models
internal/storage/  File storage and rule processing
internal/auth/     Authentication (JWT, API keys)
internal/config/   Configuration loading
internal/client/   API client (used by CLI)
internal/certs/    TLS certificate management
internal/worker/   Server-side JS engine (V8 via tommie/v8go)
web/               React + Vite frontend
docs/              Documentation site (Astro)
scripts/           Build and release scripts
```

## Documentation

Full documentation is available at [docs.hostedat.ditto.moe](https://docs.hostedat.ditto.moe).

## License

[MIT](LICENSE)
