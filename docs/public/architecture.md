# hostedat Architecture

## Overview

hostedat is a self-hosted static site hosting platform with a JavaScript worker runtime. It consists of a Go backend (Echo v4 + GORM) and a React/Vite frontend that is embedded into the binary via `web.DistFS`.

**Module path:** `github.com/cryguy/hostedat`

---

## Directory Structure

```
cmd/
  hostedat/         CLI client binary (cobra commands: login, sites, deploy, storage, storage-credentials, version)
  server/           Server entry point
internal/
  api/              HTTP handlers (Echo), middleware, route registration
  auth/             JWT generation/validation, password hashing, API key generation
  certs/            TLS certificate management
  config/           YAML config loading with defaults and validation
  models/           GORM models with nanoid PKs
  seaweedfs/        SeaweedFS IAM client (CreateUser, CreateAccessKey, PutUserPolicy, etc.)
  storage/          Local file storage manager (zip extraction, site paths, bytecode paths)
  worker/           V8/v8go JS runtime, bindings, pool management
web/
  src/              React + Vite frontend
  dist/             Build output (embedded into binary)
```

---

## Request Routing

### Site Traffic (subdomain routing)

Incoming HTTP requests are routed based on the `Host` header:

1. `example.com` ↁEAPI server + dashboard (Echo router, bare domain)
2. `storage.example.com` ↁES3 reverse proxy (proxies to SeaweedFS/MinIO)
3. `<slug>.example.com` ↁEStatic site serving or worker execution

For subdomain traffic:
- The subdomain slug is extracted from the host
- The site's `SiteRulesCache` is consulted (TTL 5min, max 1000 entries, bounded LRU)
- If the site `HasWorker` and has an active deployment, the request goes to the worker engine
- Otherwise, static files are served from `{storagePath}/{siteID}/{deployID}/`

Static serving features:
- `_redirects` file: supports `301`, `302` (redirect), and `200` (rewrite/proxy rules) with wildcard patterns and `:splat` captures
- `_headers` file: custom response headers per path pattern
- Custom `404.html` if present
- SPA mode: all 404s fall back to `index.html`

### API Traffic

All API routes are under `/api/v1`. The CLI sends an `X-Hostedat-Version` header; the server rejects versions below `min_cli_version` with `426 Upgrade Required`.

---

## Authentication

### JWT Tokens

- Generated via `auth.GenerateToken(userID, email, role, secret)`
- 24-hour expiry
- Claims include: `user_id`, `email`, `role`
- Validated on every request; user is reloaded from DB to catch role changes and deletions
- Logout: token SHA-256 hash stored in `RevokedToken` table until natural expiry

### API Keys

- Format: `hd_` prefix followed by random bytes
- Stored as SHA-256 hash in `APIKey.KeyHash`
- Identified by `strings.HasPrefix(token, "hd_")`
- `last_used_at` updated on each use

### CLI Auth Flow (PKCE)

1. CLI opens browser to `GET /auth/cli?port=PORT&state=STATE&code_challenge=CHALLENGE&code_challenge_method=S256`
2. User logs in via the HTML form
3. Server validates credentials, creates a one-time `AuthCode` (60s expiry) with the PKCE challenge
4. Browser redirects to `http://localhost:PORT/callback?code=CODE&state=STATE`
5. CLI's local server receives the code, calls `POST /auth/token` with code + verifier
6. Server verifies PKCE challenge, marks code as used, returns JWT
7. CLI exchanges JWT for an API key (stored in `~/.hostedat/config.json`)

---

## Database

**GORM** with driver support for SQLite (default), PostgreSQL, and MySQL.

### ID Generation

All primary keys use **nanoid**: 12 characters from `[0-9a-z]` alphabet.

```go
const nanoidAlphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
const nanoidLength = 12
```

### Models

| Model | Key Fields |
|-------|-----------|
| `User` | `id`, `email`, `password_hash`, `role` (`superadmin`/`admin`/`user`) |
| `Site` | `id`, `user_id`, `subdomain_slug` (unique), `name`, `spa_mode`, `has_worker`, `active_version`, `active_deploy_id` |
| `Deployment` | `id`, `site_id`, `version` (auto-increment per site), `file_hash`, `has_worker` |
| `APIKey` | `id`, `user_id`, `key_hash`, `name` |
| `Invite` | `id`, `code`, `created_by`, `max_uses`, `use_count`, `expires_at`, `active` |
| `AuthCode` | `id`, `code`, `user_id`, `code_challenge`, `used`, `expires_at` |
| `WorkerEnvVar` | `id`, `site_id`, `name`, `value`, `secret` |
| `KVNamespace` | `id`, `site_id`, `name` |
| `KVEntry` | `id`, `namespace_id`, `key`, `value`, `metadata`, `expires_at` |
| `CronSchedule` | `id`, `site_id`, `cron`, `enabled`, `last_run_at` |
| `WorkerLog` | `id`, `site_id`, `level`, `message`, `created_at` |
| `StorageBucket` | `id`, `site_id`, `name` (binding), `bucket_name` (S3), `public` |
| `S3Credential` | `id`, `user_id`, `external_key_id` (IAM username), `access_key_id`, `name` |
| `RevokedToken` | `token_hash`, `expires_at` |
| `Setting` | `key`, `value` (used for `registration_enabled`, `invite_required`) |

### Site Deletion Cascade

Deleting a site transactionally removes: `KVEntry` (via namespace IDs), `Deployment`, `WorkerEnvVar`, `KVNamespace`, `CronSchedule`, `WorkerLog`, `StorageBucket`, then the site itself. After the transaction: local files deleted, S3 buckets removed, IAM policies reconciled.

---

## Authorization Pattern

Every handler that operates on a site does an inline ownership check:

```go
if site.UserID != userID && role != "admin" && role != "superadmin" {
    return errorJSON(c, http.StatusForbidden, "access denied")
}
```

The `RequireSiteOwner(db)` middleware is available but handlers perform inline checks directly. Admin routes use the `RequireAdmin()` middleware which checks for `superadmin` or `admin` role.

---

## Deploy Pipeline

1. **Upload**: Client POSTs a zip as `multipart/form-data` with field `file`
2. **Validate**: File size checked against `storage.MaxZipSize`; hash computed (SHA-256)
3. **Pre-generate ID**: A new nanoid `deployID` is generated before any DB writes
4. **Extract**: Zip extracted to `{storagePath}/{siteID}/{deployID}/`
5. **Worker detection**: If `_worker.js` present:
   - Size checked against `MaxScriptSizeKB` (default 1024 KB)
   - Validated and cached via `Engine.CompileAndCache()`
6. **DB transaction**: `Deployment` record created, `Site.active_version` and `Site.active_deploy_id` updated atomically
7. **Pool invalidation**: Old worker pool for the previous deploy is invalidated

**Rollback** works by updating `Site.active_version` and `Site.active_deploy_id` to point at a previous deployment. The source for that deployment is already on disk.

---

## Worker Runtime

### Runtime Stack

- **V8** JavaScript engine (via `github.com/tommie/v8go`  ECGO bindings to the V8 engine)
- V8 isolate pool with per-worker contexts for sandboxed execution
- Script validation happens at deploy time; execution uses pre-warmed V8 isolates

### Pool Management

Each deployed worker site gets a pool of pre-warmed V8 isolates:

```go
type poolKey struct {
    SiteID    string
    DeployKey string  // = deployID of the active deployment
}
```

- Pools stored in `Engine.pools` (`sync.Map`)
- Pool size configurable (default 4 runtimes per site)
- On request: checkout a runtime from the pool, execute, return
- Pool invalidation: when a new deployment is pushed, the old pool is marked invalid; a fresh pool is created lazily for the new `deployKey`
- On server restart: bytecode is reloaded from disk (`bytecode.bin`), pool recreated

### Execution Flow

1. Request arrives for `<slug>.example.com`
2. Site rules looked up (cache ↁEDB)
3. Worker pool for `{siteID, activeDeployID}` checked out
4. JS `fetch` handler called: `export default { fetch(request, env, ctx) }`
5. `env` object built with: plain vars, secret vars, KV namespaces, storage buckets, `ASSETS` binding
6. Response extracted from JS, converted to Go `WorkerResponse`
7. Runtime returned to pool

### SSRF Protection

`fetch()` in workers blocks requests to private/loopback IP ranges:
- `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`
- `127.0.0.0/8`, `::1/128`
- `169.254.0.0/16` (link-local)
- Any other RFC-1918 / loopback range

Both the initial hostname resolution and redirect destinations are checked. Max 10 redirects.

---

## Object Storage

Object storage is optional and must be enabled in config (`object_storage.enabled: true`).

### Modes

**Managed:** hostedat starts and manages a SeaweedFS instance internally (`weed` binary). Data stored in `object_storage.data_dir`.

**External:** Connects to an existing S3-compatible endpoint (`object_storage.s3_endpoint`). Credentials must be provided.

### S3 Proxy

Traffic to `storage.<domain>` is proxied to the SeaweedFS/MinIO endpoint. The proxy handles SigV4 request signing and forwards to the internal endpoint.

### Bucket Naming

Bucket names must start with the site ID followed by a hyphen (e.g., `abc123def456-images`). This enforces namespace isolation. The full pattern: `^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`, 3-63 chars.

### IAM Credentials

Per-user S3 credentials are managed via the SeaweedFS IAM API:
- IAM username format: `hd-{userID}-{credentialName}`
- On credential creation: IAM user created, access key generated, policy attached
- Policy grants `s3:*` on all buckets owned by the user
- Policy reconciled when buckets are created or deleted
- On user/site deletion: IAM keys and users revoked

### Worker Storage Binding (R2-compatible)

Buckets are exposed to workers as R2-compatible objects on `env.BINDING_NAME`:

```js
// get
const obj = await env.IMAGES.get("photo.jpg");
// put
await env.IMAGES.put("photo.jpg", body);
// delete
await env.IMAGES.delete("photo.jpg");
// head
const meta = await env.IMAGES.head("photo.jpg");
// list
const list = await env.IMAGES.list({ prefix: "photos/", limit: 100 });
// presigned URL
const url = await env.IMAGES.createSignedUrl("photo.jpg", { expiresIn: 3600 });
// public URL (only for public buckets)
const url = env.IMAGES.publicUrl("photo.jpg");
```

---

## Configuration

Config is loaded from a YAML file (path passed as CLI arg). Key settings:

| Key | Default | Description |
|-----|---------|-------------|
| `domain` | required | Base domain (e.g. `example.com`) |
| `listen` | `:8080` | HTTP listen address |
| `storage_path` | `./data/sites` | Local file storage root |
| `jwt_secret` | required (32+ chars) | HMAC secret for JWTs |
| `min_cli_version` |  E| Minimum accepted CLI version (semver) |
| `database.driver` | `sqlite` | `sqlite`, `postgres`, or `mysql` |
| `database.dsn` | required | Database connection string |
| `worker.pool_size` | `4` | Runtimes per site pool |
| `worker.memory_limit_mb` | `128` | Per-runtime memory limit |
| `worker.execution_timeout` | `30000` (ms) | Per-request JS timeout |
| `worker.max_fetch_requests` | `50` | Max `fetch()` calls per request |
| `worker.fetch_timeout_sec` | `10` | Per-fetch HTTP timeout |
| `worker.max_response_bytes` | `10485760` (10 MB) | Max fetch response body size |
| `worker.max_log_retention` | `7` (days) | Worker log retention |
| `worker.max_script_size_kb` | `1024` | Max `_worker.js` size |
| `object_storage.enabled` | `false` | Enable S3-compatible object storage |
| `object_storage.managed` | `false` | Auto-manage SeaweedFS instance |
| `object_storage.s3_endpoint` | `http://127.0.0.1:8333` | S3 API endpoint |
| `object_storage.region` | `us-east-1` | S3 region |

---

## Caching

`SiteRulesCache` stores parsed redirect/header rules per site:
- Bounded: max 1000 entries
- TTL: 5 minutes
- Populated on first request, invalidated on TTL expiry
- Does not need manual invalidation on deploy (TTL handles staleness)

---

## Site Content Features

### `_redirects`

One rule per line: `<from> <to> <status>`

```
/old-path /new-path 301
/api/* https://api.external.com/:splat 200
/* /index.html 200
```

- `301`/`302`: HTTP redirect
- `200`: Internal rewrite (serves the target path without redirecting)
- Wildcards (`*`) and `:splat` captures supported

### `_headers`

```
/static/*
  Cache-Control: public, max-age=31536000
  X-Content-Type-Options: nosniff

/*
  X-Frame-Options: DENY
```

### SPA Mode

When `spa_mode: true`, all 404s serve `index.html` with status 200, enabling client-side routing.

### Custom 404

If `404.html` exists in the site root, it is served for not-found responses.

---

## CLI Client

The CLI binary (`hostedat`) communicates with the server API using API keys. Configuration stored at `~/.hostedat/config.json`.

**Auth resolution order:**
1. `--api-key` flag
2. `HOSTEDAT_API_KEY` environment variable
3. `~/.hostedat/config.json`

**Server resolution order:**
1. `--server` flag
2. `HOSTEDAT_SERVER` environment variable
3. `~/.hostedat/config.json`
4. Compiled-in default (set via `-ldflags`)

**Site resolution (CLI):** The CLI resolves site arguments (name, subdomain slug, or ID) by calling `ResolveSiteID()` which searches across all three identifiers. This is a CLI-only convenience; API endpoints accept only site IDs.

**Commands:** `login`, `sites list/create/delete`, `deploy`, `storage list/create/update/delete/upload`, `storage-credentials list/create/delete`, `version`
