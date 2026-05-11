# Project Index: goshopify-backup

Generated: 2026-04-03

## Project Overview

Pure Go CLI tool that dumps Shopify store data nightly to a directory as flat JSON files. Uses GraphQL bulk operations with REST API fallback. Implements lock-based concurrency control, partial backup recovery, and retention cleanup.

## Entry Points

- **CLI**: `main.go` - Entry point with signal handling, config validation, and module orchestration
- **Flags**: `--force` (override completed), `--health-check` (diagnostic mode)

## Project Structure

```
goshopify-backup/
├── main.go                    # CLI entry point
├── config.go                  # Env var validation (SHOPIFY_STORE, ACCESS_TOKEN, etc.)
├── constants.go               # Magic numbers (API version, rate limits, timeouts)
├── types.go                   # Shared types (Config, exit codes)
├── cleanup.go                 # Retention cleanup by date
├── backup/
│   ├── backup.go             # Bulk modules: Products, Customers, Orders, Collections
│   ├── content.go            # REST modules: Pages, Blogs, Metafields
│   ├── metaobjects.go        # GraphQL pagination for metaobjects
│   ├── redirects.go          # URL redirects (bulk + REST fallback)
│   └── util.go               # File I/O utilities, image download
├── shopify/
│   ├── client.go             # RateLimiter (leaky bucket), Config
│   ├── graphql.go            # GraphQL client, bulk operations, JSONL download
│   ├── rest.go               # REST client with pagination and retry
│   └── types.go              # Shopify API types, AccessDeniedError
├── jsonl/
│   ├── parser.go             # Streaming JSONL parse (bufio.Scanner)
│   └── reconstruct.go       # Parent-child reconstruction by GID prefix
├── logger/
│   └── logger.go            # Structured JSON logging (logrus)
├── lock/
│   └── lock.go              # Concurrent backup prevention (.lock file)
├── recovery/
│   └── recovery.go          # Partial backup resume from status.json
└── status/
    ├── writer.go            # Buffered channel status writer
    └── types.go             # Status types
```

## Core Modules

### Module: shopify/client
- **Exports**: `RateLimiter`, `Config`
- **Purpose**: Leaky bucket rate limiter (40 req/sec), shared client config

### Module: shopify/graphql
- **Exports**: `GraphQLClient`, `SubmitBulkOperation()`, `PollBulkOperation()`, `DownloadJSONL()`
- **Purpose**: GraphQL bulk operations with 10-min timeout, 1s polling

### Module: shopify/rest
- **Exports**: `RESTClient`, `Paginate()`, `Get()`
- **Purpose**: REST API with cursor pagination and retry logic

### Module: backup
- **Exports**: `ProductsModule`, `CustomersModule`, `OrdersModule`, `CollectionsModule`, `ContentModule`, `MetaobjectsModule`, `RedirectsModule`
- **Purpose**: Per-entity backup orchestration with bulk/REST fallback

### Module: jsonl
- **Exports**: `Parser`, `ReconstructBulkData()`, `NormalizeJSONL()`
- **Purpose**: Streaming JSONL parsing and nested structure reconstruction

### Module: lock
- **Exports**: `Manager`, `Acquire()`, `Release()`
- **Purpose**: Date-specific lock files, stale lock cleanup (24h)

### Module: recovery
- **Exports**: `Manager`, `ShouldResume()`, `GetModulesToRun()`
- **Purpose**: Resume incomplete backups from status.json

### Module: status
- **Exports**: `Writer`, `Update()`, `LoadStatus()`
- **Purpose**: Real-time status tracking with 5s buffered flush

## Configuration

| Variable | Required | Default | Validation |
|----------|----------|---------|------------|
| `SHOPIFY_STORE` | Yes | - | HTTPS URL (https://*.myshopify.com) |
| `SHOPIFY_ACCESS_TOKEN` | Yes | - | Non-empty string |
| `SHOPIFY_API_VERSION` | No | `2025-01` | YYYY-MM format |
| `BACKUP_DIR` | No | `/backups/shopify` | Writable directory |
| `RETENTION_DAYS` | No | `30` | 1-3650 range (clamped) |

## Backup Modules (Execution Order)

1. **products** - GraphQL bulk (images: optional, REST fallback)
2. **customers** - GraphQL bulk (REST fallback)
3. **orders** - GraphQL bulk (REST fallback)
4. **collections** - GraphQL bulk (REST fallback)
5. **content** - REST API (pages, blogs, metafields)
6. **metaobjects** - GraphQL pagination
7. **redirects** - GraphQL bulk (REST fallback)

## Output Structure

```
{BACKUP_DIR}/YYYY-MM-DD/
├── status.json               # Backup status and metadata
├── products.json
├── customers.json
├── orders.json
├── collections.json
├── pages.json
├── blogs.json
├── metafields.json
├── url-redirects.json
├── metaobjects/
│   ├── metaobject-definitions.json
│   └── {type}.json
└── images/{product_id}/
```

## Key Dependencies

- `github.com/go-resty/resty/v2` v2.17.2 - REST client
- `github.com/sirupsen/logrus` v1.9.3 - Structured logging
- `github.com/joho/godotenv` v1.5.1 - .env loading
- `golang.org/x/sync` v0.20.0 - Semaphore for concurrency

## Constants

```go
API_VERSION              = "2025-01"
MIN_REQUEST_INTERVAL_MS  = 25        // 40 req/sec
DEFAULT_POLL_TIMEOUT_MS  = 600000    // 10 minutes
IMAGE_CONCURRENCY_LIMIT   = 10
STATUS_FLUSH_INTERVAL_S   = 5
STALE_LOCK_DURATION_HOURS = 24
MaxRetentionDays         = 3650
```

## Exit Codes

- `0`: Success
- `1`: Backup failed
- `2`: Configuration error (validation failed at startup)
- `3`: Concurrent execution (use `--force` to override)

## Test Files

- `config_test.go` - Config validation
- `jsonl/parser_test.go` - JSONL parsing
- `jsonl/reconstruct_test.go` - Bulk data reconstruction
- `backup/metaobjects_test.go` - Metaobject backup
- `backup/redirects_test.go` - Redirect backup
- `lock/lock_test.go` - Lock management
- `shopify/shopify_test.go` - Shopify client
- `status/status_test.go` - Status tracking

## Quick Start

```bash
# Install
go install github.com/btafoya/goshopify-backup@latest

# Configure
cat > .env << EOF
SHOPIFY_STORE=https://store.myshopify.com
SHOPIFY_ACCESS_TOKEN=shpat_xxxxxx
SHOPIFY_API_VERSION=2025-01
BACKUP_DIR=/backups/shopify
EOF

# Run backup
goshopify-backup

# Force re-run
goshopify-backup --force

# Run tests
make test
```

## Docker

- Multi-stage build: `golang:1.25-alpine` → `alpine:latest`
- Non-root user (65534:65534)
- Health check endpoint