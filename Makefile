# Proyecto T — task runner. `make` or `make help` lists every target.

SHELL := /usr/bin/env bash
.SHELLFLAGS := -e -o pipefail -c
.DEFAULT_GOAL := help

# scripts/install-go.sh installs without sudo, under ~/.local.
export PATH := $(HOME)/.local/opt/go/bin:$(HOME)/.local/go/bin:$(HOME)/.local/bin:$(PATH)

COMPOSE     := docker compose
GO_DIR      := apps/qr-api
NODE_DIR    := apps/stats-api
WEB_DIR     := apps/web
GO_IMAGE    := golang:1.27.1-alpine
OPENAPI     := $(GO_DIR)/openapi.yaml $(NODE_DIR)/openapi.yaml
TOOLCHAIN   := $(HOME)/.local/etc/dev-toolchain.sh

.PHONY: help setup up down logs build test test-go test-node test-web test-go-docker \
        lint lint-go lint-node lint-web openapi-lint e2e contract-check demo \
        secrets deploy-plan deploy rollback

## ---- Getting started -------------------------------------------------------

help: ## Show this help
	@awk 'BEGIN {FS = ":.*?## "} \
	     /^## ----/ { sub(/^## /, ""); printf "\n\033[1m%s\033[0m\n", $$0; next } \
	     /^[a-zA-Z0-9_.-]+:.*?## / { printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
	@echo ""

setup: ## Install the local toolchain (Go, Terraform, linters) and print versions
	@bash ./scripts/install-go.sh
	@set +e; [ -f "$(TOOLCHAIN)" ] && . "$(TOOLCHAIN)"; \
	 echo "--- toolchain ---"; \
	 printf 'go            : %s\n' "$$(go version 2>/dev/null || echo 'not found')"; \
	 printf 'golangci-lint : %s\n' "$$(golangci-lint --version 2>/dev/null || echo 'not found')"; \
	 printf 'govulncheck   : %s\n' "$$(govulncheck -version 2>/dev/null | tail -n 1 || echo 'not found')"; \
	 printf 'node          : %s\n' "$$(node --version 2>/dev/null || echo 'not found')"; \
	 printf 'npm           : %s\n' "$$(npm --version 2>/dev/null || echo 'not found')"; \
	 printf 'docker        : %s\n' "$$(docker --version 2>/dev/null || echo 'not found')"; \
	 printf 'compose       : %s\n' "$$(docker compose version 2>/dev/null || echo 'not found')"; \
	 printf 'terraform     : %s\n' "$$(terraform version 2>/dev/null | head -n 1 || echo 'not found')"; \
	 exit 0

secrets: ## Create .env from .env.example and generate JWT + demo client secrets
	@bash ./scripts/gen-secrets.sh

## ---- Run -------------------------------------------------------------------

up: ## Build and start the three services (web :8081, qr-api :8080, stats-api :3000)
	@$(COMPOSE) up --build --wait

down: ## Stop the stack and remove its volumes
	@$(COMPOSE) down --volumes --remove-orphans

logs: ## Follow the logs of every service
	@$(COMPOSE) logs --follow --tail=100

build: ## Build the three Docker images without starting them
	@$(COMPOSE) build

## ---- Test ------------------------------------------------------------------

test: test-go test-node test-web ## Run every test suite

test-go: ## Go unit + integration tests with race detector and coverage
	@cd $(GO_DIR) && go test -race -covermode=atomic -coverprofile=coverage.out ./... && go tool cover -func=coverage.out | tail -n 1

test-go-docker: ## Go tests inside Docker (fallback when Go is not installed locally)
	@docker run --rm -v "$(PWD)/apps/qr-api":/src -w /src $(GO_IMAGE) go test ./...

test-node: ## stats-api tests (vitest) with coverage
	@cd $(NODE_DIR) && npm run test

test-web: ## web tests (vitest + Testing Library)
	@cd $(WEB_DIR) && npm run test

e2e: ## End-to-end smoke test against the running compose stack
	@bash ./scripts/e2e-smoke.sh


contract-check: ## Validate every live response against components.schemas of both OpenAPI docs
	@cd scripts && { [ -d node_modules ] || npm ci --no-audit --no-fund; }
	@node ./scripts/contract-check.mjs

demo: ## Guided walkthrough of the whole flow (local by default, --cloud for Cloud Run)
	@bash ./scripts/demo.sh

## ---- Quality ---------------------------------------------------------------

lint: lint-go lint-node lint-web openapi-lint ## Run every linter

lint-go: ## gofmt check, go vet and golangci-lint
	@cd $(GO_DIR) && test -z "$$(gofmt -l .)" || { echo "gofmt: files need formatting:"; gofmt -l .; exit 1; }
	@cd $(GO_DIR) && go vet ./... && golangci-lint run

lint-node: ## ESLint + tsc --noEmit for stats-api
	@cd $(NODE_DIR) && npm run lint && npm run typecheck

lint-web: ## ESLint + tsc --noEmit for web
	@cd $(WEB_DIR) && npm run lint && npm run typecheck

openapi-lint: ## Validate both OpenAPI 3.1 documents with Redocly
	@npx --yes @redocly/cli@latest lint $(OPENAPI)

## ---- Deploy ----------------------------------------------------------------

deploy-plan: ## Terraform plan for Cloud Run (no changes applied)
	@bash ./scripts/deploy-gcp.sh plan

deploy: ## Build, push and apply the Cloud Run deployment
	@bash ./scripts/deploy-gcp.sh apply
deploy-cloud: ## Same, but the images are built by Cloud Build (nothing runs on this machine)
	@bash ./scripts/deploy-gcp.sh apply --cloud-build

rollback: ## Shift 100% of a Cloud Run service's traffic back to its previous revision
	@bash ./scripts/rollback-gcp.sh

