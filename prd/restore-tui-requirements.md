# Shopify Restore CLI - TUI Requirements

## Overview

A Go CLI tool with a Bubbletea TUI interface for restoring Shopify store data from JSON backup files. Users can interactively select individual items to restore with fine-grained control.

## Functional Requirements

### 1. Entity Types to Restore

| Entity | Details |
|--------|---------|
| **Products** | Products with variants, images, and metafields |
| **Customers** | Customers with addresses and metafields |
| **Orders** | Orders with line items, fulfillments, and refunds |
| **Collections** | Manual and smart collections with rules |
| **Metaobjects** | Definitions and/or entries (user selectable) |

### 2. Target Store Configuration

- Support restoring to **same store** (data came from)
- Support restoring to **different store** (staging/dev/new store)
- User selects target store at runtime via TUI

### 3. Selection Granularity

- **Individual item selection** (most granular)
- Users can select specific products, customers, orders, collections, or metaobjects
- Checkbox list interface (space to select/deselect)
- Bulk select all with Ctrl+A

### 4. Conflict Handling

- **Interactive prompt** when data already exists
- Options per conflict:
  - Skip - leave existing data as-is
  - Overwrite - replace with backup data
  - Rename - restore with modified handle/title

### 5. Metaobjects Handling

User can select which to restore:
- Metaobject type definitions (schema, namespace, key) only
- Metaobject entries only
- Both definitions and entries together

### 6. Product Images

- **Interactive** handling per product
- Options: Restore images, Skip images, or Ask per product
- Images uploaded from `images/{product_id}/` directory

### 7. Validation & Safety Features

| Feature | Description |
|---------|-------------|
| **Pre-restore validation** | Check for required fields, valid data types before restoring |
| **Dry-run mode** | Test without API calls, validate only |
| **Preview changes** | Show detailed diff of what will change before confirmation |

### 8. Progress Display

| Feature | Description |
|---------|-------------|
| **Progress bar** | Overall progress and percentage complete |
| **Item-by-item status** | Live status of each item being restored (success/error) |
| **Log file** | Detailed log generated after restore completes |

## TUI Design Requirements

### 1. Filtering Capabilities

| Filter Type | Description |
|-------------|-------------|
| **Date range** | Filter items by created/updated date range |
| **Tags/SKU/Handle** | Filter products by tags, variants by SKU, collections by handle |
| **Status filters** | Filter by status (active/draft/archived), fulfillment status, etc. |
| **Text search** | Search by title/name text across all entities |

### 2. Selection Interface

- **Checkbox list** with space to select/deselect items
- Cursor navigation with arrow keys (up/down) and vim keys (j/k)
- Visual indicators: cursor (>) and selection ([x])

### 3. Layout Structure

Selected layouts (combined):
- **Two-pane layout**: Available backups on left, item list on right
- **Sidebar navigation**: Navigate between entity types, detail panel on right

### 4. Backup Selection

- **Interactive picker** to select backup directory
- Display available backups with date, time, and size stats
- Default to latest backup if environment variable set

## Technical Requirements

### 1. API Strategy

- **GraphQL with REST fallback**
- Try GraphQL mutations first (`bulkOperationsRunQuery`)
- Fall back to REST endpoints on errors
- REST: `POST /admin/api/{version}/{resource}.json`

### 2. Error Handling

- **Interactive retry prompt** for failed items
- Options: Retry, Skip, or Abort entire restore
- Show detailed error message per failed item

### 3. Rate Limiting

- **Auto-rate-limit with leaky bucket**
- Respect Shopify's 40 req/sec limit
- Pause when approaching limit threshold
- Same implementation as backup tool (`MIN_REQUEST_INTERVAL_MS = 25`)

### 4. Credentials Configuration

Support multiple methods (in priority order):
1. **Environment variables**: `SHOPIFY_STORE`, `SHOPIFY_ACCESS_TOKEN`
2. **TUI prompts**: Interactive prompts for credentials
3. **Saved credentials**: Encrypted credentials in `~/.config/goshopify/credentials.json`

### 5. Entity Relationship Handling

- **Warn on missing relations**
- Example: Warn if restoring orders without products present in store
- Allow user to proceed with warning

### 6. Undo/Rollback Capabilities

- **Generate rollback script** before restore begins
- Script contains mutations/REST calls to revert changes
- **Tag restored items** with `restored_from_backup:{date}` tag for easy identification

### 7. Abort Behavior

When user aborts mid-restore, prompt with options:
- **Resume** - Save state and allow resuming from current position
- **Clean up** - Remove all partially restored data
- **Leave partial** - Keep partial data for manual inspection

## Non-Functional Requirements

### Performance

- Restore 1000+ products in under 10 minutes
- Progress updates at least every 500ms
- UI remains responsive during restore operations

### Reliability

- Handle network interruptions gracefully with retry logic
- Validate backup JSON files before use
- Detect and handle corrupted backup files

### Usability

- Keyboard-first interface
- Help screen accessible via `?` or `F1`
- Clear error messages with actionable guidance
- Support mouse where terminals allow it

### Security

- Never log credentials
- Securely store saved credentials (encrypted)
- Validate target store URL format (https://*.myshopify.com)

## Input Data Format

Restores from existing backup structure:
```
{BACKUP_DIR}/YYYY-MM-DD/
├── status.json
├── products.json
├── customers.json
├── orders.json
├── collections.json
├── pages.json
├── blogs.json
├── metafields.json
├── metaobjects/
│   ├── metaobject-definitions.json
│   └── {key}.json
└── images/{product_id}/{n}.{ext}
```

## Environment Variables

| Variable | Required | Default | Validation |
|----------|----------|---------|------------|
| `SHOPIFY_STORE` | No* | - | Must be valid HTTPS URL |
| `SHOPIFY_ACCESS_TOKEN` | No* | - | Non-empty string |
| `BACKUP_DIR` | No | `/backups/shopify` | Must exist and be readable |
| `SHOPIFY_API_VERSION` | No | `2025-01` | Must match `^\d{4}-\d{2}$` |

*Required if not provided via TUI prompts or saved credentials.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Successful restore |
| 1 | Restore failed |
| 2 | Configuration error |
| 3 | User aborted |

## Keyboard Controls

| Key | Action |
|-----|--------|
| `↑`/`k` | Move cursor up |
| `↓`/`j` | Move cursor down |
| `Space` | Toggle selection |
| `Ctrl+A` | Select all |
| `Enter` | Confirm/Proceed |
| `Esc` | Go back/Cancel |
| `Ctrl+C` | Quit |
| `?`/`F1` | Show help |
| `/` | Start search/filter |
| `Tab` | Switch between panels |

## User Stories

1. **US-1**: As a store owner, I want to restore specific products from a backup so I can recover deleted inventory.
2. **US-2**: As a developer, I want to restore to a staging store from a production backup so I can test with real data.
3. **US-3**: As a store manager, I want to see a preview of what will change before restoring so I don't accidentally overwrite data.
4. **US-4**: As a store owner, I want to filter by date range so I can restore only recent changes.
5. **US-5**: As a store owner, I want a rollback option so I can undo a restore if something goes wrong.

## Open Questions

| ID | Question |
|----|----------|
| OQ-1 | Should metaobjects support filtering by type/key? |
| OQ-3 | How should the tool handle product variants that don't exist in backup but exist in target store? |
| OQ-4 | Should there be a "restore all" quick option for each entity type? |

## Success Criteria

A restore is successful when:
1. All selected items are restored to target store
2. Status file created with summary of restored items
3. Log file generated with detailed operation log
4. Rollback script generated (if items were restored)
5. Exit code 0 returned