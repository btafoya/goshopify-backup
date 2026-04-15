# Restore CLI Production-Readiness Plan

## Context

The `cmd/restore/` CLI is a Bubbletea TUI tool for restoring Shopify store data from backup files. It was built as a prototype alongside the backup tool. The backup tool is production-hardened; the restore tool is approximately 30-40% complete relative to its PRD requirements and has critical bugs that would cause data loss or silent failures in production.

**Current state**: TUI shell is functional (state machine, views, key bindings). Core restore logic has critical gaps.

---

## Critical Bugs (Must Fix First)

### C1. `IsErrorRetryable()` is dead code
- **File**: `client.go:237`
- **Bug**: `if ok := false; ok` — the condition is always false, so HTTP status code retry detection never executes
- **Impact**: No API request is ever identified as retryable by this function
- **Fix**: Replace with proper type assertion: `if httpErr, ok := err.(interface{ StatusCode() int }); ok {`

### C2. `Retry()` is never called
- **File**: `client.go:246-271`
- **Bug**: `Retry()` function exists but `Do()` and `DoGraphQL()` never invoke it
- **Impact**: 429 rate limits, 5xx server errors, and network timeouts are never retried
- **Fix**: Wrap API calls in `Retry()` with exponential backoff (3 retries, 2s base delay per `constants.go:RestoreRetryCount/RestoreRetryDelay`)

### C3. 429 rate limit detected but not handled
- **File**: `client.go:154-159`
- **Bug**: 429 is logged with `Retry-After` header but the response is returned as-is with status 429, then fails in caller
- **Fix**: Sleep for `Retry-After` duration (or default 2s), then retry via `Retry()` wrapper

### C4. `entity/common.go` has broken import
- **File**: `entity/common.go:4`
- **Bug**: `import "restore/backup"` — not a valid Go module path
- **Impact**: File won't compile
- **Fix**: Change to `"github.com/btafoya/goshopify-backup/cmd/restore/backup"` or delete the file (it duplicates types in `types.go` and is never used)

### C5. Hardcoded `ConflictSkip` in TUI
- **File**: `tui_model.go:858`
- **Bug**: `executor := NewRestoreExecutor(client, ConflictSkip)` — always skips conflicting items
- **Impact**: Existing store data silently kept, backup data silently dropped — no user control
- **Fix**: Wire conflict mode to TUI selection or `--force` flag

### C6. `-v` flag collision
- **File**: `main.go:28-30` vs `config.go:34`
- **Bug**: Both `--version` and `--verbose` claim `-v`
- **Impact**: Verbose mode unreachable via short flag
- **Fix**: Use `-V` for version or drop the short flag for one

---

## Data Integrity Gaps (Silent Data Loss)

### D1. Product variants never restored
- **File**: `mutations.go:104-175`
- **Gap**: `productCreate` only creates the product shell. `ProductVariant` type exists but `productVariantCreate` mutations are never sent
- **Fix**: After product creation, iterate `item.Variants` and call `productVariantCreate` for each

### D2. Metafields never restored for any entity
- **Files**: `mutations.go` (all restore functions)
- **Gap**: `Metafield` type exists on Product, Customer, Order, Collection but no `metafieldsSet` or REST `POST /metafields.json` is called
- **Fix**: After entity creation, call `metafieldsSet` mutation with the entity's new ID and all metafields from backup

### D3. Customer addresses never restored
- **File**: `mutations.go:219-225`
- **Gap**: `customerCreate` sends email, firstName, lastName, phone — no addresses
- **Fix**: After customer creation, call `customerAddressCreate` for each address in `item.Addresses`

### D4. Collection-to-product links never restored
- **File**: `mutations.go:373-378`
- **Gap**: `collectionCreate` sends title, handle, description, rules — but no `collectionAddProducts` for manual collections
- **Fix**: After collection creation, call `collectionAddProducts` with mapped product IDs

### D5. Metaobject definitions not restorable
- **File**: `mutations.go:413-496`
- **Gap**: Only `metaobjectCreate` (entries) exists. No `metaobjectDefinitionCreate` mutation
- **Fix**: Add `metaobjectDefinitionCreate` mutation and a UI option to restore definitions, entries, or both

### D6. Metaobjects not loadable from backup
- **File**: `backup/loader.go:106-132`
- **Gap**: `LoadEntity()` has no case for `EntityMetaobjects`. The `default` branch returns "unsupported entity type"
- **Fix**: Add case for `EntityMetaobjects` that loads from `metaobjects/metaobject-definitions.json` and `metaobjects/{type}.json`

### D7. Restore tag never applied
- **File**: `constants.go:33` defines `RestoreTag = "restored_from_backup"` but it's never used
- **Gap**: No way to identify which items were created by the restore process
- **Fix**: Append `RestoreTag` to `item.Tags` before creation. For entities without tags (orders, metaobjects), use `tagsAdd` or a note field

### D8. No entity relationship validation
- **Gap**: No pre-check that referenced entities exist (e.g., order line items referencing products, collection products by ID)
- **Fix**: Add validation pass before restore that checks references and warns the user

---

## Operational Gaps

### O1. No resume capability
- **Files**: `types.go:252-283` (RestoreState type), `executor.go:178-184` (empty stubs)
- **Gap**: `--resume` flag parsed but never used. No `.restore_state.json` written or read. `Pause()`/`Resume()` are empty.
- **Fix**: Write `RestoreState` after each item completes. On resume, skip completed items.

### O2. No rollback script generation
- **File**: `executor.go:188-218`
- **Gap**: `GetRollbackScript()` returns a skeleton with header comments only. `Commands` slice always empty. No file written to disk.
- **Fix**: During restore, record each created entity's ID and type. Generate delete/untag commands. Write to `{ROLLBACK_DIR}/rollback_{date}.sh`

### O3. No structured file logging
- **Gap**: Only `charmbracelet/log` to stderr. `LogFile` and `LogDir` constants exist but no file writer.
- **Fix**: Add a file logger that writes structured JSON log entries to `{LOG_DIR}/restore_{date}.log`

### O4. No credential management
- **Files**: `types.go:286-292` (Credential type), `constants.go:52-55` (paths)
- **Gap**: No code to load, save, validate, or encrypt credentials. Tool fails if env vars missing.
- **Fix**: Implement `~/.config/goshopify/credentials.json` read/write. Add TUI credential prompt when env vars missing.

### O5. No pre-restore data validation
- **File**: `backup/validator.go` validates backup structure only
- **Gap**: No validation of data content (required fields, valid types) before sending to Shopify
- **Fix**: Add field-level validation on Item structs before restore begins

### O6. `--force` flag parsed but never used
- **File**: `config.go:29` sets `cfg.Force` but no code checks it
- **Fix**: Use `cfg.Force` to override conflict mode to `ConflictOverwrite` or skip confirmation dialog

### O7. `ImagePolicy` parsed but never checked
- **Gap**: `cfg.RestoreImages` set from `--images-restore`/`--images-skip` but `ImageUploader` only checks `DryRun`
- **Fix**: Check `ImagePolicy` in `UploadProductImages()` — skip when policy is `ImageSkip`

### O8. Image upload uses `http.DefaultClient`
- **File**: `images.go:357,378`
- **Gap**: Staged upload PUT and HEAD requests bypass rate limiter, timeout, and auth
- **Fix**: Use a configured HTTP client with the same timeout and rate limiter

### O9. `StoreURL` used as backup directory path
- **File**: `images.go:94`
- **Bug**: `backupDir := u.client.StoreURL` — StoreURL is `https://store.myshopify.com`, not a file path
- **Fix**: Pass backup directory path to ImageUploader or resolve via config

### O10. Duplicate type definitions across packages
- **Files**: `types.go` and `backup/loader.go` both define EntityType, Item, Product, Customer, Order, Collection, Metafield, etc.
- **Impact**: Any field addition must be made in two places and manual conversion updated in `tui_model.go`
- **Fix**: Consolidate types into `backup/` package. Import from `main`. Remove duplicates from `types.go`.

---

## Test Coverage: ~2%

Only `filter_test.go` (201 lines) exists. Missing test coverage for all critical paths:

| File | Lines | Test Files | Test Lines |
|------|-------|-----------|-----------|
| client.go | 271 | 0 | 0 |
| mutations.go | 884 | 0 | 0 |
| executor.go | 218 | 0 | 0 |
| images.go | 384 | 0 | 0 |
| config.go | 146 | 0 | 0 |
| backup/loader.go | 569 | 0 | 0 |
| backup/validator.go | 140 | 0 | 0 |
| tui_model.go | 867 | 0 | 0 |
| conflict.go | 91 | 0 | 0 |

**Priority tests to add**:
1. `client_test.go` — rate limiter, retry logic, 429 handling, GraphQL error detection
2. `mutations_test.go` — each entity restore path with httptest server, conflict detection
3. `executor_test.go` — worker pool, progress tracking, cancellation
4. `backup/loader_test.go` — parsing all entity types from JSON, edge cases

---

## Implementation Phases

### Phase 1: Critical Bug Fixes (Production Blockers)
- Fix `IsErrorRetryable()` (C1)
- Wire `Retry()` into `Do()` and `DoGraphQL()` (C2, C3)
- Fix or remove `entity/common.go` (C4)
- Fix conflict mode wiring (C5)
- Fix `-v` flag collision (C6)

### Phase 2: Data Integrity
- Add product variant restore (D1)
- Add metafield restore for all entities (D2)
- Add customer address restore (D3)
- Add collection product linking (D4)
- Add metaobject definition restore (D5)
- Add metaobject loading from backup (D6)
- Apply restore tag (D7)
- Add entity relationship validation (D8)

### Phase 3: Operational Safety
- Implement resume capability (O1)
- Implement rollback script generation (O2)
- Add file logging (O3)
- Implement credential management (O4)
- Add pre-restore data validation (O5)
- Wire `--force` and `ImagePolicy` (O6, O7, O9)
- Fix image upload client (O8)

### Phase 4: Type Consolidation & Test Coverage
- Consolidate duplicate types (O10)
- Add `client_test.go`
- Add `mutations_test.go`
- Add `executor_test.go`
- Add `backup/loader_test.go`

---

## Files Modified

| Phase | Files |
|-------|-------|
| 1 | `client.go`, `entity/common.go`, `tui_model.go`, `config.go`, `main.go` |
| 2 | `mutations.go`, `backup/loader.go`, `types.go` |
| 3 | `executor.go`, `images.go`, `client.go`, `config.go`, new `logger.go` |
| 4 | `types.go`, `backup/loader.go`, `backup/types.go`, new test files |

## Verification Per Phase

1. `go build ./cmd/restore/...` compiles. `go test ./cmd/restore/...` passes. Manual test: restore one product with conflict, verify retry on 429.
2. Restore a full product (variants, images, metafields). Restore a customer with addresses. Restore a collection with products. Verify metaobjects load and restore.
3. Kill restore mid-process, resume successfully. Check rollback script has delete commands. Check log file exists with JSON entries.
4. All test files pass. No duplicate type definitions.