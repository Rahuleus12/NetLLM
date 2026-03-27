# Local Deployment Guide - AI Provider

## 📋 Table of Contents

1. [Prerequisites](#prerequisites)
2. [Quick Start](#quick-start)
3. [Detailed Setup](#detailed-setup)
4. [Configuration](#configuration)
5. [Running the Services](#running-the-services)
6. [Testing the Deployment](#testing-the-deployment)
7. [Development Workflow](#development-workflow)
8. [Troubleshooting](#troubleshooting)
9. [Stopping and Cleanup](#stopping-and-cleanup)

---

## Prerequisites

### System Requirements

- **Operating System**: Windows 10/11, macOS 10.15+, or Linux (Ubuntu 18.04+)
- **CPU**: 4 cores minimum (8 cores recommended)
- **RAM**: 8GB minimum (16GB recommended)
- **Disk Space**: 20GB minimum

### Required Software

#### Core Dependencies
- **Go**: Version 1.21 or later
  ```bash
  # Verify installation
  go version
  
  # Install if needed (macOS/Linux)
  wget https://go.dev/dl/go1.21.5.linux-amd64.tar.gz
  sudo tar -C /usr/local -xzf go1.21.5.linux-amd64.tar.gz
  export PATH=$PATH:/usr/local/go/bin
  ```

- **Docker**: Version 20.10 or later
  ```bash
  # Verify installation
  docker --version
  
  # Install Docker Desktop (Windows/macOS) or Docker Engine (Linux)
  # https://docs.docker.com/get-docker/
  ```

- **Docker Compose**: Version 2.0 or later
  ```bash
  # Verify installation
  docker-compose --version
  ```

#### Optional but Recommended
- **Make**: Build automation
- **Git**: Version control
- **curl** or **wget**: HTTP client
- **jq**: JSON processor (for API testing)

### Go Dependencies

The project uses Go modules. All dependencies are listed in `go.mod`:

```bash
# Download dependencies
go mod download

# Verify dependencies
go mod verify
```

---

## Quick Start

### Minimal Local Setup (5 minutes)

1. **Clone and Navigate**
   ```bash
   cd /path/to/Netllm/ai-provider
   ```

2. **Install Dependencies**
   ```bash
   go mod download
   ```

3. **Run Database (Docker)**
   ```bash
   docker run -d \
     --name ai-provider-db \
     -e POSTGRES_DB=ai_provider \
     -e POSTGRES_USER=ai_user \
     -e POSTGRES_PASSWORD=ai_password \
     -p 5432:5432 \
     postgres:15-alpine
   ```

4. **Configure Environment**
   ```bash
   export DB_HOST=localhost
   export DB_PORT=5432
   export DB_NAME=ai_provider
   export DB_USER=ai_user
   export DB_PASSWORD=ai_password
   export API_PORT=8080
   ```

5. **Run Migrations**
   ```bash
   # Create tables (if migration tool exists)
   go run cmd/migrate/main.go up
   ```

6. **Start the Service**
   ```bash
   go run cmd/server/main.go
   ```

7. **Verify**
   ```bash
   curl http://localhost:8080/health
   ```

---

## Detailed Setup

### 1. Environment Setup

#### 1.1 Clone the Repository
```bash
cd /path/to/your/workspace
# If using git:
# git clone <repository-url> Netllm
cd Netllm/ai-provider
```

#### 1.2 Set Go Environment Variables
```bash
# Add to your shell profile (~/.bashrc, ~/.zshrc, or ~/.profile)
export GOPATH=$HOME/go
export PATH=$PATH:$GOPATH/bin
export GO111MODULE=on

# Apply changes
source ~/.bashrc  # or your shell profile
```

#### 1.3 Install Project Dependencies
```bash
# Navigate to project directory
cd Netllm/ai-provider

# Download all Go dependencies
go mod download

# Install development tools
go install github.com/cosmtrek/air@latest  # Hot reload for development
go install github.com/swaggo/swag/cmd/swag@latest  # Swagger documentation
```

### 2. Database Setup

#### 2.1 PostgreSQL with Docker

**Option A: Simple Docker Container**
```bash
# Start PostgreSQL
docker run -d \
  --name ai-provider-postgres \
  -e POSTGRES_DB=ai_provider \
  -e POSTGRES_USER=ai_provider_user \
  -e POSTGRES_PASSWORD=ai_provider_password \
  -e POSTGRES_INITDB_ARGS="--encoding=UTF8 --locale=C.UTF-8" \
  -p 5432:5432 \
  -v ai_provider_postgres_data:/var/lib/postgresql/data \
  postgres:15-alpine \
  postgres -c max_connections=200 -c shared_buffers=256MB

# Wait for PostgreSQL to be ready
until docker exec ai-provider-postgres pg_isready -U ai_provider_user; do
  echo "Waiting for PostgreSQL..."
  sleep 2
done

echo "PostgreSQL is ready!"
```

**Option B: Docker Compose (Recommended)**
```bash
# Create docker-compose.yml for local development
cat > docker-compose.yml <<EOF
version: '3.8'

services:
  postgres:
    image: postgres:15-alpine
    container_name: ai-provider-db
    environment:
      POSTGRES_DB: ai_provider
      POSTGRES_USER: ai_provider_user
      POSTGRES_PASSWORD: ai_provider_password
      POSTGRES_INITDB_ARGS: "--encoding=UTF8 --locale=C.UTF-8"
    ports:
      - "5432:5432"
    volumes:
      - postgres_data:/var/lib/postgresql/data
      - ./scripts/init-db.sql:/docker-entrypoint-initdb.d/init.sql
    command: postgres -c max_connections=200 -c shared_buffers=256MB
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ai_provider_user"]
      interval: 10s
      timeout: 5s
      retries: 5

  redis:
    image: redis:7-alpine
    container_name: ai-provider-redis
    ports:
      - "6379:6379"
    volumes:
      - redis_data:/data
    command: redis-server --appendonly yes
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 10s
      timeout: 3s
      retries: 5

volumes:
  postgres_data:
  redis_data:
EOF

# Start services
docker-compose up -d

# Check status
docker-compose ps
```

#### 2.2 Database Initialization

```bash
# Connect to PostgreSQL
docker exec -it ai-provider-postgres psql -U ai_provider_user -d ai_provider

# Or use a local PostgreSQL client
psql -h localhost -U ai_provider_user -d ai_provider

# Run initialization scripts (if available)
psql -h localhost -U ai_provider_user -d ai_provider -f scripts/init-db.sql
```

### 3. Configuration

#### 3.1 Environment Variables

Create a `.env` file in the project root:

```bash
# .env
# Database Configuration
DB_HOST=localhost
DB_PORT=5432
DB_NAME=ai_provider
DB_USER=ai_provider_user
DB_PASSWORD=ai_provider_password
DB_SSL_MODE=disable
DB_MAX_CONNECTIONS=20

# Redis Configuration
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
REDIS_DB=0

# API Configuration
API_HOST=0.0.0.0
API_PORT=8080
API_READ_TIMEOUT=30s
API_WRITE_TIMEOUT=30s
API_IDLE_TIMEOUT=60s

# Security
JWT_SECRET=your-super-secret-jwt-key-change-this-in-production
API_KEY_HEADER=X-API-Key
CORS_ALLOWED_ORIGINS=http://localhost:3000,http://localhost:8080

# Logging
LOG_LEVEL=debug
LOG_FORMAT=json
LOG_OUTPUT=stdout

# Features
FEATURE_MULTI_TENANCY=true
FEATURE_RATE_LIMITING=true
FEATURE_MONITORING=true

# Phase 10 Features (Enterprise)
HA_ENABLED=true
MULTI_REGION_ENABLED=false  # Disable for local development
COMPLIANCE_ENABLED=false    # Disable for local development
ENTERPRISE_INTEGRATION_ENABLED=false
```

#### 3.2 Load Environment Variables

```bash
# Option 1: Export manually
export $(cat .env | xargs)

# Option 2: Use a tool like direnv or dotenv
# Install direnv: https://direnv.net/
# Then create .envrc with: dotenv

# Option 3: Source in shell profile
source .env
```

#### 3.3 Configuration Files

**Application Configuration** (`configs/config.yaml`):
```yaml
# configs/config.yaml
server:
  host: ${API_HOST:0.0.0.0}
  port: ${API_PORT:8080}
  read_timeout: ${API_READ_TIMEOUT:30s}
  write_timeout: ${API_WRITE_TIMEOUT:30s}
  idle_timeout: ${API_IDLE_TIMEOUT:60s}

database:
  host: ${DB_HOST:localhost}
  port: ${DB_PORT:5432}
  name: ${DB_NAME:ai_provider}
  user: ${DB_USER:ai_provider_user}
  password: ${DB_PASSWORD:ai_provider_password}
  ssl_mode: ${DB_SSL_MODE:disable}
  max_connections: ${DB_MAX_CONNECTIONS:20}
  max_idle: 10
  conn_lifetime: 5m

redis:
  host: ${REDIS_HOST:localhost}
  port: ${REDIS_PORT:6379}
  password: ${REDIS_PASSWORD:}
  db: ${REDIS_DB:0}
  pool_size: 10

security:
  jwt_secret: ${JWT_SECRET:dev-secret-key}
  api_key_header: ${API_KEY_HEADER:X-API-Key}
  cors:
    allowed_origins: ${CORS_ALLOWED_ORIGINS:*}
    allowed_methods: ["GET", "POST", "PUT", "DELETE", "OPTIONS"]
    allowed_headers: ["*"]

logging:
  level: ${LOG_LEVEL:debug}
  format: ${LOG_FORMAT:json}
  output: ${LOG_OUTPUT:stdout}

features:
  multi_tenancy: ${FEATURE_MULTI_TENANCY:true}
  rate_limiting: ${FEATURE_RATE_LIMITING:true}
  monitoring: ${FEATURE_MONITORING:true}
  high_availability: ${HA_ENABLED:false}
  multi_region: ${MULTI_REGION_ENABLED:false}
  compliance: ${COMPLIANCE_ENABLED:false}
  enterprise_integration: ${ENTERPRISE_INTEGRATION_ENABLED:false}
```

### 4. Build and Run

#### 4.1 Build the Application

```bash
# Build binary
go build -o bin/ai-provider cmd/server/main.go

# Build with version info
VERSION=$(git describe --tags --always --dirty)
BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
go build -ldflags "-X main.Version=$VERSION -X main.BuildTime=$BUILD_TIME" \
  -o bin/ai-provider cmd/server/main.go

# Verify build
./bin/ai-provider --version
```

#### 4.2 Run Database Migrations

```bash
# If you have a migration tool
go run cmd/migrate/main.go up

# Or run SQL scripts directly
psql -h localhost -U ai_provider_user -d ai_provider -f migrations/001_initial_schema.sql
```

#### 4.3 Start the Server

**Development Mode (with hot reload):**
```bash
# Install air if not already installed
go install github.com/cosmtrek/air@latest

# Run with hot reload
air

# Or create .air.toml configuration
cat > .air.toml <<EOF
root = "."
tmp_dir = "tmp"

[build]
  bin = "./tmp/main"
  cmd = "go build -o ./tmp/main cmd/server/main.go"
  delay = 1000
  exclude_dir = ["assets", "tmp", "vendor"]
  exclude_file = []
  exclude_regex = ["_test.go"]
  exclude_unchanged = false
  follow_symlink = false
  include_dir = []
  include_ext = ["go", "tpl", "tmpl", "html"]
  kill_delay = "0s"
  log = "build-errors.log"
  send_interrupt = false
  stop_on_error = true

[color]
  build = "yellow"
  main = "magenta"
  runner = "green"
  watcher = "cyan"

[log]
  time = false

[misc]
  clean_on_exit = true
EOF

# Run with air
air
```

**Production Mode:**
```bash
# Run the compiled binary
./bin/ai-provider

# Or run directly
go run cmd/server/main.go

# With custom configuration
./bin/ai-provider -config configs/config.yaml

# With environment variables
API_PORT=9090 ./bin/ai-provider
```

---

## Running the Services

### Individual Services

#### API Server
```bash
# Start API server
go run cmd/server/main.go

# Or with specific port
API_PORT=8080 go run cmd/server/main.go

# Check health
curl http://localhost:8080/health
```

#### Background Workers (if applicable)
```bash
# Start worker processes
go run cmd/worker/main.go

# Start specific worker
go run cmd/worker/main.go -type inference
```

### Using Make (Recommended)

Create a `Makefile`:

```makefile
# Makefile
.PHONY: help build run test clean docker-up docker-down

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the application
	@echo "Building..."
	@go build -o bin/ai-provider cmd/server/main.go

run: ## Run the application
	@echo "Running..."
	@go run cmd/server/main.go

dev: ## Run with hot reload
	@air

test: ## Run tests
	@go test -v -race -coverprofile=coverage.out ./...

test-coverage: test ## Generate test coverage report
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

docker-up: ## Start Docker services (PostgreSQL, Redis)
	@docker-compose up -d
	@echo "Docker services started"

docker-down: ## Stop Docker services
	@docker-compose down
	@echo "Docker services stopped"

docker-logs: ## Show Docker logs
	@docker-compose logs -f

migrate-up: ## Run database migrations
	@go run cmd/migrate/main.go up

migrate-down: ## Rollback database migrations
	@go run cmd/migrate/main.go down

clean: ## Clean build artifacts
	@rm -rf bin/
	@rm -rf tmp/
	@echo "Cleaned build artifacts"

deps: ## Install dependencies
	@go mod download
	@go mod verify
	@echo "Dependencies installed"

fmt: ## Format code
	@go fmt ./...
	@echo "Code formatted"

lint: ## Run linter
	@golangci-lint run ./...
	@echo "Linting complete"

swagger: ## Generate Swagger documentation
	@swag init -g cmd/server/main.go -o ./docs/swagger
	@echo "Swagger documentation generated"

all: deps fmt lint test build ## Run all checks and build
```

**Usage:**
```bash
# Install dependencies and build
make deps build

# Start services and run
make docker-up run

# Run tests
make test

# Development with hot reload
make dev

# Clean build
make clean build
```

---

## Testing the Deployment

### 1. Health Checks

```bash
# Basic health check
curl http://localhost:8080/health

# Detailed health check
curl http://localhost:8080/health/detailed

# Expected response:
# {
#   "status": "healthy",
#   "timestamp": "2025-06-17T10:00:00Z",
#   "version": "1.0.0",
#   "components": {
#     "database": "healthy",
#     "redis": "healthy"
#   }
# }
```

### 2. API Endpoints

```bash
# Get API version
curl http://localhost:8080/api/v1/version

# List available endpoints
curl http://localhost:8080/api/v1/endpoints

# Create a test model
curl -X POST http://localhost:8080/api/v1/models \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-api-key" \
  -d '{
    "name": "test-model",
    "type": "text-generation",
    "provider": "openai",
    "config": {
      "model": "gpt-3.5-turbo",
      "temperature": 0.7
    }
  }'

# List models
curl -H "X-API-Key: your-api-key" \
  http://localhost:8080/api/v1/models

# Run inference
curl -X POST http://localhost:8080/api/v1/inference \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-api-key" \
  -d '{
    "model_id": "test-model",
    "input": "Hello, world!",
    "parameters": {
      "max_tokens": 100
    }
  }'
```

### 3. Database Connectivity

```bash
# Test database connection
docker exec -it ai-provider-postgres psql -U ai_provider_user -d ai_provider -c "SELECT version();"

# Check tables
docker exec -it ai-provider-postgres psql -U ai_provider_user -d ai_provider -c "\dt"

# Check data
docker exec -it ai-provider-postgres psql -U ai_provider_user -d ai_provider -c "SELECT COUNT(*) FROM models;"
```

### 4. Performance Testing

```bash
# Install hey (HTTP load testing tool)
go install github.com/rakyll/hey@latest

# Basic load test
hey -n 100 -c 10 http://localhost:8080/health

# API load test
hey -n 1000 -c 50 -m POST \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-api-key" \
  -d '{"model_id":"test-model","input":"test"}' \
  http://localhost:8080/api/v1/inference
```

### 5. Monitoring and Metrics

```bash
# Prometheus metrics (if enabled)
curl http://localhost:8080/metrics

# Application metrics
curl http://localhost:8080/api/v1/metrics

# System status
curl http://localhost:8080/api/v1/system/status
```

---

## Development Workflow

### 1. Code-Test-Debug Cycle

```bash
# Terminal 1: Run server with hot reload
make dev

# Terminal 2: Run tests continuously
go test -v -race ./... -watch

# Terminal 3: Check logs
tail -f logs/app.log

# Terminal 4: Database operations
docker exec -it ai-provider-postgres psql -U ai_provider_user -d ai_provider
```

### 2. Database Migrations

```bash
# Create a new migration
go run cmd/migrate/main.go create add_new_table

# Apply migrations
go run cmd/migrate/main.go up

# Rollback last migration
go run cmd/migrate/main.go down

# Check migration status
go run cmd/migrate/main.go status
```

### 3. API Documentation

```bash
# Generate Swagger docs
make swagger

# Access Swagger UI
open http://localhost:8080/swagger/index.html

# Access API docs
open http://localhost:8080/docs
```

### 4. Debugging

```bash
# Enable debug logging
export LOG_LEVEL=debug

# Run with pprof (performance profiling)
go run cmd/server/main.go -enable-pprof

# Access pprof
# CPU profile: http://localhost:8080/debug/pprof/profile
# Memory profile: http://localhost:8080/debug/pprof/heap
# Goroutines: http://localhost:8080/debug/pprof/goroutine

# Use delve debugger
go install github.com/go-delve/delve/cmd/dlv@latest
dlv debug cmd/server/main.go
```

---

## Troubleshooting

### Common Issues and Solutions

#### 1. Port Already in Use

**Error**: `bind: address already in use`

**Solution**:
```bash
# Find process using port
lsof -i :8080  # macOS/Linux
netstat -ano | findstr :8080  # Windows

# Kill process
kill -9 <PID>  # macOS/Linux
taskkill /F /PID <PID>  # Windows

# Or use different port
API_PORT=9090 go run cmd/server/main.go
```

#### 2. Database Connection Failed

**Error**: `connection refused` or `no such host`

**Solution**:
```bash
# Check if PostgreSQL is running
docker ps | grep postgres

# Check PostgreSQL logs
docker logs ai-provider-postgres

# Test connection
docker exec -it ai-provider-postgres pg_isready -U ai_provider_user

# Restart PostgreSQL
docker restart ai-provider-postgres

# Check environment variables
echo $DB_HOST $DB_PORT $DB_USER $DB_PASSWORD
```

#### 3. Module Not Found

**Error**: `package xyz is not in GOROOT`

**Solution**:
```bash
# Clean module cache
go clean -modcache

# Re-download dependencies
go mod download

# Verify modules
go mod verify

# Update dependencies
go mod tidy
```

#### 4. Permission Denied

**Error**: `permission denied` when running binary

**Solution**:
```bash
# Make binary executable
chmod +x bin/ai-provider

# Check file permissions
ls -la bin/ai-provider

# Run with appropriate permissions
./bin/ai-provider
```

#### 5. Out of Memory

**Error**: `fatal error: runtime: out of memory`

**Solution**:
```bash
# Increase Go's memory limit
export GOMEMLIMIT=8GiB

# Or reduce application memory usage
export DB_MAX_CONNECTIONS=10
export REDIS_POOL_SIZE=5

# Monitor memory usage
go tool pprof http://localhost:8080/debug/pprof/heap
```

#### 6. Redis Connection Issues

**Error**: `redis: connection refused`

**Solution**:
```bash
# Check Redis status
docker ps | grep redis

# Test Redis connection
docker exec -it ai-provider-redis redis-cli ping

# Check Redis logs
docker logs ai-provider-redis

# Restart Redis
docker restart ai-provider-redis
```

### Debug Mode

Enable comprehensive debugging:

```bash
# Set debug environment
export LOG_LEVEL=debug
export LOG_FORMAT=text
export ENABLE_PPROF=true

# Run with debug flags
go run cmd/server/main.go -debug -log-level=debug

# Enable trace logging
export TRACE_ENABLED=true
```

### Log Analysis

```bash
# View real-time logs
tail -f logs/app.log

# Search logs
grep "ERROR" logs/app.log
grep "panic" logs/app.log

# Export logs for analysis
docker logs ai-provider-postgres > postgres.log 2>&1
```

---

## Stopping and Cleanup

### Stop Services

```bash
# Stop application
# Ctrl+C if running in foreground
# Or kill process
pkill -f ai-provider

# Stop Docker services
docker-compose down

# Stop specific containers
docker stop ai-provider-postgres
docker stop ai-provider-redis
```

### Clean Up

```bash
# Remove Docker containers
docker rm ai-provider-postgres ai-provider-redis

# Remove Docker volumes (WARNING: This deletes all data)
docker volume rm ai_provider_postgres_data

# Remove build artifacts
make clean

# Remove all generated files
make clean
rm -rf tmp/ bin/ coverage.out
```

### Reset Database

```bash
# Drop and recreate database
docker exec -it ai-provider-postgres psql -U ai_provider_user -d postgres \
  -c "DROP DATABASE IF EXISTS ai_provider;"
docker exec -it ai-provider-postgres psql -U ai_provider_user -d postgres \
  -c "CREATE DATABASE ai_provider;"

# Run migrations again
go run cmd/migrate/main.go up
```

---

## Production Checklist

Before deploying to production, ensure:

- [ ] All tests pass: `make test`
- [ ] Security scan completed: `make security-scan`
- [ ] Environment variables properly set
- [ ] Database migrations applied
- [ ] SSL/TLS certificates configured
- [ ] Monitoring and logging configured
- [ ] Backup procedures in place
- [ ] Rate limiting configured
- [ ] CORS settings reviewed
- [ ] API authentication enabled
- [ ] Resource limits set (CPU, memory)
- [ ] Health checks configured
- [ ] Graceful shutdown tested

---

## Additional Resources

### Documentation
- API Documentation: `http://localhost:8080/swagger/index.html`
- Architecture: `docs/ARCHITECTURE.md`
- Contributing: `CONTRIBUTING.md`

### Useful Commands
```bash
# Check dependencies for updates
go list -u -m -json all | grep "Path\|Version\|Update"

# Format code
go fmt ./...

# Run linter
golangci-lint run

# Generate mocks for testing
go generate ./...

# View Go environment
go env
```

### Support
- Check logs for errors
- Review application metrics
- Consult troubleshooting section
- Check GitHub issues

---

**Note**: This guide is for local development and testing only. For production deployment, additional security, monitoring, and infrastructure considerations are required.