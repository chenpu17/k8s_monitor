.PHONY: build clean test run deps help install lint

# Build variables
BINARY_NAME=k8s-monitor
VERSION?=v0.1.0-dev
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"

# Go commands
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Build directory
BUILD_DIR=./bin

help: ## Display this help message
	@echo "k8s-monitor Makefile commands:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

deps: ## Download dependencies
	@echo "📦 Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy
	@echo "✅ Dependencies downloaded"

build: deps ## Build the binary
	@echo "🔨 Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/k8s-monitor
	@echo "✅ Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

build-all: ## Build for multiple platforms
	@echo "🔨 Building for multiple platforms..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/k8s-monitor
	GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 ./cmd/k8s-monitor
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 ./cmd/k8s-monitor
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./cmd/k8s-monitor
	@echo "✅ Multi-platform build complete"

clean: ## Remove build artifacts
	@echo "🧹 Cleaning..."
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)
	@echo "✅ Clean complete"

test: ## Run tests
	@echo "🧪 Running tests..."
	$(GOTEST) -v -race -coverprofile=coverage.out ./...
	@echo "✅ Tests complete"

test-unit: ## Run unit tests only
	@echo "🧪 Running unit tests..."
	$(GOTEST) -v -short ./...

test-coverage: test ## Run tests with coverage report
	@echo "📊 Generating coverage report..."
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "✅ Coverage report: coverage.html"

run: build ## Build and run the application
	@echo "🚀 Running $(BINARY_NAME)..."
	$(BUILD_DIR)/$(BINARY_NAME) console

install: build ## Install the binary to $GOPATH/bin
	@echo "📥 Installing $(BINARY_NAME)..."
	cp $(BUILD_DIR)/$(BINARY_NAME) $(GOPATH)/bin/
	@echo "✅ Installed to $(GOPATH)/bin/$(BINARY_NAME)"

lint: ## Run linters
	@echo "🔍 Running linters..."
	@command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint not installed. Run: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; exit 1; }
	golangci-lint run ./...
	@echo "✅ Lint complete"

fmt: ## Format code
	@echo "💅 Formatting code..."
	$(GOCMD) fmt ./...
	@echo "✅ Format complete"

vet: ## Run go vet
	@echo "🔍 Running go vet..."
	$(GOCMD) vet ./...
	@echo "✅ Vet complete"

mod-verify: ## Verify dependencies
	@echo "🔍 Verifying dependencies..."
	$(GOMOD) verify
	@echo "✅ Dependencies verified"

# Development helpers
dev: ## Run in development mode (with auto-reload would require external tool)
	@echo "🔧 Running in development mode..."
	$(GOCMD) run ./cmd/k8s-monitor console

check: fmt vet test ## Run all checks (format, vet, test)
	@echo "✅ All checks passed"
