.PHONY: help setup dev dev-build dev-infra down logs \
        test test-unit test-integration lint build \
        smoke-test migrate-status \
        k8s-create-cluster k8s-delete-cluster k8s-configmaps k8s-secret k8s-build k8s-keda \
        k8s-up k8s-down k8s-status k8s-logs

COMPOSE      = docker compose -f infrastructure/docker-compose.yml
API_DIR      = services/api-service
JOB_DIR      = services/job-service
WORKER_DIR   = services/processing-worker
FRONTEND_DIR = services/frontend

# ── Kubernetes (chapter 5) ──────────────────────────────────────────────────
K3D_CLUSTER := image-platform
K8S_DIR     := infrastructure/kubernetes
K8S_NS      := image-platform

help:
	@echo ""
	@echo "Image Processing Platform — available targets:"
	@echo ""
	@echo "  setup             Copy .env.example → .env and fetch deps"
	@echo "  dev               Start all services (docker compose up)"
	@echo "  dev-build         Rebuild images and start"
	@echo "  dev-infra         Start infrastructure only (postgres, minio)"
	@echo "  down              Stop all services"
	@echo "  logs              Tail all service logs"
	@echo ""
	@echo "  test-unit         Run unit tests for all services (no external services)"
	@echo "  test-integration  Run integration smoke test (requires running services)"
	@echo "  lint              go vet + python syntax check"
	@echo "  build             Compile Go services locally"
	@echo ""

setup:
	@[ -f .env ] || (cp .env.example .env && echo "Created .env from .env.example — review and adjust if needed")
	@cd $(API_DIR) && go mod tidy
	@cd $(JOB_DIR) && go mod tidy
	@cd $(WORKER_DIR) && pip install -r requirements.txt -q
	@chmod +x scripts/*.sh
	@echo "Setup complete."

dev:
	$(COMPOSE) up

dev-build:
	$(COMPOSE) up --build

dev-infra:
	$(COMPOSE) up postgres minio

down:
	$(COMPOSE) down

logs:
	$(COMPOSE) logs -f

build:
	@echo "→ Building api-service..."
	@cd $(API_DIR) && go build ./cmd/api/...
	@echo "→ Building job-service..."
	@cd $(JOB_DIR) && go build ./cmd/server/...
	@echo "Build OK"

lint:
	@cd $(API_DIR) && go vet ./...
	@cd $(JOB_DIR) && go vet ./...
	@find $(WORKER_DIR)/app -name "*.py" | xargs python -m py_compile && echo "→ processing-worker: syntax OK"

test-unit:
	@echo "→ Unit tests: api-service"
	@cd $(API_DIR) && go test ./... -v -count=1 -race
	@echo "→ Unit tests: job-service"
	@cd $(JOB_DIR) && go test ./... -v -count=1 -race
	@echo "→ Unit tests: processing-worker"
	@cd $(WORKER_DIR) && python -m pytest tests/ -v

test-integration: smoke-test

smoke-test:
	@bash scripts/smoke-test.sh

# ── Kubernetes targets (chapter 5) ──────────────────────────────────────────
k8s-create-cluster: ## Create the k3d cluster (Traefik on localhost:8088)
	k3d cluster create $(K3D_CLUSTER) \
	  --port "8088:80@loadbalancer" \
	  --agents 1 \
	  --wait
	kubectl config use-context k3d-$(K3D_CLUSTER)

k8s-delete-cluster: ## Delete the k3d cluster
	- k3d cluster delete $(K3D_CLUSTER)

k8s-configmaps: ## Regenerate the db-migrations ConfigMap from source SQL
	kubectl create configmap db-migrations \
	  --namespace $(K8S_NS) \
	  --from-file=database/migrations/ \
	  --dry-run=client -o yaml > $(K8S_DIR)/configmaps/db-migrations.yaml

k8s-secret: ## Create the real secret from the template on first run, then apply it
	@test -f $(K8S_DIR)/secrets.yaml || cp $(K8S_DIR)/secrets.example/platform-secrets.example.yaml $(K8S_DIR)/secrets.yaml
	kubectl apply -f $(K8S_DIR)/secrets.yaml

IMG_TAG := dev
k8s-build: ## Build the four service images and import them into k3d
	docker build -t image-platform/api-service:$(IMG_TAG)        services/api-service
	docker build -t image-platform/job-service:$(IMG_TAG)        services/job-service
	docker build -t image-platform/processing-worker:$(IMG_TAG)  services/processing-worker
	docker build -t image-platform/frontend:$(IMG_TAG)           services/frontend
	k3d image import \
	  image-platform/api-service:$(IMG_TAG) \
	  image-platform/job-service:$(IMG_TAG) \
	  image-platform/processing-worker:$(IMG_TAG) \
	  image-platform/frontend:$(IMG_TAG) \
	  -c $(K3D_CLUSTER)

KEDA_VERSION := 2.21.1
k8s-keda: ## Install KEDA and metrics-server into the cluster
	kubectl apply --server-side -f https://github.com/kedacore/keda/releases/download/v$(KEDA_VERSION)/keda-$(KEDA_VERSION).yaml
	kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
	# k3d/k3s kubelet serving cert is self-signed; allow metrics-server to trust it.
	kubectl patch -n kube-system deployment metrics-server --type=json \
	  -p='[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--kubelet-insecure-tls"}]'
	kubectl wait --for=condition=available deployment/keda-operator -n keda --timeout=180s

k8s-up: k8s-create-cluster k8s-keda k8s-build ## Full bring-up: cluster → KEDA → images → manifests
	kubectl apply -f $(K8S_DIR)/namespace.yaml
	kubectl apply -f $(K8S_DIR)/configmaps/
	$(MAKE) k8s-secret
	kubectl apply -f $(K8S_DIR)/deployments/postgres.yaml -f $(K8S_DIR)/services/postgres.yaml
	kubectl apply -f $(K8S_DIR)/deployments/rabbitmq.yaml -f $(K8S_DIR)/services/rabbitmq.yaml
	kubectl apply -f $(K8S_DIR)/deployments/minio.yaml    -f $(K8S_DIR)/services/minio.yaml
	kubectl wait --for=condition=available deployment/postgres deployment/rabbitmq deployment/minio -n $(K8S_NS) --timeout=180s
	kubectl apply -f $(K8S_DIR)/deployments/api-service.yaml -f $(K8S_DIR)/services/api-service.yaml
	kubectl apply -f $(K8S_DIR)/deployments/job-service.yaml -f $(K8S_DIR)/services/job-service.yaml
	kubectl wait --for=condition=available deployment/api-service deployment/job-service -n $(K8S_NS) --timeout=180s
	kubectl apply -f $(K8S_DIR)/deployments/processing-worker.yaml
	kubectl apply -f $(K8S_DIR)/deployments/frontend.yaml -f $(K8S_DIR)/services/frontend.yaml
	kubectl apply -f $(K8S_DIR)/services/ingress.yaml
	kubectl apply -f $(K8S_DIR)/autoscaling/
	kubectl wait --for=condition=available deployment/frontend deployment/processing-worker -n $(K8S_NS) --timeout=180s
	@echo "Platform up → http://localhost:8088"

k8s-down: k8s-delete-cluster ## Tear down the whole cluster

k8s-status: ## Show pods, services, HPA, ScaledObject
	kubectl get pods,svc,hpa,scaledobject -n $(K8S_NS)

k8s-logs: ## Tail processing-worker logs
	kubectl logs -f -n $(K8S_NS) deploy/processing-worker
