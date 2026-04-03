# Shopify Backup - Metrics

This document describes the available metrics for monitoring the Shopify backup tool.

## Prometheus Metrics

The following metrics are exported (when metrics server is enabled):

| Metric Name | Type | Description |
|-------------|------|-------------|
| `shopify_backup_last_run_seconds` | Gauge | Seconds since last successful backup |
| `shopify_backup_duration_seconds` | Gauge | Duration of last backup in seconds |
| `shopify_backup_records_total` | Gauge | Total records backed up |
| `shopify_backup_bytes_total` | Gauge | Total bytes backed up |
| `shopify_backup_module_success` | Gauge | Whether each module succeeded (1) or failed (0) |
| `shopify_backup_module_duration_seconds` | Gauge | Duration of each module backup |
| `shopify_backup_module_records` | Gauge | Records backed up per module |

## Module Labels

The module metrics include the following labels:

- `module`: One of `products`, `customers`, `orders`, `collections`, `content`
- `fallback`: `"REST"` if REST fallback was used, empty string otherwise

## Example Prometheus Queries

### Check backup health
```promql
# Last successful backup within 24 hours
shopify_backup_last_run_seconds < 86400

# All modules succeeded
sum(shopify_backup_module_success) by (module) == count(shopify_backup_module_success) by (module)
```

### Alerting Rules

```yaml
groups:
  - name: shopify_backup
    interval: 5m
    rules:
      - alert: ShopifyBackupTooOld
        expr: shopify_backup_last_run_seconds > 86400
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Shopify backup is older than 24 hours"

      - alert: ShopifyBackupFailed
        expr: shopify_backup_module_success == 0
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Shopify backup failed"

      - alert: ShopifyBackupUsingFallback
        expr: shopify_backup_module_success == 1 and shopify_backup_module_success{fallback="REST"} == 1
        for: 1h
        labels:
          severity: warning
        annotations:
          summary: "Shopify backup using REST fallback"
```

## Grafana Dashboard

A sample Grafana dashboard JSON is available at `deploy/grafana-dashboard.json`. Import it to visualize backup metrics.