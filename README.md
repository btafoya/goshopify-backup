# Shopify Backup Tool

A Go CLI tool that dumps Shopify store data nightly to a directory as flat JSON files.

## Features

- **Bulk API Operations**: Uses Shopify GraphQL bulk operations for efficient data export
- **REST Fallback**: Automatically falls back to REST API if bulk operations are denied
- **Multiple Data Types**: Backs up products, customers, orders, collections, pages, blogs, metafields, metaobjects, and URL redirects
- **Concurrent Protection**: Lock file prevents multiple backups from running simultaneously
- **Partial Recovery**: Resumes from incomplete backups after interruption
- **Retention Policy**: Automatically cleans up old backups
- **Structured Logging**: JSON-formatted logs for easy parsing
- **Status Tracking**: Real-time status updates with `status.json`

## Quick Start

### Prerequisites

- Go 1.23+
- Shopify store with Admin API access
- Valid Shopify access token

### Installation

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
BACKUP_DIR=/path/to/backups
RETENTION_DAYS=30
```

### Running

```bash
# Run backup
./goshopify-backup

# Force re-run (override completed modules)
./goshopify-backup --force

# Check health
./goshopify-backup --health-check
```

## Output Structure

```
{BACKUP_DIR}/YYYY-MM-DD/
├── status.json          # Backup status and metadata
├── products.json        # Products with variants and images
├── customers.json        # Customers with addresses
├── orders.json          # Orders with line items and transactions
├── collections.json     # Collections with products
├── pages.json           # Store pages
├── blogs.json           # Blogs and articles
├── metafields.json      # Shop-level metafields
├── url-redirects.json   # URL redirects
├── metaobjects/         # Metaobject definitions and entries
│   ├── metaobject-definitions.json
│   ├── {type}.json      # Per metaobject type (e.g., size_chart.json)
│   └── ...
└── images/               # Product images
    └── {product_id}/
        ├── 0.jpg
        └── ...
```

## Make Commands

```bash
make build              # Build the binary
make test               # Run tests
make clean              # Clean build artifacts
make fmt                # Format code
make vet                # Run go vet
make lint               # Run linter
make docker-build       # Build Docker image
make docker-run         # Run Docker container
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
docker-compose -f docker/docker-compose.yml up -d
```

### Production Deployment

```bash
# Install as a service
sudo bash deploy/install.sh

# Check service status
systemctl status goshopify-backup

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

## Monitoring

### Prometheus Metrics

Metrics are available at `http://localhost:9090/metrics` when metrics server is enabled.

Key metrics:
- `shopify_backup_last_run_seconds`: Time since last successful backup
- `shopify_backup_duration_seconds`: Duration of last backup
- `shopify_backup_records_total`: Total records backed up
- `shopify_backup_module_success`: Module success status (0 or 1)
- `shopify_backup_module_duration_seconds`: Duration per module

### Grafana Dashboard

Import `deploy/grafana-dashboard.json` into Grafana for visualization.

### Health Checks

```bash
# Run health check
./goshopify-backup --health-check

# Docker health check
docker run --rm goshopify-backup --health-check
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
| Network timeout | 3 | Exponential | None |
| 429 Rate Limit | 3 | Exponential | None |
| 5xx Server Error | 3 | Exponential | None |
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

# Run with coverage
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
```

## License

MIT License - See LICENSE file for details.

## Contributing

Contributions are welcome! Please read the design documents in `prd/` before submitting changes.

## Support

For issues and questions, please open a GitHub issue.