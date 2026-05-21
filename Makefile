.PHONY: all build build-server build-client run-server run-client \
        test test-race lint vet fmt clean snapshot docker help

# ── Variables ─────────────────────────────────────────────────────────────────

BINARY_DIR    := ./bin
SERVER_BIN    := $(BINARY_DIR)/tunnd-server
CLIENT_BIN    := $(BINARY_DIR)/tunnd
VERSION       := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT        := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE    := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS       := -s -w \
	-X main.Version=$(VERSION) \
	-X main.CommitSHA=$(COMMIT) \
	-X main.BuildDate=$(BUILD_DATE)
GO_FLAGS      := -trimpath -ldflags "$(LDFLAGS)"
GOTEST        := go test ./...

# ── Build ─────────────────────────────────────────────────────────────────────

all: build

build: build-server build-client

build-server: ## Build the server binary
	@mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 go build $(GO_FLAGS) -o $(SERVER_BIN) ./cmd/server
	@echo "Built $(SERVER_BIN)"

build-client: ## Build the client binary
	@mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 go build $(GO_FLAGS) -o $(CLIENT_BIN) ./cmd/client
	@echo "Built $(CLIENT_BIN)"

# ── Run (development) ─────────────────────────────────────────────────────────

run-server: build-server ## Run server with example config
	$(SERVER_BIN) --config configs/tunnd-server.example.yaml

run-client: build-client ## Run client (requires prior: tunnd setup)
	$(CLIENT_BIN) http 3000

# ── Test ──────────────────────────────────────────────────────────────────────

test: ## Run all tests
	$(GOTEST) -v -count=1 -timeout 60s

test-race: ## Run tests with race detector
	$(GOTEST) -race -count=1 -timeout 120s

test-cover: ## Run tests with coverage report
	go test ./... -coverprofile=coverage.out -covermode=atomic
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# ── Code quality ──────────────────────────────────────────────────────────────

lint: ## Run golangci-lint
	golangci-lint run ./...

vet: ## Run go vet
	go vet ./...

fmt: ## Format all Go source files
	gofmt -w -s .
	goimports -w .

tidy: ## Tidy go.mod and go.sum
	go mod tidy

# ── Release ───────────────────────────────────────────────────────────────────

snapshot: ## Build snapshot release with goreleaser (no publish)
	goreleaser release --snapshot --clean

release: ## Publish a release (requires GITHUB_TOKEN)
	goreleaser release --clean

# ── Docker ────────────────────────────────────────────────────────────────────

docker: ## Build server Docker image locally
	docker build -f Dockerfile.server -t tunnd-server:$(VERSION) .

docker-compose-up: ## Start server via docker-compose
	docker compose up -d

docker-compose-down: ## Stop docker-compose stack
	docker compose down

# ── Utility ───────────────────────────────────────────────────────────────────

token-create: build-server ## Create a new auth token (usage: make token-create LABEL=mytoken)
	$(SERVER_BIN) token create $(or $(LABEL),default)

token-list: build-server ## List all auth tokens
	$(SERVER_BIN) token list

clean: ## Remove build artifacts
	rm -rf $(BINARY_DIR) dist coverage.out coverage.html

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
	  awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
