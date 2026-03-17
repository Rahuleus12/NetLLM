# AI Provider Makefile
# Common development and deployment tasks

# Variables
BINARY_NAME=ai-provider
MAIN_PATH=./cmd/server
BUILD_DIR=./bin
COVERAGE_DIR=./coverage
GO=go
GOFLAGS=-v

# Version information
VERSION?=1.0.0
BUILD_TIME=$(shell date -u +'%Y-%m-%dT%H:%M:%SZ')
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GIT_BRANCH=$(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")

# Build flags
LDFLAGS=-ldflags "-X main.Version=$(VERSION) \
				  -X main.BuildTime=$(BUILD_TIME) \
				  -X main.GitCommit=$(GIT_COMMIT) \
				  -w -s"

# Docker variables
DOCKER_IMAGE=ai-provider
DOCKER_TAG?=latest
DOCKER_REGISTRY?=
CONTAINER_NAME=ai-provider

# Colors for output
RED=\033[0;31m
GREEN=\033[0;32m
YELLOW=\033[1;33m
BLUE=\033[0;34m
NC=\033[0m

# Default target
.PHONY: help
help: ## Display this help screen
	@echo "AI Provider - Available make targets:"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(BLUE)%-20s$(NC) %s\n", $$1, $$2}'
	@echo ""
	@echo "Examples:"
	@echo "  make build              # Build the application"
	@echo "  make run                # Run the application"
	@echo "  make test               # Run tests"
	@echo "  make docker-build       # Build Docker image"
	@echo "  make all                # Run clean, deps, build, test"

# ============================================================================
# BUILD TARGETS
# ============================================================================

.PHONY: build
build: ## Build the application binary
	@echo "$(GREEN)Building $(BINARY_NAME)...$(NC)"
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "$(GREEN)Build complete: $(BUILD_DIR)/$(BINARY_NAME)$(NC)"

.PHONY: build-all
build-all: ## Build for multiple platforms
	@echo "$(GREEN)Building for multiple platforms...$(NC)"
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(MAIN_PATH)
	GOOS=darwin GOARCH=amd64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(MAIN_PATH)
	GOOS=darwin GOARCH=arm64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(MAIN_PATH)
	GOOS=windows GOARCH=amd64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(MAIN_PATH)
	@echo "$(GREEN)Multi-platform build complete$(NC)"

.PHONY: clean
clean: ## Remove build artifacts
	@echo "$(YELLOW)Cleaning build artifacts...$(NC)"
	@rm -rf $(BUILD_DIR)
	@rm -rf $(COVERAGE_DIR)
	@rm -f coverage.out
	@$(GO) clean
	@echo "$(GREEN)Clean complete$(NC)"

# ============================================================================
 DEVELOPMENT TARGETS
# ============================================================================

.PHONY: run
run: build ## Build and run the application
	@echo "$(GREEN)Running $(BINARY_NAME)...$(NC)"
	@./$(BUILD_DIR)/$(BINARY_NAME)

.PHONY: dev
dev: ## Run in development mode with hot reload (requires air)
	@which air > /dev/null || (echo "$(RED)Error: air is not installed. Install with: go install github.com/cosmtrek/air@latest$(NC)" && exit 1)
	@echo "$(GREEN)Starting development server with hot reload...$(NC)"
	@air

.PHONY: install
install: ## Install the binary to GOPATH/bin
	@echo "$(GREEN)Installing $(BINARY_NAME)...$(NC)"
	$(GO) install $(LDFLAGS) $(MAIN_PATH)
	@echo "$(GREEN)Installation complete$(NC)"

# ============================================================================
# TESTING TARGETS
# ============================================================================

.PHONY: test
test: ## Run all tests
	@echo "$(GREEN)Running tests...$(NC)"
	$(GO) test $(GOFLAGS) ./...

.PHONY: test-verbose
test-verbose: ## Run tests with verbose output
	@echo "$(GREEN)Running tests (verbose)...$(NC)"
	$(GO) test -v ./...

.PHONY: test-coverage
test-coverage: ## Run tests with coverage report
	@echo "$(GREEN)Running tests with coverage...$(NC)"
	@mkdir -p $(COVERAGE_DIR)
	$(GO) test -v -coverprofile=$(COVERAGE_DIR)/coverage.out -covermode=atomic ./...
	$(GO) tool cover -html=$(COVERAGE_DIR)/coverage.out -o $(COVERAGE_DIR)/coverage.html
	@echo "$(GREEN)Coverage report generated: $(COVERAGE_DIR)/coverage.html$(NC)"

.PHONY: test-race
test-race: ## Run tests with race detection
	@echo "$(GREEN)Running tests with race detection...$(NC)"
	$(GO) test -race ./...

.PHONY: bench
bench: ## Run benchmarks
	@echo "$(GREEN)Running benchmarks...$(NC)"
	$(GO) test -bench=. -benchmem ./...

# ============================================================================
# CODE QUALITY TARGETS
# ============================================================================

.PHONY: fmt
fmt: ## Format code
	@echo "$(GREEN)Formatting code...$(NC)"
	$(GO) fmt ./...
	@echo "$(GREEN)Code formatted$(NC)"

.PHONY: vet
vet: ## Run go vet
	@echo "$(GREEN)Running go vet...$(NC)"
	$(GO) vet ./...

.PHONY: lint
lint: ## Run golangci-lint (requires golangci-lint)
	@which golangci-lint > /dev/null || (echo "$(RED)Error: golangci-lint is not installed$(NC)" && exit 1)
	@echo "$(GREEN)Running golangci-lint...$(NC)"
	golangci-lint run ./...

.PHONY: check
check: fmt vet lint test ## Run all checks (fmt, vet, lint, test)
	@echo "$(GREEN)All checks passed!$(NC)"

# ============================================================================
# DEPENDENCIES TARGETS
# ============================================================================

.PHONY: deps
deps: ## Download dependencies
	@echo "$(GREEN)Downloading dependencies...$(NC)"
	$(GO) mod download
	$(GO) mod verify
	@echo "$(GREEN)Dependencies downloaded$(NC)"

.PHONY: deps-update
deps-update: ## Update dependencies
	@echo "$(GREEN)Updating dependencies...$(NC)"
	$(GO) get -u ./...
	$(GO) mod tidy
	@echo "$(GREEN)Dependencies updated$(NC)"

.PHONY: deps-clean
deps-clean: ## Clean module cache
	@echo "$(YELLOW)Cleaning module cache...$(NC)"
	$(GO) clean -modcache
	@echo "$(GREEN)Module cache cleaned$(NC)"

# ============================================================================
# DOCKER TARGETS
# ============================================================================

.PHONY: docker-build
docker-build: ## Build Docker image
	@echo "$(GREEN)Building Docker image: $(DOCKER_IMAGE):$(DOCKER_TAG)$(NC)"
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) -f deployments/docker/Dockerfile .
	@echo "$(GREEN)Docker image built successfully$(NC)"

.PHONY: docker-build-no-cache
docker-build-no-cache: ## Build Docker image without cache
	@echo "$(GREEN)Building Docker image (no cache): $(DOCKER_IMAGE):$(DOCKER_TAG)$(NC)"
	docker build --no-cache -t $(DOCKER_IMAGE):$(DOCKER_TAG) -f deployments/docker/Dockerfile .
	@echo "$(GREEN)Docker image built successfully$(NC)"

.PHONY: docker-run
docker-run: ## Run Docker container
	@echo "$(GREEN)Running Docker container...$(NC)"
	docker run -d --name $(CONTAINER_NAME) \
		-p 8080:8080 \
		-p 9090:9090 \
		$(DOCKER_IMAGE):$(DOCKER_TAG)
	@echo "$(GREEN)Container started: $(CONTAINER_NAME)$(NC)"

.PHONY: docker-stop
docker-stop: ## Stop Docker container
	@echo "$(YELLOW)Stopping Docker container...$(NC)"
	docker stop $(CONTAINER_NAME) || true
	docker rm $(CONTAINER_NAME) || true
	@echo "$(GREEN)Container stopped$(NC)"

.PHONY: docker-logs
docker-logs: ## View Docker container logs
	@docker logs -f $(CONTAINER_NAME)

.PHONY: docker-push
docker-push: ## Push Docker image to registry
	@if [ -z "$(DOCKER_REGISTRY)" ]; then \
		echo "$(RED)Error: DOCKER_REGISTRY is not set$(NC)"; \
		exit 1; \
	fi
	@echo "$(GREEN)Pushing Docker image to registry...$(NC)"
	docker tag $(DOCKER_IMAGE):$(DOCKER_TAG) $(DOCKER_REGISTRY)/$(DOCKER_IMAGE):$(DOCKER_TAG)
	docker push $(DOCKER_REGISTRY)/$(DOCKER_IMAGE):$(DOCKER_TAG)
	@echo "$(GREEN)Docker image pushed successfully$(NC)"

.PHONY: docker-compose-up
docker-compose-up: ## Start all services with Docker Compose
	@echo "$(GREEN)Starting services with Docker Compose...$(NC)"
	docker-compose -f deployments/docker/docker-compose.yml up -d
	@echo "$(GREEN)Services started$(NC)"

.PHONY: docker-compose-down
docker-compose-down: ## Stop all services with Docker Compose
	@echo "$(YELLOW)Stopping Docker Compose services...$(NC)"
	docker-compose -f deployments/docker/docker-compose.yml down
	@echo "$(GREEN)Services stopped$(NC)"

.PHONY: docker-compose-logs
docker-compose-logs: ## View Docker Compose logs
	@docker-compose -f deployments/docker/docker-compose.yml logs -f

# ============================================================================
# DATABASE TARGETS
# ============================================================================

.PHONY: db-migrate
db-migrate: ## Run database migrations
	@echo "$(GREEN)Running database migrations...$(NC)"
	@echo "$(YELLOW)Database migration not implemented yet$(NC)"

.PHONY: db-reset
db-reset: ## Reset database
	@echo "$(YELLOW)Resetting database...$(NC)"
	@echo "$(YELLOW)Database reset not implemented yet$(NC)"

.PHONY: db-seed
db-seed: ## Seed database with test data
	@echo "$(GREEN)Seeding database...$(NC)"
	@echo "$(YELLOW)Database seeding not implemented yet$(NC)"

# ============================================================================
# DOCUMENTATION TARGETS
# ============================================================================

.PHONY: docs
docs: ## Generate documentation
	@echo "$(GREEN)Generating documentation...$(NC)"
	$(GO) doc -all > docs/generated/package-docs.txt
	@echo "$(GREEN)Documentation generated$(NC)"

.PHONY: swagger
swagger: ## Generate Swagger documentation
	@which swag > /dev/null || (echo "$(RED)Error: swag is not installed. Install with: go install github.com/swaggo/swag/cmd/swag@latest$(NC)" && exit 1)
	@echo "$(GREEN)Generating Swagger documentation...$(NC)"
	swag init -g cmd/server/main.go -o ./api
	@echo "$(GREEN)Swagger documentation generated$(NC)"

# ============================================================================
# SECURITY TARGETS
# ============================================================================

.PHONY: security-scan
security-scan: ## Run security vulnerability scan
	@which govulncheck > /dev/null || (echo "$(RED)Error: govulncheck is not installed. Install with: go install golang.org/x/vuln/cmd/govulncheck@latest$(NC)" && exit 1)
	@echo "$(GREEN)Running security vulnerability scan...$(NC)"
	govulncheck ./...
	@echo "$(GREEN)Security scan complete$(NC)"

# ============================================================================
# UTILITY TARGETS
# ============================================================================

.PHONY: version
version: ## Display version information
	@echo "Version:    $(VERSION)"
	@echo "Git Commit: $(GIT_COMMIT)"
	@echo "Git Branch: $(GIT_BRANCH)"
	@echo "Build Time: $(BUILD_TIME)"

.PHONY: info
info: ## Display system information
	@echo "Go Version: $$(go version)"
	@echo "Go OS:      $$(go env GOOS)"
	@echo "Go Arch:    $$(go env GOARCH)"
	@echo "Go Path:    $$(go env GOPATH)"
	@echo "Module:     $$(go list -m)"

.PHONY: list-deps
list-deps: ## List all dependencies
	@$(GO) list -m all

.PHONY: graph-deps
graph-deps: ## Display dependency graph
	@$(GO) mod graph

# ============================================================================
# COMPOSITE TARGETS
# ============================================================================

.PHONY: all
all: clean deps build test ## Run clean, deps, build, and test

.PHONY: ci
ci: check security-scan ## Run all CI checks

.PHONY: pre-commit
pre-commit: fmt vet test ## Run pre-commit checks

.PHONY: release
release: clean build-all docker-build ## Prepare a release
	@echo "$(GREEN)Release $(VERSION) prepared successfully$(NC)"

# ============================================================================
# DEVELOPMENT ENVIRONMENT TARGETS
# ============================================================================

.PHONY: setup
setup: deps ## Setup development environment
	@echo "$(GREEN)Setting up development environment...$(NC)"
	@chmod +x scripts/setup.sh
	@./scripts/setup.sh
	@echo "$(GREEN)Development environment setup complete$(NC)"

.PHONY: teardown
teardown: docker-compose-down clean ## Tear down development environment
	@echo "$(YELLOW)Tearing down development environment...$(NC)"
	@echo "$(GREEN)Teardown complete$(NC)"

# ============================================================================
# KUBERNETES TARGETS
# ============================================================================

.PHONY: kube-deploy
kube-deploy: ## Deploy to Kubernetes
	@echo "$(GREEN)Deploying to Kubernetes...$(NC)"
	@chmod +x scripts/deploy.sh
	@./scripts/deploy.sh deploy -k
	@echo "$(GREEN)Kubernetes deployment complete$(NC)"

.PHONY: kube-destroy
kube-destroy: ## Destroy Kubernetes deployment
	@echo "$(YELLOW)Destroying Kubernetes deployment...$(NC)"
	@chmod +x scripts/deploy.sh
	@./scripts/deploy.sh clean -k
	@echo "$(GREEN)Kubernetes deployment destroyed$(NC)"

# ============================================================================
 PRODUCTION TARGETS
# ============================================================================

.PHONY: prod-build
prod-build: ## Build for production
	@echo "$(GREEN)Building for production...$(NC)"
	@$(MAKE) build-all
	@$(MAKE) docker-build
	@echo "$(GREEN)Production build complete$(NC)"

.PHONY: prod-deploy
prod-deploy: ## Deploy to production
	@echo "$(GREEN)Deploying to production...$(NC)"
	@$(MAKE) docker-push
	@$(MAKE) kube-deploy
	@echo "$(GREEN)Production deployment complete$(NC)"
