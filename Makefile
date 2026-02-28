BINARY_NAME := hostedat-server
CLI_NAME := hostedat
SERVER_PKG := ./cmd/server
CLI_PKG := ./cmd/hostedat
DIST_DIR := dist

VERSION := $(shell cat VERSION 2>/dev/null | tr -d '[:space:]' || echo dev)
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)

-include build.env
export

# QuickJS (default) is pure Go — no CGO required.
CGO_ENABLED ?= 0
export CGO_ENABLED

LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(GIT_COMMIT) -buildid=
CLI_LDFLAGS := $(LDFLAGS) -X main.defaultServer=$(DEFAULT_SERVER)

.PHONY: all build server server-v8 cli frontend test test-frontend
.PHONY: build-linux build-linux-v8 build-darwin build-darwin-lipo build-darwin-v8
.PHONY: build-windows build-all build-all-v8
.PHONY: docs docs-dev clean

# ── Local development ──────────────────────────────────────────────

all: frontend server cli

frontend: web/node_modules
	cd web && npm run build

# QuickJS server (default, pure Go, no CGO).
server: frontend
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME) $(SERVER_PKG)

# V8 server (requires CGO, Linux/macOS only).
server-v8: frontend
	CGO_ENABLED=1 go build -tags v8 -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME) $(SERVER_PKG)

cli:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(CLI_LDFLAGS)" -o bin/$(CLI_NAME) $(CLI_PKG)

build: server cli

test:
	CGO_ENABLED=1 go test ./...

test-frontend: web/node_modules
	cd web && npx tsc --noEmit

web/node_modules: web/package-lock.json
	cd web && npm ci

# ── Cross-compilation: QuickJS (pure Go, no toolchain needed) ─────

build-linux: frontend
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY_NAME)-linux-amd64 $(SERVER_PKG)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(CLI_LDFLAGS)" -o $(DIST_DIR)/$(CLI_NAME)-linux-amd64 $(CLI_PKG)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY_NAME)-linux-arm64 $(SERVER_PKG)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$(CLI_LDFLAGS)" -o $(DIST_DIR)/$(CLI_NAME)-linux-arm64 $(CLI_PKG)

build-darwin: frontend
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY_NAME)-darwin-amd64 $(SERVER_PKG)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags "$(CLI_LDFLAGS)" -o $(DIST_DIR)/$(CLI_NAME)-darwin-amd64 $(CLI_PKG)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY_NAME)-darwin-arm64 $(SERVER_PKG)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "$(CLI_LDFLAGS)" -o $(DIST_DIR)/$(CLI_NAME)-darwin-arm64 $(CLI_PKG)

build-windows: frontend
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY_NAME)-windows-amd64.exe $(SERVER_PKG)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$(CLI_LDFLAGS)" -o $(DIST_DIR)/$(CLI_NAME)-windows-amd64.exe $(CLI_PKG)
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY_NAME)-windows-arm64.exe $(SERVER_PKG)
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -trimpath -ldflags "$(CLI_LDFLAGS)" -o $(DIST_DIR)/$(CLI_NAME)-windows-arm64.exe $(CLI_PKG)

# All platforms, QuickJS (no toolchain required).
build-all: build-linux build-darwin build-windows

# ── Cross-compilation: V8 (requires CGO + platform toolchains) ────

# Linux V8: requires gcc/g++ (arm64 needs aarch64-linux-gnu-gcc/g++).
build-linux-v8: frontend
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -tags v8 -trimpath -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY_NAME)-v8-linux-amd64 $(SERVER_PKG)
	CGO_ENABLED=1 CC=aarch64-linux-gnu-gcc CXX=aarch64-linux-gnu-g++ GOOS=linux GOARCH=arm64 go build -tags v8 -trimpath -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY_NAME)-v8-linux-arm64 $(SERVER_PKG)

# QuickJS Darwin universal binaries (macOS only, requires lipo). Run after build-darwin.
build-darwin-lipo:
	lipo -create -output $(DIST_DIR)/$(BINARY_NAME)-darwin-universal $(DIST_DIR)/$(BINARY_NAME)-darwin-amd64 $(DIST_DIR)/$(BINARY_NAME)-darwin-arm64
	lipo -create -output $(DIST_DIR)/$(CLI_NAME)-darwin-universal $(DIST_DIR)/$(CLI_NAME)-darwin-amd64 $(DIST_DIR)/$(CLI_NAME)-darwin-arm64

# macOS V8: requires Xcode toolchain. Produces per-arch + universal binaries via lipo.
build-darwin-v8: frontend
	CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 go build -tags v8 -trimpath -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY_NAME)-v8-darwin-amd64 $(SERVER_PKG)
	CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -tags v8 -trimpath -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY_NAME)-v8-darwin-arm64 $(SERVER_PKG)
	lipo -create -output $(DIST_DIR)/$(BINARY_NAME)-v8-darwin-universal $(DIST_DIR)/$(BINARY_NAME)-v8-darwin-amd64 $(DIST_DIR)/$(BINARY_NAME)-v8-darwin-arm64

# All V8 server binaries (Linux + macOS only, no Windows).
build-all-v8: build-linux-v8 build-darwin-v8

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
