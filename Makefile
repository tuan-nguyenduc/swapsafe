BIN        := bin/swapsafe
PKG        := github.com/tuanduke/swapsafe
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE       ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS    := -s -w \
	-X $(PKG)/internal/cli.version=$(VERSION) \
	-X $(PKG)/internal/cli.commit=$(COMMIT) \
	-X $(PKG)/internal/cli.date=$(DATE)

GOLANGCI_VERSION := v2.15.0

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) \
		| awk -F':.*?## ' '{printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ## Build the swapsafe binary into bin/
	go build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN) ./cmd/swapsafe

.PHONY: test
test: ## Run unit tests with the race detector
	go test -race -coverprofile=coverage.out ./...

.PHONY: lint
lint: ## Run golangci-lint
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION) run

.PHONY: fmt
fmt: ## Format and tidy
	go fmt ./...
	go mod tidy

.PHONY: e2e
e2e: ## Run the kind-based end-to-end suite (M1-T11)
	go test -tags e2e -timeout 30m ./test/e2e/...

.PHONY: golden
golden: ## Regenerate golden report fixtures (review the diff!)
	go test ./... -run 'Golden' -update

.PHONY: snapshot
snapshot: ## Build a local goreleaser snapshot (no publish)
	goreleaser release --snapshot --clean
