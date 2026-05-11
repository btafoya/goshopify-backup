# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Go CLI tools for Shopify store data backup and restore. The backup tool dumps store data nightly to flat JSON files; the restore tool provides a TUI for selectively restoring data from those backups.

## Critical Rules

### NEVER

- Commit `.env`, credentials, API keys, or tokens
- Auto-deploy to production without explicit approval
- Hardcode credentials (use environment variables)
- Do project-wide search-and-replace renames without a plan
- Include AI attribution in commits ("Generated with Claude Code", "Co-Authored-By")

### ALWAYS

- Use context7 for reference
- Commit as author: **btafoya** — no AI attribution in messages
- Follow `.claude/skills/*`


## Build Process

This repo contains **two separate Go modules** — they must be built independently:

| Binary | Module | Source | Build Command |
|---|---|---|---|
| `goshopify-backup` | `github.com/btafoya/goshopify-backup` | `./` (project root) | `go build -o bin/goshopify-backup .` |
| `goshopify-restore` | `github.com/btafoya/goshopify-restore` | `./cmd/restore/` | `cd cmd/restore && go build -o ../../bin/goshopify-restore .` |

All binaries output to `bin/`. Use the Makefile for standard builds:

```bash
make              # Build both binaries (default: make all)
make build-backup    # Build backup only
make build-restore   # Build restore only
make build-prod      # Production builds (stripped, CGO_ENABLED=0)
make test            # Run all tests (both modules)
make check           # fmt + vet + lint + test
make clean           # Remove bin/ and coverage files
```

**Common pitfall**: Running `go build .` from the project root builds the *backup* binary, not the restore binary. The restore binary must be built from `cmd/restore/`. Use `make` to avoid this.

### Docker

Multi-stage Dockerfile (`docker/Dockerfile`) builds the backup binary only:
- Build stage: `golang:1.23-alpine`, `CGO_ENABLED=0 GOOS=linux GOARCH=amd64`
- Runtime stage: `alpine:latest`, non-root user (65534:65534)
- Resource limits: CPU 1 core, Memory 512Mi
- `docker-compose.yml` passes `SHOPIFY_ACCESS_TOKEN`, `SHOPIFY_CLIENT_ID`, `SHOPIFY_SECRET` as env vars


## Shopify Authentication

The restore tool supports two authentication methods:

### Method 1: Direct Access Token (legacy)
Set `SHOPIFY_ACCESS_TOKEN` — a long-lived token from the Shopify Admin. Sent directly in the `X-Shopify-Access-Token` header on every API call.

### Method 2: Client Credentials (recommended)
Set `SHOPIFY_CLIENT_ID` + `SHOPIFY_SECRET` — uses the [Shopify OAuth client_credentials grant](https://shopify.dev/docs/apps/build/authentication-authorization/access-tokens/client-credentials-grant) to obtain a short-lived access token.

**Flow** (`cmd/restore/client.go: ShopifyClient.Authenticate()`):

1. If `AccessToken` is already set (direct token), skip OAuth — no-op
2. Check in-memory token cache; if cached token has >60s until expiry, reuse it
3. POST to `{STORE_URL}/admin/oauth/access_token` with form body:
   - `grant_type=client_credentials`
   - `client_id={SHOPIFY_CLIENT_ID}`
   - `client_secret={SHOPIFY_SECRET}`
4. Parse response: `access_token`, `scope`, `expires_in` (~86399s / 24 hours)
5. Cache token + expiry; set `AccessToken` on client for all subsequent API calls
6. All API calls use `X-Shopify-Access-Token: {token}` header regardless of auth method

**Key constants** (`cmd/restore/constants.go`):
- `TokenRefreshBuffer = 60s` — cached token is refreshed this far before expiry
- Token lifetime from Shopify: ~86399 seconds (24 hours)

**Validation** (`cmd/restore/config.go: ValidateConfig()`):
- When `SHOPIFY_STORE` is set, requires either `SHOPIFY_ACCESS_TOKEN` OR both `SHOPIFY_CLIENT_ID` + `SHOPIFY_SECRET`
- Providing `SHOPIFY_CLIENT_ID` without `SHOPIFY_SECRET` (or vice versa) is an error
- If both are provided, `SHOPIFY_ACCESS_TOKEN` takes precedence

**Credential persistence** (`cmd/restore/credentials.go`):
- Saved to `~/.config/goshopify/credentials.json`
- `Credential` struct stores `ClientID`/`ClientSecret` (preferred) or `AccessToken`
- `GetOrPromptConfig` loads saved credentials when env vars are missing; prefers client credentials over access tokens

**Rollback scripts** (`cmd/restore/executor.go`):
- Generated shell scripts support both auth methods
- Token obtained via `curl` + `jq` in the script if no `SHOPIFY_ACCESS_TOKEN` is set
- Requires `jq` installed on the target machine for client credentials token extraction

**Client credentials are only available for apps developed by your own organization and installed in stores you own** — see Shopify docs for `shop_not_permitted` error handling.


## Environment Variables

| Variable | Required | Default | Validation |
|---|---|---|---|
| `SHOPIFY_STORE` | Yes | - | Must be valid HTTPS URL (https://*.myshopify.com) |
| `SHOPIFY_ACCESS_TOKEN` | Conditional* | - | Non-empty string; required unless using client credentials |
| `SHOPIFY_CLIENT_ID` | Conditional* | - | Required with `SHOPIFY_SECRET` when not using access token |
| `SHOPIFY_SECRET` | Conditional* | - | Required with `SHOPIFY_CLIENT_ID` when not using access token |
| `SHOPIFY_API_VERSION` | No | `2025-01` | Must be valid Shopify API version |
| `BACKUP_DIR` | No | `/backups/shopify` | Must be writable directory |
| `RETENTION_DAYS` | No | `30` | Must be 1-3650 |

*Authentication requires either `SHOPIFY_ACCESS_TOKEN` OR both `SHOPIFY_CLIENT_ID` and `SHOPIFY_SECRET`. Client credentials use OAuth client_credentials grant to obtain a short-lived access token (24h expiry, auto-refreshed).

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
├── cleanup.go                # Retention cleanup
│
└── cmd/restore/              # Separate Go module (github.com/btafoya/goshopify-restore)
    ├── main.go               # Entry point, .env loading, TUI launch
    ├── config.go             # Env var reading + CLI flag parsing + validation
    ├── client.go             # Shopify API client, OAuth token exchange, rate limiter, retry
    ├── types.go              # Config, Credential, entity types
    ├── constants.go          # API version, rate limits, token refresh buffer
    ├── credentials.go        # Credential persistence (~/.config/goshopify/credentials.json)
    ├── tui_model.go          # Bubbletea TUI state machine
    ├── tui_view.go           # TUI rendering
    ├── mutations.go          # GraphQL mutation definitions
    ├── executor.go           # Restore execution, rollback script generation
    ├── images.go             # Image upload via stagedUploadsCreate
    ├── validator.go          # Pre-restore item validation
    ├── backup/               # Backup data loader (shared package)
    └── entity/               # Entity-specific restore logic
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
- Authentication: either SHOPIFY_ACCESS_TOKEN or both SHOPIFY_CLIENT_ID + SHOPIFY_SECRET required when store is specified
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

```bash
# Build both binaries
make

# Build individually
make build-backup
make build-restore

# Production build (stripped)
make build-prod

# Run backup
go run main.go
# or
make run

# Force re-run (override completed modules)
go run main.go --force

# Run restore (interactive TUI)
cd cmd/restore && go run main.go
# or
make run-restore

# Run restore with client credentials
cd cmd/restore && go run main.go --store https://store.myshopify.com --client-id ID --client-secret SECRET

# Run all tests
make test

# Run all quality checks
make check

# Clean build artifacts
make clean
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

<!-- BEGIN BEADS INTEGRATION v:1 profile:minimal hash:ca08a54f -->
## Beads Issue Tracker

This project uses **bd (beads)** for issue tracking. Run `bd prime` to see full workflow context and commands.

### Quick Reference

```bash
bd ready              # Find available work
bd show <id>          # View issue details
bd update <id> --claim  # Claim work
bd close <id>         # Complete work
```

### Rules

- Use `bd` for ALL task tracking — do NOT use TodoWrite, TaskCreate, or markdown TODO lists
- Run `bd prime` for detailed command reference and session close protocol
- Use `bd remember` for persistent knowledge — do NOT use MEMORY.md files

## Session Completion

**When ending a work session**, you MUST complete ALL steps below. Work is NOT complete until `git push` succeeds.

**MANDATORY WORKFLOW:**

1. **File issues for remaining work** - Create issues for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **PUSH TO REMOTE** - This is MANDATORY:
   ```bash
   git pull --rebase
   bd dolt push
   git push
   git status  # MUST show "up to date with origin"
   ```
5. **Clean up** - Clear stashes, prune remote branches
6. **Verify** - All changes committed AND pushed
7. **Hand off** - Provide context for next session

**CRITICAL RULES:**
- Work is NOT complete until `git push` succeeds
- NEVER stop before pushing - that leaves work stranded locally
- NEVER say "ready to push when you are" - YOU must push
- If push fails, resolve and retry until it succeeds
<!-- END BEADS INTEGRATION -->
