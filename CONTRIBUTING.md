# Contributing to Netllm

First off, thank you for considering contributing to **Netllm**! It's people like you that make Netllm such a great tool for the AI and developer community. We welcome contributions of all kinds — bug reports, feature ideas, code improvements, documentation fixes, and more.

By participating in this project, you agree to abide by our [Code of Conduct](CODE_OF_CONDUCT.md). Please read it before contributing.

---

## Table of Contents

- [Getting Started](#getting-started)
  - [Prerequisites](#prerequisites)
- [Development Setup](#development-setup)
  - [Fork and Clone](#fork-and-clone)
  - [Create a Branch](#create-a-branch)
  - [Install Dependencies](#install-dependencies)
  - [Run Tests](#run-tests)
  - [Run Locally](#run-locally)
- [How to Contribute](#how-to-contribute)
  - [Bug Reports](#bug-reports)
  - [Feature Requests](#feature-requests)
  - [Pull Requests](#pull-requests)
- [Development Workflow](#development-workflow)
- [Coding Standards](#coding-standards)
  - [Go Style Guide](#go-style-guide)
  - [Formatting](#formatting)
  - [Linting](#linting)
  - [Test Coverage](#test-coverage)
- [Commit Messages](#commit-messages)
- [Pull Request Process](#pull-request-process)
  - [PR Template Requirements](#pr-template-requirements)
  - [Review Process](#review-process)
  - [CI Checks](#ci-checks)
- [Testing Guidelines](#testing-guidelines)
  - [Unit Tests](#unit-tests)
  - [Integration Tests](#integration-tests)
  - [Test Naming Conventions](#test-naming-conventions)
- [Documentation](#documentation)
- [Community](#community)

---

## Getting Started

### Prerequisites

Before you begin, ensure you have the following installed on your system:

| Requirement        | Minimum Version | Installation Link                                              |
| ------------------ | --------------- | -------------------------------------------------------------- |
| **Go**             | 1.21+           | [golang.org/dl](https://golang.org/dl/)                       |
| **Docker**         | 20.10+          | [docs.docker.com/get-docker](https://docs.docker.com/get-docker/) |
| **Docker Compose** | 2.0+            | Included with Docker Desktop                                   |
| **PostgreSQL**     | 15+             | [postgresql.org/download](https://www.postgresql.org/download/) |
| **Redis**          | 7+              | [redis.io/download](https://redis.io/download)                 |
| **Git**            | 2.30+           | [git-scm.com](https://git-scm.com/)                           |

#### Optional but Recommended

- **golangci-lint** — Go linter aggregator ([install guide](https://golangci-lint.run/usage/install/))
- **air** — Hot reload for Go development (`go install github.com/cosmtrek/air@latest`)
- **swag** — Swagger documentation generator (`go install github.com/swaggo/swag/cmd/swag@latest`)
- **govulncheck** — Security vulnerability scanner (`go install golang.org/x/vuln/cmd/govulncheck@latest`)
- **pre-commit** — Git hook manager ([pre-commit.com](https://pre-commit.com/))

Verify your installation:

```bash
go version          # go1.21.x or higher
docker --version    # Docker version 20.10.x or higher
docker compose version  # Docker Compose version v2.x
psql --version      # psql 15.x or higher
redis-server --version  # Redis v=7.x
git --version       # git version 2.30.x or higher
```

---

## Development Setup

### Fork and Clone

1. **Fork** the repository on GitHub by clicking the "Fork" button at the top right of the repository page.

2. **Clone** your fork locally:

```bash
git clone https://github.com/YOUR_USERNAME/ai-provider.git
cd ai-provider
```

3. **Add the upstream remote** to keep your fork in sync with the original:

```bash
git remote add upstream https://github.com/netllm/ai-provider.git
git remote -v
```

You should see both `origin` (your fork) and `upstream` (the original).

### Create a Branch

Always create a new branch for your work. Never work directly on `main`.

```bash
git checkout main
git pull upstream main
git checkout -b feat/my-new-feature
```

Use descriptive branch names prefixed with the type of change:
- `feat/add-batch-inference` — new features
- `fix/memory-leak-handler` — bug fixes
- `docs/update-api-guide` — documentation
- `refactor/simplify-config` — code refactoring
- `test/add-integration-tests` — test additions

### Install Dependencies

Download all Go module dependencies:

```bash
go mod download
go mod verify
```

Or use the Makefile shortcut:

```bash
make deps
```

### Run Tests

Verify everything is working by running the test suite:

```bash
# Run all tests
make test

# Or run with verbose output
make test-verbose

# Run with race detection
make test-race
```

### Run Locally

1. **Start infrastructure services** (PostgreSQL and Redis) using Docker Compose:

```bash
docker compose -f deployments/docker/docker-compose.yml up -d postgres redis
```

2. **Configure the application**:

```bash
cp configs/config.yaml.example configs/config.yaml
```

Edit `configs/config.yaml` with your local development settings.

3. **Run the application**:

```bash
# Build and run
make run

# Or run directly with Go
go run cmd/server/main.go

# Or use hot reload during development
make dev
```

4. **Verify** the server is running:

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

---

## How to Contribute

### Bug Reports

If you find a bug, please [open an issue](https://github.com/netllm/ai-provider/issues/new?labels=bug&template=bug_report.md) and include:

- **A clear title** describing the problem
- **Steps to reproduce** the issue
- **Expected behavior** vs. **actual behavior**
- **Environment details**: OS, Go version, Docker version, PostgreSQL/Redis versions
- **Relevant logs or error messages**
- **Screenshots** (if applicable)

Please search existing issues before opening a new one to avoid duplicates.

### Feature Requests

We love feature ideas! [Open a feature request](https://github.com/netllm/ai-provider/issues/new?labels=enhancement&template=feature_request.md) and include:

- **A clear title** summarizing the feature
- **Use case**: What problem does this solve?
- **Proposed solution**: How should it work?
- **Alternatives considered**: Other approaches you've thought about
- **Additional context**: Mockups, examples, or references

### Pull Requests

We actively welcome pull requests. Here's a quick overview:

1. Fork the repository and create your branch from `main`
2. Make your changes with appropriate tests
3. Ensure the test suite passes (`make test`)
4. Ensure code is linted (`make lint`)
5. Ensure code is formatted (`make fmt`)
6. Commit with a [conventional commit message](#commit-messages)
7. Open a pull request against the `main` branch

See the [Pull Request Process](#pull-request-process) section for full details.

---

## Development Workflow

Follow this workflow for every contribution:

```
1. Sync your fork      → git pull upstream main
2. Create a branch     → git checkout -b type/description
3. Make changes        → write code + tests
4. Run tests           → make test
5. Run linting         → make lint
6. Format code         → make fmt
7. Commit changes      → git commit -m "type: description"
8. Push to fork        → git push origin type/description
9. Open Pull Request   → against netllm/ai-provider main
10. Address reviews    → push fixes to the same branch
11. Merge              → once approved and CI passes
```

### Keeping Your Fork Updated

Regularly sync your fork with the upstream repository:

```bash
git fetch upstream
git checkout main
git merge upstream/main
git push origin main
```

---

## Coding Standards

### Go Style Guide

We follow the official [Effective Go](https://go.dev/doc/effective_go) guidelines and [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments). Key principles:

- **Package names** should be lowercase, single-word, and without underscores
- **Exported names** should have clear, descriptive doc comments
- **Error handling**: Always check errors. Never use `_` to discard error values unless explicitly justified
- **Interfaces**: Define interfaces where they are consumed, not where they are implemented
- **Contexts**: Accept `context.Context` as the first parameter in functions that perform I/O
- **Naming conventions**:
  - `camelCase` for unexported variables and functions
  - `PascalCase` for exported names
  - Acronyms should be all caps: `HTTPClient`, not `HttpClient`
  - Single-letter variables only for short scopes (e.g., loop variables)

Example:

```go
// ModelManager handles the lifecycle of AI models including registration,
// loading, unloading, and health monitoring.
type ModelManager struct {
    registry *ModelRegistry
    store    Storage
    logger   *slog.Logger
}

// RegisterModel adds a new model to the manager and persists it to storage.
// Returns an error if the model ID already exists or if storage fails.
func (m *ModelManager) RegisterModel(ctx context.Context, model *Model) error {
    if model == nil {
        return fmt.Errorf("model cannot be nil")
    }
    if err := m.registry.Add(model); err != nil {
        return fmt.Errorf("failed to register model %s: %w", model.ID, err)
    }
    return m.store.Save(ctx, model)
}
```

### Formatting

All code must be formatted with `gofmt` (or `go fmt`). Run:

```bash
make fmt
# or
go fmt ./...
```

Do not configure your editor to use alternative formatting tools. `gofmt` is the canonical Go formatter.

### Linting

We use [golangci-lint](https://golangci-lint.run/) for static analysis. Run:

```bash
make lint
# or
golangci-lint run ./...
```

The following linters are enabled (configured in `.golangci.yml`):

| Linter          | Purpose                                      |
| --------------- | -------------------------------------------- |
| `errcheck`      | Check for unchecked errors                   |
| `govet`         | Reports suspicious constructs                |
| `staticcheck`   | Advanced Go static analysis                  |
| `unused`        | Finds unused constants, functions, and types |
| `gosimple`      | Simplifies code                              |
| `ineffassign`   | Detects ineffectual assignments              |
| `typecheck`     | Type-checks the code                         |
| `gocritic`      | Opinionated style checker                    |
| `goconst`       | Finds repeated strings                       |
| `misspell`      | Checks for spelling errors                   |
| `revive`        | Fast, configurable linter                    |

All linting issues must be resolved before a PR can be merged.

### Test Coverage

We aim for a minimum of **80% test coverage** for all new code. Coverage is checked in CI.

```bash
# Generate coverage report
make test-coverage

# View coverage in browser
open coverage/coverage.html
```

---

## Commit Messages

We use [Conventional Commits](https://www.conventionalcommits.org/) for all commit messages. This allows us to automatically generate changelogs and semantically version releases.

### Format

```
<type>(<scope>): <subject>

[optional body]

[optional footer(s)]
```

### Types

| Type         | Description                                                  |
| ------------ | ------------------------------------------------------------ |
| `feat`       | A new feature                                                |
| `fix`        | A bug fix                                                    |
| `docs`       | Documentation-only changes                                   |
| `style`      | Code style changes (formatting, semicolons, etc.)           |
| `refactor`   | Code changes that neither fix a bug nor add a feature       |
| `perf`       | Performance improvements                                     |
| `test`       | Adding or updating tests                                     |
| `build`      | Changes to build system or dependencies                      |
| `ci`         | Changes to CI configuration                                  |
| `chore`      | Other changes that don't modify src or test files            |
| `revert`     | Reverts a previous commit                                    |

### Scopes

Common scopes include:

- `api` — API handlers, routes, and middleware
- `models` — Model management and registry
- `inference` — Inference engine and scheduler
- `config` — Configuration management
- `storage` — Database and caching layer
- `monitoring` — Metrics and health checks
- `docker` — Docker and container-related changes
- `auth` — Authentication and authorization
- `sdk` — SDK and client libraries

### Examples

```
feat(api): add batch inference endpoint

Implement POST /api/v1/inference/batch to support processing
multiple inference requests in a single API call. Requests are
queued and processed concurrently based on available resources.

Closes #142
```

```
fix(storage): resolve connection pool exhaustion under high load

The Redis connection pool was not properly releasing connections
when inference requests timed out. Added context-aware connection
handling with proper cleanup in deferred functions.

Fixes #198
```

```
docs(api): update authentication guide with JWT examples
```

```
refactor(inference): simplify scheduler queue management
```

```
test(models): add unit tests for model registry
```

### Rules

1. **Subject line** must be 72 characters or fewer
2. Use **imperative mood** in the subject: "add feature" not "added feature"
3. **Do not end** the subject line with a period
4. **Body** should be wrapped at 80 characters
5. Use the body to explain **what** and **why**, not **how**
6. Reference issues and PRs in the **footer**: `Closes #123` or `Fixes #456`

---

## Pull Request Process

### PR Template Requirements

Every pull request must include the following information (use our PR template):

```markdown
## Description
Brief description of the changes in this PR.

## Related Issue
Closes #<issue_number>

## Type of Change
- [ ] Bug fix (non-breaking change that fixes an issue)
- [ ] New feature (non-breaking change that adds functionality)
- [ ] Breaking change (fix or feature that would cause existing functionality to not work as expected)
- [ ] Documentation update
- [ ] Refactoring
- [ ] Performance improvement
- [ ] Test addition/update

## Changes Made
- List of specific changes made

## Testing
Describe the tests you ran to verify your changes:
- [ ] Unit tests pass (`make test`)
- [ ] Integration tests pass
- [ ] Manual testing performed (describe steps)

## Checklist
- [ ] My code follows the project's coding standards
- [ ] I have performed a self-review of my code
- [ ] I have commented my code, particularly in hard-to-understand areas
- [ ] I have made corresponding changes to the documentation
- [ ] My changes generate no new warnings
- [ ] I have added tests that prove my fix is effective or that my feature works
- [ ] New and existing unit tests pass locally with my changes
- [ ] Any dependent changes have been merged and published
```

### Review Process

1. **Automated Checks**: All CI checks must pass before review begins
2. **Self-Review**: Review your own PR first and address any obvious issues
3. **Peer Review**: At least **one** approving review from a maintainer is required
4. **Changes Requested**: Address all feedback by pushing new commits to the same branch
5. **Approval**: Once approved, a maintainer will merge your PR

### CI Checks

All pull requests must pass the following automated checks:

| Check                | Command                    | Description                         |
| -------------------- | -------------------------- | ----------------------------------- |
| **Build**            | `make build`               | Project compiles successfully       |
| **Unit Tests**       | `make test`                | All unit tests pass                 |
| **Race Detection**   | `make test-race`           | No race conditions detected         |
| **Linting**          | `make lint`                | No linting errors                   |
| **Formatting**       | `go fmt ./...`             | Code is properly formatted          |
| **Vet**              | `go vet ./...`             | No vet warnings                     |
| **Security Scan**    | `make security-scan`       | No known vulnerabilities            |
| **Coverage**         | `make test-coverage`       | Minimum 80% coverage for new code   |
| **Docker Build**     | `make docker-build`        | Docker image builds successfully    |

You can run all checks locally before pushing:

```bash
make check
```

Or run the full CI pipeline:

```bash
make ci
```

---

## Testing Guidelines

### Unit Tests

Unit tests should be placed alongside the code they test, following Go conventions:

```
internal/models/
├── manager.go
├── manager_test.go
├── registry.go
└── registry_test.go
```

Write unit tests for:
- All exported functions and methods
- Edge cases and boundary conditions
- Error handling paths
- Concurrent access patterns

Example:

```go
package models

import (
    "context"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestModelManager_RegisterModel(t *testing.T) {
    tests := []struct {
        name        string
        model       *Model
        expectError bool
    }{
        {
            name: "valid model registration",
            model: &Model{
                ID:   "test-model-001",
                Name: "Test Model",
                Type: ModelTypeLLM,
            },
            expectError: false,
        },
        {
            name:        "nil model returns error",
            model:       nil,
            expectError: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            mgr := NewModelManager(newMockRegistry(), newMockStorage())
            err := mgr.RegisterModel(context.Background(), tt.model)

            if tt.expectError {
                require.Error(t, err)
                return
            }
            require.NoError(t, err)
        })
    }
}
```

### Integration Tests

Integration tests validate the interaction between multiple components. Place them in the `tests/integration/` directory:

```
tests/
├── integration/
│   ├── api_test.go
│   ├── inference_test.go
│   ├── model_lifecycle_test.go
│   └── testhelpers/
│       └── setup.go
└── unit/
```

Integration tests should:
- Use real database connections (via Docker Compose test services)
- Test complete API flows from request to response
- Validate data persistence across service restarts
- Use dedicated test databases and namespaces

Run integration tests:

```bash
go test ./tests/integration/... -tags=integration
```

Or with verbose output:

```bash
go test -v ./tests/integration/... -tags=integration
```

### Test Naming Conventions

Follow these naming conventions for consistency:

| Pattern                                         | Example                                              |
| ----------------------------------------------- | ---------------------------------------------------- |
| `Test<Function>_<Scenario>_<ExpectedResult>`    | `TestRegisterModel_ValidInput_Success`               |
| `Test<Function>_<Input>_ReturnsError`           | `TestRegisterModel_NilModel_ReturnsError`            |
| `Test<Struct>_<Method>_<Scenario>`              | `TestModelManager_RegisterModel_DuplicateID`         |

Use **table-driven tests** for testing multiple scenarios:

```go
func TestValidateConfig(t *testing.T) {
    tests := []struct {
        name    string
        config  Config
        wantErr bool
    }{
        {
            name:    "valid config",
            config:  Config{Port: 8080, Workers: 4},
            wantErr: false,
        },
        {
            name:    "invalid port",
            config:  Config{Port: -1, Workers: 4},
            wantErr: true,
        },
        {
            name:    "zero workers",
            config:  Config{Port: 8080, Workers: 0},
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateConfig(tt.config)
            if tt.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

---

## Documentation

Good documentation is critical. When contributing, please:

### Update Documentation with Code Changes

- **New features**: Add or update relevant documentation in `docs/`
- **API changes**: Update `api/openapi.yaml` and `docs/api.md`
- **Configuration changes**: Update `docs/configuration.md` and `configs/config.yaml.example`
- **New dependencies**: Update the README prerequisites table
- **Behavior changes**: Update any affected documentation

### Documentation Standards

- Write in **clear, concise English**
- Use **active voice**: "The API returns..." not "It is returned by the API..."
- Include **code examples** for new features
- Keep **README.md** as a high-level overview; detailed guides go in `docs/`
- Use **relative links** for internal documentation references

### Generating API Documentation

If you modify API handlers, regenerate Swagger documentation:

```bash
make swagger
```

### Generating Package Documentation

```bash
make docs
```

---

## Community

We're excited to have you as part of the Netllm community! Here's where you can connect with us:

- **GitHub Discussions**: [github.com/netllm/ai-provider/discussions](https://github.com/netllm/ai-provider/discussions) — Ask questions, share ideas, and discuss the project
- **Discord**: [Join our Discord](https://discord.gg/netllm) — Chat with maintainers and contributors in real-time
- **Issue Tracker**: [github.com/netllm/ai-provider/issues](https://github.com/netllm/ai-provider/issues) — Report bugs and request features
- **Twitter/X**: [@netllm](https://twitter.com/netllm) — Follow for project updates and announcements

### Ways to Get Involved

- **Contribute code** — See [How to Contribute](#how-to-contribute)
- **Improve documentation** — Fix typos, add examples, clarify confusing sections
- **Answer questions** — Help others in GitHub Discussions and Discord
- **Review PRs** — Provide feedback on open pull requests
- **Share the project** — Star the repo, write about it, present at meetups

---

Thank you for contributing to Netllm! Your efforts help make AI more accessible and manageable for everyone. 🚀

_This contributing guide is licensed under the same license as the Netllm project._