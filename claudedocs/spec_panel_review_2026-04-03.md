# Expert Panel Review - Shopify Backup Go Specification

**Review Date**: 2026-04-03
**Expert Panel**: Wiegers, Adzic, Fowler, Nygard, Hightower, Crispin, Newman, Cockburn, Gregory
**Specification Files**: `prd/go-port-spec.md`, `prd/go-port-design.md`

---

## Executive Summary

**Overall Quality Score: 6.8/10**

| Dimension | Score | Status |
|-----------|-------|--------|
| Requirements Clarity | 7.5/10 | Good |
| Architecture Quality | 7.0/10 | Good |
| Testability | 5.0/10 | Needs Work |
| Operational Readiness | 5.5/10 | Needs Work |
| Completeness | 7.0/10 | Good |

The specification has a strong architectural foundation with clear tech stack decisions, but lacks concrete behavioral scenarios, error handling details, and operational requirements.

---

## Critical Issues (Must Fix Before Implementation)

### 1. Error Handling Undefined [HIGH SEVERITY]
**Expert**: Wiegers, Adzic, Nygard

**Problem**: "REST fallback" and "retry logic" mentioned but no specific behavior defined.

**What triggers fallback?**
- GraphQL bulk operation returns "ACCESS_DENIED" only?
- Network timeout? 403? 500?
- How many retries before switching?

**Recommendation**:
```go
// Define explicit error handling:
const (
    MAX_BULK_RETRIES = 1
    RETRY_DELAY_MS   = 5000
)

func shouldTriggerFallback(err error) bool {
    // Only ACCESS_DENIED triggers fallback
    return strings.Contains(err.Error(), "ACCESS_DENIED")
}
```

### 2. No Concrete Scenarios [HIGH SEVERITY]
**Expert**: Adzic, Cockburn

**Problem**: Specification describes "what" but not "how" in executable terms.

**Recommendation**: Add Given/When/Then scenarios:

```gherkin
Scenario: Bulk operation fails with access denied
  Given a Shopify store with 5,000 customers
  When backupCustomers() is called
  And GraphQL bulk operation returns ACCESS_DENIED
  Then retry bulk operation exactly 1 time
  And if ACCESS_DENIED again, switch to REST pagination
  And fetch all customers via GET /customers.json?page_info=...
  And write to customers.json
  And status.json includes "fallback": "REST"
```

### 3. No Startup Validation [HIGH SEVERITY]
**Expert**: Hightower

**Problem**: Environment variables documented but no validation strategy.

**Recommendation**:
```go
func ValidateConfig(cfg *Config) error {
    if !strings.HasPrefix(cfg.Store, "https://") {
        return fmt.Errorf("SHOPIFY_STORE must be HTTPS URL")
    }
    if cfg.RetentionDays < 1 || cfg.RetentionDays > 3650 {
        return fmt.Errorf("RETENTION_DAYS must be 1-3650, got %d", cfg.RetentionDays)
    }
    if cfg.AccessToken == "" {
        return fmt.Errorf("SHOPIFY_ACCESS_TOKEN is required")
    }
    return nil
}
```

### 4. Partial Backup Recovery Undefined [HIGH SEVERITY]
**Expert**: Nygard

**Problem**: If backup crashes mid-operation, what happens on restart?

**Recommendation**:
- Status file tracks module completion
- On restart, read status.json and resume from last incomplete module
- Modules check for existing output and skip if completed
- `--force` flag to re-run completed modules

---

## High Priority Issues

### 5. No Observability Specification [HIGH PRIORITY]
**Expert**: Hightower, Crispin

**Problem**: No structured logging, metrics, or monitoring defined.

**Recommendation**:
```go
// Structured logging (JSON format)
log.WithFields(log.Fields{
    "module": "products",
    "action": "bulk_submit",
    "store": cfg.Store,
}).Info("Submitted bulk operation")

// Prometheus metrics
var (
    backupDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "shopify_backup_duration_seconds",
            Help: "Backup duration by module",
        },
        []string{"module"},
    )
    backupRecords = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "shopify_backup_records_total",
            Help: "Total records backed up",
        },
        []string{"module"},
    )
)
```

### 6. Missing Test Scenarios [HIGH PRIORITY]
**Expert**: Crispin

**Problem**: Edge cases not identified for testing.

**Additional Test Cases Needed**:
- Empty store (0 products, 0 customers)
- Very large store (10,000+ products, 100,000+ orders)
- Deleted/soft-deleted entities
- Malformed JSONL from Shopify
- Unicode in titles/descriptions
- Very long descriptions (> 1MB)
- Images larger than available memory
- Concurrent backup runs (lock mechanism needed?)

### 7. Graceful Shutdown Undefined [HIGH PRIORITY]
**Expert**: Hightower, Nygard

**Problem**: Signal handling mentioned but behavior unspecified.

**Recommendation**:
```go
// SIGTERM behavior:
// 1. Stop accepting new work
// 2. Complete current module
// 3. Flush status writer
// 4. Exit with code 0 if no errors, 1 if partial

// SIGKILL:
// Immediate exit, status may be incomplete
// Recovery via status.json on restart
```

### 8. No Containerization Specs [HIGH PRIORITY]
**Expert**: Hightower

**Problem**: Dockerfile, runtime requirements, resource limits undefined.

**Recommendation**:
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

---

## Medium Priority Issues

### 9. Rate Limiter Interface Coupling [MEDIUM]
**Expert**: Fowler

**Problem**: RateLimiter couples state with execution logic.

**Current**:
```go
type RateLimiter struct {
    mu       sync.Mutex
    lastTime time.Time
    interval time.Duration
}
func (r *RateLimiter) Wait(ctx context.Context) error
```

**Better**:
```go
type RateLimitState struct {
    lastTime time.Time
}
type RateLimiter struct {
    state    *RateLimitState
    interval time.Duration
    mu       sync.Mutex
}
// Makes testing easier, state separable
```

### 10. Shopify API Version Handling [MEDIUM]
**Expert**: Newman

**Problem**: Hardcoded "2025-01" version, no deprecation handling.

**Recommendation**:
- Make API version configurable via `SHOPIFY_API_VERSION` env var
- Validate version is still supported at startup
- Document when 2025-01 will be deprecated

### 11. Image Download Failure Strategy [MEDIUM]
**Expert**: Nygard, Crispin

**Problem**: "MAX_IMAGE_RETRIES = 3" but no behavior after failures.

**Recommendation**:
```go
type ImageResult struct {
    URL     string `json:"url"`
    Status  string `json:"status"`  // "success" or "failed"
    Error   string `json:"error,omitempty"`
}

// Failed images logged in status.json
// Backup continues (don't fail entire backup)
```

---

## Low Priority / Defer

### 12. Circuit Breaker Not Needed [LOW PRIORITY]
**Expert**: Nygard (initially) → Expert Consensus

**Problem**: Nygard suggested circuit breaker, but...

**Resolution**: For single-shot nightly backup, circuit breaker adds unnecessary complexity. Simple retry with exponential backoff is sufficient.

### 13. Separate Image Downloader [LOW PRIORITY]
**Expert**: Fowler

**Problem**: backup/products.go handles both bulk operations AND image download.

**Resolution**: Defer to refactoring phase. Can separate later if image download becomes complex.

### 14. Health Check Endpoint [LOW PRIORITY]
**Expert**: Hightower

**Problem**: No health check endpoint mentioned.

**Resolution**: CLI tool doesn't need health endpoint. Exit codes and status.json sufficient.

---

## Expert Consensus Points

1. **All experts agree**: Error handling needs concrete scenarios
2. **All experts agree**: Testing strategy missing critical edge cases
3. **All experts agree**: Operational concerns (monitoring, logging) absent
4. **All experts agree**: Configuration validation at startup required

---

## Improvement Roadmap

### Phase 1 - Before Implementation (Critical)
- [ ] Add 5-7 concrete Given/When/Then scenarios
- [ ] Define backup success/failure criteria with exit codes
- [ ] Add startup validation requirements for all env vars
- [ ] Specify partial backup recovery behavior
- [ ] Add error handling scenarios with retry logic

### Phase 2 - During Implementation (High)
- [ ] Add structured logging specification
- [ ] Define comprehensive test strategy with edge cases
- [ ] Specify graceful shutdown on SIGTERM
- [ ] Add Dockerfile requirements

### Phase 3 - Refinement (Medium)
- [ ] Add observability (Prometheus metrics)
- [ ] Add API version handling strategy
- [ ] Refine RateLimiter interface

---

## Definition of Done

Before declaring this specification ready for implementation:

- [ ] All Phase 1 items completed
- [ ] Specification reviewed by developer, tester, and business stakeholder
- [ ] Test strategy documented with edge cases
- [ ] Docker/build requirements specified
- [ ] Error handling scenarios defined
- [ ] Success/failure criteria clear

---

## Quality Gates for Implementation

| Gate | Criteria |
|------|----------|
| Code Quality | golangci-lint passes, go fmt applied |
| Test Coverage | >80% coverage, table-driven tests |
| Integration | Tests pass against test store |
| Documentation | README with usage examples |
| Container | Dockerfile builds and runs |

---

## Next Steps

1. **Incorporate Phase 1 critical items into specification**
2. **Conduct specification workshop** (developer, tester, stakeholder)
3. **Refine PRD documents** with added scenarios
4. **Create test specification document**
5. **Proceed to implementation** once Phase 1 complete

---

*Report generated by Expert Panel Review System*
*Experts simulated: Wiegers, Adzic, Fowler, Nygard, Hightower, Crispin, Newman, Cockburn, Gregory*