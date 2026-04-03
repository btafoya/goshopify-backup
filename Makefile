# Makefile for goshopify-backup

.PHONY: all build test clean run docker-build docker-run docker-push fmt lint install help

# Variables
BINARY_NAME=goshopify-backup
BUILD_DIR=bin
DOCKER_IMAGE=goshopify-backup
DOCKER_TAG=latest
GO=go
GOFLAGS=-v

# Build the application
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) .

# Run the application locally
run:
	$(GO) run .

# Run with force flag
run-force:
	$(GO) run . --force

# Run tests
test:
	@echo "Running tests..."
	$(GO) test -v -race -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

# Run tests with coverage report
test-coverage:
	@echo "Running tests with coverage..."
	$(GO) test -v -race -coverprofile=coverage.out ./...
	@echo ""
	@echo "Coverage Report:"
	@$(GO) tool cover -func=coverage.out

# Format code
fmt:
	@echo "Formatting code..."
	$(GO) fmt ./...

# Run linter
lint:
	@echo "Running linter..."
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

# Run go vet
vet:
	@echo "Running go vet..."
	$(GO) vet ./...

# Install dependencies
deps:
	@echo "Installing dependencies..."
	$(GO) mod download
	$(GO) mod tidy

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

# Clean backups
clean-backups:
	@echo "Cleaning backups..."
	rm -rf backups/*

# Build Docker image
docker-build:
	@echo "Building Docker image..."
	docker build -f docker/Dockerfile -t $(DOCKER_IMAGE):$(DOCKER_TAG) .

# Run Docker container
docker-run: docker-build
	@echo "Running Docker container..."
	docker run --rm \
		--env-file .env \
		-v $(PWD)/backups:/backups \
		$(DOCKER_IMAGE):$(DOCKER_TAG)

# Run Docker container in detached mode
docker-daemon: docker-build
	@echo "Running Docker container in detached mode..."
	docker run -d \
		--name $(BINARY_NAME) \
		--env-file .env \
		-v $(PWD)/backups:/backups \
		--restart unless-stopped \
		$(DOCKER_IMAGE):$(DOCKER_TAG)

# Stop Docker container
docker-stop:
	@echo "Stopping Docker container..."
	docker stop $(BINARY_NAME) || true
	docker rm $(BINARY_NAME) || true

# View Docker logs
docker-logs:
	docker logs -f $(BINARY_NAME)

# Run docker-compose
dc-up:
	@echo "Starting services with docker-compose..."
	docker-compose -f docker/docker-compose.yml up -d

# Stop docker-compose
dc-down:
	@echo "Stopping services with docker-compose..."
	docker-compose -f docker/docker-compose.yml down

# View docker-compose logs
dc-logs:
	docker-compose -f docker/docker-compose.yml logs -f

# Install the binary
install: build
	@echo "Installing $(BINARY_NAME) to /usr/local/bin..."
	install -m 755 $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/

# Install binary to GOPATH/bin
install-go: build
	@echo "Installing $(BINARY_NAME) to GOPATH/bin..."
	$(GO) install

# Run all checks
check: fmt vet lint test

# Build for multiple platforms
build-all: clean
	@echo "Building for multiple platforms..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GO) build -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 .
	GOOS=linux GOARCH=arm64 $(GO) build -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 .
	GOOS=darwin GOARCH=amd64 $(GO) build -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 $(GO) build -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 .
	@echo "Builds created:"
	@ls -lh $(BUILD_DIR)/

# Create release tarball
release: build-all
	@echo "Creating release tarball..."
	@mkdir -p releases
	@for platform in linux-amd64 linux-arm64 darwin-amd64 darwin-arm64; do \
		tar -czf releases/$(BINARY_NAME)-$$platform.tar.gz -C $(BUILD_DIR) $(BINARY_NAME)-$$platform; \
	done
	@echo "Releases created:"
	@ls -lh releases/

# Run backup
backup: run

# Force backup
backup-force: run-force

# Show help
help:
	@echo "Available targets:"
	@echo "  build          - Build the binary"
	@echo "  run            - Run the application locally"
	@echo "  run-force      - Run with force flag"
	@echo "  test           - Run tests"
	@echo "  test-coverage  - Run tests with coverage report"
	@echo "  fmt            - Format code"
	@echo "  lint           - Run linter"
	@echo "  vet            - Run go vet"
	@echo "  deps           - Install dependencies"
	@echo "  clean          - Clean build artifacts"
	@echo "  clean-backups  - Clean backups directory"
	@echo "  docker-build   - Build Docker image"
	@echo "  docker-run     - Run Docker container"
	@echo "  docker-daemon  - Run Docker container in detached mode"
	@echo "  docker-stop    - Stop Docker container"
	@echo "  docker-logs    - View Docker logs"
	@echo "  dc-up          - Start services with docker-compose"
	@echo "  dc-down        - Stop services with docker-compose"
	@echo "  dc-logs        - View docker-compose logs"
	@echo "  install        - Install to /usr/local/bin"
	@echo "  install-go     - Install to GOPATH/bin"
	@echo "  check          - Run all checks (fmt, vet, lint, test)"
	@echo "  build-all      - Build for multiple platforms"
	@echo "  release        - Create release tarballs"
	@echo "  backup         - Run backup"
	@echo "  backup-force   - Run backup with force flag"
	@echo "  help           - Show this help message"

.DEFAULT_GOAL := build