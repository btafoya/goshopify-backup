# Shopify Restore CLI - TUI Design Document

> Reference: [Shopify GraphQL Admin API 2025-07](https://shopify.dev/docs/api/admin-graphql/2025-07)
> Reference: [Bubbletea](https://github.com/charmbracelet/bubbletea)
> Reference: [Bubbles](https://github.com/charmbracelet/bubbles)

---

## 1. Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           main.go (shopify-restore)                       │
│                         Entry point + CLI flags                           │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              tui/model.go                                   │
│                          Bubbletea Model State Machine                     │
│  ┌─────────┬──────────┬──────────┬──────────┬──────────┬───────────────┐  │
│  │ Config  │ Backup   │ Entity   │ Item     │ Preview  │ Progress      │  │
│  │ State   │ Select   │ Select   │ Select   │ Screen   │ Screen        │  │
│  └────┬────┴────┬─────┴────┬─────┴────┬─────┴────┬─────┴───────┬───────┘  │
└───────┼───────────┼───────────┼───────────┼─────────────────┼─────────────┘
        │           │           │           │                 │
        ▼           ▼           ▼           ▼                 ▼
┌──────────────┐ ┌──────────┐ ┌──────────┐ ┌──────────────┐ ┌─────────────┐
│ credentials/ │ │ backup/  │ │ entity/  │ │ diff/         │ │ restore/    │
│              │ │ loader   │ │ loader   │ │ preview       │ │ executor    │
│ - saved      │ │ - list   │ │ - items  │ │ - conflict    │ │ - GraphQL   │
│ - prompt     │ │ - read   │ │ - filter │ │ - rollback    │ │ - REST      │
└──────────────┘ └──────────┘ └──────────┘ └──────────────┘ └─────────────┘
        │                       │               │                 │
        └───────────────────────┴───────────────┴─────────────────┘
                                        │
                                        ▼
                         ┌──────────────────────────────┐
                         │   shopify/ (shared package)  │
                         │                              │
                         │ - client.go                  │
                         │ - graphql.go (mutations)     │
                         │ - rest.go (mutations)        │
                         │ - rate_limiter.go            │
                         └──────────────────────────────┘
```

---

## 2. Project Structure

```
shopify-restore/                      # New separate binary (or cmd/restore/)
├── go.mod
├── go.sum
├── main.go                           # Entry point, flag parsing
├── cmd/
│   └── restore/                      # Command directory if using cmd/ pattern
│       └── main.go
│
├── tui/                              # Bubbletea TUI components
│   ├── model.go                      # Main model with state machine
│   ├── update.go                     # Message handling
│   ├── view.go                       # View rendering
│   ├── keys.go                       # Key bindings
│   ├── style.go                      # Lipgloss styling
│   └── init.go                       # Initial model creation
│
├── tui/views/                        # View components
│   ├── backup_select.go              # Backup directory picker
│   ├── entity_select.go              # Entity type sidebar
│   ├── item_list.go                  # Checkbox list with filters
│   ├── config_screen.go              # Store configuration
│   ├── preview_screen.go             # Diff/preview before restore
│   ├── progress_screen.go            # Progress bar + status
│   ├── conflict_screen.go            # Conflict resolution prompts
│   └── abort_screen.go               # Abort confirmation dialog
│
├── tui/components/                   # Reusable Bubbletea components
│   ├── filter_bar.go                 # Filter input component
│   ├── date_range.go                 # Date range picker
│   ├── status_bar.go                 # Bottom status bar
│   ├── help_overlay.go               # Help screen overlay
│   └── confirm_dialog.go             # Yes/no confirmation dialog
│
├── backup/                           # Backup file handling
│   ├── loader.go                     # Load backup metadata
│   ├── reader.go                     # Read JSON files
│   ├── validator.go                  # Validate backup structure
│   ├── images.go                     # Handle image files
│   └── status_reader.go              # Parse status.json
│
├── entity/                           # Entity data structures
│   ├── products.go                   # Product data + variants
│   ├── customers.go                  # Customer data + addresses
│   ├── orders.go                     # Order data + relations
│   ├── collections.go                # Collection data + rules
│   ├── metaobjects.go                # Metaobject definitions + entries
│   └── common.go                     # Common interfaces
│
├── restore/                          # Restore execution
│   ├── executor.go                   # Main restore orchestrator
│   ├── graphql_mutations.go          # GraphQL mutations
│   ├── rest_mutations.go             # REST mutations
│   ├── rate_limiter.go               # Rate limiting (shared)
│   ├── conflict_resolver.go          # Conflict handling
│   ├── rollback_generator.go         # Rollback script generation
│   ├── image_uploader.go             # Product image upload
│   ├── relation_checker.go           # Entity relationship validation
│   └── state_manager.go              # Resume state persistence
│
├── diff/                             # Diff and preview
│   ├── preview.go                    # Generate preview of changes
│   ├── conflict.go                   # Detect conflicts
│   └── formatter.go                  # Format diff for display
│
├── credentials/                      # Credential management
│   ├── loader.go                     # Load from multiple sources
│   ├── saver.go                      # Save encrypted credentials
│   ├── prompt.go                     # TUI credential prompt
│   └── validator.go                  # Validate credentials
│
├── log/                              # Logging
│   ├── logger.go                     # Structured logger
│   ├── file_writer.go                # Log file writer
│   └── formatter.go                  # Log formatting
│
├── types.go                          # Shared types
├── constants.go                      # Constants
└── config.go                         # Configuration

# Shared packages (from backup tool)
shopify/                              # Shared with backup tool
├── client.go                         # Shared client factory
├── graphql.go                        # GraphQL client
├── rest.go                           # REST client
├── rate_limiter.go                   # Leaky bucket rate limiter
└── types.go                          # Shopify API types
```

---

## 3. Component Interfaces

### 3.1 Main TUI Model (`tui/model.go`)

```go
// Model is the main Bubbletea model with state machine
type Model struct {
    // State determines which view is active
    state State

    // Configuration
    cfg *Config

    // Shopify clients
    graphqlClient *shopify.GraphQLClient
    restClient    *shopify.RESTClient

    // Backup selection
    backupDir    string
    selectedDate string
    backupList   []BackupInfo

    // Entity selection
    activeEntity EntityType
    entityStates map[EntityType]EntityState

    // Restore state
    selectedItems    map[EntityType]map[string]Item // Entity type -> ID -> Item
    previewChanges   []PreviewChange
    restoreResults   []RestoreResult
    restoreProgress  *RestoreProgress
    restoreStateFile string

    // Sub-models for Bubbles components
    filterInput      textinput.Model
    dateRangePicker  datepicker.Model
    confirmDialog    *ConfirmDialogModel
    helpOverlay      *HelpOverlayModel

    // Status
    width  int
    height int
    quit   bool
}

// State represents the TUI state machine
type State int

const (
    StateConfig       State = iota // Configure target store
    StateBackupSelect             // Select backup directory
    StateEntitySelect             // Select entity type
    StateItemSelect               // Select items to restore
    StatePreview                  // Preview changes
    StateConfirm                  // Confirm restore
    StateRunning                  // Restore in progress
    StateComplete                 // Restore completed
    StateError                    // Error state
    StateAbort                    // Abort confirmation
)

// EntityType represents the types of entities to restore
type EntityType string

const (
    EntityProducts    EntityType = "products"
    EntityCustomers   EntityType = "customers"
    EntityOrders      EntityType = "orders"
    EntityCollections EntityType = "collections"
    EntityMetaobjects EntityType = "metaobjects"
)

// BackupInfo represents a backup directory
type BackupInfo struct {
    Date     time.Time
    Path     string
    Status   *BackupStatus
    FileSize int64
}

// EntityState represents the state of an entity type
type EntityState struct {
    items    []Item
    filtered []Item
    selected map[string]bool // ID -> selected
    cursor   int
    filters  FilterCriteria
}

// Item represents a single entity item
type Item struct {
    ID          string
    Title       string
    Handle      string
    CreatedAt   time.Time
    UpdatedAt   time.Time
    Status      string
    Tags        []string
    CustomData  map[string]interface{}
}

// FilterCriteria represents active filters
type FilterCriteria struct {
    SearchText     string
    DateFrom       *time.Time
    DateTo         *time.Time
    Statuses       []string
    Tags           []string
}

// PreviewChange represents a change that will be made
type PreviewChange struct {
    EntityType EntityType
    ItemID     string
    ItemTitle  string
    Action     ChangeAction
    Conflict   *ConflictInfo
}

// ChangeAction represents the type of change
type ChangeAction string

const (
    ActionCreate   ChangeAction = "create"  // New item
    ActionUpdate   ChangeAction = "update"  // Update existing
    ActionSkip     ChangeAction = "skip"    // Will skip
    ActionConflict ChangeAction = "conflict" // Needs resolution
)

// ConflictInfo represents a conflict with existing data
type ConflictInfo struct {
    ExistingID   string
    ExistingData map[string]interface{}
    BackupData   map[string]interface{}
    Diff         string
}

// RestoreProgress tracks restore progress
type RestoreProgress struct {
    totalItems     int
    completedItems int
    failedItems    int
    currentEntity  EntityType
    currentItem    string
    logs           []LogEntry
}

// RestoreResult represents the result of a single item restore
type RestoreResult struct {
    EntityType EntityType
    ItemID     string
    Success    bool
    Error      string
    RestoredID string // New ID in target store
    CreatedAt  time.Time
}

// LogEntry represents a log entry
type LogEntry struct {
    Level     string
    Message   string
    Timestamp time.Time
}

// Init initializes the model
func (m Model) Init() tea.Cmd

// Update handles messages
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd)

// View renders the TUI
func (m Model) View() string
```

### 3.2 Restore Executor (`restore/executor.go`)

```go
// Executor handles the restore operation
type Executor struct {
    cfg            *Config
    graphqlClient  *shopify.GraphQLClient
    restClient     *shopify.RESTClient
    logger         *log.Logger
    rateLimiter    *shopify.RateLimiter
    rollbackScript *RollbackScript
    progressCh     chan<- RestoreProgress
    abortCh        <-chan struct{}
}

// NewExecutor creates a new restore executor
func NewExecutor(cfg *Config, graphqlClient *shopify.GraphQLClient, restClient *shopify.RESTClient, logger *log.Logger, progressCh chan<- RestoreProgress, abortCh <-chan struct{}) *Executor

// Execute performs the restore operation
func (e *Executor) Execute(ctx context.Context, items map[EntityType]map[string]Item) ([]RestoreResult, error)

// ExecuteEntity restores all items of a specific entity type
func (e *Executor) ExecuteEntity(ctx context.Context, entityType EntityType, items map[string]Item) ([]RestoreResult, error)

// ExecuteItem restores a single item
func (e *Executor) ExecuteItem(ctx context.Context, entityType EntityType, item Item) (*RestoreResult, error)

// GenerateRollbackScript generates a rollback script before restore
func (e *Executor) GenerateRollbackScript(items map[EntityType]map[string]Item) (*RollbackScript, error)

// SaveRollbackScript saves the rollback script to disk
func (e *Executor) SaveRollbackScript(script *RollbackScript, path string) error
```

### 3.3 Conflict Resolver (`restore/conflict_resolver.go`)

```go
// ConflictResolver handles conflicts during restore
type ConflictResolver struct {
    cfg           *Config
    graphqlClient *shopify.GraphQLClient
    restClient    *shopify.RESTClient
}

// NewConflictResolver creates a new conflict resolver
func NewConflictResolver(cfg *Config, graphqlClient *shopify.GraphQLClient, restClient *shopify.RESTClient) *ConflictResolver

// CheckConflict checks if an item conflicts with existing data
func (r *ConflictResolver) CheckConflict(ctx context.Context, entityType EntityType, item Item) (*ConflictInfo, error)

// ResolveConflict resolves a conflict based on user choice
func (r *ConflictResolver) ResolveConflict(ctx context.Context, entityType EntityType, item Item, resolution ConflictResolution) error

// ConflictResolution represents how to resolve a conflict
type ConflictResolution string

const (
    ResolutionSkip     ConflictResolution = "skip"
    ResolutionOverwrite ConflictResolution = "overwrite"
    ResolutionRename   ConflictResolution = "rename"
)
```

### 3.4 Rollback Generator (`restore/rollback_generator.go`)

```go
// RollbackScript contains commands to revert a restore
type RollbackScript struct {
    GeneratedAt time.Time
    TargetStore string
    Backups     []RollbackAction
}

// RollbackAction represents a single rollback action
type RollbackAction struct {
    EntityType  EntityType
    Action      string // "delete", "update", "untag"
    ID          string
    Data        map[string]interface{}
    Description string
}

// RollbackGenerator generates rollback scripts
type RollbackGenerator struct{}

// NewRollbackGenerator creates a new rollback generator
func NewRollbackGenerator() *RollbackGenerator

// Generate generates a rollback script from restore results
func (g *RollbackGenerator) Generate(results []RestoreResult, items map[EntityType]map[string]Item) *RollbackScript

// Write writes the rollback script to a file
func (g *RollbackGenerator) Write(script *RollbackScript, path string) error
```

### 3.5 Backup Loader (`backup/loader.go`)

```go
// Loader loads and validates backup data
type Loader struct {
    backupDir string
}

// NewLoader creates a new backup loader
func NewLoader(backupDir string) *Loader

// ListBackups lists all available backups
func (l *Loader) ListBackups(baseDir string) ([]BackupInfo, error)

// LoadBackup loads a specific backup
func (l *Loader) LoadBackup(date string) (*Backup, error)

// Backup represents a loaded backup
type Backup struct {
    Date      time.Time
    Path      string
    Status    *BackupStatus
    Products  []*Product
    Customers []*Customer
    Orders    []*Order
    Collections []*Collection
    Metaobjects *MetaobjectData
}

// LoadEntity loads entity data from backup
func (l *Loader) LoadEntity(entityType EntityType) ([]Item, error)

// Validate validates backup structure
func (l *Loader) Validate() error
```

---

## 4. Shopify API Specifications

### 4.1 GraphQL Mutations

#### Product Mutations

```graphql
# Create a product
mutation productCreate($input: ProductInput!) {
  productCreate(input: $input) {
    product {
      id
      title
      handle
    }
    userErrors {
      field
      message
    }
  }
}

# Update a product
mutation productUpdate($input: ProductInput!, $id: ID!) {
  productUpdate(input: $input, id: $id) {
    product {
      id
      title
      handle
    }
    userErrors {
      field
      message
    }
  }
}

# Create a product variant
mutation productVariantCreate($productId: ID!, $input: ProductVariantInput!) {
  productVariantCreate(productId: $productId, input: $input) {
    productVariant {
      id
      title
      sku
    }
    userErrors {
      field
      message
    }
  }
}

# Create product image
mutation productCreateMedia($productId: ID!, $media: [CreateMediaInput!]!) {
  productCreateMedia(productId: $productId, media: $media) {
    media {
      ... on MediaImage {
        image {
          id
          url
        }
      }
    }
    userErrors {
      field
      message
    }
  }
}

# Add tag to product
mutation tagsAdd($id: ID!, $tags: [String!]!) {
  tagsAdd(id: $id, tags: $tags) {
    node {
      id
      ... on Product {
        tags
      }
    }
    userErrors {
      field
      message
    }
  }
}
```

#### Customer Mutations

```graphql
# Create a customer
mutation customerCreate($input: CustomerInput!) {
  customerCreate(input: $input) {
    customer {
      id
      email
      firstName
      lastName
    }
    userErrors {
      field
      message
    }
  }
}

# Update a customer
mutation customerUpdate($input: CustomerUpdateInput!, $id: ID!) {
  customerUpdate(input: $input, id: $id) {
    customer {
      id
      email
    }
    userErrors {
      field
      message
    }
  }
}

# Create customer address
mutation customerAddressCreate($customerId: ID!, $address: MailingAddressInput!) {
  customerAddressCreate(customerId: $customerId, address: $address) {
    customerAddress {
      id
      address1
    }
    userErrors {
      field
      message
    }
  }
}
```

#### Order Mutations

**Note**: Orders cannot be created via GraphQL. Use REST API.

#### Collection Mutations

```graphql
# Create a custom collection
mutation collectionCreate($input: CollectionInput!) {
  collectionCreate(input: $input) {
    collection {
      id
      title
      handle
    }
    userErrors {
      field
      message
    }
  }
}

# Update a collection
mutation collectionUpdate($input: CollectionInput!, $id: ID!) {
  collectionUpdate(input: $input, id: $id) {
    collection {
      id
      title
    }
    userErrors {
      field
      message
    }
  }
}

# Add products to collection
mutation collectionAddProducts($id: ID!, $productIds: [ID!]!) {
  collectionAddProducts(id: $id, productIds: $productIds) {
    collection {
      id
      products(first: 10) {
        edges {
          node {
            id
          }
        }
      }
    }
    userErrors {
      field
      message
    }
  }
}
```

#### Metaobject Mutations

```graphql
# Create metaobject definition
mutation metaobjectDefinitionCreate($definition: MetaobjectDefinitionInput!) {
  metaobjectDefinitionCreate(definition: $definition) {
    metaobjectDefinition {
      id
      type
      fieldDefinitions {
        key
        name
        type {
          name
        }
      }
    }
    userErrors {
      field
      message
    }
  }
}

# Create metaobject entry
mutation metaobjectCreate($metaobject: MetaobjectInput!) {
  metaobjectCreate(metaobject: $metaobject) {
    metaobject {
      id
      handle
      type
      fields {
        key
        value
      }
    }
    userErrors {
      field
      message
    }
  }
}

# Update metaobject entry
mutation metaobjectUpdate($id: ID!, $metaobject: MetaobjectUpdateInput!) {
  metaobjectUpdate(id: $id, metaobject: $metaobject) {
    metaobject {
      id
      handle
    }
    userErrors {
      field
      message
    }
  }
}
```

### 4.2 REST Mutations

#### Order Creation (REST only)

```
POST /admin/api/2025-07/orders.json
Content-Type: application/json

{
  "order": {
    "line_items": [
      {
        "variant_id": 123456789,
        "quantity": 1
      }
    ],
    "customer": {
      "id": 987654321
    },
    "billing_address": {
      "first_name": "John",
      "last_name": "Doe",
      "address1": "123 Main St",
      "city": "New York",
      "province": "NY",
      "country": "US",
      "zip": "10001"
    },
    "shipping_address": {
      "first_name": "John",
      "last_name": "Doe",
      "address1": "123 Main St",
      "city": "New York",
      "province": "NY",
      "country": "US",
      "zip": "10001"
    },
    "financial_status": "paid",
    "tags": "restored_from_backup:2026-04-03"
  }
}
```

#### Product Image Upload (REST for large files)

```
POST /admin/api/2025-07/products/{product_id}/images.json
Content-Type: multipart/form-data

image[src]=@image.jpg
image[alt_text]=Product Image
image[position]=1
```

### 4.3 Queries for Conflict Detection

```graphql
# Check if product exists by handle
query($handle: String!) {
  products(first: 1, query: $handle) {
    edges {
      node {
        id
        title
        handle
        updatedAt
      }
    }
  }
}

# Check if customer exists by email
query($email: String!) {
  customers(first: 1, query: $email) {
    edges {
      node {
        id
        email
        updatedAt
      }
    }
  }
}

# Check if collection exists by handle
query($handle: String!) {
  collections(first: 1, query: $handle) {
    edges {
      node {
        id
        title
        handle
        updatedAt
      }
    }
  }
}

# Check if metaobject exists by handle
query($type: String!, $handle: String!) {
  metaobjects(first: 1, type: $type, handle: $handle) {
    nodes {
      id
      handle
      updatedAt
    }
  }
}
```

---

## 5. TUI State Machine

```
┌─────────────────┐
│  StateConfig    │──[credentials provided]──>┌──────────────────┐
│  (Store Config) │                            │  StateBackup     │
└─────────────────┘                            │  Select          │
                                              └──────────────────┘
                                                     │
                              ┌──────────────────────┼──────────────────────┐
                              │                      │                      │
                              ▼                      ▼                      ▼
                     ┌───────────────┐    ┌───────────────┐    ┌───────────────┐
                     │ StateEntity   │    │ StateEntity   │    │ StateEntity   │
                     │ Select        │    │ Select        │    │ Select        │
                     │ (Products)    │    │ (Customers)   │    │ (Orders)      │
                     └───────┬───────┘    └───────┬───────┘    └───────┬───────┘
                             │                    │                    │
                             └────────────────────┼────────────────────┘
                                                  │
                                                  ▼
                                         ┌──────────────────┐
                                         │ StateItemSelect  │
                                         │ (Filter/Select   │
                                         │  individual      │
                                         │  items)          │
                                         └────────┬─────────┘
                                                  │
                              ┌───────────────────┼───────────────────┐
                              │                   │                   │
                              ▼                   ▼                   ▼
                         ┌──────────┐       ┌──────────┐       ┌──────────┐
                         │ State    │       │ State    │       │ State    │
                         │ Preview  │       │ Preview  │       │ Preview  │
                         │ (Diff    │       │ (Diff    │       │ (Diff    │
                         │  show)   │       │  show)   │       │  show)   │
                         └────┬─────┘       └────┬─────┘       └────┬─────┘
                              │                   │                   │
                              └───────────────────┼───────────────────┘
                                                  │
                                                  ▼
                                         ┌──────────────────┐
                                         │  StateConfirm    │
                                         │  (Final OK)      │
                                         └────────┬─────────┘
                                                  │
                                ┌─────────────────┴─────────────────┐
                                │                                   │
                                ▼                                   ▼
                         ┌─────────────┐                    ┌─────────────┐
                         │  State      │                    │  State      │
                         │  Running    │                    │  Complete   │
                         │  (Progress) │                    │  (Summary)  │
                         └──────┬──────┘                    └─────────────┘
                                │
                ┌───────────────┼───────────────┐
                │               │               │
                ▼               ▼               ▼
         ┌──────────┐   ┌──────────┐   ┌──────────┐
         │ State    │   │ State    │   │ State    │
         │ Conflict │   │ Error    │   │ Abort    │
         │ (Resolve │   │ (Show    │   │ (Prompt) │
         │  prompt) │   │  error)  │   │          │
         └──────────┘   └──────────┘   └──────────┘
```

---

## 6. Data Flow Diagrams

### 6.1 Restore Flow

```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│   Select    │    │   Load      │    │   Validate  │    │  Preview    │
│   Backup    │───▶│   Items     │───▶│  Relations  │───▶│   Changes   │
└─────────────┘    └─────────────┘    └─────────────┘    └─────────────┘
                                                                  │
                                                                  ▼
┌─────────────┐    ┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│   Generate  │    │   Execute   │    │   Tag       │    │  Generate   │
│  Rollback   │───▶│   Restore   │───▶│  Restored   │───▶│   Log &     │
│   Script    │    │             │    │  Items      │    │   Summary   │
└─────────────┘    └─────────────┘    └─────────────┘    └─────────────┘
                           │
           ┌───────────────┼───────────────┐
           │               │               │
           ▼               ▼               ▼
    ┌──────────┐   ┌──────────┐   ┌──────────┐
    │ Conflict?│   │  Error?  │   │  Abort?  │
    │  Prompt  │   │  Retry   │   │  Handle  │
    └──────────┘   └──────────┘   └──────────┘
```

### 6.2 Conflict Resolution Flow

```
┌──────────────────┐
│  Check if item   │
│  exists in store │
└────────┬─────────┘
         │
    ┌────┴────┐
    │         │
   No        Yes
    │         │
    ▼         ▼
┌────────┐ ┌──────────────────┐
│ Create │ │  Show Conflict    │
│  item  │ │  Dialog:          │
└────────┘ │  - Skip           │
           │  - Overwrite      │
           │  - Rename         │
           └────────┬─────────┘
                    │
        ┌───────────┼───────────┐
        │           │           │
        ▼           ▼           ▼
    ┌──────┐  ┌──────────┐ ┌──────────┐
    │ Skip │  │Overwrite │ │  Rename  │
    └──────┘  └──────────┘ └──────────┘
```

---

## 7. Constants

```go
package constants

const (
    // API
    APIVersion             = "2025-07"
    GraphQLEndpointFormat  = "https://%s/admin/api/%s/graphql.json"
    RESTEndpointFormat     = "https://%s/admin/api/%s/"

    // Rate Limiting
    RequestsPerSecond      = 40
    MinRequestInterval     = 25 * time.Millisecond

    // Retry
    RestoreRetryCount      = 3
    RestoreRetryDelay      = 2 * time.Second

    // TUI
    RefreshInterval        = 500 * time.Millisecond
    PageSize               = 50
    MaxDisplayItems        = 1000

    // Files
    RestoreTag             = "restored_from_backup"
    StateFile              = ".restore_state.json"
    RollbackFile           = "rollback_%s.sh"
    LogFile                = "restore_%s.log"

    // Dates
    DateFormat             = "2006-01-02"
    DateTimeFormat         = "2006-01-02T15:04:05Z"

    // Limits
    MaxConcurrentUploads   = 5
    MaxBatchSize           = 250

    // Credentials
    CredentialsDir         = "~/.config/goshopify"
    CredentialsFile        = "credentials.json"

    // Store URL
    StoreDomainPattern     = `^https://[a-zA-Z0-9][a-zA-Z0-9\-]*\.myshopify\.com$`
)

// Entity Types
var EntityTypes = []EntityType{
    EntityProducts,
    EntityCustomers,
    EntityOrders,
    EntityCollections,
    EntityMetaobjects,
}

// EntityDisplayNames maps entity types to display names
var EntityDisplayNames = map[EntityType]string{
    EntityProducts:    "Products",
    EntityCustomers:   "Customers",
    EntityOrders:      "Orders",
    EntityCollections: "Collections",
    EntityMetaobjects: "Metaobjects",
}
```

---

## 8. Configuration

```go
// Config holds restore configuration
type Config struct {
    // Source
    BackupDir     string        // Backup directory (default: /backups/shopify)
    BackupDate    string        // Specific backup date (empty = latest)

    // Target Store
    Store         string        // Target store URL
    AccessToken   string        // Shopify access token
    APIVersion    string        // Shopify API version

    // Behavior
    DryRun        bool          // Validate only, don't restore
    Force         bool          // Override conflicts without prompt
    RestoreImages ImagePolicy   // How to handle images
    Resume        bool          // Resume from interrupted restore

    // Output
    LogDir        string        // Log directory
    RollbackDir   string        // Rollback script directory
    Verbose       bool          // Verbose logging
}

// ImagePolicy defines how to handle product images
type ImagePolicy string

const (
    ImageRestore    ImagePolicy = "restore"    // Always restore
    ImageSkip       ImagePolicy = "skip"       // Always skip
    ImageInteractive ImagePolicy = "interactive" // Ask per product
)

// GetConfig reads configuration from flags, env vars, and saved credentials
func GetConfig() (*Config, error)

// ValidateConfig validates configuration
func ValidateConfig(cfg *Config) error
```

---

## 9. Credential Management

```go
// Credential represents saved store credentials
type Credential struct {
    Store       string    `json:"store"`
    AccessToken string    `json:"access_token"`
    APIVersion  string    `json:"api_version"`
    LastUsed    time.Time `json:"last_used"`
    Nickname    string    `json:"nickname,omitempty"`
}

// CredentialStore manages saved credentials
type CredentialStore struct {
    path     string
    store    map[string]Credential
    encrypted bool
}

// NewCredentialStore creates a new credential store
func NewCredentialStore(path string, encrypted bool) (*CredentialStore, error)

// Load loads credentials from disk
func (cs *CredentialStore) Load() error

// Save saves credentials to disk
func (cs *CredentialStore) Save() error

// List returns all saved credentials
func (cs *CredentialStore) List() []Credential

// Add adds a credential
func (cs *CredentialStore) Add(cred Credential) error

// Remove removes a credential
func (cs *CredentialStore) Remove(store string) error

// Get gets a credential by store
func (cs *CredentialStore) Get(store string) (Credential, bool)
```

**Priority Order for Credentials:**
1. Environment variables (`SHOPIFY_STORE`, `SHOPIFY_ACCESS_TOKEN`)
2. Command line flags (`--store`, `--token`)
3. Saved credentials from `~/.config/goshopify/credentials.json`
4. TUI prompt

---

## 10. Logging

```go
// RestoreLogger provides structured logging for restore operations
type RestoreLogger struct {
    file    *os.File
    logger  *logrus.Logger
    fields  map[string]interface{}
}

// NewRestoreLogger creates a new restore logger
func NewRestoreLogger(path string) (*RestoreLogger, error)

// Log logs a message with structured fields
func (rl *RestoreLogger) Log(level logrus.Level, message string, fields map[string]interface{})

// WithFields adds fields to subsequent log calls
func (rl *RestoreLogger) WithFields(fields map[string]interface{}) *RestoreLogger

// Close closes the logger
func (rl *RestoreLogger) Close() error

// Log Entry Format
{
  "time": "2026-04-03T14:30:00Z",
  "level": "info",
  "msg": "Restored product",
  "entity_type": "products",
  "source_id": "gid://shopify/Product/123",
  "target_id": "gid://shopify/Product/456",
  "title": "Sample Product",
  "duration": "1.2s"
}
```

---

## 11. Error Handling

### Error Types

```go
// RestoreError represents a restore error
type RestoreError struct {
    EntityType EntityType
    ItemID     string
    ItemTitle  string
    Code       string
    Message    string
    Retryable  bool
    Cause      error
}

func (e *RestoreError) Error() string

func (e *RestoreError) Unwrap() error

// IsRetryable returns true if the error is retryable
func IsRetryable(err error) bool

// Error Codes
const (
    ErrCodeNotFound     = "NOT_FOUND"
    ErrCodeConflict     = "CONFLICT"
    ErrCodeValidation   = "VALIDATION"
    ErrCodeRateLimit    = "RATE_LIMIT"
    ErrCodeNetwork      = "NETWORK"
    ErrCodeAuth         = "AUTH"
    ErrCodePermission   = "PERMISSION"
    ErrCodeUnknown      = "UNKNOWN"
)
```

### Error Recovery

| Error Type | Retryable | Fallback |
|------------|-----------|----------|
| 429 Rate Limit | Yes (exponential backoff) | Wait and retry |
| 5xx Server Error | Yes (3x max) | Continue to next item |
| Network Timeout | Yes (3x max) | Continue to next item |
| Conflict (duplicate) | No | Prompt user |
| Validation Error | No | Log and skip |
| Auth Error | No | Abort restore |

---

## 12. Rollback Script Format

```bash
#!/bin/bash
# Shopify Restore Rollback Script
# Generated: 2026-04-03T14:30:00Z
# Target Store: example.myshopify.com
# Backup Date: 2026-04-02

set -e

STORE="example.myshopify.com"
TOKEN="${SHOPIFY_ACCESS_TOKEN:-}"
API_VERSION="2025-07"

if [ -z "$TOKEN" ]; then
    echo "Error: SHOPIFY_ACCESS_TOKEN not set"
    exit 1
fi

# Helper function for GraphQL mutation
graphql_mutation() {
    local query="$1"
    curl -X POST \
        -H "X-Shopify-Access-Token: $TOKEN" \
        -H "Content-Type: application/json" \
        -d "{\"query\": \"$query\"}" \
        "https://$STORE/admin/api/$API_VERSION/graphql.json"
}

# Delete restored products
echo "Rolling back products..."
graphql_mutation "mutation { productDelete(input: {id: \"gid://shopify/Product/456\"}) { deletedId } }"

# Remove restore tag from updated items
echo "Removing restore tags..."
graphql_mutation "mutation { tagsRemove(id: \"gid://shopify/Product/123\", tags: [\"restored_from_backup:2026-04-02\"]) { node { id } } }"

echo "Rollback complete"
```

---

## 13. Resume State Format

```json
{
  "startedAt": "2026-04-03T14:30:00Z",
  "backupDate": "2026-04-02",
  "targetStore": "example.myshopify.com",
  "selectedItems": {
    "products": ["gid://shopify/Product/123", "gid://shopify/Product/456"],
    "customers": [],
    "orders": [],
    "collections": [],
    "metaobjects": []
  },
  "completedItems": [
    {
      "entityType": "products",
      "sourceId": "gid://shopify/Product/123",
      "targetId": "gid://shopify/Product/789"
    }
  ],
  "failedItems": [
    {
      "entityType": "products",
      "sourceId": "gid://shopify/Product/456",
      "error": "Conflict: Product already exists"
    }
  ],
  "progress": {
    "total": 2,
    "completed": 1,
    "failed": 0
  }
}
```

---

## 14. TUI Layout Specification

### Two-Pane + Sidebar Layout

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│  Shopify Restore                                                ? Help  Q Quit │
├──────────┬──────────────────────────────────────────────────────────────────────┤
│          │ ┌────────────────────────────────────────────────────────────────┐  │
│  Entity  │ │  Select Items to Restore                                       │  │
│          │ └────────────────────────────────────────────────────────────────┘  │
│ ┌──────┐ │ ┌────────────────────────────────────────────────────────────────┐  │
│ │      │ │ │ Filter: [____________________]  Status: [All ▼]  Date: [Range] │  │
│ │Products│ │ ├────────────────────────────────────────────────────────────────┤  │
│ │      │ │ │ > [x] Sample Product #1                   $10.00        Active │  │
│ ├──────┤ │ │   [ ] Another Product                       $25.00        Draft │  │
│ │      │ │ │   [ ] Third Product                         $15.00        Active │  │
│ │Custom│ │ │   [ ] Fourth Product                        $30.00        Active │  │
│ │ers   │ │ │   [ ] Fifth Product                         $20.00        Active │  │
│ ├──────┤ │ │   ...                                                                  │  │
│ │      │ │ │                                                                      │  │
│ │Orders │ │ │ Space: Toggle  Ctrl+A: Select All  /: Search  ?: Help                │  │
│ │      │ │ │                                                                      │  │
│ ├──────┤ │ │ Selected: 1 of 185 products   Backup: 2026-04-02   Size: 498KB       │  │
│ │      │ │ └────────────────────────────────────────────────────────────────┘  │
│ │Collec│ │ ┌────────────────────────────────────────────────────────────────┐  │
│ │tions│ │ │ Preview: 1 item will be created                                  │  │
│ └──────┘ │ │ Press Enter to continue, Esc to go back                         │  │
│          │ └────────────────────────────────────────────────────────────────┘  │
│          │                                                                      │
│ ┌──────┐ │ ┌────────────────────────────────────────────────────────────────┐  │
│ │Meta- │ │ │ Backup: 2026-04-02  |  Products: 185  |  Customers: 0           │  │
│ │objects│ │ │ Target: staging.myshopify.com  |  API: 2025-07                 │  │
│ └──────┘ │ └────────────────────────────────────────────────────────────────┘  │
└──────────┴──────────────────────────────────────────────────────────────────────┘
```

### Progress Screen Layout

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│  Shopify Restore - In Progress                                  [■■■■░░░░] 40% │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Restoring: Products                                                         │
│                                                                              │
│  Currently restoring:                                                         │
│    > Sample Product #1 ($10.00)                                             │
│                                                                              │
│  Progress:                                                                   │
│    [████████████████████████████████░░░░░░░░░░░░░░░░░░] 74/185 (40%)         │
│                                                                              │
│  Status:                                                                     │
│    ✓ Created: 74                                                            │
│    ✗ Failed: 0                                                              │
│    ⏸ Skipped: 0                                                             │
│                                                                              │
│  Last 5 events:                                                              │
│    [14:30:15] Created product: Sample Product #1                            │
│    [14:30:14] Created product: Another Product                              │
│    [14:30:13] Created product: Third Product                                 │
│    [14:30:12] Created product: Fourth Product                                │
│    [14:30:11] Created product: Fifth Product                                 │
│                                                                              │
│  Press Esc to abort (will prompt for confirmation)                            │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────────┘
```

---

## 15. Implementation Order

### Phase 1: Foundation
1. `go.mod` setup with Bubbletea dependencies
2. `constants.go`, `types.go`, `config.go`
3. `tui/model.go` - Basic state machine
4. `tui/update.go` - Message routing
5. `tui/view.go` - Basic view rendering

### Phase 2: Data Loading
1. `backup/loader.go` - Load backup files
2. `backup/reader.go` - Parse JSON data
3. `backup/validator.go` - Validate backup structure
4. Entity loaders (`entity/*.go`)

### Phase 3: TUI Views
1. `tui/views/backup_select.go` - Backup picker
2. `tui/views/entity_select.go` - Entity sidebar
3. `tui/views/item_list.go` - Item list with filters
4. `tui/components/filter_bar.go` - Filter input
5. `tui/components/status_bar.go` - Status display

### Phase 4: Restore Logic
1. `restore/executor.go` - Main orchestrator
2. `restore/graphql_mutations.go` - GraphQL mutations
3. `restore/rest_mutations.go` - REST mutations
4. `restore/conflict_resolver.go` - Conflict handling

### Phase 5: Preview & Progress
1. `diff/preview.go` - Generate preview
2. `tui/views/preview_screen.go` - Preview display
3. `tui/views/progress_screen.go` - Progress tracking
4. `log/logger.go` - Logging

### Phase 6: Safety Features
1. `restore/rollback_generator.go` - Rollback script
2. `restore/relation_checker.go` - Relation validation
3. `restore/state_manager.go` - Resume capability
4. `tui/views/conflict_screen.go` - Conflict prompts
5. `tui/views/abort_screen.go` - Abort handling

### Phase 7: Credentials
1. `credentials/loader.go` - Load credentials
2. `credentials/saver.go` - Save credentials
3. `credentials/validator.go` - Validate credentials
4. `tui/views/config_screen.go` - Config UI

### Phase 8: Polish
1. `tui/style.go` - Lipgloss styling
2. `tui/components/help_overlay.go` - Help screen
3. `tui/components/confirm_dialog.go` - Confirm dialogs
4. Tests and error handling

---

## 16. Dependencies

```go
module github.com/btafoya/goshopify-restore

go 1.25.0

require (
    github.com/charmbracelet/bubbletea v1.2.0
    github.com/charmbracelet/bubbles v0.20.0
    github.com/charmbracelet/lipgloss v1.0.0
    github.com/go-resty/resty/v2 v2.17.2
    github.com/sirupsen/logrus v1.9.3
    golang.org/x/sync v0.20.0
    golang.org/x/term v0.20.0
)
```

---

## 17. Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Successful restore |
| 1 | Restore failed (one or more items) |
| 2 | Configuration error |
| 3 | User aborted |
| 4 | Validation error |
| 5 | Network error |

---

## 18. CLI Interface

```bash
# Interactive mode (default)
shopify-restore

# Non-interactive with flags
shopify-restore \
  --backup-dir /backups/shopify \
  --backup-date 2026-04-02 \
  --store staging.myshopify.com \
  --token shpat_xxxx

# Dry-run (validate only)
shopify-restore --dry-run

# Resume interrupted restore
shopify-restore --resume

# Force mode (skip conflict prompts)
shopify-restore --force

# Help
shopify-restore --help
```