# Shopify Backup Tool

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://go.dev/dl/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Report Card](https://goreportcard.com/badge/github.com/btafoya/goshopify-backup)](https://goreportcard.com/report/github.com/btafoya/goshopify-backup)
[![pkg.go.dev](https://pkg.go.dev/badge/github.com/btafoya/goshopify-backup)](https://pkg.go.dev/github.com/btafoya/goshopify-backup)

A Go CLI tool that dumps Shopify store data nightly to a directory as flat JSON files.

## Features

- **Bulk API Operations**: Uses Shopify GraphQL bulk operations for efficient data export
- **REST Fallback**: Automatically falls back to REST API if bulk operations are denied
- **Multiple Data Types**: Backs up products, customers, orders, collections, pages, blogs, shop metafields, metaobjects, and URL redirects
- **Concurrent Protection**: Lock file prevents multiple backups from running simultaneously
- **Partial Recovery**: Resumes from incomplete backups after interruption
- **Retention Policy**: Automatically cleans up old backups
- **Structured Logging**: JSON-formatted logs for easy parsing
- **Status Tracking**: Real-time status updates with `status.json`
- **Image Download**: Optional product image downloads
- **Systemd Integration**: Scheduled backups via systemd timer

## Quick Start

### Prerequisites

- Go 1.25+
- Shopify store with Admin API access
- Valid Shopify access token

### Installation

#### Using `go install` (Recommended)

```bash
# Install the latest version
go install github.com/btafoya/goshopify-backup@latest

# Install a specific version
go install github.com/btafoya/goshopify-backup@v1.0.0

# The binary will be installed to $GOPATH/bin or $HOME/go/bin
# Add to your PATH if needed:
export PATH=$PATH:$(go env GOPATH)/bin
```

#### Download Pre-built Binaries

Download the appropriate binary for your platform from the [releases page](https://github.com/btafoya/goshopify-backup/releases):

| Platform | Binary |
|----------|--------|
| Linux x86_64 | [goshopify-backup-linux-amd64.tar.gz](https://github.com/btafoya/goshopify-backup/releases/latest/download/goshopify-backup-linux-amd64.tar.gz) |
| Linux ARM64 | [goshopify-backup-linux-arm64.tar.gz](https://github.com/btafoya/goshopify-backup/releases/latest/download/goshopify-backup-linux-arm64.tar.gz) |
| macOS x86_64 (Intel) | [goshopify-backup-darwin-amd64.tar.gz](https://github.com/btafoya/goshopify-backup/releases/latest/download/goshopify-backup-darwin-amd64.tar.gz) |
| macOS ARM64 (Apple Silicon) | [goshopify-backup-darwin-arm64.tar.gz](https://github.com/btafoya/goshopify-backup/releases/latest/download/goshopify-backup-darwin-arm64.tar.gz) |

```bash
# Download and extract
curl -LO https://github.com/btafoya/goshopify-backup/releases/latest/download/goshopify-backup-linux-amd64.tar.gz
tar -xzf goshopify-backup-linux-amd64.tar.gz
chmod +x goshopify-backup-linux-amd64
sudo mv goshopify-backup-linux-amd64 /usr/local/bin/goshopify-backup
```

#### Building from Source

```bash
# Clone the repository
git clone https://github.com/btafoya/goshopify-backup.git
cd goshopify-backup

# Install dependencies
go mod download

# Build the binary
make build
```

### Configuration

Create a `.env` file:

```bash
SHOPIFY_STORE=https://your-store.myshopify.com
SHOPIFY_ACCESS_TOKEN=shpat_xxxxxxxx
SHOPIFY_API_VERSION=2025-01
BACKUP_DIR=/backups/shopify
RETENTION_DAYS=30
```

### Running

```bash
# Run backup (if installed via go install)
goshopify-backup

# Run backup (if built from source)
./bin/goshopify-backup

# Force re-run (override completed modules)
goshopify-backup --force

# Check health
goshopify-backup --health-check

# Run via Make
make run
make run-force
```

## Output Structure

```
{BACKUP_DIR}/YYYY-MM-DD/
├── status.json               # Backup status and metadata
├── products.json             # Products with variants, images, and metafields
├── customers.json            # Customers with addresses and metafields
├── orders.json               # Orders with line items, fulfillments, refunds, and metafields
├── collections.json          # Collections with products, rules, and metafields
├── pages.json                # Store pages
├── blogs.json                # Blogs and articles
├── metafields.json           # Shop-level metafields
├── url-redirects.json        # URL redirects
├── metaobjects/              # Metaobject definitions and entries
│   ├── metaobject-definitions.json
│   ├── {type}.json           # Per metaobject type (e.g., size_chart.json)
│   └── ...
└── images/                   # Product images
    └── {product_id}/
        ├── 0.jpg
        └── ...
```

## Backup Modules

The tool runs backup modules in the following order:

| Module | Method | Output Files |
|--------|--------|--------------|
| `products` | GraphQL bulk (REST fallback) | `products.json`, `images/` |
| `customers` | GraphQL bulk (REST fallback) | `customers.json` |
| `orders` | GraphQL bulk (REST fallback) | `orders.json` |
| `collections` | GraphQL bulk (REST fallback) | `collections.json` |
| `content` | REST API | `pages.json`, `blogs.json`, `metafields.json` |
| `metaobjects` | GraphQL pagination | `metaobjects/*.json` |
| `redirects` | GraphQL bulk (REST fallback) | `url-redirects.json` |

## Make Commands

```bash
make build              # Build the binary
make run                # Run the application locally
make run-force          # Run with force flag
make test               # Run tests with coverage
make test-coverage      # Run tests with coverage report
make fmt                # Format code
make lint               # Run linter (golangci-lint)
make vet                # Run go vet
make deps               # Install dependencies
make clean              # Clean build artifacts
make clean-backups      # Clean backups directory
make docker-build       # Build Docker image
make docker-run         # Run Docker container
make docker-daemon      # Run Docker container in detached mode
make docker-stop        # Stop Docker container
make docker-logs        # View Docker logs
make dc-up              # Start services with docker-compose
make dc-down            # Stop services with docker-compose
make dc-logs            # View docker-compose logs
make install            # Install to /usr/local/bin
make install-go         # Install to GOPATH/bin
make check              # Run all checks (fmt, vet, lint, test)
make build-all          # Build for multiple platforms
make release            # Create release tarballs
make help               # Show all commands
```

## Docker Deployment

### Build and Run

```bash
# Build image
make docker-build

# Run container
docker run --rm \
  --env-file .env \
  -v $(pwd)/backups:/backups \
  goshopify-backup

# Or use docker-compose
make dc-up
```

### Docker Image Details

- Multi-stage build with `golang:1.25-alpine` for building
- Runtime based on `alpine:latest`
- Non-root user (65534:65534)
- Resource limits: CPU 1 core, Memory 512Mi
- Health check enabled

## Production Deployment

### Systemd Installation

```bash
# Install as a service
sudo bash deploy/install.sh

# Check service status
systemctl status goshopify-backup.service

# View logs
journalctl -u goshopify-backup -f

# Run backup manually
sudo systemctl start goshopify-backup.service
```

### Systemd Timer

The installer sets up a systemd timer that runs backups daily at 2am UTC.

```bash
# View timer status
systemctl status goshopify-backup.timer

# Enable/disable automatic backups
systemctl enable goshopify-backup.timer
systemctl disable goshopify-backup.timer
```

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `SHOPIFY_STORE` | Yes | - | Store URL (https://*.myshopify.com) |
| `SHOPIFY_ACCESS_TOKEN` | Yes | - | Admin API access token |
| `SHOPIFY_API_VERSION` | No | `2025-01` | Shopify API version (YYYY-MM format) |
| `BACKUP_DIR` | No | `/backups/shopify` | Directory for backup files |
| `RETENTION_DAYS` | No | `30` | Days to retain backups (1-3650) |

## Exit Codes

| Code | Description |
|------|-------------|
| 0 | Success |
| 1 | Backup failed |
| 2 | Configuration error |
| 3 | Concurrent backup in progress (use `--force` to override) |

## Error Handling

The tool implements the following error handling strategy:

| Error Type | Max Retries | Backoff | Fallback |
|------------|-------------|---------|----------|
| Network timeout | 3 | Exponential: 2s, 4s, 8s | None |
| 429 Rate Limit | 3 | Exponential: 2s, 4s, 8s | None |
| 5xx Server Error | 3 | Exponential: 2s, 4s, 8s | None |
| Bulk ACCESS_DENIED | 1 | 5 seconds | REST pagination |
| Image download | 3 | 1 second | Log and continue |

## Troubleshooting

### Backup fails with "ACCESS_DENIED"

Some Shopify plans restrict bulk operations. The tool automatically falls back to REST API.

### "Lock file exists" error

Another backup is in progress. Use `--force` to override, or wait for the current backup to complete.

### Empty backup files

If your store has no data for a particular entity (e.g., no customers), the tool will create an empty array `[]` for that file.

### Permission denied writing to backup directory

Ensure the backup directory is writable by the user running the tool.

## Development

### Running Tests

```bash
# Run all tests
make test

# Run with coverage report
make test-coverage
```

### Code Style

```bash
# Format code
make fmt

# Run linter
make lint

# Run vet
make vet

# Run all checks
make check
```

## Architecture

The project is organized into the following packages:

- `backup/` - Backup modules for each data type
- `config/` - Environment variable configuration and validation
- `constants/` - Constants and configuration values
- `jsonl/` - JSONL parsing and reconstruction for bulk operation results
- `lock/` - Concurrent execution prevention via lock files
- `logger/` - Structured JSON logging
- `recovery/` - Partial backup recovery and resume
- `shopify/` - Shopify API clients (GraphQL and REST) with rate limiting
- `status/` - Status file writing and tracking
- `types/` - Shared type definitions

## License

MIT License - See LICENSE file for details.

## Contributing

Contributions are welcome! Please read the design documents in `prd/` before submitting changes.

## Support

For issues and questions, please open a GitHub issue.