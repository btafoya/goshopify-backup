# Production Build Report

**Project**: goshopify-backup
**Date**: 2026-04-03T13:15:00Z
**Git Commit**: a075c36
**Go Version**: go1.26.0 linux/amd64
**Build Type**: Production (optimized, statically linked)

---

## Build Summary

| Component | Status |
|-----------|--------|
| Clean Build | ✓ |
| Code Format | ✓ (0 files changed) |
| Go Vet | ✓ Passed |
| Tests | ✓ All passing |
| Binary Build | ✓ Success |
| Health Check | ✓ Passed |

---

## Binary Information

```
File: bin/goshopify-backup
Size: 6.9M
Type: ELF 64-bit LSB executable, x86-64
Linking: Statically linked
Stripped: Yes
```

**Build Flags:**
- `CGO_ENABLED=0` - Disable CGO for static linking
- `GOOS=linux GOARCH=amd64` - Linux AMD64 target
- `-ldflags="-s -w"` - Strip debug symbols and DWARF
- `-X main.version` - Version info embedded
- `-X main.buildDate` - Build date embedded

---

## Modules Included

| Module | Method | Output |
|--------|--------|--------|
| products | GraphQL Bulk + REST fallback | products.json |
| customers | GraphQL Bulk + REST fallback | customers.json |
| orders | GraphQL Bulk + REST fallback | orders.json |
| collections | GraphQL Bulk + REST fallback | collections.json |
| content | REST API | pages.json, blogs.json, metafields.json |
| **metaobjects** | GraphQL Pagination (nodes + endCursor) | metaobjects/*.json |
| **redirects** | GraphQL Bulk + REST fallback | url-redirects.json |

---

## Test Results

```
ok  	github.com/btafoya/goshopify-backup
ok  	github.com/btafoya/goshopify-backup/backup
ok  	github.com/btafoya/goshopify-backup/jsonl
ok  	github.com/btafoya/goshopify-backup/lock
ok  	github.com/btafoya/goshopify-backup/shopify
ok  	github.com/btafoya/goshopify-backup/status
```

---

## Dependencies

| Package | Version | Purpose |
|---------|---------|---------|
| github.com/joho/godotenv | v1.5.1 | .env file loading |
| github.com/sirupsen/logrus | v1.9.3 | Structured JSON logging |
| github.com/go-resty/resty/v2 | v2.17.2 | REST API client |
| golang.org/x/sync | v0.20.0 | Semaphore for concurrency |
| golang.org/x/net | v0.43.0 | HTTP utilities |
| golang.org/x/sys | v0.35.0 | System calls |

---

## Deployment

### Installation

```bash
# Copy binary to installation directory
sudo cp bin/goshopify-backup /usr/local/bin/

# Make executable
sudo chmod +x /usr/local/bin/goshopify-backup
```

### Docker Image

```dockerfile
FROM alpine:latest
COPY bin/goshopify-backup /usr/local/bin/
RUN chmod +x /usr/local/bin/goshopify-backup
ENTRYPOINT ["/usr/local/bin/goshopify-backup"]
```

### Systemd Service

```ini
[Unit]
Description=Shopify Backup Service
After=network.target

[Service]
Type=oneshot
User=backup
EnvironmentFile=/etc/goshopify-backup/.env
ExecStart=/usr/local/bin/goshopify-backup
```

---

## Environment Variables Required

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| SHOPIFY_STORE | Yes | - | Store URL (https://*.myshopify.com) |
| SHOPIFY_ACCESS_TOKEN | Yes | - | Admin API access token |
| SHOPIFY_API_VERSION | No | 2025-01 | Shopify API version |
| BACKUP_DIR | No | /backups/shopify | Backup directory |
| RETENTION_DAYS | No | 30 | Days to retain backups |

---

## Exit Codes

| Code | Description |
|------|-------------|
| 0 | Success |
| 1 | Backup failed |
| 2 | Configuration error |
| 3 | Concurrent backup in progress |

---

## Output Structure

```
{BACKUP_DIR}/{YYYY-MM-DD}/
├── status.json
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

---

## Changelog

### Version 1.3 (2026-04-03)

**Bug Fixes:**
- Fixed metaobjects query minification - newlines now replaced with spaces
- Fixed empty cursor - first page omits `after` variable (API rejects empty string)
- Removed invalid `namespace` field (not in Shopify API)
- Removed invalid `createdAt` field (not in Shopify API)

### Version 1.2 (2026-04-03)

**Bug Fixes:**
- Fixed metaobjects query to use `nodes` instead of `edges { node { ... } }`
- Shopify's `metaobjects` GraphQL API returns nodes directly

### Version 1.1 (2026-04-03)

**Bug Fixes:**
- Fixed metaobjects pagination to use `pageInfo.endCursor` instead of node ID

### Version 1.0 (2026-04-03)

**New Features:**
- Metaobjects backup (per-type JSON files + definitions)
- URL redirects backup
- Lock file date-specific paths
- Updated module execution order

**Improvements:**
- Added Query() method to GraphQL client for non-bulk operations
- Enhanced test coverage for new modules

**Bug Fixes:**
- Fixed parser to skip empty lines in JSONL
- Fixed lock path to include date subdirectory
- Fixed redirects polling timeout duration

