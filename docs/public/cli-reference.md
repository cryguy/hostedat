# hostedat CLI Reference

The `hostedat` CLI manages sites, deployments, storage buckets, and S3-compatible credentials. It authenticates with API keys and communicates with a hostedat server over HTTPS.

---

## Installation

Download the binary for your platform from the releases page:

```
https://docs.example.com/downloads/hostedat-{os}-{arch}
```

Where `{os}` is one of `linux`, `darwin`, `windows` and `{arch}` is one of `amd64`, `arm64`, or `universal` (macOS only).

After downloading, make the binary executable (Linux/macOS) and place it on your `PATH`.

---

## Authentication

The CLI uses API keys for every operation. Keys are obtained via `hostedat login` (browser-based PKCE flow) and stored in `~/.hostedat/config.json`. You can also provide a key directly via the `HOSTEDAT_API_KEY` environment variable or the `--api-key` flag.

API keys use the `hd_` prefix (e.g., `hd_abc123...`).

---

## Global Flags

These flags are accepted by every subcommand:

| Flag | Env var | Type | Default | Description |
|------|---------|------|---------|-------------|
| `--server` | `HOSTEDAT_SERVER` | string | compiled default | Server base URL (e.g., `https://hostedat.ditto.moe`) |
| `--api-key` | `HOSTEDAT_API_KEY` | string | — | API key for authentication |

### Resolution Order

**Server URL** (first non-empty value wins):
1. `--server` flag
2. `HOSTEDAT_SERVER` environment variable
3. `server` field in `~/.hostedat/config.json`
4. Compiled-in default (set at build time via `-ldflags`)

**API key** (first non-empty value wins):
1. `HOSTEDAT_API_KEY` environment variable
2. `--api-key` flag
3. `api_key` field in `~/.hostedat/config.json`

Note: the env var takes priority over the flag for API key resolution. This allows CI environments to override any stored key without modifying flags.

---

## Config File

Location: `~/.hostedat/config.json`

Created automatically by `hostedat login`. File permissions are set to `0600` (owner read/write only).

```json
{
  "server": "https://hostedat.ditto.moe",
  "api_key": "hd_..."
}
```

| Field | Type | Description |
|-------|------|-------------|
| `server` | string | Server base URL, used when `--server` and `HOSTEDAT_SERVER` are absent |
| `api_key` | string | API key, used when `HOSTEDAT_API_KEY` and `--api-key` are absent |

---

## Site Resolution

Commands that accept a `<site>` argument resolve it in the following order, stopping at the first match:

1. Exact site ID (nanoid, 12 lowercase alphanumeric characters)
2. Site name (display name)
3. Subdomain slug

This means you can use any of `abc123def456`, `my-blog`, or `my-blog` (if the slug matches) interchangeably in all site commands.

---

## Command Summary

| Command | Description |
|---------|-------------|
| `hostedat login` | Authenticate via browser and save API key |
| `hostedat sites list` | List all your sites |
| `hostedat sites create <name>` | Create a new site |
| `hostedat sites delete <site>` | Delete a site and cascade all resources |
| `hostedat deploy <site> <directory>` | Deploy a directory as a new version |
| `hostedat storage list <site>` | List storage buckets for a site |
| `hostedat storage create <site>` | Create a storage bucket binding |
| `hostedat storage update <site> <bucket-id>` | Toggle bucket public/private |
| `hostedat storage delete <site> <bucket-id>` | Delete a storage bucket |
| `hostedat storage upload <site> <bucket-id> <file>` | Upload a file to a bucket |
| `hostedat storage-credentials list` | List S3-compatible credentials |
| `hostedat storage-credentials create <name>` | Create a credential (secret shown once) |
| `hostedat storage-credentials delete <id>` | Delete a credential |
| `hostedat version` | Print CLI version and default server |

---

## Commands

### hostedat login

Authenticate with the server via a browser-based PKCE OAuth flow. After the browser completes authentication, the server issues an API key that is saved to `~/.hostedat/config.json`.

**Usage:**
```
hostedat login [--server <url>]
```

**Behavior:**
- Opens the system browser to the server's login page.
- Waits for the OAuth callback and exchanges the code for an API key.
- Writes both `server` and `api_key` to `~/.hostedat/config.json`.
- Requires `--server` or `HOSTEDAT_SERVER` if no config file exists yet; the compiled default is used otherwise.

**Example:**
```bash
hostedat login --server https://hostedat.ditto.moe
# Login successful! API key saved.
```

---

### hostedat sites list

List all sites owned by the authenticated user. Alias: `hostedat sites ls`

**Usage:**
```
hostedat sites list
```

**Output columns:**

| Column | Description |
|--------|-------------|
| `ID` | Site ID (nanoid, 12 chars) |
| `NAME` | Display name |
| `SUBDOMAIN` | Subdomain slug |
| `VERSION` | Active version number (`v1`, `v2`, ...) or `-` if never deployed |
| `CREATED` | Creation date (`YYYY-MM-DD`) |

**Example:**
```bash
hostedat sites list

ID            NAME            SUBDOMAIN       VERSION  CREATED
abc123def456  my-portfolio    my-portfolio    v3       2026-01-15
def456ghi789  blog            blog            v1       2026-02-01
```

---

### hostedat sites create

Create a new site.

**Usage:**
```
hostedat sites create <name> [--subdomain <slug>]
```

**Arguments and flags:**

| Name | Required | Type | Description |
|------|----------|------|-------------|
| `<name>` | yes | string | Display name for the site |
| `--subdomain` | no | string | Custom subdomain slug. Defaults to a URL-safe version of the name if omitted. |

**Example:**
```bash
hostedat sites create "My Blog" --subdomain blog

Site created!
  ID:        def456ghi789
  Name:      My Blog
  Subdomain: blog
```

---

### hostedat sites delete

Delete a site and all its associated resources. The deletion cascades to: deployments, worker environment variables, KV namespaces and entries, cron schedules, worker logs, and storage buckets.

Prompts for confirmation unless `--yes` is passed.

**Usage:**
```
hostedat sites delete <site> [--yes]
```

**Arguments and flags:**

| Name | Required | Type | Description |
|------|----------|------|-------------|
| `<site>` | yes | string | Site ID, name, or subdomain slug |
| `--yes` | no | bool | Skip the confirmation prompt |

**Example (interactive):**
```bash
hostedat sites delete my-blog

Delete site my-blog? This cannot be undone. [y/N] y
Site deleted.
```

**Example (non-interactive):**
```bash
hostedat sites delete my-blog --yes

Site deleted.
```

---

### hostedat deploy

Deploy a local directory to a site as a new version. The CLI zips the directory and uploads it via a multipart POST to the server. Each successful deploy increments the version number.

If `--spa` is passed and the site does not already have SPA mode enabled, the CLI enables it after the upload completes.

If `--spa` is not passed and SPA mode is not already enabled, the CLI performs a heuristic check to detect single-page apps:
- Directory contains `index.html` with `<script>` tags
- Fewer than 3 `.html` files total
- No `_redirects` file with a catch-all `/*` rewrite to `200`

If the heuristic fires, the CLI prints a warning suggesting the user re-deploy with `--spa` or add a `_redirects` file.

**Usage:**
```
hostedat deploy <site> <directory> [--spa]
```

**Arguments and flags:**

| Name | Required | Type | Description |
|------|----------|------|-------------|
| `<site>` | yes | string | Site ID, name, or subdomain slug |
| `<directory>` | yes | string | Path to the directory to deploy (e.g., `./dist`) |
| `--spa` | no | bool | Enable SPA mode on the site after a successful deploy |

**Example (standard deploy):**
```bash
hostedat deploy my-portfolio ./dist

Deploying ./dist to site my-portfolio...
Deployed! Version: v4
```

**Example (SPA deploy):**
```bash
hostedat deploy my-app ./dist --spa

Deploying ./dist to site my-app...
Deployed! Version: v2
SPA mode enabled.
```

**Example (SPA warning):**
```bash
hostedat deploy my-app ./dist

Deploying ./dist to site my-app...
Deployed! Version: v3

This looks like a single-page app (SPA).
Client-side routing won't work without SPA mode or a _redirects file.
To enable SPA mode, re-deploy with --spa or toggle it in the dashboard.
```

---

### hostedat storage list

List all storage buckets associated with a site.

**Usage:**
```
hostedat storage list <site>
```

**Arguments:**

| Name | Required | Type | Description |
|------|----------|------|-------------|
| `<site>` | yes | string | Site ID, name, or subdomain slug |

**Output columns:**

| Column | Description |
|--------|-------------|
| `ID` | Bucket record ID (nanoid, 12 chars) |
| `BINDING` | Worker binding name (e.g., `IMAGES`) |
| `BUCKET NAME` | Underlying S3 bucket name |
| `PUBLIC` | `yes` if public read is enabled, `no` otherwise |
| `CREATED` | Creation date (`YYYY-MM-DD`) |

**Example:**
```bash
hostedat storage list my-app

ID            BINDING  BUCKET NAME                PUBLIC  CREATED
xyz789abc123  IMAGES   abc123def456-images        yes     2026-01-20
pqr456stu789  UPLOADS  abc123def456-uploads       no      2026-02-01
```

---

### hostedat storage create

Create a storage bucket and bind it to a site so it is accessible to the site's worker via `env.<BINDING>`.

**Usage:**
```
hostedat storage create <site> --name <BINDING> --bucket <bucket-name> [--public]
```

**Arguments and flags:**

| Name | Required | Type | Description |
|------|----------|------|-------------|
| `<site>` | yes | string | Site ID, name, or subdomain slug |
| `--name` | yes | string | Binding name used in worker code (e.g., `IMAGES`). Must match `^[A-Z][A-Z0-9_]{0,63}$`. |
| `--bucket` | yes | string | S3 bucket name. Must start with the site ID prefix. |
| `--public` | no | bool | Enable unauthenticated public read access (default: `false`) |

**Naming constraints:**
- `--name` (binding): uppercase, starts with a letter, only `A-Z`, `0-9`, `_`, max 64 characters.
- `--bucket`: must be prefixed with the site ID (e.g., `abc123def456-images`).

**Example:**
```bash
hostedat storage create my-app --name IMAGES --bucket abc123def456-images --public

Storage bucket created!
  ID:          xyz789abc123
  Binding:     IMAGES
  Bucket name: abc123def456-images
  Public:      true
```

---

### hostedat storage update

Toggle a bucket between public and private access. Exactly one of `--public` or `--private` must be passed; passing both or neither is an error.

**Usage:**
```
hostedat storage update <site> <bucket-id> --public | --private
```

**Arguments and flags:**

| Name | Required | Type | Description |
|------|----------|------|-------------|
| `<site>` | yes | string | Site ID, name, or subdomain slug |
| `<bucket-id>` | yes | string | Bucket record ID (from `storage list`) |
| `--public` | mutually exclusive | bool | Set bucket to public read |
| `--private` | mutually exclusive | bool | Set bucket to private (authenticated only) |

**Example:**
```bash
# Make public
hostedat storage update my-app xyz789abc123 --public

Storage bucket updated!
  ID:          xyz789abc123
  Binding:     IMAGES
  Bucket name: abc123def456-images
  Public:      true

# Make private
hostedat storage update my-app xyz789abc123 --private

Storage bucket updated!
  ID:          xyz789abc123
  Binding:     IMAGES
  Bucket name: abc123def456-images
  Public:      false
```

---

### hostedat storage delete

Delete a storage bucket and all objects stored in it. Prompts for confirmation unless `--yes` is passed.

**Usage:**
```
hostedat storage delete <site> <bucket-id> [--yes]
```

**Arguments and flags:**

| Name | Required | Type | Description |
|------|----------|------|-------------|
| `<site>` | yes | string | Site ID, name, or subdomain slug |
| `<bucket-id>` | yes | string | Bucket record ID (from `storage list`) |
| `--yes` | no | bool | Skip the confirmation prompt |

**Example:**
```bash
hostedat storage delete my-app xyz789abc123

Delete storage bucket xyz789abc123? This will remove all stored data. [y/N] y
Storage bucket deleted.
```

---

### hostedat storage upload

Upload a local file to a storage bucket. The server issues a presigned S3 URL; the CLI uploads the file bytes directly to that URL. No size limit is imposed by the CLI itself.

If `--key` is omitted, the object key defaults to the base filename of `<file>`.

**Usage:**
```
hostedat storage upload <site> <bucket-id> <file> [--key <object-key>]
```

**Arguments and flags:**

| Name | Required | Type | Description |
|------|----------|------|-------------|
| `<site>` | yes | string | Site ID, name, or subdomain slug |
| `<bucket-id>` | yes | string | Bucket record ID (from `storage list`) |
| `<file>` | yes | string | Local file path to upload |
| `--key` | no | string | Object key (path within the bucket). Defaults to the filename portion of `<file>`. |

**Example:**
```bash
# Upload using filename as key
hostedat storage upload my-app xyz789abc123 ./photo.jpg

Uploaded ./photo.jpg as photo.jpg

# Upload with a custom key
hostedat storage upload my-app xyz789abc123 ./photo.jpg --key images/2026/photo.jpg

Uploaded ./photo.jpg as images/2026/photo.jpg
```

---

### hostedat storage-credentials list

List all S3-compatible credentials owned by the authenticated user.

Command aliases: `hostedat storage-creds list`, `hostedat storage-credentials ls`, `hostedat storage-creds ls`

**Usage:**
```
hostedat storage-credentials list
```

**Output columns:**

| Column | Description |
|--------|-------------|
| `ID` | Credential record ID (nanoid, 12 chars) |
| `NAME` | Human-readable label |
| `ACCESS KEY ID` | S3 access key ID |
| `LAST USED` | Date the credential was last used, or `Never` |
| `CREATED` | Creation date (`YYYY-MM-DD`) |

**Example:**
```bash
hostedat storage-credentials list

ID            NAME        ACCESS KEY ID         LAST USED   CREATED
lmn012opq345  ci-deploy   AKIAIOSFODNN7EXAMPLE  2026-02-15  2026-01-01
rst678uvw901  backups     AKIAI44QH8DHBEXAMPLE  Never       2026-02-10
```

---

### hostedat storage-credentials create

Create a new S3-compatible credential. The secret access key is displayed exactly once immediately after creation and cannot be retrieved again. Save it before the terminal is cleared.

Command alias: `hostedat storage-creds create`

**Usage:**
```
hostedat storage-credentials create <name>
```

**Arguments:**

| Name | Required | Type | Description |
|------|----------|------|-------------|
| `<name>` | yes | string | Human-readable label for the credential |

**Example:**
```bash
hostedat storage-credentials create ci-deploy

Storage credential created!
  Name:              ci-deploy
  Access Key ID:     AKIAIOSFODNN7EXAMPLE
  Secret Access Key: wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY

Save the secret access key now — it will not be shown again.
```

---

### hostedat storage-credentials delete

Delete an S3-compatible credential. Any integration (e.g., CI pipeline, backup tool) using this credential will immediately lose access to the S3 endpoint.

Prompts for confirmation unless `--yes` is passed.

Command alias: `hostedat storage-creds delete`

**Usage:**
```
hostedat storage-credentials delete <id> [--yes]
```

**Arguments and flags:**

| Name | Required | Type | Description |
|------|----------|------|-------------|
| `<id>` | yes | string | Credential record ID (from `storage-credentials list`) |
| `--yes` | no | bool | Skip the confirmation prompt |

**Example:**
```bash
hostedat storage-credentials delete lmn012opq345

Delete storage credential lmn012opq345? Any integrations using it will stop working. [y/N] y
Storage credential deleted.
```

---

### hostedat version

Print the CLI version, git commit hash, and compiled-in default server URL (if set).

**Usage:**
```
hostedat version
```

**Example:**
```bash
hostedat version

hostedat v0.3.1 (a1b2c3d)
server:  https://hostedat.ditto.moe
```

The `server:` line is omitted when no default server was compiled in.

---

## Error Handling

All errors are printed to stderr with the format:
```
Error: <message>
```

The CLI exits with code `1` on any error.

Common error messages:

| Message | Cause |
|---------|-------|
| `not authenticated — run 'hostedat login' or set HOSTEDAT_API_KEY` | No API key found from any source |
| `server URL is required (use --server or HOSTEDAT_SERVER)` | No server URL found and no compiled default |
| `both --name and --bucket are required` | `storage create` called without required flags |
| `exactly one of --public or --private is required` | `storage update` called with both or neither flag |

---

## Confirmation Prompts

The following commands prompt `[y/N]` before proceeding with a destructive action. Only the exact input `y` (case-insensitive) proceeds; any other input (including empty) aborts.

| Command | Prompt text |
|---------|-------------|
| `sites delete` | `Delete site <site>? This cannot be undone. [y/N]` |
| `storage delete` | `Delete storage bucket <id>? This will remove all stored data. [y/N]` |
| `storage-credentials delete` | `Delete storage credential <id>? Any integrations using it will stop working. [y/N]` |

Pass `--yes` to skip the prompt in scripts and CI pipelines.

---

## Environment Variables Reference

| Variable | Description |
|----------|-------------|
| `HOSTEDAT_SERVER` | Server base URL. Overrides config file; overridden by `--server`. |
| `HOSTEDAT_API_KEY` | API key. Highest priority source; overrides both `--api-key` and config file. |
