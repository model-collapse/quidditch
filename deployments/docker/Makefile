# Quidditch Docker Makefile

.PHONY: help build build-master build-coordination build-data push pull run stop clean logs test

# Variables
VERSION ?= latest
REGISTRY ?= ghcr.io/quidditch/quidditch
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
VCS_REF := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Colors for output
GREEN  := $(shell tput -Txterm setaf 2)
YELLOW := $(shell tput -Txterm setaf 3)
RESET  := $(shell tput -Txterm sgr0)

help: ## Show this help message
	@echo "$(GREEN)Quidditch Docker Operations$(RESET)"
	@echo ""
	@echo "Usage:"
	@echo "  make $(YELLOW)<target>$(RESET)"
	@echo ""
	@echo "Targets:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  $(YELLOW)%-20s$(RESET) %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: build-master build-coordination build-data ## Build all Docker images

build-master: ## Build master node image
	@echo "$(GREEN)Building master node image...$(RESET)"
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		--build-arg VCS_REF=$(VCS_REF) \
		-t quidditch/master:$(VERSION) \
		-t quidditch/master:latest \
		-f Dockerfile.master \
		../..

build-coordination: ## Build coordination node image
	@echo "$(GREEN)Building coordination node image...$(RESET)"
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		--build-arg VCS_REF=$(VCS_REF) \
		-t quidditch/coordination:$(VERSION) \
		-t quidditch/coordination:latest \
		-f Dockerfile.coordination \
		../..

build-data: ## Build data node image
	@echo "$(GREEN)Building data node image...$(RESET)"
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		--build-arg VCS_REF=$(VCS_REF) \
		-t quidditch/data:$(VERSION) \
		-t quidditch/data:latest \
		-f Dockerfile.data \
		../..

buildx: ## Build multi-arch images using buildx
	@echo "$(GREEN)Building multi-arch images...$(RESET)"
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		--build-arg VERSION=$(VERSION) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		--build-arg VCS_REF=$(VCS_REF) \
		-t quidditch/master:$(VERSION) \
		-f Dockerfile.master \
		--push \
		../..

tag: ## Tag images for registry
	@echo "$(GREEN)Tagging images for $(REGISTRY)...$(RESET)"
	docker tag quidditch/master:$(VERSION) $(REGISTRY)/master:$(VERSION)
	docker tag quidditch/coordination:$(VERSION) $(REGISTRY)/coordination:$(VERSION)
	docker tag quidditch/data:$(VERSION) $(REGISTRY)/data:$(VERSION)

push: tag ## Push images to registry
	@echo "$(GREEN)Pushing images to $(REGISTRY)...$(RESET)"
	docker push $(REGISTRY)/master:$(VERSION)
	docker push $(REGISTRY)/coordination:$(VERSION)
	docker push $(REGISTRY)/data:$(VERSION)

pull: ## Pull images from registry
	@echo "$(GREEN)Pulling images from $(REGISTRY)...$(RESET)"
	docker pull $(REGISTRY)/master:$(VERSION)
	docker pull $(REGISTRY)/coordination:$(VERSION)
	docker pull $(REGISTRY)/data:$(VERSION)

run: ## Start the cluster using docker-compose
	@echo "$(GREEN)Starting Quidditch cluster...$(RESET)"
	docker-compose up -d

stop: ## Stop the cluster
	@echo "$(YELLOW)Stopping Quidditch cluster...$(RESET)"
	docker-compose down

restart: stop run ## Restart the cluster

clean: stop ## Stop cluster and remove volumes
	@echo "$(YELLOW)Cleaning up...$(RESET)"
	docker-compose down -v
	docker system prune -f

logs: ## Show logs from all services
	docker-compose logs -f

logs-master: ## Show master node logs
	docker-compose logs -f master-1 master-2 master-3

logs-coordination: ## Show coordination node logs
	docker-compose logs -f coordination-1

logs-data: ## Show data node logs
	docker-compose logs -f data-1 data-2

ps: ## Show running containers
	docker-compose ps

health: ## Check health of all services
	@echo "$(GREEN)Checking cluster health...$(RESET)"
	@docker-compose ps --format json | jq -r '.[] | "\(.Name): \(.Health)"'

test: ## Run basic cluster tests
	@echo "$(GREEN)Testing cluster...$(RESET)"
	@echo "Waiting for coordination node to be ready..."
	@timeout 60 bash -c 'until curl -f http://localhost:9200/ 2>/dev/null; do sleep 2; done'
	@echo "$(GREEN)✓ Cluster is ready$(RESET)"
	@echo ""
	@echo "Testing root endpoint..."
	@curl -s http://localhost:9200/ | jq .
	@echo ""
	@echo "Testing cluster health..."
	@curl -s http://localhost:9200/_cluster/health | jq .

shell-master: ## Shell into master node
	docker-compose exec master-1 sh

shell-coordination: ## Shell into coordination node
	docker-compose exec coordination-1 sh

shell-data: ## Shell into data node
	docker-compose exec data-1 sh

images: ## List all Quidditch images
	@docker images | grep quidditch

size: ## Show image sizes
	@echo "$(GREEN)Image sizes:$(RESET)"
	@docker images quidditch/* --format "table {{.Repository}}\t{{.Tag}}\t{{.Size}}"

inspect-master: ## Inspect master image
	@docker inspect quidditch/master:$(VERSION) | jq '.[0]'

inspect-coordination: ## Inspect coordination image
	@docker inspect quidditch/coordination:$(VERSION) | jq '.[0]'

inspect-data: ## Inspect data image
	@docker inspect quidditch/data:$(VERSION) | jq '.[0]'

scan: ## Scan images for vulnerabilities using trivy
	@echo "$(GREEN)Scanning images for vulnerabilities...$(RESET)"
	@command -v trivy >/dev/null 2>&1 || { echo "trivy not installed. Install: https://github.com/aquasecurity/trivy"; exit 1; }
	trivy image quidditch/master:$(VERSION)
	trivy image quidditch/coordination:$(VERSION)
	trivy image quidditch/data:$(VERSION)

.DEFAULT_GOAL := help
