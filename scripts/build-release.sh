#!/usr/bin/env bash
set -euo pipefail

# Build release binaries, generate checksums, and prepare the docs site.
#
# Binaries are uploaded to the docs site's S3 storage bucket (DOWNLOADS)
# instead of being bundled into the deployment, keeping the deploy under
# the 25 MB zip limit.
#
# Usage:
#   bash scripts/build-release.sh                      # build all + docs
#   bash scripts/build-release.sh --skip-build         # skip Go builds, just regenerate manifest
#   bash scripts/build-release.sh --skip-upload        # skip bucket upload
#   bash scripts/build-release.sh --skip-docs          # skip docs site build
#
# Required environment (via deploy.env or exported):
#   HOSTEDAT_SERVER, HOSTEDAT_API_KEY          - for deploy + bucket upload
#   DOCS_SITE                                   - site name/ID for the docs site
#   DOWNLOADS_BUCKET                            - bucket ID for binaries

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
DIST_DIR="$ROOT_DIR/dist"
MANIFEST="$ROOT_DIR/docs/public/downloads.json"

# Get version from git
VERSION=$(cat "$ROOT_DIR/VERSION" 2>/dev/null | tr -d '[:space:]' || echo "dev")
# Use git commit date for reproducibility (falls back to current date)
DATE=$(git -C "$ROOT_DIR" log -1 --format=%cd --date=short 2>/dev/null || date -u +%Y-%m-%d)

SKIP_BUILD=false
SKIP_UPLOAD=false
SKIP_DOCS=false
for arg in "$@"; do
  case "$arg" in
    --skip-build) SKIP_BUILD=true ;;
    --skip-upload) SKIP_UPLOAD=true ;;
    --skip-docs) SKIP_DOCS=true ;;
  esac
done

# Step 1: Cross-compile if not skipping
if [ "$SKIP_BUILD" = false ]; then
  echo "==> Building all platforms..."
  cd "$ROOT_DIR"
  make build-all
  echo ""
fi

# Step 2: Ensure dist directory has binaries
if [ ! -d "$DIST_DIR" ] || [ -z "$(ls -A "$DIST_DIR" 2>/dev/null)" ]; then
  echo "Error: No binaries found in $DIST_DIR"
  echo "Run 'make build-all' first or remove --skip-build"
  exit 1
fi

# Step 3: Generate checksums and manifest (from dist/ directly)
echo "==> Generating checksums and manifest..."

# Start JSON
echo '{' > "$MANIFEST"
echo "  \"version\": \"$VERSION\"," >> "$MANIFEST"
echo "  \"date\": \"$DATE\"," >> "$MANIFEST"
echo '  "files": [' >> "$MANIFEST"

first=true
for binary in "$DIST_DIR"/*; do
  filename=$(basename "$binary")

  # Determine OS
  os=""
  case "$filename" in
    *-linux-*)  os="linux" ;;
    *-darwin-*) os="darwin" ;;
    *-windows-*) os="windows" ;;
  esac

  # Determine arch
  arch=""
  case "$filename" in
    *-amd64*) arch="amd64" ;;
    *-arm64*) arch="arm64" ;;
  esac

  # Determine type (cli vs server)
  type=""
  case "$filename" in
    hostedat-server*) type="server" ;;
    hostedat-*)       type="cli" ;;
  esac

  # Skip if we couldn't parse
  if [ -z "$os" ] || [ -z "$arch" ] || [ -z "$type" ]; then
    echo "  Warning: Skipping unrecognized binary: $filename"
    continue
  fi

  # Get file size
  size=$(wc -c < "$binary" | tr -d ' ')

  # Get SHA-256
  if command -v sha256sum &>/dev/null; then
    sha256=$(sha256sum "$binary" | awk '{print $1}')
  elif command -v shasum &>/dev/null; then
    sha256=$(shasum -a 256 "$binary" | awk '{print $1}')
  elif command -v powershell &>/dev/null; then
    # PowerShell fallback (Git Bash on Windows)
    sha256=$(powershell -Command "(Get-FileHash -Path '$binary' -Algorithm SHA256).Hash.ToLower()" 2>/dev/null)
    if [ -z "$sha256" ]; then
      echo "Error: Failed to compute SHA-256 for $filename"
      exit 1
    fi
  else
    echo "Error: No SHA-256 tool found (sha256sum, shasum, or powershell required)"
    exit 1
  fi

  # Write JSON entry
  if [ "$first" = true ]; then
    first=false
  else
    echo '    ,' >> "$MANIFEST"
  fi

  cat >> "$MANIFEST" << ENTRY
    {
      "name": "$filename",
      "os": "$os",
      "arch": "$arch",
      "type": "$type",
      "size": $size,
      "sha256": "$sha256"
    }
ENTRY

  echo "  $filename ($type, $os/$arch, $(numfmt --to=iec-i --suffix=B "$size" 2>/dev/null || echo "${size} bytes"))"
done

echo '  ]' >> "$MANIFEST"
echo '}' >> "$MANIFEST"

echo ""
echo "==> Manifest written to $MANIFEST"
echo "==> Version: $VERSION"
echo "==> Date: $DATE"
echo ""

# Step 4: Upload binaries to S3 bucket via hostedat CLI
if [ "$SKIP_UPLOAD" = false ]; then
  echo "==> Uploading binaries to S3 bucket via hostedat CLI..."

  # Load deploy env if available
  if [ -f "$ROOT_DIR/deploy.env" ]; then
    set -a
    . "$ROOT_DIR/deploy.env"
    set +a
  fi

  : "${HOSTEDAT_SERVER:?HOSTEDAT_SERVER is required}"
  : "${HOSTEDAT_API_KEY:?HOSTEDAT_API_KEY is required}"
  : "${DOCS_SITE:?DOCS_SITE is required}"
  : "${DOWNLOADS_BUCKET:?DOWNLOADS_BUCKET is required}"

  HOSTEDAT="$ROOT_DIR/bin/hostedat"
  if [ ! -f "$HOSTEDAT" ]; then
    echo "Error: hostedat CLI not found at $HOSTEDAT"
    echo "Run 'make cli' first."
    exit 1
  fi

  for binary in "$DIST_DIR"/*; do
    filename=$(basename "$binary")
    echo "  Uploading $filename..."
    "$HOSTEDAT" storage upload "$DOCS_SITE" "$DOWNLOADS_BUCKET" "$binary" --key "$filename"
  done

  echo "==> All binaries uploaded to bucket: $DOWNLOADS_BUCKET"
  echo ""
fi

# Step 5: Build the docs site
if [ "$SKIP_DOCS" = false ]; then
  echo "==> Building docs site..."
  cd "$ROOT_DIR/docs"
  npm run build

  echo ""
  echo "==> Release build complete!"
  echo "    Docs site ready at: docs/dist/"
  echo "    Deploy with: make deploy-docs"
else
  echo ""
  echo "==> Upload complete! (docs build skipped)"
fi
