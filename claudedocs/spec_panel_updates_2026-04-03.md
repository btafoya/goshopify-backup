# Specification Updates - Phase 1 Complete

**Date**: 2026-04-03
**Based on**: Expert Panel Review (`claudedocs/spec_panel_review_2026-04-03.md`)

---

## Phase 1 Critical Items - COMPLETED

All critical items from the expert panel review have been incorporated into the specification.

### 1. ✅ Concrete Scenarios Added

**File**: `prd/go-port-spec.md`

Added 8 Given/When/Then scenarios:
- Successful Bulk Operation Backup
- Bulk Operation Access Denied → REST Fallback
- Image Download with Failures
- Backup Interrupted Mid-Operation
- Retention Cleanup
- Empty Store
- Startup Validation Failure
- Partial Backup Recovery
- Concurrent Backup Runs

### 2. ✅ Success/Failure Criteria Defined

**Files**: `prd/go-port-spec.md`, `CLAUDE.md`

**Exit Codes**:
- `0`: Successful backup (all modules completed)
- `1`: Backup failed (at least one module failed)
- `2`: Configuration error (validation failed at startup)
- `3`: Concurrent execution (another backup in progress)

**Success Criteria**:
- All modules completed with `"status": "completed"`
- status.json is valid JSON
- All output files are valid JSON
- Duration logged

### 3. ✅ Startup Validation Requirements

**Files**: `prd/go-port-spec.md`, `prd/go-port-design.md`, `CLAUDE.md`

**Environment Variable Validation**:
| Variable | Validation Rule |
|----------|----------------|
| `SHOPIFY_STORE` | Must be valid HTTPS URL (https://*.myshopify.com) |
| `SHOPIFY_ACCESS_TOKEN` | Non-empty string |
| `SHOPIFY_API_VERSION` | Must match `^\d{4}-\d{2}$` |
| `BACKUP_DIR` | Must be writable directory (create if doesn't exist) |
| `RETENTION_DAYS` | Must be 1-3650 (clamp if out of range) |

All validations occur at startup. Tool exits with code 2 on validation failure.

### 4. ✅ Partial Backup Recovery Strategy

**Files**: `prd/go-port-design.md`, `CLAUDE.md`

**Recovery Logic**:
1. On startup, check for `status.json` in output directory
2. If exists and any module shows `"status": "failed"`, resume from that module
3. Skip modules with `"status": "completed"` (unless `--force` flag set)
4. Overwrite output files for modules being re-run
5. Continue until all modules complete

New component: `recovery/recovery.go` with `RecoveryManager` struct.

### 5. ✅ Error Handling Scenarios with Retry Logic

**Files**: `prd/go-port-spec.md`, `CLAUDE.md`

**Retry Strategy**:
| Error Type | Max Retries | Backoff | Fallback |
|------------|-------------|---------|----------|
| Network timeout | 3 | Exponential: 2s, 4s, 8s | None |
| 429 Rate Limit | 3 | Exponential: 2s, 4s, 8s | None |
| 5xx Server Error | 3 | Exponential: 2s, 4s, 8s | None |
| ACCESS_DENIED (bulk) | 1 | 5 seconds | REST pagination |
| Image download | 3 | 1 second | Log and continue |

**Error Categories**:
- **Fatal Errors** (exit code 1): Invalid config (code 2), failed to create dir, all modules failed
- **Module Errors** (continue with other modules): Single module fails with non-fatal error
- **Non-Fatal Errors** (continue within module): Individual image download fails, single REST page fails

---

## Additional Improvements

### Lock Management

**New Component**: `lock/lock.go` with `LockManager` struct
- Prevents concurrent backups for same date
- Lock file at `{BACKUP_DIR}/{YYYY-MM-DD}/.lock`
- Contains PID and start time
- Stale locks (> 24 hours) are cleared
- Exit code 3 if recent lock exists (use `--force` to override)

### Structured Logging

**New Component**: `logger/logger.go`
- Uses `github.com/sirupsen/logrus`
- JSON format with structured fields
- Fields: `time`, `level`, `msg`, `module`, `action`, `store`, `count`, `error`, `duration`

### Observability

**Status File Format**: Full JSON structure defined with all fields
**Containerization**: Dockerfile requirements specified with multi-stage build

---

## Architecture Updates

**File**: `prd/go-port-design.md`, `CLAUDE.md`

Updated package structure:
```
shopify-backup/
├── logger/
│   └── logger.go            # Structured JSON logging
├── status/
│   ├── writer.go            # Buffered channel status writer
│   └── types.go             # Status types
├── lock/
│   └── lock.go              # Concurrent backup prevention
├── recovery/
│   └── recovery.go          # Partial backup recovery
```

---

## Design Document Updates

**File**: `prd/go-port-design.md`

Added new interface sections:
- 3.7 Structured Logging (`logger/logger.go`)
- 3.8 Status Writer (`status/writer.go`)
- 3.9 Lock Management
- 3.10 Partial Backup Recovery
- 3.11 Backup Module Interface (renumbered from 3.8)

Updated Config interface:
- Added `APIVersion` field
- Added `Force` flag for --force option
- Added `ValidateConfig()` function

---

## Files Modified

| File | Changes |
|------|---------|
| `prd/go-port-spec.md` | Added scenarios, error handling, observability, containerization |
| `prd/go-port-design.md` | Added logger, lock, recovery components; updated config |
| `CLAUDE.md` | Added startup validation, exit codes, retry strategy, observability |

---

## Files Created

| File | Purpose |
|------|---------|
| `claudedocs/spec_panel_review_2026-04-03.md` | Expert panel review report |
| `claudedocs/spec_panel_updates_2026-04-03.md` | This update summary |

---

## Phase 2 - Remaining Items (High Priority)

These should be addressed during implementation:

1. **Comprehensive Test Strategy** - Define specific edge case tests
2. **Graceful Shutdown Specification** - SIGTERM behavior details
3. **API Version Handling** - Deprecation detection strategy
4. **Refine RateLimiter Interface** - Separate state from execution (optional)

---

## Ready for Implementation

The specification now includes all Phase 1 critical items. The project is ready to proceed with implementation of:
1. Foundation files (`go.mod`, `constants.go`, `types.go`, `config.go`)
2. Core clients (`shopify/client.go`, `shopify/graphql.go`, `shopify/rest.go`)
3. JSONL handling (`jsonl/parser.go`, `jsonl/reconstruct.go`)
4. Status writer (`status/writer.go`)
5. Backup modules (products, customers, orders, collections, content)
6. Lock and recovery management
7. Orchestration (`main.go`, `cleanup.go`)