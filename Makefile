BINARY_NAME := hostedat-server
CLI_NAME := hostedat
SERVER_PKG := ./cmd/server
CLI_PKG := ./cmd/hostedat
DIST_DIR := dist

VERSION := $(shell cat VERSION 2>/dev/null | tr -d '[:space:]' || echo dev)
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)

-include build.env
export

CGO_ENABLED ?= 1
export CGO_ENABLED

LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(GIT_COMMIT) -buildid=
CLI_LDFLAGS := $(LDFLAGS) -X main.defaultServer=$(DEFAULT_SERVER)

.PHONY: all build server cli frontend test test-frontend
.PHONY: build-linux build-darwin build-cli-windows
.PHONY: docs docs-dev clean

# ── Local development ──────────────────────────────────────────────

all: frontend server cli

frontend: web/node_modules
	cd web && npm run build

server: frontend
	CGO_ENABLED=$(CGO_ENABLED) go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME) $(SERVER_PKG)

cli:
	CGO_ENABLED=$(CGO_ENABLED) go build -trimpath -ldflags "$(CLI_LDFLAGS)" -o bin/$(CLI_NAME) $(CLI_PKG)

build:
	CGO_ENABLED=$(CGO_ENABLED) go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME) $(SERVER_PKG)
	CGO_ENABLED=$(CGO_ENABLED) go build -trimpath -ldflags "$(CLI_LDFLAGS)" -o bin/$(CLI_NAME) $(CLI_PKG)

test:
	CGO_ENABLED=1 go test ./...

test-frontend: web/node_modules
	cd web && npx tsc --noEmit

web/node_modules: web/package-lock.json
	cd web && npm ci

# ── Cross-compilation (used by CI, see .github/workflows/release.yml) ──

# Linux ARM64 requires: apt install gcc-aarch64-linux-gnu g++-aarch64-linux-gnu
build-linux: frontend
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY_NAME)-linux-amd64 $(SERVER_PKG)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(CLI_LDFLAGS)" -o $(DIST_DIR)/$(CLI_NAME)-linux-amd64 $(CLI_PKG)
	CGO_ENABLED=1 CC=aarch64-linux-gnu-gcc CXX=aarch64-linux-gnu-g++ GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY_NAME)-linux-arm64 $(SERVER_PKG)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$(CLI_LDFLAGS)" -o $(DIST_DIR)/$(CLI_NAME)-linux-arm64 $(CLI_PKG)

# Requires macOS with Xcode toolchain. Produces universal (amd64+arm64) binaries via lipo.
build-darwin: frontend
	CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/.server-amd64 $(SERVER_PKG)
	CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/.server-arm64 $(SERVER_PKG)
	lipo -create -output $(DIST_DIR)/$(BINARY_NAME)-darwin-universal $(DIST_DIR)/.server-amd64 $(DIST_DIR)/.server-arm64
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags "$(CLI_LDFLAGS)" -o $(DIST_DIR)/.cli-amd64 $(CLI_PKG)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "$(CLI_LDFLAGS)" -o $(DIST_DIR)/.cli-arm64 $(CLI_PKG)
	lipo -create -output $(DIST_DIR)/$(CLI_NAME)-darwin-universal $(DIST_DIR)/.cli-amd64 $(DIST_DIR)/.cli-arm64
	rm -f $(DIST_DIR)/.server-* $(DIST_DIR)/.cli-*

build-cli-windows:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$(CLI_LDFLAGS)" -o $(DIST_DIR)/$(CLI_NAME)-windows-amd64.exe $(CLI_PKG)
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -trimpath -ldflags "$(CLI_LDFLAGS)" -o $(DIST_DIR)/$(CLI_NAME)-windows-arm64.exe $(CLI_PKG)

# ── Docs ───────────────────────────────────────────────────────────

docs: docs/node_modules
	cd docs && npm run build

docs-dev: docs/node_modules
	cd docs && npm run dev

docs/node_modules: docs/package-lock.json
	cd docs && npm ci

# ── Cleanup ────────────────────────────────────────────────────────

clean:
	rm -rf bin/ $(DIST_DIR)/ web/dist/ web/node_modules/ docs/dist/ docs/node_modules/
