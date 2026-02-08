# hostedat

Self-hosted static site hosting platform. Upload a zip of your built site and get a live subdomain — like Cloudflare Pages, but on your own server with single-command deploys.

## Features

- **Subdomain routing** — each site gets `<name>.yourdomain.com`, served instantly
- **Single-command deploys** — `hostedat deploy my-site ./dist` from your terminal or CI
- **Netlify-compatible `_redirects`** — redirects, rewrites, and SPA fallback rules
- **Custom `_headers`** — per-path response headers
- **Custom `404.html`** — drop one in your upload and it just works
- **SPA mode** — auto-detected on deploy or toggled per site
- **Automatic HTTPS** — wildcard certs via Let's Encrypt + CertMagic (DNS-01 with Cloudflare)
- **User management** — roles (superadmin, admin, user), invite system, API keys
- **Dashboard** — React frontend for managing sites, deployments, users, and settings
- **CLI client** — deploy from anywhere, integrates with CI/CD
- **Portable** — single binary, SQLite by default, swap to Postgres/MySQL via config

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
hostedat login                     # Authenticate with the server
hostedat sites list                # List your sites
hostedat sites create <name>       # Create a new site
hostedat sites delete <name>       # Delete a site
hostedat deploy <site> <dir>       # Deploy a directory
hostedat version                   # Print version info
```

The CLI auto-detects SPA projects (single `index.html` with scripts, few HTML files) and enables SPA mode automatically. Override with `--spa` or `--no-spa`.

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
web/               React + Vite frontend
docs/              Documentation site (Astro)
```

## Documentation

Full documentation is available at [docs.hostedat.ditto.moe](https://docs.hostedat.ditto.moe).

## License

[MIT](LICENSE)
