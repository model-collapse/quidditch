# Quidditch Development Setup Guide

**Version**: 1.0.0
**Date**: 2026-01-25
**Audience**: Engineers joining the Quidditch implementation team

---

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Development Environment](#development-environment)
3. [Repository Setup](#repository-setup)
4. [Build Tools](#build-tools)
5. [Dependencies](#dependencies)
6. [IDE Configuration](#ide-configuration)
7. [Running Tests](#running-tests)
8. [Local Deployment](#local-deployment)
9. [Debugging](#debugging)
10. [Contributing Guidelines](#contributing-guidelines)

---

## 1. Prerequisites

### Required Software

**Operating System**:
- Linux (Ubuntu 22.04+ recommended)
- macOS 13+ (for development only)
- Docker Desktop (for containerized development)

**Core Development Tools**:
```bash
# Go 1.22+ (master and coordination nodes)
wget https://go.dev/dl/go1.22.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.22.0.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin

# C++ Compiler (GCC 11+ or Clang 14+)
sudo apt-get update
sudo apt-get install -y build-essential cmake clang-14

# Python 3.11+ (pipeline development)
sudo apt-get install -y python3.11 python3.11-dev python3-pip
```

**Additional Tools**:
```bash
# Protocol Buffers compiler
sudo apt-get install -y protobuf-compiler
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Build tools
sudo apt-get install -y make git curl

# Kubernetes tools (for testing)
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
sudo install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl

# Docker
sudo apt-get install -y docker.io docker-compose
sudo usermod -aG docker $USER
```

### Resource Requirements

**Minimum Development Machine**:
- CPU: 4 cores (8 threads recommended)
- RAM: 16 GB (32 GB recommended)
- Storage: 100 GB SSD
- Network: 100 Mbps+

**Recommended Development Machine**:
- CPU: 8 cores (16 threads)
- RAM: 64 GB
- Storage: 500 GB NVMe SSD
- Network: 1 Gbps

---

## 2. Development Environment

### Project Structure

```
quidditch/
├── cmd/                          # Entry points for binaries
│   ├── master/                   # Master node binary
│   │   └── main.go
│   ├── coordination/             # Coordination node binary
│   │   └── main.go
│   └── qctl/                     # CLI tool
│       └── main.go
├── pkg/                          # Shared Go packages
│   ├── master/                   # Master node logic
│   │   ├── raft/                 # Raft consensus
│   │   ├── allocation/           # Shard allocation
│   │   └── metadata/             # Index metadata
│   ├── coordination/             # Coordination node logic
│   │   ├── parser/               # DSL/PPL parser
│   │   ├── planner/              # Calcite integration
│   │   └── executor/             # Query executor
│   ├── common/                   # Shared utilities
│   │   ├── api/                  # API types
│   │   ├── proto/                # Protobuf definitions
│   │   └── config/               # Configuration
│   └── python/                   # Python integration (CGO)
│       └── bridge/               # Go-Python bridge
├── diagon/                       # Diagon core (C++ submodule)
│   ├── include/                  # C++ headers
│   ├── src/                      # C++ implementation
│   └── bindings/                 # Go/CGO bindings
│       └── go/
├── python/                       # Python components
│   ├── quidditch/                # Python package
│   │   ├── pipeline/             # Pipeline SDK
│   │   ├── processors/           # Built-in processors
│   │   └── __init__.py
│   └── setup.py
├── calcite/                      # Apache Calcite integration
│   ├── quidditch-planner/        # Java module
│   │   ├── src/
│   │   └── pom.xml
│   └── grpc-server/              # Calcite gRPC service
├── operator/                     # Kubernetes operator
│   ├── api/                      # CRD definitions
│   ├── controllers/              # Reconciliation logic
│   └── config/                   # Deployment configs
├── docs/                         # Documentation
│   ├── design/                   # Design documents
│   └── api/                      # API documentation
├── test/                         # Integration tests
│   ├── e2e/                      # End-to-end tests
│   └── benchmark/                # Performance benchmarks
├── scripts/                      # Build/deployment scripts
│   ├── build.sh
│   ├── test.sh
│   └── docker-build.sh
├── deployments/                  # Deployment configs
│   ├── kubernetes/
│   └── docker-compose/
├── go.mod                        # Go dependencies
├── go.sum
├── Makefile                      # Build automation
└── README.md
```

### Environment Variables

Create `~/.quidditch/env`:

```bash
# Go Configuration
export GOPATH=$HOME/go
export PATH=$PATH:$GOPATH/bin:/usr/local/go/bin

# Quidditch Configuration
export QUIDDITCH_HOME=/home/ubuntu/quidditch
export QUIDDITCH_DATA_DIR=$HOME/.quidditch/data
export QUIDDITCH_LOG_DIR=$HOME/.quidditch/logs

# Development Settings
export QUIDDITCH_ENV=development
export QUIDDITCH_LOG_LEVEL=debug

# Diagon Settings
export DIAGON_HOME=$QUIDDITCH_HOME/diagon
export LD_LIBRARY_PATH=$DIAGON_HOME/lib:$LD_LIBRARY_PATH

# Python Settings
export PYTHONPATH=$QUIDDITCH_HOME/python:$PYTHONPATH

# Calcite Settings
export CALCITE_SERVICE_PORT=50051
```

Add to `~/.bashrc`:
```bash
source ~/.quidditch/env
```

---

## 3. Repository Setup

### Clone Repository

```bash
# Clone main repository
git clone https://github.com/your-org/quidditch.git
cd quidditch

# Initialize submodules (Diagon)
git submodule init
git submodule update

# Set up Git hooks
cp scripts/pre-commit.sh .git/hooks/pre-commit
chmod +x .git/hooks/pre-commit
```

### Branch Strategy

```
main          (production-ready, protected)
  ├── develop (integration branch)
  │   ├── feature/phase0-diagon-core
  │   ├── feature/phase1-master-node
  │   └── feature/phase1-coordination-node
  └── release/v1.0.0 (release branch)
```

**Branch Naming Convention**:
- `feature/phase{N}-{component}` - New features
- `bugfix/{issue-number}-{description}` - Bug fixes
- `hotfix/{issue-number}-{description}` - Production hotfixes
- `refactor/{component}` - Code refactoring

---

## 4. Build Tools

### Makefile Targets

```makefile
# Build all components
make all

# Build specific components
make master          # Build master node binary
make coordination    # Build coordination node binary
make diagon          # Build Diagon C++ library
make calcite         # Build Calcite planner
make python          # Build Python package
make operator        # Build Kubernetes operator

# Generate code
make proto           # Generate protobuf code
make mocks           # Generate test mocks

# Testing
make test            # Run all tests
make test-go         # Run Go tests
make test-cpp        # Run C++ tests
make test-python     # Run Python tests
make test-e2e        # Run end-to-end tests
make bench           # Run benchmarks

# Code quality
make lint            # Run linters (golangci-lint, clang-tidy)
make fmt             # Format code
make vet             # Run go vet

# Docker
make docker-build    # Build all Docker images
make docker-push     # Push to registry

# Cleanup
make clean           # Clean build artifacts
make clean-all       # Clean everything including dependencies
```

### Build Commands

**Quick Build (Development)**:
```bash
# Build everything
make all

# Run locally
./bin/quidditch-master --config=config/dev-master.yaml
./bin/quidditch-coordination --config=config/dev-coordination.yaml
```

**Production Build**:
```bash
# Optimized build with all tests
make clean
make test
make all BUILD_MODE=release

# Create distribution package
make dist
```

---

## 5. Dependencies

### Go Dependencies

```bash
# Core dependencies
go get github.com/hashicorp/raft           # Raft consensus
go get google.golang.org/grpc              # gRPC
go get google.golang.org/protobuf          # Protobuf
go get github.com/gin-gonic/gin            # HTTP framework
go get github.com/spf13/viper              # Configuration
go get github.com/spf13/cobra              # CLI framework
go get go.etcd.io/etcd/client/v3           # Etcd client (optional)

# Testing
go get github.com/stretchr/testify         # Test assertions
go get github.com/golang/mock              # Mocking

# Observability
go get go.opentelemetry.io/otel            # OpenTelemetry
go get github.com/prometheus/client_golang # Prometheus

# Update all dependencies
go mod tidy
go mod download
```

### C++ Dependencies (Diagon)

```bash
# Install C++ dependencies
cd diagon
./scripts/install-dependencies.sh

# Dependencies include:
# - Lucene++ (inverted index)
# - ClickHouse libraries (columnar storage)
# - SIMD libraries (AVX2, NEON)
# - Compression libraries (LZ4, ZSTD)
# - Google Test (testing)
```

### Python Dependencies

```bash
# Create virtual environment
cd python
python3 -m venv venv
source venv/bin/activate

# Install dependencies
pip install -r requirements.txt

# requirements.txt includes:
# - numpy>=1.24.0
# - pandas>=2.0.0
# - onnxruntime>=1.15.0
# - protobuf>=4.23.0
# - grpcio>=1.54.0
# - pytest>=7.3.0
```

### Calcite Dependencies

```bash
# Build Calcite planner
cd calcite/quidditch-planner
mvn clean install

# Dependencies managed by Maven (pom.xml)
```

---

## 6. IDE Configuration

### Visual Studio Code

**Recommended Extensions**:
```json
{
  "recommendations": [
    "golang.go",
    "ms-vscode.cpptools",
    "ms-python.python",
    "ms-kubernetes-tools.vscode-kubernetes-tools",
    "zxh404.vscode-proto3",
    "redhat.vscode-yaml"
  ]
}
```

**`.vscode/settings.json`**:
```json
{
  "go.testFlags": ["-v", "-race"],
  "go.buildFlags": ["-tags=integration"],
  "go.lintTool": "golangci-lint",
  "go.lintOnSave": "package",
  "editor.formatOnSave": true,
  "editor.codeActionsOnSave": {
    "source.organizeImports": true
  },
  "C_Cpp.default.includePath": [
    "${workspaceFolder}/diagon/include"
  ],
  "python.linting.enabled": true,
  "python.linting.pylintEnabled": true,
  "python.formatting.provider": "black"
}
```

### GoLand / IntelliJ IDEA

1. Open project directory
2. Configure Go SDK (Settings → Go → GOROOT)
3. Enable Go modules (Settings → Go → Go Modules)
4. Configure CGO (Settings → Go → Build Tags & Vendoring)
   - Build tags: `integration`
   - CGO enabled: ✓
5. Set up run configurations for master/coordination nodes

### Vim/Neovim

```vim
" .vimrc / init.vim
Plug 'fatih/vim-go'
Plug 'neoclide/coc.nvim'
Plug 'dense-analysis/ale'

" Go settings
let g:go_fmt_command = "goimports"
let g:go_auto_type_info = 1
let g:go_def_mode='gopls'
```

---

## 7. Running Tests

### Unit Tests

```bash
# Run all Go tests
make test-go

# Run specific package tests
go test ./pkg/master/... -v

# Run with coverage
go test ./pkg/... -coverprofile=coverage.out
go tool cover -html=coverage.out

# Run C++ tests
make test-cpp
cd diagon/build
ctest --output-on-failure

# Run Python tests
make test-python
cd python
pytest tests/ -v
```

### Integration Tests

```bash
# Start test cluster
make test-cluster-up

# Run integration tests
make test-e2e

# Cleanup
make test-cluster-down
```

### Benchmarks

```bash
# Run Go benchmarks
go test -bench=. -benchmem ./pkg/...

# Run C++ benchmarks
cd diagon/build
./benchmarks/search_benchmark

# Run full benchmark suite
make bench
```

---

## 8. Local Deployment

### Single-Node Development Cluster

```bash
# Start local cluster with Docker Compose
cd deployments/docker-compose
docker-compose up -d

# Services:
# - Master node: localhost:9300 (gRPC), localhost:9200 (REST)
# - Coordination node: localhost:9301
# - Data node: localhost:9302
# - Calcite planner: localhost:50051

# Check cluster health
curl http://localhost:9200/_cluster/health

# Create test index
curl -X PUT http://localhost:9200/test-index \
  -H 'Content-Type: application/json' \
  -d '{"settings": {"number_of_shards": 1}}'

# Stop cluster
docker-compose down
```

### Kubernetes (Kind)

```bash
# Create local Kubernetes cluster
kind create cluster --name quidditch-dev

# Install operator
make operator-install

# Deploy development cluster
kubectl apply -f deployments/kubernetes/dev-cluster.yaml

# Port forward to access API
kubectl port-forward svc/quidditch-coordination 9200:9200

# Access API
curl http://localhost:9200/_cluster/health
```

---

## 9. Debugging

### Go Debugging (Delve)

```bash
# Install Delve
go install github.com/go-delve/delve/cmd/dlv@latest

# Debug master node
dlv debug ./cmd/master -- --config=config/dev-master.yaml

# Attach to running process
dlv attach $(pgrep quidditch-master)

# Common commands:
# (dlv) break main.main
# (dlv) continue
# (dlv) print variable_name
# (dlv) step
```

### C++ Debugging (GDB)

```bash
# Build with debug symbols
cd diagon
cmake -DCMAKE_BUILD_TYPE=Debug ..
make

# Debug with GDB
gdb ./bin/diagon_test
(gdb) run
(gdb) backtrace
```

### Python Debugging

```python
# Use pdb for debugging
import pdb; pdb.set_trace()

# Or use pytest with pdb
pytest --pdb tests/test_pipeline.py
```

### Distributed Debugging

```bash
# Enable debug logging
export QUIDDITCH_LOG_LEVEL=debug

# Trace gRPC calls
export GRPC_TRACE=all
export GRPC_VERBOSITY=DEBUG

# Use OpenTelemetry traces
# Configure Jaeger backend in config file
```

---

## 10. Contributing Guidelines

### Code Style

**Go**:
- Follow [Effective Go](https://golang.org/doc/effective_go.html)
- Use `gofmt` for formatting
- Run `golangci-lint` before committing
- Minimum test coverage: 80%

**C++**:
- Follow [Google C++ Style Guide](https://google.github.io/styleguide/cppguide.html)
- Use `clang-format` with provided `.clang-format`
- Use `clang-tidy` for static analysis
- Minimum test coverage: 75%

**Python**:
- Follow [PEP 8](https://www.python.org/dev/peps/pep-0008/)
- Use `black` for formatting
- Use `pylint` for linting
- Type hints required
- Minimum test coverage: 85%

### Commit Messages

```
<type>(<scope>): <subject>

<body>

<footer>
```

**Types**: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`, `perf`

**Example**:
```
feat(master): implement Raft-based cluster state management

- Add Raft consensus library integration
- Implement cluster state FSM
- Add leader election logic

Closes #42
```

### Pull Request Process

1. Create feature branch from `develop`
2. Write code + tests (maintain coverage)
3. Run `make test lint`
4. Update documentation if needed
5. Push and create PR
6. Wait for CI checks (GitHub Actions)
7. Request reviews from 2+ team members
8. Address review comments
9. Merge after approval

### Code Review Checklist

- [ ] Code follows style guidelines
- [ ] Tests pass and coverage is maintained
- [ ] Documentation updated
- [ ] No breaking API changes (or documented)
- [ ] Performance impact considered
- [ ] Security implications reviewed
- [ ] Error handling is comprehensive

---

## Quick Start Checklist

**First Day Setup** (2-3 hours):
- [ ] Install prerequisites (Go, C++, Python)
- [ ] Clone repository and submodules
- [ ] Set up environment variables
- [ ] Install dependencies (`make deps`)
- [ ] Build project (`make all`)
- [ ] Run tests (`make test`)
- [ ] Start local cluster (`docker-compose up`)
- [ ] Configure IDE

**First Week**:
- [ ] Read all design documentation (start with INDEX.md)
- [ ] Explore codebase
- [ ] Run benchmarks to understand performance
- [ ] Fix a "good first issue"
- [ ] Submit first PR

---

## Troubleshooting

### Common Issues

**1. CGO Build Failures**:
```bash
# Ensure C++ compiler is available
export CC=clang-14
export CXX=clang++-14

# Verify Diagon libraries are built
cd diagon && make clean && make
```

**2. gRPC Connection Issues**:
```bash
# Check if ports are available
netstat -tuln | grep 9300

# Verify gRPC service is running
grpcurl -plaintext localhost:9300 list
```

**3. Python Import Errors**:
```bash
# Verify PYTHONPATH
echo $PYTHONPATH

# Install in development mode
cd python && pip install -e .
```

**4. Out of Memory During Build**:
```bash
# Limit parallel builds
make -j2 all

# Or increase swap
sudo fallocate -l 8G /swapfile
sudo mkswap /swapfile
sudo swapon /swapfile
```

---

## Resources

- **Internal Wiki**: https://wiki.quidditch.io
- **Slack**: #quidditch-dev
- **Issue Tracker**: https://github.com/your-org/quidditch/issues
- **CI/CD Dashboard**: https://ci.quidditch.io
- **Monitoring**: https://grafana.quidditch.io

---

## Getting Help

1. Check documentation (INDEX.md)
2. Search existing issues
3. Ask in #quidditch-dev Slack
4. Create GitHub issue with details
5. Tag relevant team members

---

**Status**: Ready for Phase 0 Implementation
**Last Updated**: 2026-01-25
**Maintainers**: Quidditch Core Team
