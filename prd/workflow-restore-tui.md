# Shopify Restore TUI - Implementation Workflow

> **Document Version**: 1.0
> **Created**: 2026-04-04
> **Status**: Ready for Implementation

---

## Overview

This workflow outlines the step-by-step implementation of the Shopify Restore CLI with Bubbletea TUI interface. The workflow is organized into 8 phases with clear dependencies, checkpoints, and validation steps.

**Estimated Duration**: 3-5 days of focused development

---

## Phase 1: Foundation Setup

### Goal
Establish project structure, dependencies, and core types.

### Tasks

| ID | Task | File(s) | Dependencies | Est. Time |
|----|------|---------|--------------|-----------|
| 1.1 | Create new Go module for restore tool | `go.mod`, `go.sum` | None | 15 min |
| 1.2 | Add Bubbletea dependencies | `go.mod` | 1.1 | 10 min |
| 1.3 | Create constants file | `constants.go` | 1.1 | 20 min |
| 1.4 | Create shared types | `types.go` | 1.3 | 30 min |
| 1.5 | Create config loader | `config.go` | 1.4 | 30 min |
| 1.6 | Create main entry point | `main.go` | 1.5 | 20 min |
| 1.7 | Verify compiles | - | All above | 5 min |

### Task Details

#### 1.1 Create Go Module
```bash
mkdir -p cmd/restore
cd cmd/restore
go mod init github.com/btafoya/goshopify-restore
# Add replace directive for shared shopify package
go mod edit -replace github.com/btafoya/goshopify-backup/shopify=../../shopify
```

#### 1.2 Add Dependencies
```go
require (
    github.com/charmbracelet/bubbletea v1.2.0
    github.com/charmbracelet/bubbles v0.20.0
    github.com/charmbracelet/lipgloss v1.0.0
    github.com/btafoya/goshopify-backup/shopify v0.0.0
    github.com/sirupsen/logrus v1.9.3
    golang.org/x/term v0.20.0
)
```

#### 1.3 Constants (`constants.go`)
Create `constants.go` with all constants from design doc:
- API version strings
- Rate limiting constants
- TUI refresh intervals
- Entity type definitions
- File path patterns
- Error codes

#### 1.4 Types (`types.go`)
Create shared types:
- `State` enum for TUI state machine
- `EntityType` enum
- `Config` struct
- `BackupInfo` struct
- `Item` struct
- `FilterCriteria` struct
- `PreviewChange` struct
- `RestoreResult` struct
- `RestoreProgress` struct

#### 1.5 Config (`config.go`)
Implement config loading with priority:
1. Command line flags
2. Environment variables
3. Saved credentials
4. Default values

#### 1.6 Main (`main.go`)
Create basic entry point:
- Parse CLI flags
- Load config
- Initialize logger
- Start Bubbletea program

### Checkpoint 1: Foundation Complete
**Validation Criteria:**
- [ ] `go build` succeeds without errors
- [ ] `go run main.go --help` shows help text
- [ ] Config validation works (test with invalid config)
- [ ] All constants and types are defined

---

## Phase 2: Backup Data Loading

### Goal
Implement backup file loading and entity data structures.

### Tasks

| ID | Task | File(s) | Dependencies | Est. Time |
|----|------|---------|--------------|-----------|
| 2.1 | Create backup loader | `backup/loader.go` | 1.4 | 45 min |
| 2.2 | Create backup reader | `backup/reader.go` | 2.1 | 30 min |
| 2.3 | Create backup validator | `backup/validator.go` | 2.2 | 30 min |
| 2.4 | Create entity common types | `entity/common.go` | 1.4 | 20 min |
| 2.5 | Create product entity | `entity/products.go` | 2.4 | 30 min |
| 2.6 | Create customer entity | `entity/customers.go` | 2.4 | 30 min |
| 2.7 | Create order entity | `entity/orders.go` | 2.4 | 30 min |
| 2.8 | Create collection entity | `entity/collections.go` | 2.4 | 30 min |
| 2.9 | Create metaobject entity | `entity/metaobjects.go` | 2.4 | 30 min |
| 2.10 | Create backup status reader | `backup/status_reader.go` | 2.1 | 15 min |

### Task Details

#### 2.1 Backup Loader
```go
type Loader struct {
    backupDir string
}

// ListBackups returns all available backup directories
func (l *Loader) ListBackups() ([]BackupInfo, error)

// LoadBackup loads a specific backup
func (l *Loader) LoadBackup(date string) (*Backup, error)
```

#### 2.2 Backup Reader
```go
type Reader struct {
    backupPath string
}

// ReadProducts reads products.json
func (r *Reader) ReadProducts() ([]*Product, error)

// ReadCustomers reads customers.json
func (r *Reader) ReadCustomers() ([]*Customer, error)
// ... similar for other entities
```

#### 2.3 Backup Validator
```go
type Validator struct{}

// Validate checks backup structure
func (v *Validator) Validate(path string) error

// CheckRequiredFiles verifies all required files exist
func (v *Validator) CheckRequiredFiles(path string) error
```

#### 2.4 Entity Common Types
```go
type Entity interface {
    GetID() string
    GetTitle() string
    GetHandle() string
    GetCreatedAt() time.Time
    GetUpdatedAt() time.Time
    GetStatus() string
    ToItem() Item
}
```

### Checkpoint 2: Data Loading Complete
**Validation Criteria:**
- [ ] Can list available backups
- [ ] Can load a backup successfully
- [ ] Validation rejects corrupted backups
- [ ] All entity types can be parsed from JSON
- [ ] Unit tests for loader/reader pass

---

## Phase 3: Basic TUI Framework

### Goal
Implement basic Bubbletea TUI with state machine and core views.

### Tasks

| ID | Task | File(s) | Dependencies | Est. Time |
|----|------|---------|--------------|-----------|
| 3.1 | Create TUI model | `tui/model.go` | 1.4, 1.5 | 45 min |
| 3.2 | Create update handler | `tui/update.go` | 3.1 | 30 min |
| 3.3 | Create view renderer | `tui/view.go` | 3.2 | 30 min |
| 3.4 | Create key bindings | `tui/keys.go` | 3.2 | 15 min |
| 3.5 | Create styling | `tui/style.go` | 3.3 | 20 min |
| 3.6 | Create initial model | `tui/init.go` | 3.1 | 15 min |
| 3.7 | Wire TUI to main | `main.go` (update) | 3.6 | 10 min |

### Task Details

#### 3.1 TUI Model
```go
type Model struct {
    state          State
    cfg            *Config
    width          int
    height         int
    quit           bool
    // ... sub-models for Bubbles components
}

func (m Model) Init() tea.Cmd
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd)
func (m Model) View() string
```

#### 3.2 Update Handler
```go
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        return m.handleKeyMsg(msg)
    case tea.WindowSizeMsg:
        return m.handleResize(msg)
    // ... other message types
    }
}
```

#### 3.3 View Renderer
```go
func (m Model) View() string {
    switch m.state {
    case StateConfig:
        return m.configView()
    case StateBackupSelect:
        return m.backupSelectView()
    // ... other states
    }
}
```

### Checkpoint 3: Basic TUI Complete
**Validation Criteria:**
- [ ] TUI launches without crashing
- [ ] State transitions work correctly
- [ ] Window resize is handled
- [ ] Ctrl+C quits cleanly
- [ ] Help screen displays (? key)

---

## Phase 4: Backup Selection View

### Goal
Implement backup directory picker and store configuration views.

### Tasks

| ID | Task | File(s) | Dependencies | Est. Time |
|----|------|---------|--------------|-----------|
| 4.1 | Create backup select view | `tui/views/backup_select.go` | 3.1, 2.1 | 30 min |
| 4.2 | Create config screen view | `tui/views/config_screen.go` | 3.1, 1.5 | 30 min |
| 4.3 | Create filter bar component | `tui/components/filter_bar.go` | 3.1 | 20 min |
| 4.4 | Create status bar component | `tui/components/status_bar.go` | 3.1 | 15 min |
| 4.5 | Connect config to state | `tui/update.go` (update) | 4.2 | 20 min |
| 4.6 | Add credential loading | `credentials/loader.go` | 1.5 | 30 min |

### Task Details

#### 4.1 Backup Select View
- Display list of available backups with date/size/status
- Allow navigation with arrow keys
- Select with Enter

#### 4.2 Config Screen View
- Display saved credentials (if any)
- Show current target store
- Option to enter new credentials
- Validate store URL format

#### 4.3 Filter Bar Component
```go
type FilterBarModel struct {
    textInput textinput.Model
    active     bool
}

func NewFilterBar() FilterBarModel
func (m FilterBarModel) Update(msg tea.Msg) (FilterBarModel, tea.Cmd)
func (m FilterBarModel) View() string
```

#### 4.4 Status Bar Component
```go
type StatusBarModel struct {
    leftText  string
    rightText string
}

func NewStatusBar(left, right string) StatusBarModel
func (m StatusBarModel) View() string
```

### Checkpoint 4: Selection Complete
**Validation Criteria:**
- [ ] Can select a backup from the list
- [ ] Backup details display correctly
- [ ] Can configure target store
- [ ] Saved credentials load and display
- [ ] Filter bar works (for searching)

---

## Phase 5: Item Selection Views

### Goal
Implement entity sidebar, item list, and filtering functionality.

### Tasks

| ID | Task | File(s) | Dependencies | Est. Time |
|----|------|---------|--------------|-----------|
| 5.1 | Create entity select view | `tui/views/entity_select.go` | 3.1, 4.1 | 20 min |
| 5.2 | Create item list view | `tui/views/item_list.go` | 3.1, 2.2 | 45 min |
| 5.3 | Implement checkbox selection | `tui/views/item_list.go` (update) | 5.2 | 30 min |
| 5.4 | Implement date range filter | `tui/components/date_range.go` | 3.1 | 30 min |
| 5.5 | Implement status filter | `tui/views/item_list.go` (update) | 5.2 | 20 min |
| 5.6 | Implement tag filter | `tui/views/item_list.go` (update) | 5.2 | 20 min |
| 5.7 | Implement bulk select (Ctrl+A) | `tui/update.go` (update) | 5.3 | 15 min |

### Task Details

#### 5.1 Entity Select View
- Sidebar showing all entity types
- Highlight active entity
- Navigate with Tab/arrow keys

#### 5.2 Item List View
- Display items as checkbox list
- Show cursor (>) and selection ([x])
- Display key info (title, status, price, etc.)
- Paginate if more than PageSize

#### 5.3 Checkbox Selection
```go
type ItemListModel struct {
    items    []Item
    selected map[string]bool
    cursor   int
}

func (m ItemListModel) ToggleSelection(id string)
func (m ItemListModel) SelectAll()
func (m ItemListModel) ClearSelection()
```

#### 5.4 Date Range Component
```go
type DateRangePicker struct {
    from      time.Time
    to        time.Time
    editing   string // "from" or "to" or ""
}
```

### Checkpoint 5: Selection Complete
**Validation Criteria:**
- [ ] Entity sidebar navigation works
- [ ] Item list displays correctly for each entity type
- [ ] Checkbox toggle works (Space)
- [ ] Cursor navigation works (arrows + j/k)
- [ ] All filters work (date, status, tags, search)
- [ ] Ctrl+A selects all items

---

## Phase 6: Restore Core

### Goal
Implement restore execution with GraphQL and REST mutations.

### Tasks

| ID | Task | File(s) | Dependencies | Est. Time |
|----|------|---------|--------------|-----------|
| 6.1 | Create restore executor | `restore/executor.go` | 1.5, 5.3 | 45 min |
| 6.2 | Create GraphQL mutations | `restore/graphql_mutations.go` | 6.1 | 60 min |
| 6.3 | Create REST mutations | `restore/rest_mutations.go` | 6.1 | 45 min |
| 6.4 | Create conflict resolver | `restore/conflict_resolver.go` | 6.1 | 45 min |
| 6.5 | Create image uploader | `restore/image_uploader.go` | 6.2 | 30 min |
| 6.6 | Implement rate limiting | `restore/rate_limiter.go` (or use shared) | 6.1 | 15 min |
| 6.7 | Create logger | `log/logger.go` | 6.1 | 20 min |

### Task Details

#### 6.1 Restore Executor
```go
type Executor struct {
    cfg           *Config
    graphqlClient *shopify.GraphQLClient
    restClient    *shopify.RESTClient
    logger        *log.Logger
    rateLimiter   *shopify.RateLimiter
    progressCh    chan<- RestoreProgress
    abortCh       <-chan struct{}
}

func (e *Executor) Execute(ctx context.Context, items map[EntityType]map[string]Item) ([]RestoreResult, error)
func (e *Executor) ExecuteItem(ctx context.Context, entityType EntityType, item Item) (*RestoreResult, error)
```

#### 6.2 GraphQL Mutations
```go
// ProductCreate creates a product via GraphQL
func (m *Mutations) ProductCreate(ctx context.Context, input ProductInput) (string, []UserError, error)

// CustomerCreate creates a customer via GraphQL
func (m *Mutations) CustomerCreate(ctx context.Context, input CustomerInput) (string, []UserError, error)

// CollectionCreate creates a collection via GraphQL
func (m *Mutations) CollectionCreate(ctx context.Context, input CollectionInput) (string, []UserError, error)

// MetaobjectCreate creates a metaobject via GraphQL
func (m *Mutations) MetaobjectCreate(ctx context.Context, input MetaobjectInput) (string, []UserError, error)
```

#### 6.3 REST Mutations
```go
// OrderCreate creates an order via REST (GraphQL not supported)
func (m *RestMutations) OrderCreate(ctx context.Context, input OrderInput) (string, error)

// ProductImageUpload uploads a product image via REST
func (m *RestMutations) ProductImageUpload(ctx context.Context, productID string, imagePath string) (string, error)
```

#### 6.4 Conflict Resolver
```go
type ConflictResolver struct {
    graphqlClient *shopify.GraphQLClient
    restClient    *shopify.RESTClient
}

// CheckConflict checks if item exists in target store
func (r *ConflictResolver) CheckConflict(ctx context.Context, entityType EntityType, item Item) (*ConflictInfo, error)

// ResolveConflict applies the resolution strategy
func (r *ConflictResolver) ResolveConflict(ctx context.Context, entityType EntityType, item Item, resolution ConflictResolution) error
```

### Checkpoint 6: Restore Core Complete
**Validation Criteria:**
- [ ] Can restore a single product
- [ ] Can restore a single customer
- [ ] Can restore a single collection
- [ ] Can restore a single metaobject
- [ ] Conflict detection works
- [ ] Rate limiting is applied
- [ ] Logging captures all operations

---

## Phase 7: Progress & Safety

### Goal
Implement progress tracking, preview, rollback, and resume features.

### Tasks

| ID | Task | File(s) | Dependencies | Est. Time |
|----|------|---------|--------------|-----------|
| 7.1 | Create progress screen view | `tui/views/progress_screen.go` | 3.1, 6.1 | 30 min |
| 7.2 | Create conflict screen view | `tui/views/conflict_screen.go` | 3.1, 6.4 | 30 min |
| 7.3 | Create abort screen view | `tui/views/abort_screen.go` | 3.1 | 20 min |
| 7.4 | Create diff/preview generator | `diff/preview.go` | 6.4 | 45 min |
| 7.5 | Create preview screen view | `tui/views/preview_screen.go` | 3.1, 7.4 | 30 min |
| 7.6 | Create rollback generator | `restore/rollback_generator.go` | 6.1 | 30 min |
| 7.7 | Create relation checker | `restore/relation_checker.go` | 6.1 | 30 min |
| 7.8 | Create state manager for resume | `restore/state_manager.go` | 6.1 | 30 min |
| 7.9 | Implement --resume flag | `main.go` (update) | 7.8 | 15 min |
| 7.10 | Implement --dry-run flag | `main.go` (update) | 7.4 | 15 min |

### Task Details

#### 7.1 Progress Screen
- Show overall progress bar
- Show current entity and item
- Show completed/failed/skipped counts
- Show last N log entries
- Handle abort (Esc)

#### 7.2 Conflict Screen
- Display conflict details
- Show options: Skip, Overwrite, Rename
- Handle user selection
- Apply resolution

#### 7.3 Abort Screen
- Display abort options: Resume, Clean up, Leave partial
- Handle user selection
- Execute chosen action

#### 7.4 Diff/Preview Generator
```go
type PreviewGenerator struct {
    conflictResolver *ConflictResolver
}

// Generate creates a list of preview changes
func (g *PreviewGenerator) Generate(ctx context.Context, items map[EntityType]map[string]Item) ([]PreviewChange, error)
```

#### 7.6 Rollback Generator
```go
type RollbackGenerator struct{}

// Generate creates a rollback script from restore results
func (g *RollbackGenerator) Generate(results []RestoreResult, items map[EntityType]map[string]Item) *RollbackScript

// Write saves the rollback script to disk
func (g *RollbackGenerator) Write(script *RollbackScript, path string) error
```

#### 7.7 Relation Checker
```go
type RelationChecker struct{}

// CheckMissingRelations warns about missing related entities
func (r *RelationChecker) CheckMissingRelations(ctx context.Context, items map[EntityType]map[string]Item) []RelationWarning
```

#### 7.8 State Manager
```go
type StateManager struct {
    statePath string
}

// Save saves the current restore state
func (s *StateManager) Save(state *RestoreState) error

// Load loads a saved restore state
func (s *StateManager) Load() (*RestoreState, error)

// GetCompletedItems returns items that were successfully restored
func (s *StateManager) GetCompletedItems() []CompletedItem
```

### Checkpoint 7: Progress & Safety Complete
**Validation Criteria:**
- [ ] Progress screen updates in real-time
- [ ] Conflict dialog works correctly
- [ ] Abort dialog offers all three options
- [ ] Preview shows all changes
- [ ] Rollback script generates correctly
- [ ] Missing relations are warned about
- [ ] Resume functionality works

---

## Phase 8: Polish & Testing

### Goal
Add remaining UI components, styling, and comprehensive testing.

### Tasks

| ID | Task | File(s) | Dependencies | Est. Time |
|----|------|---------|--------------|-----------|
| 8.1 | Create help overlay | `tui/components/help_overlay.go` | 3.1 | 20 min |
| 8.2 | Create confirm dialog | `tui/components/confirm_dialog.go` | 3.1 | 15 min |
| 8.3 | Refine styling | `tui/style.go` (update) | 8.1 | 30 min |
| 8.4 | Add complete screen view | `tui/views/complete_screen.go` | 3.1 | 20 min |
| 8.5 | Add error screen view | `tui/views/error_screen.go` | 3.1 | 15 min |
| 8.6 | Create credential saver | `credentials/saver.go` | 4.6 | 20 min |
| 8.7 | Add unit tests for core logic | `*_test.go` | All phases | 60 min |
| 8.8 | Add integration tests | `tests/integration_test.go` | 6.1 | 45 min |
| 8.9 | Create README and docs | `README.md` | All phases | 30 min |
| 8.10 | Final verification | - | All above | 15 min |

### Task Details

#### 8.1 Help Overlay
- Display all keyboard shortcuts
- Show per-screen context help
- Accessible via ? or F1

#### 8.2 Confirm Dialog
```go
type ConfirmDialog struct {
    message string
    yes     bool // cursor position
}

func (m ConfirmDialog) Update(msg tea.Msg) (ConfirmDialog, tea.Cmd)
func (m ConfirmDialog) View() string
```

#### 8.3 Styling
- Apply consistent color scheme
- Use Lipgloss for borders and padding
- Ensure good contrast
- Handle color blindness considerations

#### 8.6 Credential Saver
```go
type CredentialSaver struct {
    path     string
    encrypted bool
}

// Save saves credentials to disk
func (s *CredentialSaver) Save(cred Credential) error

// Remove removes saved credentials
func (s *CredentialSaver) Remove(store string) error
```

#### 8.9 Documentation
Create README.md with:
- Installation instructions
- Usage examples
- CLI flags reference
- Keyboard shortcuts reference
- Troubleshooting guide

### Checkpoint 8: Polish Complete
**Validation Criteria:**
- [ ] Help overlay displays correctly
- [ ] All dialogs work smoothly
- [ ] Styling is consistent and professional
- [ ] Unit tests pass (≥80% coverage)
- [ ] Integration tests pass
- [ ] README is complete
- [ ] Binary builds successfully

---

## Dependencies Summary

```
Phase 1 (Foundation)
    └── Phase 2 (Data Loading)
            ├── Phase 3 (Basic TUI)
            │       └── Phase 4 (Backup Selection)
            │               └── Phase 5 (Item Selection)
            │                       └── Phase 6 (Restore Core)
            │                               └── Phase 7 (Progress & Safety)
            │                                       └── Phase 8 (Polish)
            │
            └── Phase 4 (Backup Selection) [alternate path]
                    └── Phase 5 (Item Selection)
```

---

## Checkpoints Summary

| Checkpoint | What to Verify |
|------------|----------------|
| **CP1** | Module builds, help works, config validates |
| **CP2** | Can load backups, parse entities, validate structure |
| **CP3** | TUI launches, state works, resize handled |
| **CP4** | Can select backup, configure store, filter works |
| **CP5** | Entity nav works, item list displays, all filters work |
| **CP6** | Can restore items, conflict detection works |
| **CP7** | Progress updates, dialogs work, rollback/resume work |
| **CP8** | All components polished, tests pass, docs complete |

---

## Risk Mitigation

| Risk | Mitigation |
|------|------------|
| **Bubbletea learning curve** | Start with simple example, refer to docs |
| **GraphQL complexity** | Use shared client, test mutations in Playground first |
| **State management** | Keep state simple, use channels for progress |
| **Rate limiting** | Reuse existing RateLimiter from backup tool |
| **Large dataset performance** | Implement pagination, limit display items |

---

## Success Criteria

The implementation is considered complete when:

1. **Functional Requirements**
   - [ ] All 5 entity types can be restored
   - [ ] Individual item selection works
   - [ ] Both GraphQL and REST mutations work
   - [ ] Conflict resolution works
   - [ ] Progress tracking displays
   - [ ] Rollback script generates

2. **TUI Requirements**
   - [ ] Keyboard navigation works (arrows, j/k, Space, Enter, Esc)
   - [ ] Help screen accessible (?)
   - [ ] Filters work (date, status, tags, search)
   - [ ] State transitions smooth

3. **Safety Requirements**
   - [ ] Pre-restore validation works
   - [ ] Dry-run mode works
   - [ ] Preview displays changes
   - [ ] Resume functionality works
   - [ ] Rollback script generates

4. **Quality Requirements**
   - [ ] Unit tests pass (≥80% coverage)
   - [ ] Integration tests pass
   - [ ] Binary builds without warnings
   - [ ] Documentation is complete

---

## Next Steps After This Workflow

1. **Execute Implementation**: Use `/sc:implement` to follow this workflow step by step
2. **Create Tests**: Write comprehensive tests after each phase
3. **Documentation**: Update docs as features are implemented
4. **Release**: Build and release binary after all checkpoints pass

---

## Appendix: File Creation Order

```
1. go.mod
2. constants.go
3. types.go
4. config.go
5. main.go
6. backup/loader.go
7. backup/reader.go
8. backup/validator.go
9. entity/common.go
10. entity/products.go
11. entity/customers.go
12. entity/orders.go
13. entity/collections.go
14. entity/metaobjects.go
15. backup/status_reader.go
16. tui/model.go
17. tui/update.go
18. tui/view.go
19. tui/keys.go
20. tui/style.go
21. tui/init.go
22. tui/views/backup_select.go
23. tui/views/config_screen.go
24. tui/components/filter_bar.go
25. tui/components/status_bar.go
26. credentials/loader.go
27. tui/views/entity_select.go
28. tui/views/item_list.go
29. tui/components/date_range.go
30. restore/executor.go
31. restore/graphql_mutations.go
32. restore/rest_mutations.go
33. restore/conflict_resolver.go
34. restore/image_uploader.go
35. log/logger.go
36. tui/views/progress_screen.go
37. tui/views/conflict_screen.go
38. tui/views/abort_screen.go
39. diff/preview.go
40. tui/views/preview_screen.go
41. restore/rollback_generator.go
42. restore/relation_checker.go
43. restore/state_manager.go
44. tui/components/help_overlay.go
45. tui/components/confirm_dialog.go
46. tui/views/complete_screen.go
47. tui/views/error_screen.go
48. credentials/saver.go
49. README.md
```

**Total Files**: 49 files
**Estimated Lines of Code**: ~4,000-5,000 lines

---

**End of Implementation Workflow**