# Quidditch Makefile
# Build automation for all components

.PHONY: all clean test lint fmt help

# Build configuration
BUILD_MODE ?= debug
VERSION ?= 1.0.0-dev
COMMIT_HASH := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')

# Go configuration
GOCMD := go
GOBUILD := $(GOCMD) build
GOTEST := $(GOCMD) test
GOGET := $(GOCMD) get
GOMOD := $(GOCMD) mod
GOFMT := gofmt
GOLINT := golangci-lint

# Build flags
GO_LDFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.CommitHash=$(COMMIT_HASH) -X main.BuildTime=$(BUILD_TIME)"
ifeq ($(BUILD_MODE),release)
	GO_LDFLAGS += -s -w
	GO_BUILD_FLAGS := -trimpath
else
	GO_BUILD_FLAGS := -race
endif

# Directories
BIN_DIR := bin
PKG_DIR := pkg
CMD_DIR := cmd
DIAGON_DIR := diagon
PYTHON_DIR := python
CALCITE_DIR := calcite
OPERATOR_DIR := operator

# Output binaries
MASTER_BIN := $(BIN_DIR)/quidditch-master
COORDINATION_BIN := $(BIN_DIR)/quidditch-coordination
QCTL_BIN := $(BIN_DIR)/qctl

# Docker configuration
DOCKER := docker
DOCKER_REGISTRY ?= quidditch
DOCKER_TAG ?= $(VERSION)

# Kubernetes configuration
KUBECTL := kubectl
KUSTOMIZE := kustomize
HELM := helm

##@ General

help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

all: master coordination qctl ## Build all binaries

clean: clean-go clean-diagon clean-python clean-calcite ## Clean all build artifacts
	rm -rf $(BIN_DIR)

##@ Build

master: $(MASTER_BIN) ## Build master node binary

$(MASTER_BIN): $(shell find $(CMD_DIR)/master $(PKG_DIR) -name '*.go')
	@echo "Building master node..."
	@mkdir -p $(BIN_DIR)
	$(GOBUILD) $(GO_BUILD_FLAGS) $(GO_LDFLAGS) -o $@ ./$(CMD_DIR)/master

coordination: $(COORDINATION_BIN) ## Build coordination node binary

$(COORDINATION_BIN): $(shell find $(CMD_DIR)/coordination $(PKG_DIR) -name '*.go')
	@echo "Building coordination node..."
	@mkdir -p $(BIN_DIR)
	$(GOBUILD) $(GO_BUILD_FLAGS) $(GO_LDFLAGS) -o $@ ./$(CMD_DIR)/coordination

qctl: $(QCTL_BIN) ## Build qctl CLI tool

$(QCTL_BIN): $(shell find $(CMD_DIR)/qctl $(PKG_DIR) -name '*.go')
	@echo "Building qctl..."
	@mkdir -p $(BIN_DIR)
	$(GOBUILD) $(GO_BUILD_FLAGS) $(GO_LDFLAGS) -o $@ ./$(CMD_DIR)/qctl

diagon: ## Build Diagon C++ library
	@echo "Building Diagon..."
	@cd $(DIAGON_DIR) && \
		mkdir -p build && \
		cd build && \
		cmake -DCMAKE_BUILD_TYPE=$(BUILD_MODE) .. && \
		make -j$$(nproc)

python: ## Build Python package
	@echo "Building Python package..."
	@cd $(PYTHON_DIR) && \
		pip install -e .

calcite: ## Build Apache Calcite planner
	@echo "Building Calcite planner..."
	@cd $(CALCITE_DIR)/quidditch-planner && \
		mvn clean package -DskipTests

operator: ## Build Kubernetes operator
	@echo "Building Kubernetes operator..."
	@cd $(OPERATOR_DIR) && \
		$(GOBUILD) $(GO_BUILD_FLAGS) -o bin/manager main.go

##@ Code Generation

proto: ## Generate protobuf code
	@echo "Generating protobuf code..."
	@protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		$(PKG_DIR)/common/proto/*.proto

mocks: ## Generate test mocks
	@echo "Generating mocks..."
	@go generate ./...

##@ Testing

test: test-go ## Run all tests

test-go: ## Run Go unit tests
	@echo "Running Go tests..."
	$(GOTEST) -v -race -coverprofile=coverage.out ./$(PKG_DIR)/...

test-cpp: diagon ## Run C++ tests
	@echo "Running C++ tests..."
	@cd $(DIAGON_DIR)/build && ctest --output-on-failure

test-python: ## Run Python tests
	@echo "Running Python tests..."
	@cd $(PYTHON_DIR) && pytest tests/ -v

test-e2e: ## Run end-to-end tests
	@echo "Running e2e tests..."
	$(GOTEST) -v -tags=integration ./test/e2e/...

test-integration: test-cluster-up ## Run integration tests
	@echo "Running integration tests..."
	$(GOTEST) -v -tags=integration ./test/...
	@$(MAKE) test-cluster-down

bench: ## Run benchmarks
	@echo "Running benchmarks..."
	$(GOTEST) -bench=. -benchmem ./$(PKG_DIR)/...

coverage: test-go ## Generate test coverage report
	@echo "Generating coverage report..."
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

##@ Code Quality

lint: lint-go lint-cpp lint-python ## Run all linters

lint-go: ## Run Go linters
	@echo "Running golangci-lint..."
	$(GOLINT) run ./...

lint-cpp: ## Run C++ linters
	@echo "Running clang-tidy..."
	@cd $(DIAGON_DIR) && \
		find src include -name '*.cpp' -o -name '*.h' | \
		xargs clang-tidy -p build

lint-python: ## Run Python linters
	@echo "Running pylint..."
	@cd $(PYTHON_DIR) && \
		pylint quidditch/

fmt: fmt-go fmt-cpp fmt-python ## Format all code

fmt-go: ## Format Go code
	@echo "Formatting Go code..."
	$(GOFMT) -s -w $(CMD_DIR) $(PKG_DIR)

fmt-cpp: ## Format C++ code
	@echo "Formatting C++ code..."
	@cd $(DIAGON_DIR) && \
		find src include -name '*.cpp' -o -name '*.h' | \
		xargs clang-format -i

fmt-python: ## Format Python code
	@echo "Formatting Python code..."
	@cd $(PYTHON_DIR) && \
		black quidditch/ tests/

vet: ## Run go vet
	@echo "Running go vet..."
	$(GOCMD) vet ./...

##@ Dependencies

deps: deps-go deps-cpp deps-python ## Install all dependencies

deps-go: ## Download Go dependencies
	@echo "Downloading Go dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

deps-cpp: ## Install C++ dependencies
	@echo "Installing C++ dependencies..."
	@cd $(DIAGON_DIR) && ./scripts/install-dependencies.sh

deps-python: ## Install Python dependencies
	@echo "Installing Python dependencies..."
	@cd $(PYTHON_DIR) && pip install -r requirements.txt

##@ Docker

docker-build: docker-build-master docker-build-coordination ## Build all Docker images

docker-build-master: ## Build master node Docker image
	@echo "Building master node Docker image..."
	$(DOCKER) build -t $(DOCKER_REGISTRY)/quidditch-master:$(DOCKER_TAG) \
		-f deployments/docker/Dockerfile.master .

docker-build-coordination: ## Build coordination node Docker image
	@echo "Building coordination node Docker image..."
	$(DOCKER) build -t $(DOCKER_REGISTRY)/quidditch-coordination:$(DOCKER_TAG) \
		-f deployments/docker/Dockerfile.coordination .

docker-push: ## Push Docker images to registry
	@echo "Pushing Docker images..."
	$(DOCKER) push $(DOCKER_REGISTRY)/quidditch-master:$(DOCKER_TAG)
	$(DOCKER) push $(DOCKER_REGISTRY)/quidditch-coordination:$(DOCKER_TAG)

##@ Local Deployment

test-cluster-up: ## Start local test cluster with Docker Compose
	@echo "Starting local test cluster..."
	@cd deployments/docker-compose && docker-compose up -d

test-cluster-down: ## Stop local test cluster
	@echo "Stopping local test cluster..."
	@cd deployments/docker-compose && docker-compose down

test-cluster-logs: ## Show test cluster logs
	@cd deployments/docker-compose && docker-compose logs -f

##@ Kubernetes

k8s-install-operator: ## Install Kubernetes operator
	@echo "Installing Kubernetes operator..."
	@cd $(OPERATOR_DIR) && make install
	@cd $(OPERATOR_DIR) && make deploy

k8s-deploy-dev: ## Deploy development cluster to Kubernetes
	@echo "Deploying development cluster..."
	$(KUBECTL) apply -f deployments/kubernetes/dev-cluster.yaml

k8s-deploy-prod: ## Deploy production cluster to Kubernetes
	@echo "Deploying production cluster..."
	$(KUBECTL) apply -f deployments/kubernetes/prod-cluster.yaml

k8s-uninstall: ## Uninstall from Kubernetes
	@echo "Uninstalling from Kubernetes..."
	$(KUBECTL) delete -f deployments/kubernetes/

##@ Distribution

dist: all ## Create distribution package
	@echo "Creating distribution package..."
	@mkdir -p dist/quidditch-$(VERSION)
	@cp -r $(BIN_DIR) dist/quidditch-$(VERSION)/
	@cp -r deployments dist/quidditch-$(VERSION)/
	@cp -r docs dist/quidditch-$(VERSION)/
	@cp README.md dist/quidditch-$(VERSION)/
	@cd dist && tar czf quidditch-$(VERSION).tar.gz quidditch-$(VERSION)
	@echo "Distribution package: dist/quidditch-$(VERSION).tar.gz"

##@ Cleanup

clean-go: ## Clean Go build artifacts
	@echo "Cleaning Go artifacts..."
	@$(GOCMD) clean
	@rm -f coverage.out coverage.html

clean-diagon: ## Clean Diagon build artifacts
	@echo "Cleaning Diagon artifacts..."
	@rm -rf $(DIAGON_DIR)/build

clean-python: ## Clean Python artifacts
	@echo "Cleaning Python artifacts..."
	@cd $(PYTHON_DIR) && \
		rm -rf build dist *.egg-info __pycache__ .pytest_cache

clean-calcite: ## Clean Calcite artifacts
	@echo "Cleaning Calcite artifacts..."
	@cd $(CALCITE_DIR)/quidditch-planner && mvn clean

clean-all: clean ## Clean everything including dependencies
	@rm -rf vendor
	@cd $(PYTHON_DIR) && pip uninstall -y quidditch

##@ Info

version: ## Show version information
	@echo "Version:     $(VERSION)"
	@echo "Commit:      $(COMMIT_HASH)"
	@echo "Build Time:  $(BUILD_TIME)"
	@echo "Build Mode:  $(BUILD_MODE)"

info: ## Show build information
	@echo "Go version:     $$(go version)"
	@echo "GCC version:    $$(gcc --version | head -n1)"
	@echo "Python version: $$(python3 --version)"
	@echo "Docker version: $$(docker --version)"
	@echo "Kubectl version: $$(kubectl version --client --short)"
