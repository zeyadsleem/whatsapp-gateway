SHELL := /bin/bash
APP_NAME := whatsapp-gateway
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
	-X github.com/zeyad/whatsapp-gateway/internal/version.Version=$(VERSION) \
	-X github.com/zeyad/whatsapp-gateway/internal/version.Commit=$(COMMIT) \
	-X github.com/zeyad/whatsapp-gateway/internal/version.BuildTime=$(BUILD_TIME)

.PHONY: help
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-18s %s\n", $$1, $$2}'

.PHONY: build
build: ## Build the Go server (expects web/admin/dist to exist)
	CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o bin/server ./cmd/server

.PHONY: run
run: ## Run the server locally
	go run ./cmd/server

.PHONY: web
web: ## Build the admin dashboard
	cd web/admin && pnpm install && pnpm build

.PHONY: test
test: ## Run unit tests (integration tests skip without TEST_DATABASE_URL)
	go test ./...

.PHONY: test-race
test-race: ## Run tests with the race detector
	go test -race ./...

.PHONY: test-integration
test-integration: ## Run all tests against TEST_DATABASE_URL
	@test -n "$$TEST_DATABASE_URL" || (echo "TEST_DATABASE_URL is required" && exit 1)
	go test ./...

.PHONY: vet
vet: ## Static analysis
	go vet ./...

.PHONY: fmt
fmt: ## Format Go code
	gofmt -w cmd internal web

.PHONY: lint
lint: fmt vet ## Format and vet

.PHONY: quality
quality: build test-race ## Local quality gate
	@echo "quality gate passed"

.PHONY: docker-build
docker-build: ## Build the production image
	docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) -t $(APP_NAME):$(VERSION) .

.PHONY: up
up: ## Start with docker compose
	docker compose up -d --build

.PHONY: down
down: ## Stop docker compose
	docker compose down

.PHONY: logs
logs: ## Follow application logs
	docker compose logs -f app

.PHONY: privacy-audit
privacy-audit: ## Inspect the database schema for forbidden tables and columns
	./scripts/privacy-audit.sh
