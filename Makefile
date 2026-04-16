# Makefile for goshopify-backup + goshopify-restore
#
# Two separate Go modules:
#   - backup:  github.com/btafoya/goshopify-backup  (project root)
#   - restore: github.com/btafoya/goshopify-restore (cmd/restore/)
#
# All binaries go to bin/

.PHONY: all build build-backup build-restore test test-backup test-restore \
        vet vet-backup vet-restore fmt lint clean run run-restore \
        docker-build docker-run docker-daemon docker-stop docker-logs \
        dc-up dc-down dc-logs install install-restore deps check \
        build-all release backup backup-force help

# Variables
BACKUP_BINARY  = goshopify-backup
RESTORE_BINARY = goshopify-restore
BUILD_DIR      = bin
RESTORE_DIR    = cmd/restore
DOCKER_IMAGE   = goshopify-backup
DOCKER_TAG     = latest
GO             = go
GOFLAGS        = -v
LDFLAGS        = -ldflags="-s -w"

# ---------------------------------------------------------------------------
# Build
# ---------------------------------------------------------------------------

all: build-backup build-restore

build: build-backup

build-backup:
	@echo "Building $(BACKUP_BINARY)..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$(BACKUP_BINARY) .

build-restore:
	@echo "Building $(RESTORE_BINARY)..."
	@mkdir -p $(BUILD_DIR)
	cd $(RESTORE_DIR) && $(GO) build $(GOFLAGS) -o ../../$(BUILD_DIR)/$(RESTORE_BINARY) .

# Production builds (stripped, no debug)
build-prod: build-backup-prod build-restore-prod

build-backup-prod:
	@echo "Building $(BACKUP_BINARY) (production)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BACKUP_BINARY) .

build-restore-prod:
	@echo "Building $(RESTORE_BINARY) (production)..."
	@mkdir -p $(BUILD_DIR)
	cd $(RESTORE_DIR) && CGO_ENABLED=0 $(GO) build $(LDFLAGS) -o ../../$(BUILD_DIR)/$(RESTORE_BINARY) .

# ---------------------------------------------------------------------------
# Test
# ---------------------------------------------------------------------------

test: test-backup test-restore

test-backup:
	@echo "Running backup tests..."
	$(GO) test -v -race -coverprofile=coverage-backup.out ./...
	$(GO) tool cover -html=coverage-backup.out -o coverage-backup.html

test-restore:
	@echo "Running restore tests..."
	cd $(RESTORE_DIR) && $(GO) test -v -race -coverprofile=../../coverage-restore.out ./...
	cd $(RESTORE_DIR) && $(GO) tool cover -html=../../coverage-restore.out -o ../../coverage-restore.html

test-coverage: test

# ---------------------------------------------------------------------------
# Lint / Vet / Fmt
# ---------------------------------------------------------------------------

fmt:
	@echo "Formatting code..."
	$(GO) fmt ./...
	cd $(RESTORE_DIR) && $(GO) fmt ./...

vet: vet-backup vet-restore

vet-backup:
	@echo "Running go vet (backup)..."
	$(GO) vet ./...

vet-restore:
	@echo "Running go vet (restore)..."
	cd $(RESTORE_DIR) && $(GO) vet ./...

lint:
	@echo "Running linter..."
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run ./...; \
		cd $(RESTORE_DIR) && golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed. Install: go install github.com/golangci-lint/cmd/golangci-lint@latest"; \
	fi

check: fmt vet lint test

# ---------------------------------------------------------------------------
# Run
# ---------------------------------------------------------------------------

run:
	$(GO) run .

run-restore:
	cd $(RESTORE_DIR) && $(GO) run .

run-force:
	$(GO) run . --force

backup: run

backup-force: run-force

# ---------------------------------------------------------------------------
# Dependencies
# ---------------------------------------------------------------------------

deps:
	@echo "Installing dependencies..."
	$(GO) mod download && $(GO) mod tidy
	cd $(RESTORE_DIR) && $(GO) mod download && $(GO) mod tidy

# ---------------------------------------------------------------------------
# Clean
# ---------------------------------------------------------------------------

clean:
	@echo "Cleaning..."
	rm -rf $(BUILD_DIR)
	rm -f coverage-backup.out coverage-backup.html
	rm -f coverage-restore.out coverage-restore.html

clean-backups:
	@echo "Cleaning backups..."
	rm -rf backups/*

# ---------------------------------------------------------------------------
# Docker
# ---------------------------------------------------------------------------

docker-build:
	@echo "Building Docker image..."
	docker build -f docker/Dockerfile -t $(DOCKER_IMAGE):$(DOCKER_TAG) .

docker-run: docker-build
	@echo "Running Docker container..."
	docker run --rm \
		--env-file .env \
		-v $(PWD)/backups:/backups \
		$(DOCKER_IMAGE):$(DOCKER_TAG)

docker-daemon: docker-build
	@echo "Running Docker container in detached mode..."
	docker run -d \
		--name $(BACKUP_BINARY) \
		--env-file .env \
		-v $(PWD)/backups:/backups \
		--restart unless-stopped \
		$(DOCKER_IMAGE):$(DOCKER_TAG)

docker-stop:
	@echo "Stopping Docker container..."
	docker stop $(BACKUP_BINARY) || true
	docker rm $(BACKUP_BINARY) || true

docker-logs:
	docker logs -f $(BACKUP_BINARY)

dc-up:
	@echo "Starting services with docker-compose..."
	docker-compose -f docker/docker-compose.yml up -d

dc-down:
	@echo "Stopping services with docker-compose..."
	docker-compose -f docker/docker-compose.yml down

dc-logs:
	docker-compose -f docker/docker-compose.yml logs -f

# ---------------------------------------------------------------------------
# Install
# ---------------------------------------------------------------------------

install: build
	@echo "Installing $(BACKUP_BINARY) to /usr/local/bin..."
	install -m 755 $(BUILD_DIR)/$(BACKUP_BINARY) /usr/local/bin/

install-restore: build-restore
	@echo "Installing $(RESTORE_BINARY) to /usr/local/bin..."
	install -m 755 $(BUILD_DIR)/$(RESTORE_BINARY) /usr/local/bin/

install-go: build
	@echo "Installing $(BACKUP_BINARY) to GOPATH/bin..."
	$(GO) install

# ---------------------------------------------------------------------------
# Cross-compile / Release
# ---------------------------------------------------------------------------

build-all: clean
	@echo "Building for multiple platforms..."
	@mkdir -p $(BUILD_DIR)
	@# Backup
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BACKUP_BINARY)-linux-amd64 .
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BACKUP_BINARY)-linux-arm64 .
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BACKUP_BINARY)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BACKUP_BINARY)-darwin-arm64 .
	@# Restore (must build from cmd/restore/)
	cd $(RESTORE_DIR) && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO) build $(LDFLAGS) -o ../../$(BUILD_DIR)/$(RESTORE_BINARY)-linux-amd64 .
	cd $(RESTORE_DIR) && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $(GO) build $(LDFLAGS) -o ../../$(BUILD_DIR)/$(RESTORE_BINARY)-linux-arm64 .
	cd $(RESTORE_DIR) && GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 $(GO) build $(LDFLAGS) -o ../../$(BUILD_DIR)/$(RESTORE_BINARY)-darwin-amd64 .
	cd $(RESTORE_DIR) && GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 $(GO) build $(LDFLAGS) -o ../../$(BUILD_DIR)/$(RESTORE_BINARY)-darwin-arm64 .
	@echo "Builds created:"
	@ls -lh $(BUILD_DIR)/

release: build-all
	@echo "Creating release tarballs..."
	@mkdir -p releases
	@for binary in $(BACKUP_BINARY) $(RESTORE_BINARY); do \
		for platform in linux-amd64 linux-arm64 darwin-amd64 darwin-arm64; do \
			tar -czf releases/$$binary-$$platform.tar.gz -C $(BUILD_DIR) $$binary-$$platform; \
		done; \
	done
	@echo "Releases created:"
	@ls -lh releases/

# ---------------------------------------------------------------------------
# Help
# ---------------------------------------------------------------------------

help:
	@echo "Available targets:"
	@echo ""
	@echo "Build:"
	@echo "  all             - Build both backup and restore (default)"
	@echo "  build           - Build backup binary only"
	@echo "  build-backup    - Build backup binary"
	@echo "  build-restore   - Build restore binary"
	@echo "  build-prod      - Production builds (stripped, no debug)"
	@echo "  build-all       - Cross-compile for linux/darwin amd64/arm64"
	@echo "  release         - Create release tarballs"
	@echo ""
	@echo "Test:"
	@echo "  test            - Run all tests (backup + restore)"
	@echo "  test-backup     - Run backup tests"
	@echo "  test-restore    - Run restore tests"
	@echo "  test-coverage   - Run tests with coverage"
	@echo ""
	@echo "Quality:"
	@echo "  fmt             - Format code (both modules)"
	@echo "  vet             - Run go vet (both modules)"
	@echo "  lint            - Run golangci-lint"
	@echo "  check           - Run fmt + vet + lint + test"
	@echo ""
	@echo "Run:"
	@echo "  run             - Run backup locally"
	@echo "  run-restore     - Run restore TUI locally"
	@echo "  run-force       - Run backup with --force"
	@echo ""
	@echo "Docker:"
	@echo "  docker-build   - Build Docker image"
	@echo "  docker-run     - Run Docker container"
	@echo "  docker-daemon  - Run Docker container detached"
	@echo "  docker-stop    - Stop Docker container"
	@echo "  docker-logs    - View Docker logs"
	@echo "  dc-up          - Start docker-compose"
	@echo "  dc-down        - Stop docker-compose"
	@echo "  dc-logs        - View docker-compose logs"
	@echo ""
	@echo "Install:"
	@echo "  install         - Install backup to /usr/local/bin"
	@echo "  install-restore - Install restore to /usr/local/bin"
	@echo "  install-go      - Install backup to GOPATH/bin"
	@echo ""
	@echo "Other:"
	@echo "  deps            - Download and tidy dependencies"
	@echo "  clean           - Remove bin/ and coverage files"
	@echo "  clean-backups   - Remove backup data"
	@echo "  help            - Show this help message"

.DEFAULT_GOAL := all