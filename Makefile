.PHONY: help build test test-race vet fmt fmt-check check proto tools clean clean-state \
	run-server run-cli run-watch run-spot run-futures run-hedger run-noise run-arbitrage

BIN_DIR := bin
DATA_DIR := data
GOBIN := $(shell go env GOPATH)/bin
BOTS := spot futures hedger noise arbitrage

help: ## Show this help
	@awk 'BEGIN {FS = ":.*## "} /^##@/ {printf "\n%s\n", substr($$0, 5); next} /^[a-zA-Z_-]+:.*## / {printf "  %-14s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

##@ Build & check

build: ## Build all binaries into bin/
	@mkdir -p $(BIN_DIR)
	@go build -o $(BIN_DIR)/server ./cmd/server
	@go build -o $(BIN_DIR)/cli ./cmd/cli
	@for bot in $(BOTS); do go build -o $(BIN_DIR)/$$bot ./cmd/bots/strategies/$$bot; done

test: ## Run tests
	@go test ./...

test-race: ## Run tests with the race detector
	@go test ./... -race -count=1

vet: ## Run go vet
	@go vet ./...

fmt: ## Format all Go files
	@gofmt -w .

fmt-check: ## Check formatting without modifying files
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)

check: fmt-check vet test-race ## Run all checks (format, vet, race tests)

tools: ## Install protoc-gen-go and protoc-gen-go-grpc
	@go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	@go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

proto: ## Regenerate protobuf/gRPC code from proto/janus.proto
	@PATH="$(PATH):$(GOBIN)" protoc \
		--go_out=internal/api/proto --go_opt=paths=source_relative \
		--go-grpc_out=internal/api/proto --go-grpc_opt=paths=source_relative \
		-I proto proto/janus.proto

clean: ## Remove built binaries
	@rm -rf $(BIN_DIR)

clean-state: ## Remove the persisted exchange snapshot, so the next run-server starts fresh
	@rm -rf $(DATA_DIR)

##@ Run (pass flags/symbols with ARGS="...", e.g. make run-spot ARGS="AAPL")

run-server: ## Run the exchange server, e.g. ARGS="-addr :50051"
	@mkdir -p $(BIN_DIR)
	@go build -o $(BIN_DIR)/server ./cmd/server
	-@$(BIN_DIR)/server $(ARGS)

run-cli: ## Run the interactive CLI, e.g. ARGS="AAPL"
	@mkdir -p $(BIN_DIR)
	@go build -o $(BIN_DIR)/cli ./cmd/cli
	-@$(BIN_DIR)/cli $(ARGS)

run-watch: ## Watch live trades for a symbol, e.g. ARGS="AAPL"
	@mkdir -p $(BIN_DIR)
	@go build -o $(BIN_DIR)/cli ./cmd/cli
	-@$(BIN_DIR)/cli $(ARGS) watch

run-spot: ## Run the spot market-maker bot, e.g. ARGS="AAPL"
	@mkdir -p $(BIN_DIR)
	@go build -o $(BIN_DIR)/spot ./cmd/bots/strategies/spot
	-@$(BIN_DIR)/spot $(ARGS)

run-futures: ## Run the futures market-maker bot, e.g. ARGS="-spot AAPL AAPLF"
	@mkdir -p $(BIN_DIR)
	@go build -o $(BIN_DIR)/futures ./cmd/bots/strategies/futures
	-@$(BIN_DIR)/futures $(ARGS)

run-hedger: ## Run the hedger bot, e.g. ARGS="-futures AAPLF -spot AAPL"
	@mkdir -p $(BIN_DIR)
	@go build -o $(BIN_DIR)/hedger ./cmd/bots/strategies/hedger
	-@$(BIN_DIR)/hedger $(ARGS)

run-noise: ## Run the noise-trader bot, e.g. ARGS="-symbols AAPL,AAPLF"
	@mkdir -p $(BIN_DIR)
	@go build -o $(BIN_DIR)/noise ./cmd/bots/strategies/noise
	-@$(BIN_DIR)/noise $(ARGS)

run-arbitrage: ## Run the arbitrage bot, e.g. ARGS="-spot AAPL -futures AAPLF"
	@mkdir -p $(BIN_DIR)
	@go build -o $(BIN_DIR)/arbitrage ./cmd/bots/strategies/arbitrage
	-@$(BIN_DIR)/arbitrage $(ARGS)
