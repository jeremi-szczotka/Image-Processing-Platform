.PHONY: help setup dev dev-build dev-infra down logs \
        test test-unit test-integration lint build \
        smoke-test migrate-status

COMPOSE  = docker compose -f infrastructure/docker-compose.yml
API_DIR  = services/api-service
JOB_DIR  = services/job-service

help:
	@echo ""
	@echo "Image Processing Platform — available targets:"
	@echo ""
	@echo "  setup             Copy .env.example → .env and fetch Go deps"
	@echo "  dev               Start all services (docker compose up)"
	@echo "  dev-build         Rebuild images and start"
	@echo "  dev-infra         Start infrastructure only (postgres, minio)"
	@echo "  down              Stop all services"
	@echo "  logs              Tail all service logs"
	@echo ""
	@echo "  test-unit         Run Go unit tests (no external services needed)"
	@echo "  test-integration  Run integration smoke test (requires running services)"
	@echo "  lint              go vet on all Go services"
	@echo "  build             Compile all Go services locally"
	@echo ""

setup:
	@[ -f .env ] || (cp .env.example .env && echo "Created .env from .env.example — review and adjust if needed")
	@cd $(API_DIR) && go mod tidy
	@cd $(JOB_DIR) && go mod tidy
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

test-unit:
	@echo "→ Unit tests: api-service"
	@cd $(API_DIR) && go test ./... -v -count=1 -race
	@echo "→ Unit tests: job-service"
	@cd $(JOB_DIR) && go test ./... -v -count=1 -race

test-integration: smoke-test

smoke-test:
	@bash scripts/smoke-test.sh
