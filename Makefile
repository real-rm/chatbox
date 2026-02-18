# Makefile for Chat Application WebSocket Service

.PHONY: help build test test-unit test-integration test-property test-coverage clean run docker-build docker-run docker-compose-up docker-compose-down lint fmt vet deps tidy check install deploy k8s-deploy k8s-delete k8s-logs k8s-status

# Variables
APP_NAME := chatbox
BINARY_NAME := chatbox-server
DOCKER_IMAGE := chatbox:latest
GO := go
GOTEST := $(GO) test
GOBUILD := $(GO) build
GOCLEAN := $(GO) clean
GOGET := $(GO) get
GOMOD := $(GO) mod
GOVET := $(GO) vet
GOFMT := gofmt

# Build variables
BUILD_DIR := ./bin
CMD_DIR := ./cmd/server
MAIN_FILE := $(CMD_DIR)/main.go

# Test variables
TEST_TIMEOUT := 2m
COVERAGE_FILE := coverage.out
COVERAGE_HTML := coverage.html

# Kubernetes variables
K8S_DIR := ./deployments/kubernetes
K8S_NAMESPACE := default

# Colors for output
COLOR_RESET := \033[0m
COLOR_BOLD := \033[1m
COLOR_GREEN := \033[32m
COLOR_YELLOW := \033[33m
COLOR_BLUE := \033[34m

##@ General

help: ## Display this help message
	@echo "$(COLOR_BOLD)Chat Application WebSocket Service - Makefile$(COLOR_RESET)"
	@echo ""
	@awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make $(COLOR_BLUE)<target>$(COLOR_RESET)\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  $(COLOR_BLUE)%-20s$(COLOR_RESET) %s\n", $$1, $$2 } /^##@/ { printf "\n$(COLOR_BOLD)%s$(COLOR_RESET)\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

deps: ## Download dependencies
	@echo "$(COLOR_GREEN)Downloading dependencies...$(COLOR_RESET)"
	$(GOGET) -v ./...

tidy: ## Tidy go.mod and go.sum
	@echo "$(COLOR_GREEN)Tidying go modules...$(COLOR_RESET)"
	$(GOMOD) tidy

fmt: ## Format Go code
	@echo "$(COLOR_GREEN)Formatting code...$(COLOR_RESET)"
	$(GOFMT) -w -s .

vet: ## Run go vet
	@echo "$(COLOR_GREEN)Running go vet...$(COLOR_RESET)"
	$(GOVET) ./...

lint: fmt vet ## Run linters (fmt + vet)
	@echo "$(COLOR_GREEN)Linting complete!$(COLOR_RESET)"

check: lint test ## Run all checks (lint + test)
	@echo "$(COLOR_GREEN)All checks passed!$(COLOR_RESET)"

##@ Build

build: ## Build the application
	@echo "$(COLOR_GREEN)Building $(BINARY_NAME)...$(COLOR_RESET)"
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_FILE)
	@echo "$(COLOR_GREEN)Build complete: $(BUILD_DIR)/$(BINARY_NAME)$(COLOR_RESET)"

install: build ## Install the binary to $GOPATH/bin
	@echo "$(COLOR_GREEN)Installing $(BINARY_NAME)...$(COLOR_RESET)"
	$(GO) install $(MAIN_FILE)

clean: ## Clean build artifacts
	@echo "$(COLOR_YELLOW)Cleaning build artifacts...$(COLOR_RESET)"
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)
	rm -f $(COVERAGE_FILE) $(COVERAGE_HTML)
	@echo "$(COLOR_GREEN)Clean complete!$(COLOR_RESET)"

##@ Testing

test: ## Run all tests
	@echo "$(COLOR_GREEN)Running all tests...$(COLOR_RESET)"
	$(GOTEST) -v -timeout $(TEST_TIMEOUT) ./...

test-unit: ## Run unit tests only (skip integration tests)
	@echo "$(COLOR_GREEN)Running unit tests...$(COLOR_RESET)"
	$(GOTEST) -v -short -timeout $(TEST_TIMEOUT) ./...

test-integration: ## Run integration tests only
	@echo "$(COLOR_GREEN)Running integration tests...$(COLOR_RESET)"
	$(GOTEST) -v -run TestIntegration -timeout $(TEST_TIMEOUT) ./...

test-property: ## Run property-based tests only
	@echo "$(COLOR_GREEN)Running property-based tests...$(COLOR_RESET)"
	$(GOTEST) -v -run Property -timeout $(TEST_TIMEOUT) ./...

test-coverage: ## Run tests with coverage report
	@echo "$(COLOR_GREEN)Running tests with coverage...$(COLOR_RESET)"
	$(GOTEST) -v -timeout $(TEST_TIMEOUT) -coverprofile=$(COVERAGE_FILE) ./...
	$(GO) tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_HTML)
	@echo "$(COLOR_GREEN)Coverage report generated: $(COVERAGE_HTML)$(COLOR_RESET)"

test-verbose: ## Run tests with verbose output
	@echo "$(COLOR_GREEN)Running tests (verbose)...$(COLOR_RESET)"
	$(GOTEST) -v -timeout $(TEST_TIMEOUT) -count=1 ./...

##@ Running

run: ## Run the application locally
	@echo "$(COLOR_GREEN)Running $(APP_NAME)...$(COLOR_RESET)"
	$(GO) run $(MAIN_FILE)

run-dev: ## Run the application in development mode with hot reload
	@echo "$(COLOR_GREEN)Running $(APP_NAME) in development mode...$(COLOR_RESET)"
	@which air > /dev/null || (echo "Installing air..." && go install github.com/cosmtrek/air@latest)
	air

##@ Docker

docker-build: ## Build Docker image
	@echo "$(COLOR_GREEN)Building Docker image: $(DOCKER_IMAGE)...$(COLOR_RESET)"
	docker build -t $(DOCKER_IMAGE) .
	@echo "$(COLOR_GREEN)Docker image built: $(DOCKER_IMAGE)$(COLOR_RESET)"

docker-run: ## Run Docker container
	@echo "$(COLOR_GREEN)Running Docker container...$(COLOR_RESET)"
	docker run -p 8080:8080 --env-file .env $(DOCKER_IMAGE)

docker-compose-up: ## Start services with docker-compose
	@echo "$(COLOR_GREEN)Starting services with docker-compose...$(COLOR_RESET)"
	docker-compose up -d
	@echo "$(COLOR_GREEN)Services started!$(COLOR_RESET)"
	@echo "$(COLOR_YELLOW)Run 'make docker-compose-logs' to view logs$(COLOR_RESET)"

docker-compose-down: ## Stop services with docker-compose
	@echo "$(COLOR_YELLOW)Stopping services with docker-compose...$(COLOR_RESET)"
	docker-compose down
	@echo "$(COLOR_GREEN)Services stopped!$(COLOR_RESET)"

docker-compose-logs: ## View docker-compose logs
	docker-compose logs -f

docker-compose-test: docker-compose-up ## Run tests against docker-compose environment
	@echo "$(COLOR_GREEN)Waiting for services to be ready...$(COLOR_RESET)"
	@sleep 5
	@echo "$(COLOR_GREEN)Running integration tests...$(COLOR_RESET)"
	./test_integration.sh || true
	@$(MAKE) docker-compose-down

##@ Kubernetes

k8s-deploy: ## Deploy to Kubernetes
	@echo "$(COLOR_GREEN)Deploying to Kubernetes...$(COLOR_RESET)"
	kubectl apply -f $(K8S_DIR)/configmap.yaml
	kubectl apply -f $(K8S_DIR)/secret.yaml
	kubectl apply -f $(K8S_DIR)/deployment.yaml
	kubectl apply -f $(K8S_DIR)/service.yaml
	@echo "$(COLOR_GREEN)Deployment complete!$(COLOR_RESET)"
	@echo "$(COLOR_YELLOW)Run 'make k8s-status' to check deployment status$(COLOR_RESET)"

k8s-delete: ## Delete Kubernetes resources
	@echo "$(COLOR_YELLOW)Deleting Kubernetes resources...$(COLOR_RESET)"
	kubectl delete -f $(K8S_DIR)/service.yaml --ignore-not-found=true
	kubectl delete -f $(K8S_DIR)/deployment.yaml --ignore-not-found=true
	kubectl delete -f $(K8S_DIR)/secret.yaml --ignore-not-found=true
	kubectl delete -f $(K8S_DIR)/configmap.yaml --ignore-not-found=true
	@echo "$(COLOR_GREEN)Resources deleted!$(COLOR_RESET)"

k8s-status: ## Check Kubernetes deployment status
	@echo "$(COLOR_BLUE)Checking deployment status...$(COLOR_RESET)"
	kubectl get pods -l app=chatbox -n $(K8S_NAMESPACE)
	kubectl get services -l app=chatbox -n $(K8S_NAMESPACE)

k8s-logs: ## View Kubernetes pod logs
	@echo "$(COLOR_BLUE)Fetching logs...$(COLOR_RESET)"
	kubectl logs -l app=chatbox -n $(K8S_NAMESPACE) --tail=100 -f

k8s-describe: ## Describe Kubernetes resources
	@echo "$(COLOR_BLUE)Describing deployment...$(COLOR_RESET)"
	kubectl describe deployment chatbox -n $(K8S_NAMESPACE)
	@echo ""
	@echo "$(COLOR_BLUE)Describing service...$(COLOR_RESET)"
	kubectl describe service chatbox -n $(K8S_NAMESPACE)

k8s-restart: ## Restart Kubernetes deployment
	@echo "$(COLOR_GREEN)Restarting deployment...$(COLOR_RESET)"
	kubectl rollout restart deployment/chatbox -n $(K8S_NAMESPACE)
	kubectl rollout status deployment/chatbox -n $(K8S_NAMESPACE)

##@ CI/CD

ci: lint test ## Run CI pipeline (lint + test)
	@echo "$(COLOR_GREEN)CI pipeline complete!$(COLOR_RESET)"

ci-full: clean deps tidy lint test-coverage ## Run full CI pipeline
	@echo "$(COLOR_GREEN)Full CI pipeline complete!$(COLOR_RESET)"

release: clean deps tidy lint test build ## Prepare release build
	@echo "$(COLOR_GREEN)Release build complete!$(COLOR_RESET)"

##@ Utilities

logs: ## View application logs
	@echo "$(COLOR_BLUE)Viewing logs...$(COLOR_RESET)"
	tail -f logs/*.log

clean-logs: ## Clean log files
	@echo "$(COLOR_YELLOW)Cleaning log files...$(COLOR_RESET)"
	rm -f logs/*.log
	@echo "$(COLOR_GREEN)Logs cleaned!$(COLOR_RESET)"

version: ## Show Go version
	@$(GO) version

info: ## Show project information
	@echo "$(COLOR_BOLD)Project Information$(COLOR_RESET)"
	@echo "  App Name:     $(APP_NAME)"
	@echo "  Binary Name:  $(BINARY_NAME)"
	@echo "  Docker Image: $(DOCKER_IMAGE)"
	@echo "  Go Version:   $$($(GO) version)"
	@echo "  Build Dir:    $(BUILD_DIR)"

.DEFAULT_GOAL := help
