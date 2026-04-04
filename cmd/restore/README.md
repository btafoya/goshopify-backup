# Shopify Restore CLI

A terminal user interface (TUI) for restoring Shopify store data from JSON backup files.

## Features

- **Interactive TUI** - Beautiful terminal interface built with Bubbletea
- **Selective Restore** - Choose exactly which items to restore (products, customers, orders, collections, metaobjects)
- **Advanced Filtering** - Filter by search text, status, tags, and date range
- **Conflict Resolution** - Handle conflicts with existing data (skip, overwrite, rename)
- **Preview Changes** - See exactly what will be restored before committing
- **Dry Run Mode** - Test the restore without making changes
- **Progress Tracking** - Real-time progress updates during restore
- **Rollback Support** - Generate rollback scripts to undo changes
- **Rate Limiting** - Built-in Shopify API rate limit compliance

## Installation

```bash
go build -o shopify-restore ./cmd/restore
```

## Configuration

The CLI can be configured via environment variables or command-line flags.

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `SHOPIFY_STORE` | Yes | - | Your Shopify store URL (e.g., https://store.myshopify.com) |
| `SHOPIFY_ACCESS_TOKEN` | Yes | - | Admin API access token |
| `SHOPIFY_API_VERSION` | No | 2025-01 | Shopify API version to use |
| `BACKUP_DIR` | No | ./backups | Directory containing backup JSON files |
| `ROLLBACK_DIR` | No | ./rollbacks | Directory to store rollback scripts |

### Command-Line Flags

| Flag | Description |
|------|-------------|
| `--store` | Shopify store URL |
| `--token` | API access token |
| `--api-version` | API version |
| `--backup-dir` | Backup directory |
| `--rollback-dir` | Rollback directory |
| `--backup-date` | Specific backup date to restore (YYYY-MM-DD) |
| `--dry-run` | Perform dry run without changes |
| `--force` | Overwrite existing data by default |
| `--help` | Show help message |
| `--version` | Show version |

## Usage

```bash
# Interactive restore
./shopify-restore

# Restore specific backup date
./shopify-restore --backup-date 2024-03-15

# Dry run to preview changes
./shopify-restore --dry-run

# With environment variables
export SHOPIFY_STORE="https://mystore.myshopify.com"
export SHOPIFY_ACCESS_TOKEN="shpat_xxxxxxxxxxxx"
./shopify-restore
```

## TUI Navigation

### Global Keys

| Key | Action |
|-----|--------|
| `?` | Show help overlay |
| `q`, `Ctrl+C` | Quit application |
| `Esc` | Go back / Cancel |

### Navigation

| Key | Action |
|-----|--------|
| `↑`, `k` | Move cursor up |
| `↓`, `j` | Move cursor down |
| `Enter` | Select / Confirm |
| `Esc` | Go back |

### Selection

| Key | Action |
|-----|--------|
| `Space` | Toggle item selection |
| `Ctrl+A` | Select all visible items |
| `/` | Start search filter |
| `F` | Open advanced filter dialog |

### Advanced Filter Dialog

| Key | Action |
|-----|--------|
| `↑`, `↓` | Navigate filter options |
| `Enter`, `Space` | Edit filter value |
| `Esc` | Close dialog |

### Restore Actions

| Key | Action |
|-----|--------|
| `Y` | Confirm restore |
| `N` | Go back without restoring |
| `Esc` | Abort restore (shows options) |

## Workflow

1. **Select Backup** - Choose which backup date to restore from
2. **Select Entity Type** - Choose products, customers, orders, collections, or metaobjects
3. **Select Items** - Browse and select individual items to restore
4. **Apply Filters** - Use `/` for quick search or `F` for advanced filtering
5. **Preview** - Review selected items and any conflicts
6. **Confirm** - Approve the restore operation
7. **Monitor** - Watch real-time progress during restore
8. **Complete** - View results and rollback script location

## Conflict Modes

When restoring items that already exist in the target store:

- **Skip** - Leave existing items unchanged (default)
- **Overwrite** - Delete existing items and restore from backup
- **Rename** - Rename handles/keys to avoid conflicts

## Backup File Format

The CLI expects JSON backup files in the following structure:

```
{BACKUP_DIR}/
├── YYYY-MM-DD/
│   ├── status.json
│   ├── products.json
│   ├── customers.json
│   ├── orders.json
│   ├── collections.json
│   └── metaobjects.json
```

Each JSON file should contain an array of items with the appropriate fields.

## Dry Run Mode

Dry run mode allows you to test the restore without making changes to your Shopify store:

```bash
./shopify-restore --dry-run
```

The CLI will:
- Validate all selected items
- Check for conflicts
- Simulate the restore process
- Generate a detailed report

No actual API calls will be made to modify data.

## Rollback

After a successful restore, a rollback script is generated:

```bash
./rollbacks/rollback_2024-03-15.sh
```

Review the script carefully before running to restore your store to its previous state.

## Rate Limiting

The CLI implements Shopify's rate limits:
- GraphQL: 40 requests/second (leaky bucket)
- REST: 40 requests/second (leaky bucket)
- Automatic retries on 429 responses

## Troubleshooting

### Authentication Error

Ensure your access token has the correct permissions:
- Read access for all entity types
- Write access for entities you want to restore

### Rate Limit Exceeded

The CLI handles rate limiting automatically. If you encounter issues:
1. Wait a few minutes and retry
2. Check your API usage in Shopify Admin

### Backup Not Found

Ensure the backup directory contains valid JSON files:
```bash
ls -la ./backups/2024-03-15/
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests: `go test ./...`
5. Submit a pull request

## License

See LICENSE file for details.