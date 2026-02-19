BINARY_NAME := hostedat-server
CLI_NAME := hostedat
SERVER_PKG := ./cmd/server
CLI_PKG := ./cmd/hostedat
DIST_DIR := dist

# Build version from git
VERSION := $(shell cat VERSION 2>/dev/null | tr -d '[:space:]' || echo dev)
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)

# Read build-time config from build.env
-include build.env
export

# Reproducible build flags
GOFLAGS := -trimpath
CGO_ENABLED ?= 1
export CGO_ENABLED

# Linker flags — inject version and build config into binaries
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(GIT_COMMIT) -buildid=
CLI_LDFLAGS := $(LDFLAGS) -X main.defaultServer=$(DEFAULT_SERVER)

.PHONY: all clean frontend server cli build test
.PHONY: build-linux build-darwin build-cli-all build-all
.PHONY: docs docs-dev release deploy-docs full-release

# Default: build frontend + server + cli for current platform
all: frontend server cli

# Frontend (auto-installs deps if node_modules is missing or stale)
web/node_modules: web/package-lock.json
	cd web && npm ci

frontend: web/node_modules
	cd web && npm run build

# Server binary (current platform)
server: frontend
	CGO_ENABLED=$(CGO_ENABLED) go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME) $(SERVER_PKG)

# CLI binary (current platform)
cli:
	CGO_ENABLED=$(CGO_ENABLED) go build -trimpath -ldflags "$(CLI_LDFLAGS)" -o bin/$(CLI_NAME) $(CLI_PKG)

# Just the Go builds without rebuilding frontend
build:
	CGO_ENABLED=$(CGO_ENABLED) go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME) $(SERVER_PKG)
	CGO_ENABLED=$(CGO_ENABLED) go build -trimpath -ldflags "$(CLI_LDFLAGS)" -o bin/$(CLI_NAME) $(CLI_PKG)

# Tests (CGO required for v8go worker tests)
test:
	CGO_ENABLED=1 go test ./...

test-frontend: web/node_modules
	cd web && npx tsc --noEmit

# Cross-compilation targets (server requires CGO_ENABLED=1 for v8go, CLI does not)
build-linux: frontend
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY_NAME)-linux-amd64 $(SERVER_PKG)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(CLI_LDFLAGS)" -o $(DIST_DIR)/$(CLI_NAME)-linux-amd64 $(CLI_PKG)
	CGO_ENABLED=1 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY_NAME)-linux-arm64 $(SERVER_PKG)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$(CLI_LDFLAGS)" -o $(DIST_DIR)/$(CLI_NAME)-linux-arm64 $(CLI_PKG)

build-darwin: frontend
	CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY_NAME)-darwin-amd64 $(SERVER_PKG)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags "$(CLI_LDFLAGS)" -o $(DIST_DIR)/$(CLI_NAME)-darwin-amd64 $(CLI_PKG)
	CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY_NAME)-darwin-arm64 $(SERVER_PKG)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "$(CLI_LDFLAGS)" -o $(DIST_DIR)/$(CLI_NAME)-darwin-arm64 $(CLI_PKG)

# CLI-only targets (no v8go dependency, pure Go)
build-cli-all: frontend
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(CLI_LDFLAGS)" -o $(DIST_DIR)/$(CLI_NAME)-linux-amd64 $(CLI_PKG)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$(CLI_LDFLAGS)" -o $(DIST_DIR)/$(CLI_NAME)-linux-arm64 $(CLI_PKG)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags "$(CLI_LDFLAGS)" -o $(DIST_DIR)/$(CLI_NAME)-darwin-amd64 $(CLI_PKG)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "$(CLI_LDFLAGS)" -o $(DIST_DIR)/$(CLI_NAME)-darwin-arm64 $(CLI_PKG)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$(CLI_LDFLAGS)" -o $(DIST_DIR)/$(CLI_NAME)-windows-amd64.exe $(CLI_PKG)
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -trimpath -ldflags "$(CLI_LDFLAGS)" -o $(DIST_DIR)/$(CLI_NAME)-windows-arm64.exe $(CLI_PKG)

# Build server for all supported platforms (linux + darwin, requires CGO)
# and CLI for all platforms (pure Go, includes Windows)
build-all: build-linux build-darwin build-cli-all

# Documentation site (auto-installs deps if node_modules is missing or stale)
docs/node_modules: docs/package-lock.json
	cd docs && npm ci

docs: docs/node_modules
	cd docs && npm run build

docs-dev: docs/node_modules
	cd docs && npm run dev

# Full release: cross-compile + checksums + docs
release:
	bash scripts/build-release.sh

# Upload binaries to S3 bucket via hostedat CLI (reads credentials from deploy.env)
upload-downloads:
	set -a && . ./deploy.env && set +a && bash scripts/build-release.sh --skip-build --skip-docs

# Deploy docs site to hostedat (reads credentials from deploy.env)
deploy-docs:
	set -a && . ./deploy.env && set +a && bin/$(CLI_NAME) deploy $(DOCS_SITE) docs/dist

full-release: release deploy-docs

clean:
	rm -rf bin/ $(DIST_DIR)/ web/dist/ web/node_modules/ docs/dist/ docs/node_modules/
