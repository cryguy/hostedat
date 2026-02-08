#!/usr/bin/env bash
set -euo pipefail

# Build release binaries, generate checksums, and prepare the docs site.
#
# Usage:
#   bash scripts/build-release.sh          # build all + docs
#   bash scripts/build-release.sh --skip-build  # skip Go builds, just regenerate manifest

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
DIST_DIR="$ROOT_DIR/dist"
DOCS_DL_DIR="$ROOT_DIR/docs/public/downloads"
MANIFEST="$ROOT_DIR/docs/public/downloads.json"

# Get version from git
VERSION=$(cd "$ROOT_DIR" && git describe --tags --always --dirty 2>/dev/null || echo "dev")
DATE=$(date -u +%Y-%m-%d)

SKIP_BUILD=false
for arg in "$@"; do
  case "$arg" in
    --skip-build) SKIP_BUILD=true ;;
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

# Step 3: Copy binaries to docs/public/downloads/
echo "==> Copying binaries to docs site..."
mkdir -p "$DOCS_DL_DIR"
rm -f "$DOCS_DL_DIR"/*

for binary in "$DIST_DIR"/*; do
  cp "$binary" "$DOCS_DL_DIR/"
done

# Step 4: Generate checksums and manifest
echo "==> Generating checksums and manifest..."

# Start JSON
echo '{' > "$MANIFEST"
echo "  \"version\": \"$VERSION\"," >> "$MANIFEST"
echo "  \"date\": \"$DATE\"," >> "$MANIFEST"
echo '  "files": [' >> "$MANIFEST"

first=true
for binary in "$DOCS_DL_DIR"/*; do
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
  else
    # PowerShell fallback (Git Bash on Windows)
    sha256=$(powershell -Command "(Get-FileHash -Path '$binary' -Algorithm SHA256).Hash.ToLower()" 2>/dev/null || echo "unknown")
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

# Step 5: Build the docs site
echo "==> Building docs site..."
cd "$ROOT_DIR/docs"
npm run build

echo ""
echo "==> Release build complete!"
echo "    Docs site ready at: docs/dist/"
echo "    Deploy with: make deploy-docs"
