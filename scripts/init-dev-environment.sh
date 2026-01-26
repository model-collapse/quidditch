#!/bin/bash

# Quidditch Development Environment Initialization Script
# This script sets up your development environment for the first time

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Helper functions
print_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

check_command() {
    if command -v $1 &> /dev/null; then
        print_info "$1 is installed"
        return 0
    else
        print_warn "$1 is not installed"
        return 1
    fi
}

# Banner
echo "========================================="
echo "  Quidditch Development Environment"
echo "  Initialization Script"
echo "========================================="
echo ""

# Check prerequisites
print_info "Checking prerequisites..."

# Check Go
if ! check_command go; then
    print_error "Go is required. Please install Go 1.22+ from https://golang.org/dl/"
    exit 1
fi

GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
print_info "Go version: $GO_VERSION"

# Check GCC
if ! check_command gcc; then
    print_error "GCC is required. Run: sudo apt-get install build-essential"
    exit 1
fi

GCC_VERSION=$(gcc --version | head -n1)
print_info "GCC: $GCC_VERSION"

# Check Python
if ! check_command python3; then
    print_error "Python 3.11+ is required. Run: sudo apt-get install python3.11"
    exit 1
fi

PYTHON_VERSION=$(python3 --version)
print_info "Python: $PYTHON_VERSION"

# Check Docker
if ! check_command docker; then
    print_warn "Docker is not installed. Some features may not work."
    print_info "Install Docker: sudo apt-get install docker.io"
else
    DOCKER_VERSION=$(docker --version)
    print_info "Docker: $DOCKER_VERSION"
fi

# Check make
if ! check_command make; then
    print_error "Make is required. Run: sudo apt-get install make"
    exit 1
fi

# Check protoc
if ! check_command protoc; then
    print_warn "protoc not found. Installing..."
    sudo apt-get update
    sudo apt-get install -y protobuf-compiler
fi

print_info "All prerequisites satisfied!"
echo ""

# Create directory structure
print_info "Creating directory structure..."
mkdir -p ~/.quidditch/{data,logs,config}
mkdir -p bin

# Set up environment variables
print_info "Setting up environment variables..."
ENV_FILE=~/.quidditch/env

cat > $ENV_FILE << 'EOF'
# Quidditch Environment Variables

# Go Configuration
export GOPATH=$HOME/go
export PATH=$PATH:$GOPATH/bin:/usr/local/go/bin

# Quidditch Configuration
export QUIDDITCH_HOME=$PWD
export QUIDDITCH_DATA_DIR=$HOME/.quidditch/data
export QUIDDITCH_LOG_DIR=$HOME/.quidditch/logs

# Development Settings
export QUIDDITCH_ENV=development
export QUIDDITCH_LOG_LEVEL=debug

# Python Settings
export PYTHONPATH=$QUIDDITCH_HOME/python:$PYTHONPATH
EOF

print_info "Environment file created: $ENV_FILE"
print_warn "Add 'source ~/.quidditch/env' to your ~/.bashrc or ~/.zshrc"

# Source environment for current session
source $ENV_FILE

# Install Go dependencies
print_info "Installing Go dependencies..."
if [ -f go.mod ]; then
    go mod download
    print_info "Go dependencies installed"
else
    print_warn "go.mod not found, skipping Go dependencies"
fi

# Install protoc plugins
print_info "Installing protoc Go plugins..."
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
print_info "protoc plugins installed"

# Install Python dependencies
print_info "Setting up Python environment..."
if [ -d python ]; then
    cd python
    if [ ! -d venv ]; then
        python3 -m venv venv
        print_info "Python virtual environment created"
    fi
    source venv/bin/activate
    pip install --upgrade pip
    if [ -f requirements.txt ]; then
        pip install -r requirements.txt
        print_info "Python dependencies installed"
    fi
    cd ..
else
    print_warn "python/ directory not found, skipping Python setup"
fi

# Initialize git submodules (for Diagon)
if [ -f .gitmodules ]; then
    print_info "Initializing git submodules..."
    git submodule init
    git submodule update
    print_info "Submodules initialized"
fi

# Generate protobuf code
if [ -d pkg/common/proto ]; then
    print_info "Generating protobuf code..."
    make proto 2>/dev/null || print_warn "Failed to generate protobuf code (run 'make proto' manually)"
fi

# Create default configs
print_info "Creating default configuration files..."
cp -n config/dev-master.yaml ~/.quidditch/config/master.yaml 2>/dev/null || true
cp -n config/dev-coordination.yaml ~/.quidditch/config/coordination.yaml 2>/dev/null || true
print_info "Configuration files copied to ~/.quidditch/config/"

# Build project
print_info "Building project..."
if make all 2>&1 | tee /tmp/quidditch-build.log; then
    print_info "Build successful!"
else
    print_error "Build failed. Check /tmp/quidditch-build.log for details"
    exit 1
fi

# Run tests
print_info "Running tests..."
if make test-go 2>&1 | tee /tmp/quidditch-test.log; then
    print_info "Tests passed!"
else
    print_warn "Some tests failed. Check /tmp/quidditch-test.log for details"
fi

echo ""
echo "========================================="
echo "  ✅ Development Environment Ready!"
echo "========================================="
echo ""
echo "Next steps:"
echo "  1. Source environment: source ~/.quidditch/env"
echo "  2. Read documentation: cat INDEX.md"
echo "  3. Start local cluster: cd deployments/docker-compose && docker-compose up -d"
echo "  4. Test API: curl http://localhost:9200/_cluster/health"
echo ""
echo "Helpful commands:"
echo "  make help         - Show all make targets"
echo "  make test         - Run all tests"
echo "  make lint         - Run linters"
echo "  make docker-build - Build Docker images"
echo ""
echo "Documentation:"
echo "  INDEX.md                   - Master navigation"
echo "  GETTING_STARTED.md         - Quick introduction"
echo "  DEVELOPMENT_SETUP.md       - Complete setup guide"
echo "  PROJECT_KICKOFF.md         - Team kickoff guide"
echo ""
print_info "Happy coding! 🚀"
