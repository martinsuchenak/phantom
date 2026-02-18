.PHONY: all build build-linux build-darwin build-all clean install uninstall deps test lint fmt help dist release

# Binary name
BINARY_NAME=phantom

# Version info
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT?=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=gofmt

# Build flags
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildTime=$(BUILD_TIME)"

# Output directory for cross-platform builds
DIST_DIR=dist

# Supported platforms
PLATFORMS=linux/amd64 linux/arm64 darwin/amd64 darwin/arm64

# Main targets
all: deps build

build: ## Build the binary for current platform
	$(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME) ./cmd

# Cross-platform builds
build-linux: ## Build for Linux (amd64, arm64)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-linux-arm64 ./cmd

build-darwin: ## Build for macOS (amd64, arm64)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-darwin-amd64 ./cmd
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-darwin-arm64 ./cmd

build-all: ## Build for all platforms
	@mkdir -p $(DIST_DIR)
	@echo "Building for all platforms..."
	@echo "  -> linux/amd64"
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd
	@echo "  -> linux/arm64"
	@CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-linux-arm64 ./cmd
	@echo "  -> darwin/amd64"
	@CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-darwin-amd64 ./cmd
	@echo "  -> darwin/arm64"
	@CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-darwin-arm64 ./cmd
	@echo "Done!"
	@ls -lh $(DIST_DIR)/

# Build for a specific platform (usage: make build-platform GOOS=linux GOARCH=arm64)
build-platform: ## Build for a specific platform (set GOOS and GOARCH)
	@mkdir -p $(DIST_DIR)
	@echo "Building for $(GOOS)/$(GOARCH)..."
	@CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) $(GOBUILD) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-$(GOOS)-$(GOARCH) ./cmd

dist: clean build-all ## Create distribution with checksums
	@echo "Generating checksums..."
	@cd $(DIST_DIR) && sha256sum * > checksums.txt
	@echo "Distribution created in $(DIST_DIR)/"
	@cat $(DIST_DIR)/checksums.txt

clean: ## Clean build artifacts
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -rf $(DIST_DIR)

install: build ## Install the binary to /usr/local/bin
	install -m 755 $(BINARY_NAME) /usr/local/bin/

uninstall: ## Uninstall the binary from /usr/local/bin
	rm -f /usr/local/bin/$(BINARY_NAME)

deps: ## Download and tidy dependencies
	$(GOMOD) download
	$(GOMOD) tidy

test: ## Run tests
	$(GOTEST) -v ./...

coverage: ## Run tests with coverage
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

lint: ## Run golangci-lint
	@which golangci-lint > /dev/null || (echo "golangci-lint not found, please install it: https://golangci-lint.run/usage/install/" && exit 1)
	golangci-lint run ./...

fmt: ## Format code
	$(GOFMT) -s -w .

vet: ## Run go vet
	$(GOCMD) vet ./...

# Development targets
dev: deps build ## Development build
	./$(BINARY_NAME) --help

# Release targets
release: dist ## Create a release (build-all + checksums)
	@echo "Release $(VERSION) ready in $(DIST_DIR)/"
	@ls -lh $(DIST_DIR)/

# Quick aliases
linux: build-linux
darwin: build-darwin
all-platforms: build-all

help: ## Show this help
	@echo "Overlay FS CLI Tool - Build System"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'
	@echo ""
	@echo "Examples:"
	@echo "  make build              # Build for current platform"
	@echo "  make build-all          # Build for all platforms"
	@echo "  make dist               # Build all + checksums"
	@echo "  make release            # Create a release"
	@echo ""
	@echo "  make build-platform GOOS=linux GOARCH=arm64  # Build for specific platform"
