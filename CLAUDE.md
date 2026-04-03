# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Pure Go CLI tool that dumps Shopify store data nightly to a directory as flat JSON files. This is a greenfield project - implementation is planned but not yet started.

## Critical Rules

### NEVER

- Commit `.env`, credentials, API keys, or tokens
- Auto-deploy to production without explicit approval
- Hardcode credentials (use environment variables)
- Do project-wide search-and-replace renames without a plan
- Include AI attribution in commits ("Generated with Claude Code", "Co-Authored-By")

### ALWAYS

- Commit as author: **btafoya** — no AI attribution in messages
- Follow `.claude/skills/anti-slop/SKILL.md` for quality standards
- Follow `.claude/skills/golang-pro/SKILL.md`
- Follow `.claude/skills/shopify-admin-graphql/SKILL.md`


## Environment Variables

| Variable | Required | Default | Validation |
|---|---|---|---|
| `SHOPIFY_STORE` | Yes | - | Must be valid HTTPS URL (https://*.myshopify.com) |
| `SHOPIFY_ACCESS_TOKEN` | Yes | - | Non-empty string |
| `SHOPIFY_API_VERSION` | No | `2025-01` | Must be valid Shopify API version |
| `BACKUP_DIR` | No | `/backups/shopify` | Must be writable directory |
| `RETENTION_DAYS` | No | `30` | Must be 1-3650 |

**IMPORTANT**: All environment variables are validated at startup. Tool exits with code 2 if validation fails.

## Planned Tech Stack

- **GraphQL Client**: `github.com/hasura/go-graphql-client`
- **REST Client**: `github.com/go-resty/resty`
- **Rate Limiting**: Manual 40 req/sec leaky bucket
- **Concurrency**: `golang.org/x/sync/semaphore`
- **Logging**: `github.com/sirupsen/logrus` (structured JSON logging)
- **Testing**: Table-driven Go tests

## Architecture

```
shopify-backup/
├── main.go                    # Entry point, signal handling, phase orchestration
├── config.go                 # Env var reading + validation (startup validation required)
├── types.go                  # Shared types
├── constants.go              # Magic numbers
├── shopify/
│   ├── client.go             # Client factory, rate limiter
│   ├── graphql.go            # GraphQL client + bulk operations
│   ├── rest.go              # REST client + pagination + retry
│   └── types.go             # Shopify API types
├── logger/
│   └── logger.go            # Structured JSON logging (logrus)
├── backup/
│   ├── products.go           # Product bulk + image download
│   ├── customers.go         # Customer bulk + REST fallback
│   ├── orders.go            # Order bulk + REST fallback
│   ├── collections.go       # Collection bulk
│   └── content.go           # Pages, blogs, metafields
├── jsonl/
│   ├── parser.go            # Streaming JSONL parse
│   └── reconstruct.go       # Parent-child reconstruction
├── status/
│   ├── writer.go            # Buffered channel status writer
│   └── types.go             # Status types
├── lock/
│   └── lock.go              # Concurrent backup prevention (.lock file)
├── recovery/
│   └── recovery.go          # Partial backup recovery
└── cleanup.go                # Retention cleanup
```

## Key Design Decisions

### Rate Limiting
Leaky bucket implementation with `MIN_REQUEST_INTERVAL_MS = 25` (40 req/sec Shopify limit). Uses `sync.Mutex` for thread safety.

### Bulk Operations
- Use `bulkOperationRunQuery` GraphQL mutation for products, customers, orders, collections
- Set `groupObjects: false` to reduce operation time for large datasets
- Poll `currentBulkOperation` every 1000ms until COMPLETED/FAILED/CANCELED or 10-minute timeout
- Fallback to REST pagination if bulk operation access is denied

### JSONL Processing
Shopify bulk operations return flat JSONL with `__parentId` references. The `jsonl/reconstruct.go` module builds nested structures:
- Children are routed to parents by GID prefix (e.g., `gid://shopify/LineItem/` → Order parent)
- Streaming parser uses `bufio.Scanner` to avoid loading entire file into memory

### Status Writing
Buffered channel writer with 5-second flush interval. Each module sends non-blocking status updates; single writer goroutine batches and flushes.

### Retry Strategy

| Error Type | Max Retries | Backoff | Fallback |
|------------|-------------|---------|----------|
| Network timeout | 3 | Exponential: 2s, 4s, 8s | None |
| 429 Rate Limit | 3 | Exponential: 2s, 4s, 8s | None |
| 5xx Server Error | 3 | Exponential: 2s, 4s, 8s | None |
| ACCESS_DENIED (bulk) | 1 | 5 seconds | REST pagination |
| Image download | 3 | 1 second | Log and continue |

### Metafields Strategy
- **Entity metafields** (product, variant, customer, order, collection): included in bulk query JSON files
- **Shop metafields**: fetched via REST `GET /admin/api/{version}/metafields.json`, written to `metafields.json`

### Exit Codes
- `0`: Successful backup (all modules completed)
- `1`: Backup failed (at least one module failed)
- `2`: Configuration error (validation failed at startup)
- `3`: Concurrent execution (another backup in progress)

### Startup Validation
All environment variables validated before any backup work begins:
- SHOPIFY_STORE must be valid HTTPS URL
- SHOPIFY_ACCESS_TOKEN must be non-empty
- SHOPIFY_API_VERSION must match format `^\d{4}-\d{2}$`
- BACKUP_DIR must be writable (create if doesn't exist)
- RETENTION_DAYS clamped to 1-3650 range (warn if clamped)

### Lock Management
Prevents concurrent backups for same date:
- Lock file at `{BACKUP_DIR}/{YYYY-MM-DD}/.lock`
- Contains PID and start time
- Stale locks (> 24 hours) are cleared
- Exit code 3 if recent lock exists (use `--force` to override)

### Partial Backup Recovery
- On restart, read `status.json` to determine what's done
- Skip completed modules (unless `--force` flag)
- Resume from failed/incomplete module
- Lock file checked first for concurrent execution

## Output Structure

```
{BACKUP_DIR}/YYYY-MM-DD/
├── status.json
├── products.json
├── customers.json
├── orders.json
├── pages.json
├── collections.json
├── blogs.json
├── metafields.json
└── images/{product_id}/{n}.{ext}
```

## Commands

Once implemented:

```bash
# Run backup
go run main.go

# Force re-run (override completed modules)
go run main.go --force

# Run tests
go test ./...

# Run specific test
go test ./jsonl -run TestReconstructBulkData

# Build binary
go build -o shopify-backup main.go
```

## Important Constants

```go
API_VERSION              = "2025-01"
MIN_REQUEST_INTERVAL_MS  = 25        // 40 req/sec
DEFAULT_POLL_TIMEOUT_MS  = 600000    // 10 minutes
IMAGE_CONCURRENCY_LIMIT   = 10
STATUS_FLUSH_INTERVAL_S   = 5
STALE_LOCK_DURATION_HOURS = 24
```

## Observability

### Structured Logging
JSON format with fields: `time`, `level`, `msg`, `module`, `action`, `store`, `count`, `error`, `duration`

### Status File Format
```json
{
  "startedAt": "2026-04-03T02:00:00Z",
  "completedAt": "2026-04-03T02:15:30Z",
  "duration": "15m30s",
  "modules": {
    "products": {
      "status": "completed",
      "startedAt": "2026-04-03T02:00:00Z",
      "completedAt": "2026-04-03T02:05:00Z",
      "count": 1523,
      "fileSize": 4582930
    }
  },
  "totalSize": 6924164
}
```

### Containerization
Multi-stage Dockerfile with:
- `golang:1.23-alpine` for build
- `alpine:latest` for runtime
- Non-root user (65534:65534)
- Resource limits: CPU 1 core, Memory 512Mi

## Design Documents

See `prd/go-port-spec.md` for detailed specifications and `prd/go-port-design.md` for architectural design.