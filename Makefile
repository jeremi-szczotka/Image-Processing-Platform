.PHONY: help setup dev dev-build dev-infra down logs \
        test test-unit test-integration lint build \
        smoke-test migrate-status

COMPOSE      = docker compose -f infrastructure/docker-compose.yml
API_DIR      = services/api-service
JOB_DIR      = services/job-service
WORKER_DIR   = services/processing-worker

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
