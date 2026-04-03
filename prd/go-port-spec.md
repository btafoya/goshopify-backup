# Shopify Backup Go Port - Specification

## Overview

Port the existing Node.js + TypeScript Shopify backup tool to Go. The tool dumps Shopify store data nightly to a directory for B2/Backblaze backup ingestion.

## Tech Stack

| Component | Library/Approach |
|---|---|
| GraphQL Client | `github.com/hasura/go-graphql-client` |
| REST Client | `github.com/go-resty/resty` |
| Rate Limiting | Manual 40 req/sec leaky bucket |
| Retry Logic | `github.com/sethgrid/pestretry` |
| Concurrency | `golang.org/x/sync/semaphore` |
| Status Writer | Buffered channel (batched, 5s flush) |
| JSONL Parsing | `bufio.Scanner` streaming |
| Testing | Fresh Go table-driven tests |

## Environment Variables

| Variable | Required | Default | Validation |
|---|---|---|---|
| `SHOPIFY_STORE` | Yes | - | Must be valid HTTPS URL (https://*.myshopify.com) |
| `SHOPIFY_ACCESS_TOKEN` | Yes | - | Non-empty string |
| `SHOPIFY_API_VERSION` | No | `2025-01` | Must be valid Shopify API version |
| `BACKUP_DIR` | No | `/backups/shopify` | Must be writable directory |
| `RETENTION_DAYS` | No | `30` | Must be 1-3650 |

All environment variables are validated at startup. Tool exits with error code 2 if validation fails.

## Architecture

```
shopify-backup/
├── main.go                    # Entry point
├── config.go                 # Env var reading
├── types.go                  # Shared types
├── constants.go              # Magic numbers
├── shopify/
│   ├── graphql.go            # GraphQL client + bulk operations
│   ├── rest.go              # REST client + pagination
│   └── types.go             # Shopify API types
├── backup/
│   ├── products.go           # Product bulk + image download
│   ├── customers.go         # Customer bulk + REST fallback
│   ├── orders.go            # Order bulk + REST fallback
│   ├── collections.go       # Collection bulk
│   └── content.go           # Pages, blogs, metafields
├── jsonl/
│   ├── parser.go            # Streaming JSONL parse
│   └── reconstruct.go       # Parent-child reconstruction
└── cleanup.go                # Retention cleanup
```

## Status File

Buffered channel writer:
- Each module sends status updates via non-blocking channel
- Single writer goroutine batches updates
- Flushes every 5 seconds OR when a module completes
- Final flush on shutdown

## Output Structure

```
/backups/shopify/YYYY-MM-DD/
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

## Success/Failure Criteria

### Successful Backup
Exit code 0 and:
- All modules completed with `"status": "completed"`
- status.json is valid JSON
- All output files are valid JSON
- Duration logged in status.json

### Failed Backup
Exit code 1 and:
- At least one module failed
- status.json contains error details
- Partial data may exist (files are not deleted)

### Configuration Error
Exit code 2 and:
- Environment variable validation failed
- Clear error message printed
- No backup directories created

### Concurrent Execution Error
Exit code 3 and:
- Another backup in progress for same date
- Lock file exists and is recent (< 24 hours)
- Message: "Backup already in progress. Use --force to override."

### Operator Verification
To verify backup success:
1. Check exit code is 0
2. Validate status.json is valid JSON
3. Verify all modules show `"status": "completed"`
4. Optionally: record count > 0 (if store has data)

## Testing

Table-driven Go tests:
- JSONL parsing and reconstruction
- GraphQL query building
- REST pagination logic
- Config validation
- Cleanup retention logic

Integration tests:
- Against test Shopify store (small dataset)
- Rate limiter behavior under concurrent load
- Status writer flush timing

Edge case tests:
- Empty store (0 records)
- Large store (10,000+ records)
- Malformed JSONL from Shopify
- Unicode in titles/descriptions
- Concurrent backup runs (lock mechanism)

## Observability

### Structured Logging
JSON format with log levels (info, warn, error):
```json
{
  "time": "2026-04-03T02:00:00Z",
  "level": "info",
  "module": "products",
  "action": "bulk_submit",
  "store": "example.myshopify.com",
  "message": "Submitted bulk operation"
}
```

### Metrics (Optional - Prometheus)
- `shopify_backup_duration_seconds` - Histogram by module
- `shopify_backup_records_total` - Counter by module
- `shopify_backup_errors_total` - Counter by module

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
    },
    "customers": {
      "status": "completed",
      "fallback": "REST",
      "startedAt": "2026-04-03T02:00:00Z",
      "completedAt": "2026-04-03T02:08:00Z",
      "count": 8921,
      "fileSize": 2341234
    }
  },
  "totalSize": 6924164
}
```

## Containerization

### Dockerfile
```dockerfile
# Multi-stage build
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o shopify-backup

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/shopify-backup .
USER 65534:65534  # non-root
CMD ["./shopify-backup"]
```

### Resource Limits
Recommended:
- CPU: 1 core (burstable)
- Memory: 512Mi
- Disk: 10Gi (adjust based on store size)

## Constants

```go
API_VERSION              = "2025-01"
MIN_REQUEST_INTERVAL_MS  = 25        // 40 req/sec = 25ms between calls
RATE_LIMIT_BASE_DELAY_MS  = 2000
MAX_DELAY_MS              = 30000
DEFAULT_POLL_INTERVAL_MS  = 1000
DEFAULT_POLL_TIMEOUT_MS   = 600000    // 10 minutes
IMAGE_CONCURRENCY_LIMIT   = 10
MAX_IMAGE_RETRIES         = 3
STATUS_FLUSH_INTERVAL_S   = 5
```

## API Endpoints

### GraphQL
- Endpoint: `https://{SHOPIFY_STORE}/admin/api/2025-01/graphql.json`
- Header: `X-Shopify-Access-Token: {SHOPIFY_ACCESS_TOKEN}`

### REST
- Base: `https://{SHOPIFY_STORE}/admin/api/2025-01/`
- Auth: `X-Shopify-Access-Token` header

## Bulk Queries

Identical to Node.js version:
- `CUSTOMER_BULK_QUERY` - addresses, metafields
- `ORDER_BULK_QUERY` - line items, transactions, fulfillments, refunds, metafields
- `PRODUCT_BULK_QUERY` - variants, images, metafields, variant metafields
- `COLLECTION_BULK_QUERY` - products, smart collection rules, metafields

## Behavior Scenarios (Given/When/Then)

### Scenario: Successful Bulk Operation Backup
**Given** a Shopify store with 1,000 products
**When** backupProducts() is called
**Then**:
- GraphQL bulk operation submitted within 5 seconds
- Poll completes successfully with status "COMPLETED"
- products.json contains all 1,000 products with nested variants/images
- status.json shows `"products": { "status": "completed", "count": 1000 }`
- Module duration logged

### Scenario: Bulk Operation Access Denied → REST Fallback
**Given** a Shopify store with 5,000 customers
**And** GraphQL bulk operation returns "ACCESS_DENIED"
**When** backupCustomers() is called
**Then**:
- Retry bulk operation exactly 1 time after 5 seconds
- If ACCESS_DENIED again, switch to REST pagination
- Fetch customers via `GET /customers.json?page_info=...`
- Continue until Link header has no "next" relation
- Write to customers.json
- status.json includes `"fallback": "REST"`

### Scenario: Image Download with Failures
**Given** a product with 5 images
**And** IMAGE_CONCURRENCY_LIMIT = 10, MAX_IMAGE_RETRIES = 3
**When** images are downloaded
**And** 1 image fails after 3 retries
**Then**:
- All 5 images attempt download in parallel
- Failed image logged in status.json with URL and error
- Backup continues (don't fail entire module)
- 4 images stored at `images/{product_id}/0.jpg`, etc.

### Scenario: Backup Interrupted Mid-Operation
**Given** a backup is running
**And** products module is in progress
**When** process receives SIGTERM
**Then**:
- Stop accepting new work
- Complete current module if possible
- Flush status writer with current progress
- Exit with code 0 if no errors, 1 if partial

### Scenario: Retention Cleanup
**Given** BACKUP_DIR contains backups for 2026-03-01 through 2026-04-03 (34 days)
**And** RETENTION_DAYS = 30
**When** cleanup is executed
**Then**:
- Directories older than 30 days (2026-03-01, 2026-03-02, 2026-03-03) are deleted
- Today's directory (2026-04-03) is preserved
- Date comparison uses UTC
- Deletion errors logged but don't stop cleanup

### Scenario: Empty Store
**Given** a Shopify store with 0 products, 0 customers, 0 orders
**When** backup runs
**Then**:
- All modules complete with `"status": "completed"`
- Each module has `"count": 0`
- Empty arrays written to respective JSON files
- status.json shows successful completion

### Scenario: Startup Validation Failure
**Given** SHOPIFY_STORE = "not-a-url"
**When** backup is started
**Then**:
- Validation fails immediately
- Error message: "SHOPIFY_STORE must be valid HTTPS URL"
- Exit code 2
- No backup directories created

### Scenario: Partial Backup Recovery
**Given** a previous backup failed during products module
**And** status.json exists with `"products": { "status": "failed" }`
**When** backup is re-run for same date
**Then**:
- Read status.json on startup
- Resume from failed module (products)
- Skip completed modules (customers, orders, etc.)
- Overwrite failed module's output file
- `--force` flag required to re-run completed modules

### Scenario: Concurrent Backup Runs
**Given** a backup is running for date 2026-04-03
**When** another instance tries to backup same date
**Then**:
- Check for lock file `{BACKUP_DIR}/2026-04-03/.lock`
- If lock exists and < 24 hours old, exit with error code 3
- Lock file contains PID and start time
- Stale locks (> 24 hours) are cleared

## Error Handling

### Retry Strategy

| Error Type | Max Retries | Backoff | Fallback |
|------------|-------------|---------|----------|
| Network timeout | 3 | Exponential: 2s, 4s, 8s | None |
| 429 Rate Limit | 3 | Exponential: 2s, 4s, 8s | None |
| 5xx Server Error | 3 | Exponential: 2s, 4s, 8s | None |
| ACCESS_DENIED (bulk) | 1 | 5 seconds | REST pagination |
| Image download | 3 | 1 second | Log and continue |

### Error Categories

**Fatal Errors** (exit with code 1, no partial backup):
- Invalid configuration (caught at startup, code 2)
- Failed to create output directory
- All modules failed

**Module Errors** (continue with other modules):
- Single module fails with non-fatal error
- Status logged, backup continues

**Non-Fatal Errors** (continue within module):
- Individual image download fails
- Single REST page fails (retry)
- JSONL parse error on single line (log, skip)
