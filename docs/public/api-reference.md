# hostedat API Reference

Base path: `/api/v1`

## Authentication

All protected endpoints require:
```
Authorization: Bearer <token>
```

Where `<token>` is either:
- A JWT obtained from `/auth/login` or `/auth/token` (24h expiry)
- An API key with the `hd_` prefix

Endpoints that require admin access additionally require the authenticated user to have `role: "admin"` or `role: "superadmin"`.

## Error Responses

All errors return JSON:
```json
{ "error": "description of the error" }
```

Common status codes: `400` Bad Request, `401` Unauthorized, `403` Forbidden, `404` Not Found, `409` Conflict, `429` Too Many Requests, `500` Internal Server Error.

---

## Auth

### POST /auth/register

Register a new user. The first user automatically becomes `superadmin`. Subsequent registrations may require an invite code depending on server settings.

**Auth:** None (rate-limited: 5 req/s per IP)

**Request:**
```json
{
  "email": "user@example.com",
  "password": "minimum8chars",
  "invite_code": "optional-hex-code"
}
```

**Response:** `201 Created`
```json
{
  "token": "eyJ...",
  "user": {
    "id": "abc123def456",
    "email": "user@example.com",
    "role": "user",
    "created_at": "2024-01-01T00:00:00Z"
  }
}
```

```bash
curl -X POST https://example.com/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com","password":"mypassword"}'
```

---

### POST /auth/login

**Auth:** None (rate-limited: 5 req/s per IP)

**Request:**
```json
{
  "email": "user@example.com",
  "password": "mypassword"
}
```

**Response:** `200 OK`
```json
{
  "token": "eyJ...",
  "user": {
    "id": "abc123def456",
    "email": "user@example.com",
    "role": "user",
    "created_at": "2024-01-01T00:00:00Z"
  }
}
```

```bash
curl -X POST https://example.com/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com","password":"mypassword"}'
```

---

### POST /auth/logout

Revokes the current JWT. Has no effect on API keys.

**Auth:** JWT or API key

**Response:** `200 OK`
```json
{ "message": "logged out" }
```

```bash
curl -X POST https://example.com/api/v1/auth/logout \
  -H "Authorization: Bearer eyJ..."
```

---

### GET /auth/cli

Serves an HTML login page for the CLI browser-based auth flow (PKCE).

**Auth:** None

**Query params:**
| Param | Type | Required | Description |
|-------|------|----------|-------------|
| `port` | int | yes | Local callback port (1-65535) |
| `state` | string | yes | CSRF state value |
| `code_challenge` | string | yes | PKCE code challenge (SHA-256 of verifier) |
| `code_challenge_method` | string | yes | Must be `S256` |

**Response:** `200 OK` — HTML login form

---

### POST /auth/cli

Validates credentials and returns a redirect URL for the CLI flow.

**Auth:** None

**Request:**
```json
{
  "email": "user@example.com",
  "password": "mypassword",
  "port": "37123",
  "state": "random-state",
  "code_challenge": "base64url-sha256-of-verifier"
}
```

**Response:** `200 OK`
```json
{ "redirect": "http://localhost:37123/callback?code=AUTHCODE&state=random-state" }
```

---

### POST /auth/token

Exchanges a one-time auth code (from the CLI flow) for a JWT.

**Auth:** None

**Request:**
```json
{
  "code": "one-time-auth-code",
  "code_verifier": "pkce-code-verifier"
}
```

**Response:** `200 OK`
```json
{ "token": "eyJ..." }
```

---

## Version

### GET /api/v1/version

Public endpoint returning server and minimum CLI version.

**Auth:** None

**Response:** `200 OK`
```json
{
  "version": "1.2.3",
  "min_cli_version": "1.0.0"
}
```

---

## Sites

### POST /sites

Create a new site. Subdomain slug is auto-generated from name if not provided.

**Auth:** JWT or API key

**Request:**
```json
{
  "name": "My Site",
  "subdomain_slug": "my-site"
}
```

- `name` (string, required): Display name
- `subdomain_slug` (string, optional): 3-63 chars, lowercase alphanumeric and hyphens, must not be reserved

**Response:** `201 Created`
```json
{
  "id": "abc123def456",
  "user_id": "xyz789uvw012",
  "subdomain_slug": "my-site",
  "name": "My Site",
  "spa_mode": false,
  "has_worker": false,
  "active_version": null,
  "active_deploy_id": null,
  "created_at": "2024-01-01T00:00:00Z"
}
```

```bash
curl -X POST https://example.com/api/v1/sites \
  -H "Authorization: Bearer eyJ..." \
  -H "Content-Type: application/json" \
  -d '{"name":"My Site","subdomain_slug":"my-site"}'
```

---

### GET /sites

List all sites for the authenticated user. Admins can pass `?all=true` to list all sites.

**Auth:** JWT or API key

**Query params:** `?all=true` (admin only)

**Response:** `200 OK`
```json
[
  {
    "id": "abc123def456",
    "user_id": "xyz789uvw012",
    "subdomain_slug": "my-site",
    "name": "My Site",
    "spa_mode": false,
    "has_worker": false,
    "active_version": 3,
    "active_deploy_id": "dep123abc456",
    "created_at": "2024-01-01T00:00:00Z"
  }
]
```

```bash
curl https://example.com/api/v1/sites \
  -H "Authorization: Bearer eyJ..."
```

---

### GET /sites/:id

Get a single site by ID.

**Auth:** JWT or API key (must own site or be admin)

**Response:** `200 OK` — Site object (same shape as above)

```bash
curl https://example.com/api/v1/sites/abc123def456 \
  -H "Authorization: Bearer eyJ..."
```

---

### PATCH /sites/:id

Update site settings.

**Auth:** JWT or API key (must own site or be admin)

**Request:**
```json
{
  "name": "New Name",
  "spa_mode": true
}
```

All fields optional. Only provided fields are updated.

**Response:** `200 OK` — Updated site object

```bash
curl -X PATCH https://example.com/api/v1/sites/abc123def456 \
  -H "Authorization: Bearer eyJ..." \
  -H "Content-Type: application/json" \
  -d '{"spa_mode":true}'
```

---

### DELETE /sites/:id

Delete a site and all associated data (deployments, env vars, KV, cron schedules, logs, storage buckets).

**Auth:** JWT or API key (must own site or be admin)

**Response:** `200 OK`
```json
{ "message": "site deleted" }
```

```bash
curl -X DELETE https://example.com/api/v1/sites/abc123def456 \
  -H "Authorization: Bearer eyJ..."
```

---

## Deployments

### POST /sites/:id/deploy

Deploy a zip file to a site. Accepts multipart form data. Rate-limited to 2 req/s per IP.

If `_worker.js` is present in the zip, it is compiled to QuickJS bytecode. If `_worker.js` exceeds the configured size limit, the deploy is rejected.

**Auth:** JWT or API key (must own site or be admin)

**Request:** `multipart/form-data`
- `file` (file, required): Zip archive containing site files

**Response:** `201 Created`
```json
{
  "id": "dep123abc456",
  "site_id": "abc123def456",
  "version": 4,
  "file_hash": "sha256hex...",
  "has_worker": false,
  "uploaded_at": "2024-01-01T00:00:00Z"
}
```

```bash
curl -X POST https://example.com/api/v1/sites/abc123def456/deploy \
  -H "Authorization: Bearer eyJ..." \
  -F "file=@dist.zip"
```

---

### GET /sites/:id/deployments

List deployments for a site (paginated, 20 per page, newest first).

**Auth:** JWT or API key (must own site or be admin)

**Query params:** `?page=N` (default: 1)

**Response:** `200 OK`
```json
{
  "deployments": [
    {
      "id": "dep123abc456",
      "site_id": "abc123def456",
      "version": 4,
      "file_hash": "sha256hex...",
      "has_worker": false,
      "uploaded_at": "2024-01-01T00:00:00Z"
    }
  ],
  "total": 42,
  "page": 1
}
```

```bash
curl "https://example.com/api/v1/sites/abc123def456/deployments?page=2" \
  -H "Authorization: Bearer eyJ..."
```

---

### POST /sites/:id/deployments/:version/rollback

Roll back the site's active deployment to a previous version.

**Auth:** JWT or API key (must own site or be admin)

**Path params:** `:version` — integer version number

**Response:** `200 OK`
```json
{
  "message": "rolled back",
  "active_version": 2
}
```

```bash
curl -X POST https://example.com/api/v1/sites/abc123def456/deployments/2/rollback \
  -H "Authorization: Bearer eyJ..."
```

---

## Worker

### POST /sites/:id/worker/env

Create or update an environment variable (upsert by name).

**Auth:** JWT or API key (must own site or be admin)

**Request:**
```json
{
  "name": "API_KEY",
  "value": "secret-value",
  "secret": true
}
```

- `name` (string, required): Variable name
- `value` (string, required): Variable value
- `secret` (bool, optional): If true, value is masked as `********` in list responses

**Response:** `200 OK`
```json
{
  "id": "env123abc456",
  "site_id": "abc123def456",
  "name": "API_KEY",
  "value": "secret-value",
  "secret": true
}
```

```bash
curl -X POST https://example.com/api/v1/sites/abc123def456/worker/env \
  -H "Authorization: Bearer eyJ..." \
  -H "Content-Type: application/json" \
  -d '{"name":"API_KEY","value":"mykey","secret":true}'
```

---

### GET /sites/:id/worker/env

List all environment variables. Secret values are masked as `********`.

**Auth:** JWT or API key (must own site or be admin)

**Response:** `200 OK`
```json
[
  {
    "id": "env123abc456",
    "site_id": "abc123def456",
    "name": "API_KEY",
    "value": "********",
    "secret": true
  }
]
```

---

### DELETE /sites/:id/worker/env/:varId

Delete an environment variable.

**Auth:** JWT or API key (must own site or be admin)

**Response:** `204 No Content`

```bash
curl -X DELETE https://example.com/api/v1/sites/abc123def456/worker/env/env123abc456 \
  -H "Authorization: Bearer eyJ..."
```

---

### POST /sites/:id/worker/kv

Create a KV namespace.

**Auth:** JWT or API key (must own site or be admin)

**Request:**
```json
{ "name": "MY_KV" }
```

**Response:** `201 Created`
```json
{
  "id": "kv123abc456",
  "site_id": "abc123def456",
  "name": "MY_KV",
  "created_at": "2024-01-01T00:00:00Z"
}
```

```bash
curl -X POST https://example.com/api/v1/sites/abc123def456/worker/kv \
  -H "Authorization: Bearer eyJ..." \
  -H "Content-Type: application/json" \
  -d '{"name":"MY_KV"}'
```

---

### GET /sites/:id/worker/kv

List KV namespaces for a site.

**Auth:** JWT or API key (must own site or be admin)

**Response:** `200 OK` — Array of KVNamespace objects

---

### DELETE /sites/:id/worker/kv/:nsId

Delete a KV namespace and all its entries.

**Auth:** JWT or API key (must own site or be admin)

**Response:** `204 No Content`

---

### POST /sites/:id/worker/crons

Create a cron schedule. Uses standard 5-field cron format.

**Auth:** JWT or API key (must own site or be admin)

**Request:**
```json
{
  "cron": "0 * * * *",
  "enabled": true
}
```

- `cron` (string, required): 5-field cron expression
- `enabled` (bool, optional): Default `true`

**Response:** `201 Created`
```json
{
  "id": "cron123abc456",
  "site_id": "abc123def456",
  "cron": "0 * * * *",
  "enabled": true,
  "last_run_at": null,
  "created_at": "2024-01-01T00:00:00Z"
}
```

---

### GET /sites/:id/worker/crons

List cron schedules for a site.

**Auth:** JWT or API key (must own site or be admin)

**Response:** `200 OK` — Array of CronSchedule objects

---

### DELETE /sites/:id/worker/crons/:cronId

Delete a cron schedule.

**Auth:** JWT or API key (must own site or be admin)

**Response:** `204 No Content`

---

### GET /sites/:id/worker/logs

Get the last 100 worker console logs for a site (newest first).

**Auth:** JWT or API key (must own site or be admin)

**Response:** `200 OK`
```json
[
  {
    "id": "log123abc456",
    "site_id": "abc123def456",
    "level": "info",
    "message": "Request received",
    "created_at": "2024-01-01T00:00:00Z"
  }
]
```

---

## Storage

Storage endpoints are only available when object storage is enabled in the server config. The S3-compatible endpoint is at `https://storage.<domain>`.

### POST /sites/:id/storage/buckets

Create a storage bucket for a site.

**Auth:** JWT or API key (must own site or be admin)

**Request:**
```json
{
  "name": "IMAGES",
  "bucket_name": "siteid-images",
  "public": false
}
```

- `name` (string, required): Binding name used in worker `env`. Must match `^[A-Z][A-Z0-9_]{0,63}$`. Reserved names: `ASSETS`, `__PROTO__`, `PROTOTYPE`, `CONSTRUCTOR`.
- `bucket_name` (string, required): S3 bucket name. Must start with the site ID followed by `-` (e.g. `abc123def456-images`). 3-63 chars, lowercase alphanumeric, dots, hyphens.
- `public` (bool, optional): Allow unauthenticated read access. Default `false`.

**Response:** `201 Created`
```json
{
  "id": "bkt123abc456",
  "site_id": "abc123def456",
  "name": "IMAGES",
  "bucket_name": "abc123def456-images",
  "public": false,
  "created_at": "2024-01-01T00:00:00Z"
}
```

```bash
curl -X POST https://example.com/api/v1/sites/abc123def456/storage/buckets \
  -H "Authorization: Bearer eyJ..." \
  -H "Content-Type: application/json" \
  -d '{"name":"IMAGES","bucket_name":"abc123def456-images","public":false}'
```

---

### GET /sites/:id/storage/buckets

List storage buckets for a site.

**Auth:** JWT or API key (must own site or be admin)

**Response:** `200 OK` — Array of StorageBucket objects

---

### PATCH /sites/:id/storage/buckets/:bucketId

Update a storage bucket's settings.

**Auth:** JWT or API key (must own site or be admin)

**Request:**
```json
{ "public": true }
```

**Response:** `200 OK` — Updated StorageBucket object

---

### DELETE /sites/:id/storage/buckets/:bucketId

Delete a storage bucket and all its objects.

**Auth:** JWT or API key (must own site or be admin)

**Response:** `204 No Content`

---

### POST /sites/:id/storage/buckets/:bucketId/upload-url

Generate a presigned PUT URL for direct browser uploads.

**Auth:** JWT or API key (must own site or be admin)

**Request:**
```json
{
  "key": "path/to/object.jpg",
  "expires_in": 3600
}
```

- `key` (string, required): Object key/path
- `expires_in` (int, optional): Seconds until URL expires. Default 3600, max 604800 (7 days).

**Response:** `200 OK`
```json
{
  "upload_url": "https://storage.example.com/bucket-name/path/to/object.jpg?X-Amz-...",
  "key": "path/to/object.jpg",
  "bucket": "abc123def456-images",
  "expires_in": 3600
}
```

```bash
# Generate presigned URL
RESULT=$(curl -X POST https://example.com/api/v1/sites/abc123def456/storage/buckets/bkt123/upload-url \
  -H "Authorization: Bearer eyJ..." \
  -H "Content-Type: application/json" \
  -d '{"key":"photo.jpg","expires_in":3600}')

# Use presigned URL to upload
curl -X PUT "$(echo $RESULT | jq -r .upload_url)" \
  -H "Content-Type: image/jpeg" \
  --data-binary @photo.jpg
```

---

## S3 Credentials

Per-user S3 credentials for programmatic access to storage buckets. The secret access key is only shown once at creation time.

### POST /s3-credentials

Create an S3 credential. The `secret_access_key` is returned only in this response.

**Auth:** JWT or API key

**Request:**
```json
{ "name": "my-credential" }
```

- `name`: 1-32 chars, letters, digits, underscores, hyphens.

**Response:** `201 Created`
```json
{
  "id": "crd123abc456",
  "access_key_id": "AKIAIOSFODNN7EXAMPLE",
  "secret_access_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
  "name": "my-credential",
  "created_at": "2024-01-01T00:00:00Z"
}
```

```bash
curl -X POST https://example.com/api/v1/s3-credentials \
  -H "Authorization: Bearer eyJ..." \
  -H "Content-Type: application/json" \
  -d '{"name":"my-credential"}'
```

---

### GET /s3-credentials

List S3 credentials (no secrets returned).

**Auth:** JWT or API key

**Response:** `200 OK`
```json
[
  {
    "id": "crd123abc456",
    "user_id": "xyz789uvw012",
    "access_key_id": "AKIAIOSFODNN7EXAMPLE",
    "external_key_id": "hd-xyz789uvw012-my-credential",
    "name": "my-credential",
    "last_used_at": null,
    "created_at": "2024-01-01T00:00:00Z"
  }
]
```

---

### DELETE /s3-credentials/:id

Delete an S3 credential and revoke its IAM access key.

**Auth:** JWT or API key (must own credential)

**Response:** `204 No Content`

---

## API Keys

### POST /keys

Create a new API key. The raw key is only shown once.

**Auth:** JWT or API key

**Request:**
```json
{ "name": "ci-deploy" }
```

**Response:** `201 Created`
```json
{
  "id": "key123abc456",
  "name": "ci-deploy",
  "key": "hd_abc123...",
  "created_at": "2024-01-01T00:00:00Z"
}
```

```bash
curl -X POST https://example.com/api/v1/keys \
  -H "Authorization: Bearer eyJ..." \
  -H "Content-Type: application/json" \
  -d '{"name":"ci-deploy"}'
```

---

### GET /keys

List API keys for the authenticated user. Raw key values are never returned.

**Auth:** JWT or API key

**Response:** `200 OK`
```json
[
  {
    "id": "key123abc456",
    "user_id": "xyz789uvw012",
    "name": "ci-deploy",
    "last_used_at": "2024-01-15T10:30:00Z",
    "created_at": "2024-01-01T00:00:00Z"
  }
]
```

---

### DELETE /keys/:id

Delete an API key.

**Auth:** JWT or API key (must own key)

**Response:** `200 OK`
```json
{ "message": "API key deleted" }
```

---

## Admin

All admin endpoints require `role: "admin"` or `role: "superadmin"`.

### GET /admin/users

List all users (paginated, 20 per page).

**Auth:** JWT or API key (admin)

**Query params:** `?page=N` (default: 1)

**Response:** `200 OK`
```json
{
  "users": [
    {
      "id": "abc123def456",
      "email": "user@example.com",
      "role": "user",
      "invited_by": null,
      "created_at": "2024-01-01T00:00:00Z"
    }
  ],
  "total": 42,
  "page": 1
}
```

```bash
curl "https://example.com/api/v1/admin/users?page=1" \
  -H "Authorization: Bearer eyJ..."
```

---

### PATCH /admin/users/:id/role

Change a user's role. Cannot change `superadmin` role.

**Auth:** JWT or API key (admin)

**Request:**
```json
{ "role": "admin" }
```

Valid roles: `"admin"`, `"user"`

**Response:** `200 OK` — Updated User object

---

### DELETE /admin/users/:id

Delete a user and all their data. Cannot delete `superadmin`.

**Auth:** JWT or API key (admin)

**Response:** `200 OK`
```json
{ "message": "user deleted" }
```

---

### GET /admin/settings

Get instance-level registration settings.

**Auth:** JWT or API key (admin)

**Response:** `200 OK`
```json
{
  "registration_enabled": true,
  "invite_required": false
}
```

---

### PATCH /admin/settings

Update registration settings.

**Auth:** JWT or API key (admin)

**Request:**
```json
{
  "registration_enabled": true,
  "invite_required": true
}
```

All fields optional. Returns the same shape as GET /admin/settings.

---

### POST /admin/invites

Create an invite code.

**Auth:** JWT or API key (admin)

**Request:**
```json
{
  "max_uses": 10,
  "expires_at": "2024-12-31T23:59:59Z"
}
```

All fields optional. If omitted, the invite has unlimited uses and never expires.

**Response:** `201 Created`
```json
{
  "id": "inv123abc456",
  "code": "3f7a9c2d1b4e8f6a...",
  "created_by": "abc123def456",
  "max_uses": 10,
  "use_count": 0,
  "expires_at": "2024-12-31T23:59:59Z",
  "active": true,
  "created_at": "2024-01-01T00:00:00Z"
}
```

---

### GET /admin/invites

List all invite codes.

**Auth:** JWT or API key (admin)

**Response:** `200 OK` — Array of Invite objects

---

### DELETE /admin/invites/:id

Revoke an invite (sets `active: false`).

**Auth:** JWT or API key (admin)

**Response:** `200 OK`
```json
{ "message": "invite revoked" }
```
