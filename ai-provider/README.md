# AI Provider

A network-accessible, containerized local AI provider application that can run specified AI models, be tweaked via APIs, and provide complete management capabilities.

## Overview

AI Provider is a comprehensive platform for managing and serving AI models locally. It provides a robust API gateway, model orchestration, and containerized model runtime environments with built-in monitoring, scaling, and resource management.

## Features

### Core Capabilities
- **Model Management**: Download, version, and manage multiple AI models
- **Containerized Runtime**: Each model runs in its own isolated container
- **RESTful API**: Complete API for model management and inference
- **WebSocket Support**: Real-time streaming for inference responses
- **Auto-scaling**: Automatic scaling based on demand
- **Resource Management**: Intelligent GPU/CPU allocation and optimization
- **Health Monitoring**: Comprehensive health checks and metrics

### Advanced Features
- **Batch Inference**: Process multiple requests efficiently
- **Model Fine-tuning**: APIs for customizing models
- **Plugin System**: Extensible architecture for custom functionality
- **CLI Tools**: Command-line interface for easy management
- **Monitoring Dashboard**: Real-time metrics and visualization

### Production Ready
- **High Availability**: Failover and recovery mechanisms
- **Security**: Authentication, authorization, and encryption
- **Performance**: Optimized for low latency and high throughput
- **Scalability**: Support for 10+ concurrent models
- **Reliability**: 99.9% uptime target

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    API Gateway Layer                         │
│  (REST API + WebSocket for streaming + GraphQL optional)    │
└────────────────────┬────────────────────────────────────────┘
                     │
┌────────────────────┴────────────────────────────────────────┐
│                  Orchestration Layer                         │
│  ┌──────────────┬──────────────┬──────────────┬──────────┐ │
│  │   Model      │   Resource   │   Config     │  Health  │ │
│  │   Manager    │   Manager    │   Manager    │  Monitor │ │
│  └──────────────┴──────────────┴──────────────┴──────────┘ │
└────────────────────┬────────────────────────────────────────┘
                     │
┌────────────────────┴────────────────────────────────────────┐
│                  Model Runtime Layer                         │
│  ┌──────────────┬──────────────┬──────────────┬──────────┐ │
│  │  Model A     │  Model B     │  Model C     │  Model N │ │
│  │  Container   │  Container   │  Container   │ Container│ │
│  └──────────────┴──────────────┴──────────────┴──────────┘ │
└────────────────────┬────────────────────────────────────────┘
                     │
┌────────────────────┴────────────────────────────────────────┐
│              Infrastructure Layer                            │
│  ┌──────────────┬──────────────┬──────────────┬──────────┐ │
│  │   Storage    │   GPU/FPGA   │   Network    │  Logging │ │
│  │   (Models)   │   Resources  │   Layer      │  & Metrics│ │
│  └──────────────┴──────────────┴──────────────┴──────────┘ │
└─────────────────────────────────────────────────────────────┘
```

## Prerequisites

- **Go**: 1.21 or higher
- **Docker**: 20.10 or higher
- **Docker Compose**: 2.0 or higher (for local development)
- **PostgreSQL**: 15 or higher
- **Redis**: 7 or higher
- **GPU**: NVIDIA GPU with CUDA support (optional, for GPU acceleration)

## Quick Start

### 1. Clone the Repository

```bash
git clone <repository-url>
cd ai-provider
```

### 2. Install Dependencies

```bash
go mod download
```

### 3. Configure the Application

Copy the example configuration and adjust as needed:

```bash
cp configs/config.yaml.example configs/config.yaml
```

Edit `configs/config.yaml` with your settings:

```yaml
system:
  host: 0.0.0.0
  port: 8080
  workers: 4

compute:
  gpu_enabled: true
  gpu_devices: [0]
  cpu_threads: 8
  memory_limit: 16GB

models:
  max_concurrent: 10
  auto_scale: true
  scale_threshold: 0.8
  idle_timeout: 300

storage:
  models_path: ./models
  cache_size: 50GB

api:
  rate_limit: 1000
  auth_enabled: false
  cors_origins: ["*"]

logging:
  level: INFO
  file: /var/log/ai-provider.log

monitoring:
  prometheus_enabled: true
  metrics_interval: 15
```

### 4. Start Infrastructure Services

```bash
docker-compose -f deployments/docker/docker-compose.yml up -d postgres redis
```

### 5. Run the Application

```bash
go run cmd/server/main.go
```

Or build and run:

```bash
go build -o bin/ai-provider cmd/server/main.go
./bin/ai-provider
```

### 6. Verify the Installation

Check if the service is running:

```bash
curl http://localhost:8080/health
```

Expected response:
```json
{
  "status": "healthy",
  "version": "1.0.0",
  "uptime": "10s"
}
```

## API Documentation

Once the server is running, access the API documentation at:
- **Swagger UI**: http://localhost:8080/api/docs
- **OpenAPI Spec**: http://localhost:8080/api/openapi.yaml

### Key Endpoints

#### Model Management
- `GET /api/v1/models` - List all models
- `POST /api/v1/models` - Register a new model
- `GET /api/v1/models/{id}` - Get model details
- `PUT /api/v1/models/{id}` - Update model configuration
- `DELETE /api/v1/models/{id}` - Remove a model

#### Inference
- `POST /api/v1/inference/{model_id}` - Run inference
- `POST /api/v1/inference/{model_id}/stream` - Stream inference via WebSocket
- `POST /api/v1/inference/batch` - Batch inference

#### Configuration
- `GET /api/v1/config` - Get current configuration
- `PUT /api/v1/config` - Update configuration
- `GET /api/v1/config/models/{id}` - Get model-specific config

#### Monitoring
- `GET /health` - Health check
- `GET /metrics` - Prometheus metrics
- `GET /api/v1/status` - System status

For detailed API documentation, see [docs/api.md](docs/api.md).

## Development

### Project Structure

```
ai-provider/
├── cmd/
│   └── server/
│       └── main.go              # Application entry point
├── internal/
│   ├── api/
│   │   ├── handlers/            # HTTP request handlers
│   │   ├── middleware/          # HTTP middleware
│   │   └── routes/              # Route definitions
│   ├── models/
│   │   ├── registry.go          # Model registry
│   │   ├── manager.go           # Model management
│   │   └── container.go         # Container operations
│   ├── inference/
│   │   ├── engine.go            # Inference engine
│   │   ├── queue.go             # Request queue
│   │   └── scheduler.go         # Task scheduler
│   ├── config/
│   │   ├── manager.go           # Configuration management
│   │   └── validator.go         # Config validation
│   ├── storage/
│   │   ├── database.go          # Database operations
│   │   └── cache.go             # Caching layer
│   └── monitoring/
│       ├── metrics.go           # Metrics collection
│       └── health.go            # Health checks
├── pkg/
│   ├── container/
│   │   └── runtime.go           # Container runtime
│   └── utils/
│       └── helpers.go           # Utility functions
├── api/
│   └── openapi.yaml             # OpenAPI specification
├── deployments/
│   ├── docker/
│   │   ├── Dockerfile           # Docker image definition
│   │   └── docker-compose.yml   # Docker Compose setup
│   └── kubernetes/
│       └── manifests/           # Kubernetes manifests
├── configs/
│   ├── config.yaml              # Application configuration
│   └── models/                  # Model configurations
├── scripts/
│   ├── setup.sh                 # Setup script
│   └── deploy.sh                # Deployment script
├── docs/
│   ├── api.md                   # API documentation
│   ├── deployment.md            # Deployment guide
│   └── configuration.md         # Configuration guide
├── tests/
│   ├── unit/                    # Unit tests
│   └── integration/             # Integration tests
├── go.mod                       # Go module file
├── go.sum                       # Go dependencies checksum
└── README.md                    # This file
```

### Running Tests

```bash
# Run all tests
go test ./...

# Run unit tests
go test ./tests/unit/...

# Run integration tests
go test ./tests/integration/...

# Run with coverage
go test -cover ./...
```

### Building

```bash
# Build for current platform
go build -o bin/ai-provider cmd/server/main.go

# Build for Linux
GOOS=linux GOARCH=amd64 go build -o bin/ai-provider-linux cmd/server/main.go

# Build Docker image
docker build -t ai-provider:latest -f deployments/docker/Dockerfile .
```

## Deployment

### Docker Compose (Development)

```bash
docker-compose -f deployments/docker/docker-compose.yml up -d
```

### Kubernetes (Production)

```bash
# Apply Kubernetes manifests
kubectl apply -f deployments/kubernetes/manifests/

# Or using the deployment script
./scripts/deploy.sh
```

For detailed deployment instructions, see [docs/deployment.md](docs/deployment.md).

## Configuration

Configuration is managed through YAML files and can be overridden via environment variables.

### Environment Variables

All configuration options can be overridden using environment variables with the prefix `AI_PROVIDER_`:

```bash
export AI_PROVIDER_SYSTEM_PORT=9090
export AI_PROVIDER_COMPUTE_GPU_ENABLED=false
export AI_PROVIDER_LOGGING_LEVEL=DEBUG
```

For detailed configuration options, see [docs/configuration.md](docs/configuration.md).

## Monitoring

### Prometheus Metrics

Access metrics at `http://localhost:8080/metrics`

Key metrics include:
- Request latency and throughput
- Model inference times
- Resource utilization (CPU, GPU, memory)
- Container health status
- Queue lengths and processing rates

### Grafana Dashboard

Import the provided Grafana dashboard from `deployments/monitoring/grafana-dashboard.json` for visualization.

### Logging

Logs are written to both stdout and the configured log file. Log levels: DEBUG, INFO, WARN, ERROR

```bash
# View logs
tail -f /var/log/ai-provider.log

# Or with Docker
docker logs -f ai-provider
```

## Performance Targets

- **API Response Time**: < 100ms for model listing
- **Concurrent Models**: Support 10+ models
- **Uptime**: 99.9% availability
- **GPU Utilization**: > 80% when under load
- **Memory Efficiency**: Optimal allocation and cleanup

## Security

- **Authentication**: API key or JWT-based authentication
- **Authorization**: Role-based access control (RBAC)
- **Network Security**: TLS encryption, CORS policies
- **Container Isolation**: Each model runs in isolated container
- **Resource Limits**: Prevent resource exhaustion attacks

For security best practices, see [docs/security.md](docs/security.md).

## Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Support

- **Documentation**: [docs/](docs/)
- **Issues**: GitHub Issues
- **Discussions**: GitHub Discussions

## Roadmap

- [ ] Phase 1: Core Infrastructure (Week 1-2)
- [ ] Phase 2: Model Management (Week 3-4)
- [ ] Phase 3: Inference Engine (Week 5-6)
- [ ] Phase 4: Orchestration & Scaling (Week 7-8)
- [ ] Phase 5: Advanced Features (Week 9-10)
- [ ] Phase 6: Testing & Documentation (Week 11-12)

## Status

**Current Phase**: Phase 1 - Core Infrastructure
**Version**: 1.0.0-alpha
**Last Updated**: 2025-06-17

---

Built with ❤️ by the AI Architecture Team