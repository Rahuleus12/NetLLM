# Netllm

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.21%2B-00ADD8)](ai-provider/go.mod)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](CONTRIBUTING.md)
[![Code of Conduct](https://img.shields.io/badge/Contributor%20Covenant-v2.1-ff69b4.svg)](CODE_OF_CONDUCT.md)

A network-accessible, containerized local AI provider platform that runs AI models with full management capabilities, API access, and enterprise-grade features.

## Overview

Netllm provides a comprehensive solution for managing and serving AI models locally. It features a robust API gateway, model orchestration, containerized model runtime environments, and built-in monitoring, scaling, and security—making it ideal for organizations that need private, on-premises AI infrastructure.

## Quick Links

| Resource | Description |
|----------|-------------|
| [Contributing Guide](CONTRIBUTING.md) | How to contribute to Netllm |
| [Code of Conduct](CODE_OF_CONDUCT.md) | Community standards (Contributor Covenant v2.1) |
| [Security Policy](SECURITY.md) | How to report vulnerabilities |
| [Changelog](CHANGELOG.md) | Release history and changes |
| [Project Source](ai-provider/) | Main application source code |
| [API Documentation](ai-provider/api/openapi.yaml) | OpenAPI 3.0 specification |
| [Python SDK](ai-provider/sdk/python/netllm_client.py) | Python client library (zero dependencies) |
| [JavaScript SDK](ai-provider/sdk/javascript/netllm-client.js) | JavaScript client library |
| [Roadmap & Planning](Plan/) | Project roadmap and phase documentation |

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    API Gateway Layer                        │
│  (REST API + SSE Streaming + Auth + Middleware Stack)       │
└────────────────────┬────────────────────────────────────────┘
                     │
┌────────────────────┴────────────────────────────────────────┐
│                  Orchestration Layer                        │
│  ┌──────────────┬──────────────┬──────────────┬──────────┐  │
│  │   Model      │   Resource   │   Config     │  Health  │  │
│  │   Manager    │   Manager    │   Manager    │  Monitor │  │
│  └──────────────┴──────────────┴──────────────┴──────────┘  │
└────────────────────┬────────────────────────────────────────┘
                     │
┌────────────────────┴────────────────────────────────────────┐
│                  Model Runtime Layer                        │
│  ┌──────────────┬──────────────┬──────────────┬──────────┐  │
│  │  Model A     │  Model B     │  Model C     │  Model N │  │
│  │  Container   │  Container   │  Container   │ Container│  │
│  └──────────────┴──────────────┴──────────────┴──────────┘  │
└────────────────────┬────────────────────────────────────────┘
                     │
┌────────────────────┴────────────────────────────────────────┐
│              Infrastructure Layer                           │
│  ┌──────────────┬──────────────┬──────────────┬──────────┐  │
│  │   Storage    │   GPU/FPGA   │   Network    │  Logging │  │
│  │   (Models)   │   Resources  │   Layer      │ & Metrics│  │
│  └──────────────┴──────────────┴──────────────┴──────────┘  │
└─────────────────────────────────────────────────────────────┘
```

## Features

### Core Capabilities
- **Model Management** — Download, version, validate, and manage multiple AI models
- **Containerized Runtime** — Each model runs in its own isolated container
- **RESTful API** — Complete API for model management and inference
- **WebSocket Streaming** — Real-time streaming for inference responses
- **Batch Inference** — Process multiple requests efficiently
- **Resource Management** — Intelligent GPU/CPU allocation and optimization

### Security & Authentication
- **JWT Authentication** — Secure token-based authentication
- **OAuth2 Integration** — Support for external identity providers
- **RBAC Authorization** — Role-based access control
- **API Key Management** — Scoped API keys for programmatic access
- **2FA Support** — TOTP-based two-factor authentication
- **Audit Logging** — Comprehensive audit trail for all operations
- **Encryption** — Data encryption at rest and in transit

### Multi-Tenancy
- **Organization Management** — Hierarchical organization structures
- **Workspace System** — Isolated workspaces per team/project
- **Resource Isolation** — Tenant-specific resource allocation
- **Usage Tracking** — Per-tenant usage metrics and quotas
- **Billing Integration** — Framework for usage-based billing

### Monitoring & Analytics
- **Prometheus Metrics** — Comprehensive metrics collection
- **Health Monitoring** — Real-time health checks and alerts
- **Cost Analytics** — GPU/compute cost tracking and optimization
- **Performance Dashboard** — Real-time visualization
- **Alerting System** — Configurable alerting rules
- **Reporting** — Scheduled and on-demand reports

### Integration & Extensibility
- **Plugin System** — Extensible architecture for custom functionality
- **Integration Hub** — Pre-built integrations with popular tools
- **Webhook System** — Event-driven notifications
- **Multi-Language SDKs** — Go, Python, JavaScript, Java
- **CLI Tools** — Command-line interface for management

### Deployment & Operations
- **Kubernetes Native** — Full K8s support with Helm charts
- **GitOps Ready** — ArgoCD/Flux integration
- **Docker Compose** — Simple local development setup
- **Disaster Recovery** — Backup and restore capabilities
- **Infrastructure as Code** — Terraform support

## Project Status

| Phase | Description | Status |
|-------|-------------|--------|
| 1 | Core Infrastructure | ✅ Complete |
| 2 | Model Management | ✅ Complete |
| 3 | Inference Engine | ✅ Complete |
| 4 | Advanced Features | ✅ Complete |
| 5 | Security & Authentication | ✅ Complete |
| 6 | Multi-tenancy | ✅ Complete |
| 7 | Monitoring & Analytics | ✅ Complete |
| 8 | Integration & Extensibility | ✅ Complete |
| 9 | Deployment & Operations | ✅ Complete |
| 10 | Enterprise Features | ✅ Complete |

**Overall Progress**: 100% | **Lines of Code**: ~75,000 | **Version**: 2.0.0

## Features

- **Model Management** — Download, version, validate, activate/deactivate AI models (GGUF, ONNX, PyTorch)
- **Inference API** — Synchronous, asynchronous, batch, and SSE streaming inference endpoints
- **Authentication** — API Key and JWT Bearer token auth with RBAC, scopes, and rate limiting
- **System Monitoring** — Health checks, Prometheus metrics, runtime diagnostics, and GC stats
- **Multi-Tenancy** — Tenant isolation, quotas, organization and workspace management
- **Plugin System** — Extensible architecture with sandboxed plugins and marketplace
- **High Availability** — Failover, load balancing, rolling updates, multi-region support
- **Enterprise Billing** — Subscription plans, Stripe integration, usage metering, and invoicing
- **SDKs** — Python and JavaScript client libraries with zero external dependencies
- **Deployment** — Docker, Docker Compose, Kubernetes manifests, and GitOps support
- **CI/CD** — GitHub Actions workflows for testing, security scanning, and multi-platform releases

## Quick Start

### Prerequisites

- Go 1.21+
- Docker 20.10+
- Docker Compose 2.0+
- PostgreSQL 15+
- Redis 7+
- NVIDIA GPU with CUDA support (optional, for GPU acceleration)

### Installation

```bash
# Clone the repository
git clone https://github.com/your-org/Netllm.git
cd Netllm/ai-provider

# Install dependencies
go mod download

# Copy and edit configuration
cp configs/config.yaml.example configs/config.yaml
```

### Running with Docker Compose

```bash
# Start all services (app + PostgreSQL + Redis)
docker-compose -f deployments/docker/docker-compose.yml up -d

# View logs
docker-compose -f deployments/docker/docker-compose.yml logs -f
```

### Running Locally

```bash
# Start infrastructure services
docker-compose -f deployments/docker/docker-compose.yml up -d postgres redis

# Run the application
go run cmd/server/main.go

# Or build and run
make build
./bin/ai-provider
```

### Verify Installation

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

## Project Structure

```
Netllm/
├── .github/                    # GitHub Actions CI/CD + Issue/PR templates
├── ai-provider/                # Main Go application
│   ├── cmd/                    # Entry points (server, CLI)
│   ├── internal/               # Internal packages (30+ modules)
│   │   ├── api/handlers/       # HTTP handlers (models, inference, system, auth)
│   │   ├── inference/          # Inference engine, scheduler, GPU, batch, cache
│   │   ├── models/             # Model registry, download manager, versions
│   │   ├── auth/               # JWT, API keys, OAuth2, MFA, sessions
│   │   ├── authz/              # RBAC, ACL, policies, permissions
│   │   ├── config/             # Configuration management (Viper)
│   │   ├── storage/            # PostgreSQL database, Redis cache
│   │   ├── monitoring/         # Health checks, Prometheus metrics
│   │   ├── plugins/            # Plugin system, loader, marketplace, sandbox
│   │   ├── tenant/             # Multi-tenancy, isolation, quotas
│   │   ├── billing/            # Plans, invoices, Stripe integration
│   │   └── ...                 # Analytics, audit, dashboard, HA, and more
│   ├── pkg/                    # Public packages (container runtime, utils)
│   ├── api/                    # OpenAPI 3.0 specification
│   ├── sdk/                    # Client SDKs (Python, JavaScript, Go, Java)
│   ├── deployments/            # Docker, Kubernetes, GitOps configs
│   ├── configs/                # Configuration files
│   ├── scripts/                # Setup and deployment scripts
│   ├── docs/                   # API docs, deployment guides
│   └── tests/                  # Unit and integration tests
├── Plan/                       # Project roadmap and phase documentation
├── LICENSE                     # MIT License
├── CONTRIBUTING.md             # Contribution guidelines
├── CODE_OF_CONDUCT.md          # Contributor Covenant v2.1
├── SECURITY.md                 # Security policy
├── CHANGELOG.md                # Release changelog
└── README.md                   # This file
```

## SDKs

### Python (zero dependencies)

```python
from netllm_client import NetllmClient

client = NetllmClient(base_url="http://localhost:8080", api_key="your-key")

# Text completion
response = client.inference("llama-3-8b", prompt="Hello, world!")
print(response.content)

# Chat completion
response = client.chat("llama-3-8b", messages=[
    {"role": "user", "content": "What is AI?"},
])

# Streaming
for chunk in client.inference_stream("llama-3-8b", prompt="Tell me a story"):
    print(chunk.delta, end="", flush=True)

# Batch inference
result = client.batch_inference("llama-3-8b", prompts=["Hi", "Hello", "Hey"])
```

### JavaScript (Node.js 18+ / Browser)

```javascript
import { NetllmClient } from './netllm-client.js';

const client = new NetllmClient({
  baseUrl: 'http://localhost:8080',
  apiKey: 'your-key',
});

// Text completion
const response = await client.inference('llama-3-8b', {
  prompt: 'Hello, world!',
});
console.log(response.content);

// Streaming
for await (const chunk of client.inferenceStream('llama-3-8b', {
  prompt: 'Tell me a story',
})) {
  process.stdout.write(chunk.delta);
}
```

## API Documentation

Access the full API documentation at:
- **Swagger UI**: http://localhost:8080/api/docs
- **OpenAPI Spec**: `ai-provider/api/openapi.yaml`

### Key Endpoints

#### Model Management
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/v1/models` | List all models |
| POST | `/api/v1/models` | Register a new model |
| GET | `/api/v1/models/{id}` | Get model details |
| PUT | `/api/v1/models/{id}` | Update model configuration |
| DELETE | `/api/v1/models/{id}` | Remove a model |

#### Inference
| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/v1/inference/{model_id}` | Run inference |
| WS | `/api/v1/inference/{model_id}/stream` | Stream inference via WebSocket |
| POST | `/api/v1/inference/batch` | Batch inference |
| POST | `/api/v1/chat/completions` | Chat completion (OpenAI-compatible) |

#### Authentication
| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/v1/auth/login` | User login |
| POST | `/api/v1/auth/register` | User registration |
| POST | `/api/v1/auth/refresh` | Refresh token |
| POST | `/api/v1/auth/2fa/enable` | Enable 2FA |

#### Monitoring
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Health check |
| GET | `/metrics` | Prometheus metrics |
| GET | `/api/v1/status` | System status |
| GET | `/api/v1/analytics/dashboard` | Analytics data |

## Project Structure

```
Netllm/
├── README.md                    # This file
├── Plan/                        # Project planning documents
│   ├── COMPLETE_ROADMAP.md      # Full project roadmap
│   ├── STATUS_SUMMARY.md        # Current status summary
│   └── PHASE*_*.md              # Phase-specific plans & summaries
└── ai-provider/                 # Main application
    ├── cmd/
    │   ├── server/              # API server entry point
    │   └── cli/                 # CLI tool entry point
    ├── internal/
    │   ├── api/                 # HTTP handlers, middleware, routes
    │   ├── auth/                # Authentication (JWT, OAuth2, 2FA)
    │   ├── authz/               # Authorization (RBAC)
    │   ├── billing/             # Billing integration
    │   ├── compliance/          # Compliance features
    │   ├── config/              # Configuration management
    │   ├── crypto/              # Cryptographic utilities
    │   ├── dashboard/           # Dashboard data providers
    │   ├── disaster/            # Disaster recovery
    │   ├── enterprise/          # Enterprise features
    │   ├── ha/                  # High availability
    │   ├── inference/           # Inference engine (~10,000 LOC)
    │   ├── integrations/        # External integrations
    │   ├── models/              # Model management (~5,500 LOC)
    │   ├── monitoring/          # Metrics & health checks
    │   ├── multiregion/         # Multi-region support
    │   ├── operations/          # Operational tools
    │   ├── organization/        # Multi-tenancy
    │   ├── plugins/             # Plugin system
    │   ├── reporting/           # Report generation
    │   ├── security/            # Security hardening
    │   ├── storage/             # Database & cache
    │   ├── support/             # Support tools
    │   ├── tenant/              # Tenant management
    │   ├── usage/               # Usage tracking
    │   ├── webhooks/            # Webhook system
    │   └── workspace/           # Workspace management
    ├── pkg/
    │   ├── container/           # Container runtime
    │   └── utils/               # Utility functions
    ├── sdk/                     # Client SDKs
    │   ├── go/                  # Go SDK
    │   ├── python/              # Python SDK
    │   ├── javascript/          # JavaScript SDK
    │   └── java/                # Java SDK
    ├── deployments/
    │   ├── docker/              # Docker & Docker Compose
    │   ├── kubernetes/          # K8s manifests & Helm charts
    │   └── gitops/              # GitOps configurations
    ├── configs/                 # Configuration files
    ├── scripts/                 # Setup & deployment scripts
    ├── docs/                    # Documentation
    ├── tests/                   # Test suites
    ├── api/                     # OpenAPI specifications
    ├── Makefile                 # Build & development tasks
    ├── go.mod                   # Go module definition
    └── go.sum                   # Dependency checksums
```

## Development

### Common Commands

```bash
make help              # Show all available commands
make build             # Build the application
make run               # Build and run
make dev               # Run with hot reload (requires air)
make test              # Run all tests
make test-coverage     # Run tests with coverage report
make check             # Run fmt, vet, lint, and tests
make docker-build      # Build Docker image
make docker-compose-up # Start with Docker Compose
make kube-deploy       # Deploy to Kubernetes
make security-scan     # Run vulnerability scan
```

### Configuration

Configuration is managed through YAML files with environment variable overrides:

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

security:
  auth_enabled: true
  jwt_secret: "your-secret-key"
  rbac_enabled: true

monitoring:
  prometheus_enabled: true
  metrics_interval: 15
```

Override any setting via environment variables with the `AI_PROVIDER_` prefix:

```bash
export AI_PROVIDER_SYSTEM_PORT=9090
export AI_PROVIDER_COMPUTE_GPU_ENABLED=false
export AI_PROVIDER_LOGGING_LEVEL=DEBUG
```

### Monitoring

- **Prometheus Metrics**: http://localhost:9090/metrics
- **Health Check**: http://localhost:8080/health
- **Logs**: `docker logs -f ai-provider` or `/var/log/ai-provider.log`

Key metrics include:
- Request latency and throughput
- Model inference times
- GPU/CPU/memory utilization
- Container health status
- Queue lengths and processing rates

## Deployment

### Docker (Development)

```bash
docker-compose -f deployments/docker/docker-compose.yml up -d
```

### Kubernetes (Production)

```bash
# Using Helm
helm install ai-provider deployments/kubernetes/helm/ai-provider

# Using kubectl
kubectl apply -f deployments/kubernetes/manifests/

# Using GitOps (ArgoCD)
kubectl apply -f deployments/gitops/
```

### Production Build

```bash
make prod-build        # Build for all platforms + Docker image
make prod-deploy       # Push to registry + deploy to K8s
```

See [docs/LOCAL_DEPLOYMENT_GUIDE.md](ai-provider/docs/LOCAL_DEPLOYMENT_GUIDE.md) for detailed deployment instructions.

## SDKs

### Go

```go
import "github.com/ai-provider/sdk/go"

client := sdk.NewClient("http://localhost:8080", "your-api-key")
result, err := client.Inference.Run(ctx, "model-id", prompt)
```

### Python

```python
from ai_provider import Client

client = Client(base_url="http://localhost:8080", api_key="your-api-key")
result = client.inference.run("model-id", "Your prompt here")
```

### JavaScript

```javascript
import { Client } from '@ai-provider/sdk';

const client = new Client({ baseUrl: 'http://localhost:8080', apiKey: 'your-api-key' });
const result = await client.inference.run('model-id', 'Your prompt here');
```

## Performance Targets

| Metric | Target |
|--------|--------|
| API Response Time | < 100ms (model listing) |
| Concurrent Models | 10+ |
| Uptime | 99.9% |
| GPU Utilization | > 80% under load |
| Memory Efficiency | Optimal allocation |

## Tech Stack

- **Language**: Go 1.21
- **Web Framework**: Gorilla Mux
- **Database**: PostgreSQL
- **Cache**: Redis
- **Configuration**: Viper
- **Metrics**: Prometheus Client
- **Authentication**: JWT, OAuth2, TOTP
- **Containerization**: Docker, Kubernetes
- **CI/CD**: GitOps (ArgoCD/Flux)

## Documentation

| Document | Description |
|----------|-------------|
| [API Documentation](ai-provider/docs/api.md) | Complete API reference |
| [Local Deployment Guide](ai-provider/docs/LOCAL_DEPLOYMENT_GUIDE.md) | Step-by-step deployment |
| [Plan/Roadmap](Plan/COMPLETE_ROADMAP.md) | Full project roadmap |
| [Status Summary](Plan/STATUS_SUMMARY.md) | Current project status |

## Contributing

Contributions are welcome! Please follow these steps:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/my-feature`)
3. Run `make check` to ensure code quality
4. Commit your changes (`git commit -am 'Add new feature'`)
5. Push to the branch (`git push origin feature/my-feature`)
6. Create a Pull Request

## License

This project is licensed under the MIT License.

## Support

- **Issues**: GitHub Issues
- **Discussions**: GitHub Discussions
- **Documentation**: [docs/](ai-provider/docs/)