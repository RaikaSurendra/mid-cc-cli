.PHONY: build clean test run deps install

# Build configuration
BINARY_DIR=bin
SERVER_BINARY=$(BINARY_DIR)/claude-terminal-service
POLLER_BINARY=$(BINARY_DIR)/ecc-poller

# Go configuration
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Build flags
LDFLAGS=-ldflags "-s -w"

all: clean deps build

build: build-server build-poller

build-server:
	@echo "Building Claude Terminal Service..."
	@mkdir -p $(BINARY_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(SERVER_BINARY) ./cmd/server

build-poller:
	@echo "Building ECC Queue Poller..."
	@mkdir -p $(BINARY_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(POLLER_BINARY) ./cmd/ecc-poller

clean:
	@echo "Cleaning..."
	@$(GOCLEAN)
	@rm -rf $(BINARY_DIR)

test:
	@echo "Running tests..."
	@$(GOTEST) -v ./...

test-coverage:
	@echo "Running tests with coverage..."
	@$(GOTEST) -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

test-race:
	@echo "Running tests with race detector..."
	@$(GOTEST) -race ./...

bench:
	@echo "Running benchmarks..."
	@$(GOTEST) -bench=. -benchmem -benchtime=3s ./...

bench-full:
	@echo "Running comprehensive benchmarks..."
	@./scripts/run-benchmarks.sh

test-all:
	@echo "Running all tests..."
	@./scripts/run-tests.sh

deps:
	@echo "Downloading dependencies..."
	@$(GOMOD) download
	@$(GOMOD) tidy

run-server:
	@echo "Running Claude Terminal Service..."
	@$(GOBUILD) -o $(SERVER_BINARY) ./cmd/server
	@$(SERVER_BINARY)

run-poller:
	@echo "Running ECC Queue Poller..."
	@$(GOBUILD) -o $(POLLER_BINARY) ./cmd/ecc-poller
	@$(POLLER_BINARY)

install:
	@echo "Installing binaries to /usr/local/bin..."
	@sudo cp $(SERVER_BINARY) /usr/local/bin/
	@sudo cp $(POLLER_BINARY) /usr/local/bin/
	@echo "Installation complete!"

# Docker targets (optional)
docker-build:
	@echo "Building Docker image..."
	@docker build -t claude-terminal-service:latest .

docker-run:
	@echo "Running Docker container..."
	@docker run --env-file .env -p 3000:3000 claude-terminal-service:latest

# Development helpers
fmt:
	@echo "Formatting code..."
	@go fmt ./...

lint:
	@echo "Linting code..."
	@golangci-lint run

# Version info
version:
	@echo "Go version: $(shell go version)"
	@echo "Server binary: $(SERVER_BINARY)"
	@echo "Poller binary: $(POLLER_BINARY)"
