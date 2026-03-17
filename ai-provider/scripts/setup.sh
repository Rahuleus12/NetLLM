#!/bin/bash

# AI Provider Setup Script
# This script sets up the development environment for the AI Provider application

set -e  # Exit on error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Version requirements
MIN_GO_VERSION="1.21"
MIN_DOCKER_VERSION="20.10"

# Print colored output
print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Compare version numbers
version_ge() {
    # Returns 0 if $1 >= $2
    printf '%s\n%s\n' "$2" "$1" | sort -V -C
}

# Check Go installation
check_go() {
    print_info "Checking Go installation..."

    if ! command_exists go; then
        print_error "Go is not installed. Please install Go $MIN_GO_VERSION or higher."
        print_info "Visit: https://golang.org/doc/install"
        exit 1
    fi

    GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')

    if ! version_ge "$GO_VERSION" "$MIN_GO_VERSION"; then
        print_error "Go version $GO_VERSION is installed, but version $MIN_GO_VERSION or higher is required."
        exit 1
    fi

    print_success "Go $GO_VERSION is installed"
}

# Check Docker installation
check_docker() {
    print_info "Checking Docker installation..."

    if ! command_exists docker; then
        print_error "Docker is not installed. Please install Docker $MIN_DOCKER_VERSION or higher."
        print_info "Visit: https://docs.docker.com/get-docker/"
        exit 1
    fi

    DOCKER_VERSION=$(docker --version | awk '{print $3}' | sed 's/,//')

    if ! version_ge "$DOCKER_VERSION" "$MIN_DOCKER_VERSION"; then
        print_warning "Docker version $DOCKER_VERSION is installed. Version $MIN_DOCKER_VERSION or higher is recommended."
    fi

    # Check if Docker daemon is running
    if ! docker info >/dev/null 2>&1; then
        print_error "Docker daemon is not running. Please start Docker."
        exit 1
    fi

    print_success "Docker $DOCKER_VERSION is installed and running"
}

# Check Docker Compose installation
check_docker_compose() {
    print_info "Checking Docker Compose installation..."

    if command_exists docker-compose; then
        COMPOSE_CMD="docker-compose"
    elif docker compose version >/dev/null 2>&1; then
        COMPOSE_CMD="docker compose"
    else
        print_error "Docker Compose is not installed."
        print_info "Install Docker Compose: https://docs.docker.com/compose/install/"
        exit 1
    fi

    print_success "Docker Compose is installed"
}

# Install Go dependencies
install_dependencies() {
    print_info "Installing Go dependencies..."

    cd "$(dirname "$0")/.."

    if [ ! -f "go.mod" ]; then
        print_error "go.mod not found. Are you in the correct directory?"
        exit 1
    fi

    go mod download
    go mod verify

    print_success "Go dependencies installed successfully"
}

# Setup configuration files
setup_config() {
    print_info "Setting up configuration files..."

    cd "$(dirname "$0")/.."

    # Create config file from example if it doesn't exist
    if [ ! -f "configs/config.yaml" ]; then
        if [ -f "configs/config.yaml.example" ]; then
            cp configs/config.yaml.example configs/config.yaml
            print_success "Created configs/config.yaml from example"
        else
            print_warning "configs/config.yaml not found. Using default configuration."
        fi
    else
        print_info "configs/config.yaml already exists"
    fi

    # Create necessary directories
    mkdir -p /tmp/ai-provider
    mkdir -p /var/log/ai-provider 2>/dev/null || true

    print_success "Configuration setup complete"
}

# Setup environment file
setup_env_file() {
    print_info "Setting up environment file..."

    cd "$(dirname "$0")/.."

    if [ ! -f ".env" ]; then
        cat > .env << 'EOF'
# AI Provider Environment Configuration

# System Configuration
AI_PROVIDER_SYSTEM_HOST=0.0.0.0
AI_PROVIDER_SYSTEM_PORT=8080
AI_PROVIDER_SYSTEM_WORKERS=4

# Database Configuration
AI_PROVIDER_STORAGE_DATABASE_HOST=localhost
AI_PROVIDER_STORAGE_DATABASE_PORT=5432
AI_PROVIDER_STORAGE_DATABASE_NAME=aiprovider
AI_PROVIDER_STORAGE_DATABASE_USER=admin
AI_PROVIDER_STORAGE_DATABASE_PASSWORD=secret

# Redis Configuration
AI_PROVIDER_STORAGE_CACHE_HOST=localhost
AI_PROVIDER_STORAGE_CACHE_PORT=6379

# Logging
AI_PROVIDER_LOGGING_LEVEL=INFO

# GPU Configuration
AI_PROVIDER_COMPUTE_GPU_ENABLED=false

# API Configuration
AI_PROVIDER_API_AUTH_ENABLED=false
AI_PROVIDER_API_RATE_LIMIT=100
EOF
        print_success "Created .env file"
    else
        print_info ".env file already exists"
    fi
}

# Start infrastructure services
start_infrastructure() {
    print_info "Starting infrastructure services..."

    cd "$(dirname "$0")/../deployments/docker"

    if [ ! -f "docker-compose.yml" ]; then
        print_warning "docker-compose.yml not found. Creating basic infrastructure..."
        create_docker_compose
    fi

    # Start PostgreSQL and Redis
    $COMPOSE_CMD up -d postgres redis

    print_info "Waiting for services to be ready..."
    sleep 5

    # Check if services are healthy
    if $COMPOSE_CMD ps | grep -q "postgres.*Up"; then
        print_success "PostgreSQL is running"
    else
        print_error "PostgreSQL failed to start"
        exit 1
    fi

    if $COMPOSE_CMD ps | grep -q "redis.*Up"; then
        print_success "Redis is running"
    else
        print_error "Redis failed to start"
        exit 1
    fi
}

# Create basic docker-compose.yml if it doesn't exist
create_docker_compose() {
    cat > docker-compose.yml << 'EOF'
version: '3.8'

services:
  postgres:
    image: postgres:15-alpine
    container_name: ai-provider-postgres
    environment:
      POSTGRES_DB: aiprovider
      POSTGRES_USER: admin
      POSTGRES_PASSWORD: secret
    ports:
      - "5432:5432"
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U admin -d aiprovider"]
      interval: 10s
      timeout: 5s
      retries: 5

  redis:
    image: redis:7-alpine
    container_name: ai-provider-redis
    ports:
      - "6379:6379"
    volumes:
      - redisdata:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 10s
      timeout: 5s
      retries: 5

  prometheus:
    image: prom/prometheus:latest
    container_name: ai-provider-prometheus
    ports:
      - "9090:9090"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
      - prometheusdata:/prometheus
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'
      - '--storage.tsdb.path=/prometheus'

  grafana:
    image: grafana/grafana:latest
    container_name: ai-provider-grafana
    ports:
      - "3000:3000"
    volumes:
      - grafanadata:/var/lib/grafana
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin

volumes:
  pgdata:
  redisdata:
  prometheusdata:
  grafanadata:
EOF
    print_success "Created docker-compose.yml"
}

# Initialize database
init_database() {
    print_info "Initializing database..."

    cd "$(dirname "$0")/.."

    # Wait for PostgreSQL to be ready
    print_info "Waiting for PostgreSQL to be ready..."
    for i in {1..30}; do
        if docker exec ai-provider-postgres pg_isready -U admin -d aiprovider >/dev/null 2>&1; then
            print_success "PostgreSQL is ready"
            break
        fi
        if [ $i -eq 30 ]; then
            print_error "PostgreSQL did not become ready in time"
            exit 1
        fi
        sleep 1
    done

    print_success "Database initialization complete"
}

# Build the application
build_application() {
    print_info "Building application..."

    cd "$(dirname "$0")/.."

    go build -o bin/ai-provider cmd/server/main.go

    print_success "Application built successfully"
}

# Run tests
run_tests() {
    print_info "Running tests..."

    cd "$(dirname "$0")/.."

    go test ./... -v

    print_success "Tests completed"
}

# Display next steps
show_next_steps() {
    cat << EOF

${GREEN}========================================
  Setup Complete!
========================================${NC}

${BLUE}Next Steps:${NC}

1. ${YELLOW}Review Configuration:${NC}
   - Edit configs/config.yaml to customize settings
   - Update .env file with your environment variables

2. ${YELLOW}Start the Application:${NC}
   ${GREEN}./bin/ai-provider${NC}

   Or run directly:
   ${GREEN}go run cmd/server/main.go${NC}

3. ${YELLOW}Access the Services:${NC}
   - API: ${BLUE}http://localhost:8080${NC}
   - Health Check: ${BLUE}http://localhost:8080/health${NC}
   - Metrics: ${BLUE}http://localhost:8080/metrics${NC}
   - Grafana: ${BLUE}http://localhost:3000${NC} (admin/admin)
   - Prometheus: ${BLUE}http://localhost:9090${NC}

4. ${YELLOW}API Documentation:${NC}
   - Check docs/api.md for API usage

5. ${YELLOW}Development:${NC}
   - Run tests: ${GREEN}go test ./...${NC}
   - Format code: ${GREEN}go fmt ./...${NC}
   - Lint code: ${GREEN}golangci-lint run${NC}

6. ${YELLOW}Docker Deployment:${NC}
   - Build image: ${GREEN}docker build -t ai-provider:latest -f deployments/docker/Dockerfile .${NC}
   - Run with compose: ${GREEN}docker-compose -f deployments/docker/docker-compose.yml up -d${NC}

${BLUE}Useful Commands:${NC}
   - Stop services: ${GREEN}docker-compose -f deployments/docker/docker-compose.yml down${NC}
   - View logs: ${GREEN}docker-compose -f deployments/docker/docker-compose.yml logs -f${NC}
   - Clean up: ${GREEN}make clean${NC}

${YELLOW}Note:${NC} Make sure to change default passwords and secrets before production deployment!

${GREEN}Happy Coding! 🚀${NC}

EOF
}

# Cleanup function
cleanup() {
    print_warning "Setup interrupted. Cleaning up..."
    exit 1
}

# Set trap for cleanup
trap cleanup INT TERM

# Main setup process
main() {
    cat << EOF

${GREEN}========================================
  AI Provider Setup Script
========================================${NC}

This script will set up your development environment for the AI Provider application.

${BLUE}Setup Steps:${NC}
1. Check prerequisites (Go, Docker)
2. Install dependencies
3. Setup configuration
4. Start infrastructure services
5. Initialize database
6. Build application

${YELLOW}Press Ctrl+C at any time to abort${NC}

EOF

    sleep 2

    # Run setup steps
    check_go
    check_docker
    check_docker_compose
    install_dependencies
    setup_config
    setup_env_file
    start_infrastructure
    init_database
    build_application

    # Optional: Run tests
    read -p "$(echo -e ${BLUE}Would you like to run tests? [y/N]:${NC} )" -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        run_tests
    fi

    # Show next steps
    show_next_steps
}

# Run main function
main "$@"
