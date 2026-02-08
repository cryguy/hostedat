BINARY_NAME := hostedat-server
CLI_NAME := hostedat
SERVER_PKG := ./cmd/server
CLI_PKG := ./cmd/hostedat
DIST_DIR := dist

# Build version from git
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all clean frontend server cli build test
.PHONY: build-linux build-darwin build-windows build-all

# Default: build frontend + server + cli for current platform
all: frontend server cli

# Frontend
frontend:
	cd web && npm ci && npm run build

# Server binary (current platform)
server: frontend
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME) $(SERVER_PKG)

# CLI binary (current platform)
cli:
	go build -ldflags "$(LDFLAGS)" -o bin/$(CLI_NAME) $(CLI_PKG)

# Just the Go builds without rebuilding frontend
build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME) $(SERVER_PKG)
	go build -ldflags "$(LDFLAGS)" -o bin/$(CLI_NAME) $(CLI_PKG)

# Tests
test:
	go test ./...

test-frontend:
	cd web && npm test

# Cross-compilation targets
build-linux: frontend
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY_NAME)-linux-amd64 $(SERVER_PKG)
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(CLI_NAME)-linux-amd64 $(CLI_PKG)
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY_NAME)-linux-arm64 $(SERVER_PKG)
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(CLI_NAME)-linux-arm64 $(CLI_PKG)

build-darwin: frontend
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY_NAME)-darwin-amd64 $(SERVER_PKG)
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(CLI_NAME)-darwin-amd64 $(CLI_PKG)
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY_NAME)-darwin-arm64 $(SERVER_PKG)
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(CLI_NAME)-darwin-arm64 $(CLI_PKG)

build-windows: frontend
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY_NAME)-windows-amd64.exe $(SERVER_PKG)
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(CLI_NAME)-windows-amd64.exe $(CLI_PKG)
	GOOS=windows GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY_NAME)-windows-arm64.exe $(SERVER_PKG)
	GOOS=windows GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(CLI_NAME)-windows-arm64.exe $(CLI_PKG)

# Build everything for all platforms
build-all: build-linux build-darwin build-windows

clean:
	rm -rf bin/ $(DIST_DIR)/ web/dist/ web/node_modules/
