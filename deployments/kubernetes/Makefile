# Makefile for Kubernetes deployment of chatbox service

# Variables
NAMESPACE ?= default
IMAGE_NAME ?= chatbox-websocket
IMAGE_TAG ?= latest
REGISTRY ?= docker.io/yourorg
FULL_IMAGE = $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)

# Kubernetes context
KUBE_CONTEXT ?= $(shell kubectl config current-context)

# Colors for output
GREEN  := $(shell tput -Txterm setaf 2)
YELLOW := $(shell tput -Txterm setaf 3)
RED    := $(shell tput -Txterm setaf 1)
RESET  := $(shell tput -Txterm sgr0)

.PHONY: help
help: ## Show this help message
	@echo "$(GREEN)Chatbox Kubernetes Deployment$(RESET)"
	@echo ""
	@echo "$(YELLOW)Usage:$(RESET)"
	@echo "  make [target]"
	@echo ""
	@echo "$(YELLOW)Targets:$(RESET)"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(GREEN)%-20s$(RESET) %s\n", $$1, $$2}'

.PHONY: check-context
check-context: ## Check current Kubernetes context
	@echo "$(YELLOW)Current Kubernetes context:$(RESET) $(KUBE_CONTEXT)"
	@echo "$(YELLOW)Current namespace:$(RESET) $(NAMESPACE)"
	@read -p "Continue? [y/N] " -n 1 -r; \
	echo; \
	if [[ ! $$REPLY =~ ^[Yy]$$ ]]; then \
		echo "$(RED)Aborted$(RESET)"; \
		exit 1; \
	fi

.PHONY: build
build: ## Build Docker image
	@echo "$(GREEN)Building Docker image...$(RESET)"
	cd ../.. && docker build -t $(IMAGE_NAME):$(IMAGE_TAG) .
	@echo "$(GREEN)Image built: $(IMAGE_NAME):$(IMAGE_TAG)$(RESET)"

.PHONY: tag
tag: ## Tag Docker image for registry
	@echo "$(GREEN)Tagging image...$(RESET)"
	docker tag $(IMAGE_NAME):$(IMAGE_TAG) $(FULL_IMAGE)
	@echo "$(GREEN)Image tagged: $(FULL_IMAGE)$(RESET)"

.PHONY: push
push: tag ## Push Docker image to registry
	@echo "$(GREEN)Pushing image to registry...$(RESET)"
	docker push $(FULL_IMAGE)
	@echo "$(GREEN)Image pushed: $(FULL_IMAGE)$(RESET)"

.PHONY: build-push
build-push: build push ## Build and push Docker image

.PHONY: validate
validate: ## Validate Kubernetes manifests
	@echo "$(GREEN)Validating manifests...$(RESET)"
	kubectl apply --dry-run=client -f configmap.yaml
	kubectl apply --dry-run=client -f secret.yaml
	kubectl apply --dry-run=client -f deployment.yaml
	kubectl apply --dry-run=client -f service.yaml
	@echo "$(GREEN)All manifests are valid$(RESET)"

.PHONY: create-namespace
create-namespace: ## Create namespace if it doesn't exist
	@kubectl get namespace $(NAMESPACE) >/dev/null 2>&1 || \
		(echo "$(GREEN)Creating namespace $(NAMESPACE)...$(RESET)" && \
		kubectl create namespace $(NAMESPACE))

.PHONY: deploy-config
deploy-config: create-namespace ## Deploy ConfigMap and Secret
	@echo "$(GREEN)Deploying ConfigMap and Secret...$(RESET)"
	kubectl apply -f configmap.yaml -n $(NAMESPACE)
	kubectl apply -f secret.yaml -n $(NAMESPACE)
	@echo "$(GREEN)Configuration deployed$(RESET)"

.PHONY: deploy-app
deploy-app: ## Deploy application (Deployment and Service)
	@echo "$(GREEN)Deploying application...$(RESET)"
	kubectl apply -f deployment.yaml -n $(NAMESPACE)
	kubectl apply -f service.yaml -n $(NAMESPACE)
	@echo "$(GREEN)Application deployed$(RESET)"

.PHONY: deploy
deploy: check-context validate deploy-config deploy-app ## Full deployment (config + app)
	@echo "$(GREEN)Deployment complete!$(RESET)"
	@echo ""
	@echo "$(YELLOW)Check status with:$(RESET)"
	@echo "  make status"
	@echo "  make logs"

.PHONY: status
status: ## Check deployment status
	@echo "$(GREEN)Deployment status:$(RESET)"
	kubectl get deployment chatbox-websocket -n $(NAMESPACE)
	@echo ""
	@echo "$(GREEN)Pod status:$(RESET)"
	kubectl get pods -n $(NAMESPACE) -l app=chatbox
	@echo ""
	@echo "$(GREEN)Service status:$(RESET)"
	kubectl get svc chatbox-websocket -n $(NAMESPACE)
	@echo ""
	@echo "$(GREEN)Ingress status:$(RESET)"
	kubectl get ingress chatbox-websocket-ingress -n $(NAMESPACE) 2>/dev/null || echo "No ingress found"

.PHONY: logs
logs: ## View logs from all pods
	kubectl logs -n $(NAMESPACE) -l app=chatbox --tail=100 -f

.PHONY: logs-pod
logs-pod: ## View logs from specific pod (usage: make logs-pod POD=pod-name)
	@if [ -z "$(POD)" ]; then \
		echo "$(RED)Error: POD variable not set$(RESET)"; \
		echo "Usage: make logs-pod POD=chatbox-websocket-xxxxx-yyyyy"; \
		exit 1; \
	fi
	kubectl logs -n $(NAMESPACE) $(POD) -f

.PHONY: describe
describe: ## Describe deployment
	kubectl describe deployment chatbox-websocket -n $(NAMESPACE)

.PHONY: events
events: ## Show recent events
	kubectl get events -n $(NAMESPACE) --sort-by='.lastTimestamp' | tail -20

.PHONY: shell
shell: ## Open shell in a pod (usage: make shell POD=pod-name)
	@if [ -z "$(POD)" ]; then \
		POD=$$(kubectl get pods -n $(NAMESPACE) -l app=chatbox -o jsonpath='{.items[0].metadata.name}'); \
	fi; \
	echo "$(GREEN)Opening shell in pod: $$POD$(RESET)"; \
	kubectl exec -it -n $(NAMESPACE) $$POD -- sh

.PHONY: port-forward
port-forward: ## Forward local port to service (default: 8080:80)
	@echo "$(GREEN)Forwarding localhost:8080 to service...$(RESET)"
	@echo "$(YELLOW)Access at: http://localhost:8080/chat/healthz$(RESET)"
	kubectl port-forward -n $(NAMESPACE) svc/chatbox-websocket 8080:80

.PHONY: scale
scale: ## Scale deployment (usage: make scale REPLICAS=5)
	@if [ -z "$(REPLICAS)" ]; then \
		echo "$(RED)Error: REPLICAS variable not set$(RESET)"; \
		echo "Usage: make scale REPLICAS=5"; \
		exit 1; \
	fi
	@echo "$(GREEN)Scaling to $(REPLICAS) replicas...$(RESET)"
	kubectl scale deployment chatbox-websocket -n $(NAMESPACE) --replicas=$(REPLICAS)
	@echo "$(GREEN)Scaled to $(REPLICAS) replicas$(RESET)"

.PHONY: restart
restart: ## Restart deployment (rolling restart)
	@echo "$(GREEN)Restarting deployment...$(RESET)"
	kubectl rollout restart deployment chatbox-websocket -n $(NAMESPACE)
	@echo "$(GREEN)Restart initiated$(RESET)"

.PHONY: rollout-status
rollout-status: ## Check rollout status
	kubectl rollout status deployment chatbox-websocket -n $(NAMESPACE)

.PHONY: rollback
rollback: ## Rollback to previous deployment
	@echo "$(YELLOW)Rolling back deployment...$(RESET)"
	kubectl rollout undo deployment chatbox-websocket -n $(NAMESPACE)
	@echo "$(GREEN)Rollback complete$(RESET)"

.PHONY: update-config
update-config: ## Update ConfigMap and restart pods
	@echo "$(GREEN)Updating ConfigMap...$(RESET)"
	kubectl apply -f configmap.yaml -n $(NAMESPACE)
	@echo "$(GREEN)Restarting pods to pick up new config...$(RESET)"
	kubectl rollout restart deployment chatbox-websocket -n $(NAMESPACE)
	@echo "$(GREEN)Config updated and pods restarted$(RESET)"

.PHONY: update-secret
update-secret: ## Update Secret and restart pods
	@echo "$(YELLOW)Warning: This will update sensitive data$(RESET)"
	@read -p "Continue? [y/N] " -n 1 -r; \
	echo; \
	if [[ ! $$REPLY =~ ^[Yy]$$ ]]; then \
		echo "$(RED)Aborted$(RESET)"; \
		exit 1; \
	fi
	@echo "$(GREEN)Updating Secret...$(RESET)"
	kubectl apply -f secret.yaml -n $(NAMESPACE)
	@echo "$(GREEN)Restarting pods to pick up new secret...$(RESET)"
	kubectl rollout restart deployment chatbox-websocket -n $(NAMESPACE)
	@echo "$(GREEN)Secret updated and pods restarted$(RESET)"

.PHONY: health
health: ## Check health endpoints
	@echo "$(GREEN)Checking health endpoints...$(RESET)"
	@POD=$$(kubectl get pods -n $(NAMESPACE) -l app=chatbox -o jsonpath='{.items[0].metadata.name}'); \
	echo "$(YELLOW)Health check:$(RESET)"; \
	kubectl exec -n $(NAMESPACE) $$POD -- wget -qO- http://localhost:8080/chat/healthz; \
	echo ""; \
	echo "$(YELLOW)Readiness check:$(RESET)"; \
	kubectl exec -n $(NAMESPACE) $$POD -- wget -qO- http://localhost:8080/chat/readyz; \
	echo ""

.PHONY: metrics
metrics: ## Show pod metrics (requires metrics-server)
	@echo "$(GREEN)Pod metrics:$(RESET)"
	kubectl top pods -n $(NAMESPACE) -l app=chatbox

.PHONY: hpa-status
hpa-status: ## Check HPA status
	@echo "$(GREEN)HPA status:$(RESET)"
	kubectl get hpa chatbox-websocket-hpa -n $(NAMESPACE)
	@echo ""
	kubectl describe hpa chatbox-websocket-hpa -n $(NAMESPACE)

.PHONY: delete
delete: ## Delete all resources
	@echo "$(RED)Warning: This will delete all chatbox resources$(RESET)"
	@read -p "Continue? [y/N] " -n 1 -r; \
	echo; \
	if [[ ! $$REPLY =~ ^[Yy]$$ ]]; then \
		echo "$(RED)Aborted$(RESET)"; \
		exit 1; \
	fi
	@echo "$(YELLOW)Deleting resources...$(RESET)"
	kubectl delete -f service.yaml -n $(NAMESPACE) || true
	kubectl delete -f deployment.yaml -n $(NAMESPACE) || true
	kubectl delete -f configmap.yaml -n $(NAMESPACE) || true
	kubectl delete -f secret.yaml -n $(NAMESPACE) || true
	@echo "$(GREEN)Resources deleted$(RESET)"

.PHONY: clean
clean: delete ## Alias for delete

.PHONY: backup
backup: ## Backup current configuration
	@echo "$(GREEN)Backing up configuration...$(RESET)"
	@mkdir -p backup
	kubectl get configmap chat-config -n $(NAMESPACE) -o yaml > backup/configmap-$$(date +%Y%m%d-%H%M%S).yaml
	kubectl get secret chat-secrets -n $(NAMESPACE) -o yaml > backup/secret-$$(date +%Y%m%d-%H%M%S).yaml
	kubectl get deployment chatbox-websocket -n $(NAMESPACE) -o yaml > backup/deployment-$$(date +%Y%m%d-%H%M%S).yaml
	kubectl get service chatbox-websocket -n $(NAMESPACE) -o yaml > backup/service-$$(date +%Y%m%d-%H%M%S).yaml
	@echo "$(GREEN)Backup complete in backup/ directory$(RESET)"

.PHONY: test-connection
test-connection: ## Test WebSocket connection (requires wscat)
	@echo "$(GREEN)Testing WebSocket connection...$(RESET)"
	@echo "$(YELLOW)Note: Requires wscat (npm install -g wscat)$(RESET)"
	@echo "$(YELLOW)Note: Replace YOUR_JWT_TOKEN with actual token$(RESET)"
	@kubectl port-forward -n $(NAMESPACE) svc/chatbox-websocket 8080:80 & \
	sleep 2; \
	wscat -c ws://localhost:8080/chat/ws?token=YOUR_JWT_TOKEN; \
	pkill -f "kubectl port-forward"

.PHONY: k3s-deploy
k3s-deploy: ## Deploy to K3s (uses Traefik ingress)
	@echo "$(GREEN)Deploying to K3s...$(RESET)"
	@echo "$(YELLOW)Note: K3s uses Traefik ingress by default$(RESET)"
	$(MAKE) deploy

.PHONY: info
info: ## Show deployment information
	@echo "$(GREEN)Deployment Information$(RESET)"
	@echo "$(YELLOW)Namespace:$(RESET) $(NAMESPACE)"
	@echo "$(YELLOW)Image:$(RESET) $(FULL_IMAGE)"
	@echo "$(YELLOW)Context:$(RESET) $(KUBE_CONTEXT)"
	@echo ""
	@echo "$(GREEN)Endpoints:$(RESET)"
	@echo "  WebSocket: ws://your-domain/chat/ws"
	@echo "  Health: http://your-domain/chat/healthz"
	@echo "  Ready: http://your-domain/chat/readyz"
	@echo "  Admin: http://your-domain/chat/admin/*"
