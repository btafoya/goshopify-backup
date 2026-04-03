# Shopify Backup Go Port - Design Document

> Reference: [Shopify GraphQL Admin API 2025-07](https://shopify.dev/docs/api/admin-graphql/2025-07)
> Reference: [go-graphql-client](https://github.com/hasura/go-graphql-client)
> Reference: [Resty](https://github.com/go-resty/resty)

---

## 1. Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                         main.go                                  │
│                   Entry point + signal handling                   │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                        config.go                                 │
│            Env var reading + validation (singleton)              │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                      backup.Run(ctx)                             │
│         Phase 1          │         Phase 2          │  Phase 3   │
│   ┌─────┬─────┬─────┐   │   ┌──────┬──────┬─────┐ │  ┌──────┐  │
│   │ Prod│ Cust│ Order│   │   │ Pages│ Blogs│Meta │ │  │Images│  │
│   │     │     │     │   │   │      │      │ fields│ │  │      │  │
│   └──┬──┴──┬──┴──┬──┘   │   └──┬───┴──┬──┴──┬──┘ │  └──┬───┘  │
│      │     │     │      │       │      │      │      │       │
│      ▼     ▼     ▼      │       ▼      ▼      ▼      ▼       │
│   GraphQL Bulk Ops      │       REST API (rate-limited)         │
│                         │                                       │
└─────────────────────────┴───────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                      cleanup.go                                  │
│            Phase 4: Retention-based cleanup (UTC)                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## 2. Project Structure

```
shopify-backup/
├── go.mod
├── go.sum
├── main.go                          # Entry point, signal handling, phase orchestration
├── config.go                        # Environment variable reading and validation
├── config_test.go
├── types.go                         # Shared types: BackupConfig, BackupResult, BackupStatus
├── constants.go                     # Magic numbers and constants
├── constants_test.go
│
├── shopify/
│   ├── client.go                    # Client factory, rate limiter, shared config
│   ├── graphql.go                   # GraphQL client + bulk operation submission/polling
│   ├── graphql_test.go
│   ├── rest.go                      # REST client with pagination + retry
│   ├── rest_test.go
│   └── types.go                     # Shopify API types (generated from spec)
│
├── jsonl/
│   ├── parser.go                    # Streaming JSONL parse with bufio.Scanner
│   ├── parser_test.go
│   ├── reconstruct.go               # Parent-child reconstruction from __parentId
│   └── reconstruct_test.go
│
├── backup/
│   ├── products.go                  # backupProducts(), image download with semaphore
│   ├── products_test.go
│   ├── customers.go                 # backupCustomers() - bulk + REST fallback
│   ├── customers_test.go
│   ├── orders.go                    # backupOrders() - bulk + REST fallback
│   ├── orders_test.go
│   ├── collections.go                # backupCollections() - bulk only
│   ├── collections_test.go
│   └── content.go                   # backupContent() - pages, blogs, shop metafields
│   └── content_test.go
│
├── status/
│   ├── writer.go                    # Buffered channel writer
│   ├── writer_test.go
│   └── types.go                     # Status types
│
├── cleanup.go                        # cleanupOldBackups()
└── cleanup_test.go
```

### 2.1 Metafields Backup Strategy

Metafields are backed up in **two categories**:

**A. Entity Metafields** (included in bulk operation JSON files):
- Product metafields → nested in `products.json`
- Variant metafields → nested in `products.json`
- Customer metafields → nested in `customers.json`
- Order metafields → nested in `orders.json`
- Collection metafields → nested in `collections.json`

**B. Shop Metafields** (REST backup):
- Fetched separately via REST → written to `metafields.json`
- Endpoint: `GET /admin/api/{version}/metafields.json`
- Called in Phase 2 as part of `backupContent()`

---

## 3. Component Interfaces

### 3.1 Config (`config.go`)

```go
// Config holds all configuration from environment variables
type Config struct {
    Store         string        // SHOPIFY_STORE (required)
    AccessToken   string        // SHOPIFY_ACCESS_TOKEN (required)
    APIVersion    string        // SHOPIFY_API_VERSION (default: 2025-01)
    BackupDir     string        // BACKUP_DIR (default: /backups/shopify)
    RetentionDays int           // RETENTION_DAYS (default: 30, max: 3650)
    Force         bool          // --force flag to re-run completed modules
    PollTimeout   time.Duration // Computed from constants
}

// GetConfig() reads and validates environment variables
// Returns error if required vars are missing or invalid
func GetConfig() (*Config, error)

// ValidateConfig performs startup validation of all config values
// Returns error with specific message for each validation failure
func ValidateConfig(cfg *Config) error
```

**Validation Rules:**
- `SHOPIFY_STORE`: Must start with `https://`, end with `.myshopify.com`
- `SHOPIFY_ACCESS_TOKEN`: Non-empty string
- `SHOPIFY_API_VERSION`: Must match `^\d{4}-\d{2}$`
- `BACKUP_DIR`: Must be writable directory (create if doesn't exist)
- `RETENTION_DAYS`: Must be 1-3650 (clamp if out of range, warn)

### 3.2 Rate Limiter (`shopify/client.go`)

```go
// RateLimiter implements a leaky bucket algorithm for 40 req/sec
// Uses sync.Mutex to ensure thread-safe interval enforcement
type RateLimiter struct {
    mu        sync.Mutex
    lastTime  time.Time
    interval  time.Duration // 25ms for 40 req/sec
}

// Wait blocks until enough time has passed since the last call
func (r *RateLimiter) Wait(ctx context.Context) error

// NewRateLimiter creates a RateLimiter with the specified requests-per-second
func NewRateLimiter(requestsPerSecond int) *RateLimiter
```

### 3.3 GraphQL Client (`shopify/graphql.go`)

```go
// GraphQLClient wraps hasura/go-graphql-client for Shopify GraphQL API
type GraphQLClient struct {
    client    *graphql.Client
    store     string
    accessToken string
    limiter   *RateLimiter
}

// NewGraphQLClient creates a new Shopify GraphQL client
func NewGraphQLClient(cfg *Config, limiter *RateLimiter) *GraphQLClient

// SubmitBulkOperation submits a bulk operation and returns the operation ID
// Uses bulkOperationRunQuery mutation
func (c *GraphQLClient) SubmitBulkOperation(ctx context.Context, query string) (string, error)

// PollBulkOperation polls currentBulkOperation until COMPLETED, FAILED, or timeout
// Returns the result URL for JSONL download
func (c *GraphQLClient) PollBulkOperation(ctx context.Context, timeout time.Duration) (string, error)

// BulkOperationStatus represents the status of a bulk operation
type BulkOperationStatus string

const (
    StatusCreated   BulkOperationStatus = "CREATED"
    StatusRunning   BulkOperationStatus = "RUNNING"
    StatusCompleted BulkOperationStatus = "COMPLETED"
    StatusFailed    BulkOperationStatus = "FAILED"
    StatusCanceled  BulkOperationStatus = "CANCELED"
)
```

### 3.4 REST Client (`shopify/rest.go`)

```go
// RESTClient wraps resty.Client for Shopify REST API
type RESTClient struct {
    client      *resty.Client
    store       string
    accessToken string
    limiter     *RateLimiter
}

// NewRESTClient creates a new Shopify REST client with retry configured
func NewRESTClient(cfg *Config, limiter *RateLimiter) *RESTClient

// Get performs a GET request with rate limiting and retry
func (c *RESTClient) Get(ctx context.Context, path string, result interface{}) error

// GetPages fetches all pages using cursor pagination (page_info)
// Returns accumulated results or writes through via callback
func (c *RESTClient) GetPages(ctx context.Context, path string, params map[string]string, result interface{}, writeThrough func(page int)) error

// AccessDeniedError indicates GraphQL bulk operation access was denied
// Used to trigger REST fallback
type AccessDeniedError struct {
    Message string
}

func (e *AccessDeniedError) Error() string
```

### 3.5 JSONL Parser (`jsonl/parser.go`)

```go
// Parser streaming-parses JSONL using bufio.Scanner
// Avoids loading entire file into memory
type Parser struct {
    scanner *bufio.Scanner
    reader  io.Reader
}

// NewParser creates a new JSONL parser from an io.Reader
func NewParser(r io.Reader) *Parser

// Scan advances to the next JSON line
// Returns false when scanning is done or error
func (p *Parser) Scan() bool

// Decode decodes the current JSON line into dst
func (p *Parser) Decode(dst interface{}) error

// DecodeRaw decodes the current JSON line and returns raw bytes
func (p *Parser) DecodeRaw() ([]byte, error)

// Err returns any error encountered during scanning
func (p *Parser) Err() error
```

### 3.6 JSONL Reconstructor (`jsonl/reconstruct.go`)

```go
// Reconstructor builds nested objects from flat JSONL with __parentId references
// Shopify bulk operations return flat JSONL; this rebuilds the hierarchy

// GID type prefixes for routing children to parents
const (
    GIDLineItem         = "gid://shopify/LineItem/"
    GIDOrderTransaction  = "gid://shopify/OrderTransaction/"
    GIDOrderFulfillment = "gid://shopify/Fulfillment/"
    GIDOrderRefund       = "gid://shopify/Refund/"
    GIDProductVariant    = "gid://shopify/ProductVariant/"
    GIDProductImage      = "gid://shopify/ProductImage/"
    GIDMetafield         = "gid://shopify/Metafield/"
    GIDCustomerAddress   = "gid://shopify/MailingAddress/"
)

// reconstructBulkData takes flat JSONL records and builds nested structures
// Parent types are identified by their GID prefix; children are routed accordingly
func ReconstructBulkData(records []map[string]interface{}) (map[string]interface{}, error)

// reconstructRecord adds a single record to the appropriate parent
// Returns true if the record was a root object
func ReconstructRecord(root map[string]interface{}, record map[string]interface{}) bool
```

### 3.7 Structured Logging (`logger/logger.go`)

```go
// Logger provides structured JSON logging
type Logger struct {
    logger *logrus.Logger
}

// LogFields for structured logging
type LogFields struct {
    Module string `json:"module"`
    Action string `json:"action"`
    Store  string `json:"store,omitempty"`
    Count  int    `json:"count,omitempty"`
    Error  string `json:"error,omitempty"`
    Duration string `json:"duration,omitempty"`
}

// NewLogger creates a new structured logger with JSON formatter
func NewLogger(level string) *Logger

// Info logs at info level with fields
func (l *Logger) Info(message string, fields LogFields)

// Warn logs at warn level with fields
func (l *Logger) Warn(message string, fields LogFields)

// Error logs at error level with fields
func (l *Logger) Error(message string, fields LogFields)
```

**Log Format (JSON):**
```json
{
  "time": "2026-04-03T02:00:00Z",
  "level": "info",
  "msg": "Submitted bulk operation",
  "module": "products",
  "action": "bulk_submit",
  "store": "example.myshopify.com"
}
```

### 3.8 Status Writer (`status/writer.go`)

```go
// Writer is a buffered channel-based status file writer
// Batches updates and flushes every 5 seconds or on module completion
type Writer struct {
    ch       chan StatusUpdate
    done     chan struct{}
    flushInterval time.Duration
}

// StatusUpdate represents a single status change
type StatusUpdate struct {
    Module    string            // "products", "customers", etc.
    Status    ModuleStatus      // "running", "completed", "failed"
    Count     int               // Records processed
    Error     string            // Error message if failed
    Timestamp time.Time
}

// NewWriter creates a buffered status writer
// Starts a background goroutine that handles writes
func NewWriter(flushInterval time.Duration) *Writer

// Update sends a status update (non-blocking)
func (w *Writer) Update(ctx context.Context, update StatusUpdate)

// Close flushes pending updates and stops the writer
func (w *Writer) Close(ctx context.Context) error

// WriteStatus writes the current status map to disk
func (w *Writer) WriteStatus(dir string, status BackupStatus) error
```

### 3.9 Lock Management

```go
// LockManager handles concurrent backup prevention
type LockManager struct {
    lockDir   string
    staleLockDuration time.Duration // 24 hours
}

// LockFile represents the lock file content
type LockFile struct {
    PID      int       `json:"pid"`
    StartedAt time.Time `json:"startedAt"`
}

// Acquire attempts to acquire a lock for the backup date
// Returns error if lock exists and is recent
func (lm *LockManager) Acquire(ctx context.Context, date string) error

// Release removes the lock file
func (lm *LockManager) Release(date string) error

// IsStale checks if a lock file is stale (> 24 hours)
func (lm *LockManager) IsStale(date string) (bool, error)
```

**Lock File Location:** `{BACKUP_DIR}/{YYYY-MM-DD}/.lock`

### 3.10 Partial Backup Recovery

```go
// RecoveryManager handles resuming from incomplete backups
type RecoveryManager struct {
    statusDir string
}

// LoadStatus reads the existing status.json if it exists
func (rm *RecoveryManager) LoadStatus(date string) (*BackupStatus, error)

// GetCompletedModules returns list of modules that completed successfully
func (rm *RecoveryManager) GetCompletedModules(status *BackupStatus) []string

// ShouldResume returns true if backup should resume (not start fresh)
func (rm *RecoveryManager) ShouldResume(date string) (bool, *BackupStatus, error)

// MarkModuleCompleted updates status when a module finishes
func (rm *RecoveryManager) MarkModuleCompleted(date string, moduleName string, count int, fileSize int64) error
```

**Recovery Logic:**
1. On startup, check for `status.json` in output directory
2. If exists and any module shows `"status": "failed"`, resume from that module
3. Skip modules with `"status": "completed"` (unless `--force` flag set)
4. Overwrite output files for modules being re-run
5. Continue until all modules complete

### 3.11 Backup Module Interface

```go
// BackupModule defines the interface for each backup module
type BackupModule interface {
    // Name returns the module name (used in status)
    Name() string

    // Run executes the backup
    // ctx: cancellation context
    // client: GraphQL client for bulk operations
    // restClient: REST client for fallback
    // outputDir: directory to write JSON files
    // statusCh: channel for status updates
    Run(ctx context.Context, client *shopify.GraphQLClient, restClient *shopify.RESTClient, outputDir string, statusCh chan<- status.StatusUpdate) error
}

// Available modules:
// - productsModule: backupProducts + image download
// - customersModule: backupCustomers (bulk + REST fallback)
// - ordersModule: backupOrders (bulk + REST fallback)
// - collectionsModule: backupCollections
// - contentModule: backupContent (pages, blogs, metafields)
```

---

## 4. API Specifications

### 4.1 Shopify GraphQL API

**Endpoint**: `https://{SHOPIFY_STORE}/admin/api/2025-07/graphql.json`

**Authentication**:
```
X-Shopify-Access-Token: {SHOPIFY_ACCESS_TOKEN}
```

**Mutations**

#### `bulkOperationRunQuery`

```graphql
mutation bulkOperationRunQuery($query: String!, $groupObjects: Boolean!) {
  bulkOperationRunQuery(query: $query, groupObjects: $groupObjects) {
    bulkOperation {
      id
      status
    }
    userErrors {
      field
      message
    }
  }
}
```

**Variables**:
```json
{
  "query": "{ products(first: 250) { edges { node { id title } } } }",
  "groupObjects": false
}
```

**Note**: `groupObjects: false` is recommended for large datasets to reduce operation time and failure risk.

---

**Queries**

#### `currentBulkOperation`

```graphql
query currentBulkOperation {
  currentBulkOperation {
    id
    status
    completedAt
    errorCode
    fileSize
    objectCount
    url
  }
}
```

**Polling**: Poll every 1000ms until status is `COMPLETED`, `FAILED`, `CANCELED`, or `EXPIRED`.

---

### 4.2 Shopify REST API

**Base URL**: `https://{SHOPIFY_STORE}/admin/api/2025-07/`

**Authentication**: `X-Shopify-Access-Token` header

**Pagination**: Cursor-based using `page_info` parameter

```
GET /admin/api/2025-07/products.json?limit=250&page_info=xxx
```

**Response Headers**:
- `Link`: Contains `rel="next"` with `page_info` cursor for next page
- `X-Shopify-Api-Request-Unique-Request-Id`: For deduplication

**Pagination Parsing**:
```go
func parseLinkHeader(header string) (nextPageInfo string, hasNext bool)
```

---

### 4.3 Bulk Query Definitions

#### Products Query

```graphql
query {
  products {
    edges {
      node {
        id
        title
        handle
        descriptionHtml
        createdAt
        updatedAt
        vendor
        productType
        tags
        status
        variants(first: 250) {
          edges {
            node {
              id
              title
              price
              sku
              inventoryQuantity
              weight
              weightUnit
              compareAtPrice
              metafields(first: 50) {
                edges {
                  node {
                    id
                    namespace
                    key
                    value
                    type
                  }
                }
              }
            }
          }
        }
        images(first: 250) {
          edges {
            node {
              id
              src
              altText
              width
              height
            }
          }
        }
        metafields(first: 50) {
          edges {
            node {
              id
              namespace
              key
              value
              type
            }
          }
        }
      }
    }
  }
}
```

#### Customers Query

```graphql
query {
  customers {
    edges {
      node {
        id
        email
        firstName
        lastName
        phone
        createdAt
        updatedAt
        ordersCount
        state
        totalSpent
        addresses {
          id
          address1
          address2
          city
          province
          country
          zip
          phone
        }
        metafields(first: 50) {
          edges {
            node {
              id
              namespace
              key
              value
              type
            }
          }
        }
      }
    }
  }
}
```

#### Orders Query

```graphql
query {
  orders(first: 250) {
    edges {
      node {
        id
        name
        orderNumber
        createdAt
        updatedAt
        processedAt
        fulfillmentStatus
        financialStatus
        totalPrice { amount currencyCode }
        subtotalPrice { amount currencyCode }
        totalTax { amount currencyCode }
        totalDiscounts { amount currencyCode }
        lineItems(first: 250) {
          edges {
            node {
              id
              title
              quantity
              price { amount currencyCode }
              variant { id title }
            }
          }
        }
        transactions(first: 250) {
          edges {
            node {
              id
              kind
              status
              amount { amount currencyCode }
            }
          }
        }
        fulfillments(first: 250) {
          edges {
            node {
              id
              status
              trackingCompany
              trackingNumber
            }
          }
        }
        refunds(first: 250) {
          edges {
            node {
              id
              createdAt
              amount { amount currencyCode }
            }
          }
        }
        metafields(first: 50) {
          edges {
            node {
              id
              namespace
              key
              value
              type
            }
          }
        }
      }
    }
  }
}
```

#### REST Content Endpoints (Phase 2)

These endpoints are called via REST in `backupContent()`:

##### Pages
```
GET /admin/api/2025-01/pages.json?limit=250&page_info=xxx
```

##### Blogs
```
GET /admin/api/2025-01/blogs.json?limit=250&page_info=xxx
```

##### Articles (nested under blogs)
```
GET /admin/api/2025-01/blogs/{blog_id}/articles.json?limit=250&page_info=xxx
```

##### Shop Metafields
```
GET /admin/api/2025-01/metafields.json?limit=250&page_info=xxx
```

**Response**:
```json
[
  {
    "id": 123456789,
    "namespace": "custom",
    "key": "featured_product",
    "value": "gid://shopify/Product/123",
    "type": "product_reference"
  }
]
```

All REST endpoints use cursor pagination via `page_info` token in the `Link` header response.

### 5. Metafields Backup Strategy

Metafields are backed up in **three layers**:

| Source | Endpoint | Output File | Method |
|--------|----------|-------------|--------|
| **Shop metafields** | `GET /admin/api/{version}/metafields.json` | `metafields.json` | REST paginated |
| **Product metafields** | Bulk query includes `products { metafields }` | `products.json` | GraphQL bulk |
| **Variant metafields** | Bulk query includes `variants { metafields }` | `products.json` | GraphQL bulk |
| **Customer metafields** | Bulk query includes `customers { metafields }` | `customers.json` | GraphQL bulk |
| **Order metafields** | Bulk query includes `orders { metafields }` | `orders.json` | GraphQL bulk |
| **Collection metafields** | Bulk query includes `collections { metafields }` | `collections.json` | GraphQL bulk |

**Shop metafields** are fetched via REST in `backupContent()` because they are not available via bulk operations (only entity-specific metafields are included in bulk queries).

#### Collections Query

```graphql
query {
  collections(first: 250) {
    edges {
      node {
        id
        title
        handle
        description
        createdAt
        updatedAt
        sortOrder
        products(first: 250) {
          edges {
            node { id }
          }
        }
        ruleSet {
          rules {
            field
            relation
            value
          }
        }
        metafields(first: 50) {
          edges {
            node {
              id
              namespace
              key
              value
              type
            }
          }
        }
      }
    }
  }
}
```

---

## 5. Data Models

### 5.1 Bulk Operation Response Types

```go
// BulkOperationRunQueryResponse is the response from bulkOperationRunQuery mutation
type BulkOperationRunQueryResponse struct {
    Data struct {
        BulkOperationRunQuery struct {
            BulkOperation struct {
                ID     string `json:"id"`
                Status string `json:"status"`
            } `json:"bulkOperation"`
            UserErrors []UserError `json:"userErrors"`
        } `json:"bulkOperationRunQuery"`
    } `json:"data"`
}

// CurrentBulkOperationResponse is the response from currentBulkOperation query
type CurrentBulkOperationResponse struct {
    Data struct {
        CurrentBulkOperation *BulkOperation `json:"currentBulkOperation"`
    } `json:"data"`
}

// BulkOperation represents a Shopify bulk operation
type BulkOperation struct {
    ID          string    `json:"id"`
    Status      string    `json:"status"`
    CompletedAt string    `json:"completedAt"`
    ErrorCode   string    `json:"errorCode"`
    FileSize    int64     `json:"fileSize"`
    ObjectCount int64     `json:"objectCount"`
    URL         string    `json:"url"`
}

// UserError represents a GraphQL user error
type UserError struct {
    Field   []string `json:"field"`
    Message string   `json:"message"`
}
```

### 5.2 JSONL Record Types

```go
// JSONL records are flat maps with __parentId for hierarchy
// Example LineItem record:
// {
//   "id": "gid://shopify/LineItem/1",
//   "title": "Widget",
//   "__parentId": "gid://shopify/Order/123",
//   "__typename": "LineItem"
// }

// reconstructBulkData builds nested structures
// Root objects: Order, Product, Customer, Collection
// Children are routed by __typename or GID prefix
```

### 5.3 Status Types

```go
// BackupStatus is written to status.json
type BackupStatus struct {
    StartedAt   time.Time         `json:"startedAt"`
    CompletedAt time.Time         `json:"completedAt,omitempty"`
    Duration    string            `json:"duration,omitempty"`
    Modules     map[string]ModuleStatus `json:"modules"`
    TotalSize   int64             `json:"totalSize,omitempty"`
}

// ModuleStatus represents the status of a single backup module
type ModuleStatus struct {
    Status     string    `json:"status"`      // "pending", "running", "completed", "failed"
    StartedAt  time.Time `json:"startedAt"`
    CompletedAt time.Time `json:"completedAt,omitempty"`
    Count      int       `json:"count"`
    Error      string    `json:"error,omitempty"`
    FileSize   int64     `json:"fileSize,omitempty"`
}
```

---

## 6. Error Handling

### 6.1 Error Types

```go
// BackupError is a general backup failure
type BackupError struct {
    Module string
    Cause  error
    Msg    string
}

func (e *BackupError) Error() string

// AccessDeniedError triggers REST fallback
type AccessDeniedError struct {
    Message string
}

// NetworkError is retryable
type NetworkError struct {
    Cause error
}

// IsRetryable returns true for 429, 5xx, and network errors
func IsRetryable(err error) bool
```

### 6.2 Retry Configuration

Using `sethgrid/pestretry` with resty:

```go
// On resty client:
client.SetRetryCount(3).
    SetRetryWaitTime(2 * time.Second).
    SetRetryMaxWaitTime(30 * time.Second).
    AddRetryCondition(func(r *resty.Response, err error) bool {
        if err != nil {
            return true // Network error
        }
        return r.StatusCode() == 429 || r.StatusCode() >= 500
    })
```

---

## 7. Constants

```go
package constants

const (
    // API
    APIVersion             = "2025-01"         // Shopify API version
    GraphQLEndpointFormat  = "https://%s/admin/api/%s/graphql.json"
    RESTEndpointFormat     = "https://%s/admin/api/%s/"

    // Rate Limiting
    RequestsPerSecond      = 40                 // Shopify rate limit
    MinRequestInterval     = 25 * time.Millisecond // 1000ms / 40 = 25ms

    // Retry
    RetryCount             = 3
    RetryBaseDelay         = 2 * time.Second
    RetryMaxDelay          = 30 * time.Second

    // Bulk Operations
    PollInterval           = 1 * time.Second
    PollTimeout            = 10 * time.Minute

    // Image Download
    ImageConcurrency       = 10
    ImageMaxRetries        = 3

    // Status Writer
    StatusFlushInterval    = 5 * time.Second

    // Retention
    MaxRetentionDays       = 3650

    // Output
    DateFormat             = "2006-01-02"
)

// AllowedDomains for bulk operation JSONL download URLs
var AllowedDomains = []string{
    "storage.shopifycloud.com",
    "shopify.com",
}
```

---

## 8. File Output

### 8.1 Output Directory Structure

```
{BACKUP_DIR}/
└── {YYYY-MM-DD}/
    ├── status.json
    ├── products.json
    ├── customers.json
    ├── orders.json
    ├── collections.json
    ├── pages.json
    ├── blogs.json
    ├── metafields.json
    └── images/
        └── {product_id}/
            ├── 0.jpg
            ├── 1.png
            └── ...
```

### 8.2 status.json Format

```json
{
  "startedAt": "2026-04-02T02:00:00Z",
  "completedAt": "2026-04-02T02:15:30Z",
  "duration": "15m30s",
  "modules": {
    "products": {
      "status": "completed",
      "startedAt": "2026-04-02T02:00:00Z",
      "completedAt": "2026-04-02T02:05:00Z",
      "count": 1523,
      "fileSize": 4582930
    },
    "customers": {
      "status": "completed",
      "startedAt": "2026-04-02T02:00:00Z",
      "completedAt": "2026-04-02T02:08:00Z",
      "count": 8921,
      "fileSize": 2341234
    },
    "orders": {
      "status": "failed",
      "startedAt": "2026-04-02T02:00:00Z",
      "error": "ACCESS_DENIED: GraphQL bulk operation access denied",
      "fallback": "REST"
    }
  },
  "totalSize": 6924164
}
```

---

## 10. Testing Strategy

### 10.1 Table-Driven Tests

```go
func TestReconstructBulkData(t *testing.T) {
    tests := []struct {
        name    string
        records []map[string]interface{}
        want    int // expected root object count
    }{
        {
            name: "simple order with line items",
            records: []map[string]interface{}{
                {"id": "gid://shopify/Order/1", "__typename": "Order"},
                {"id": "gid://shopify/LineItem/1", "__parentId": "gid://shopify/Order/1", "__typename": "LineItem"},
            },
            want: 1,
        },
        // ... more cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := ReconstructBulkData(tt.records)
            if err != nil {
                t.Fatalf("ReconstructBulkData() error = %v", err)
            }
            if len(got) != tt.want {
                t.Errorf("got %d root objects, want %d", len(got), tt.want)
            }
        })
    }
}
```

### 10.2 Test Categories

| File | What to Test |
|---|---|
| `parser_test.go` | Valid JSONL, invalid JSON, empty lines, UTF-8 |
| `reconstruct_test.go` | Parent-child routing, multi-level nesting, GID prefixes |
| `rest_test.go` | Link header parsing, pagination logic |
| `config_test.go` | Required vars, defaults, validation |
| `cleanup_test.go` | Retention calculation, directory deletion |

---

## 11. Implementation Order

1. **Foundation**: `go.mod`, `constants.go`, `types.go`, `config.go`
2. **Core clients**: `shopify/client.go`, `shopify/graphql.go`, `shopify/rest.go`
3. **JSONL handling**: `jsonl/parser.go`, `jsonl/reconstruct.go`
4. **Status writer**: `status/writer.go`
5. **Backup modules**:
   - `backup/products.go` (most complex - includes images)
   - `backup/customers.go` (bulk + REST fallback)
   - `backup/orders.go` (bulk + REST fallback)
   - `backup/collections.go`
   - `backup/content.go`
6. **Orchestration**: `main.go`, `cleanup.go`
7. **Tests**: Table-driven tests for each package
8. **Docker**: `docker/Dockerfile`
