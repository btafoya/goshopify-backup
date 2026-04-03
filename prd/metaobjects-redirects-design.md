# Design Specification: Metaobjects & URL Redirects Backup

## Document Information
- **Version**: 1.0
- **Date**: 2026-04-03
- **Status**: Ready for Implementation

---

## 1. Overview

This document specifies the design for two new backup modules:
1. **MetaobjectsModule** - Backup Shopify metaobject definitions and entries
2. **RedirectsModule** - Backup URL redirects

Both modules follow existing patterns from the codebase (ProductsModule, CustomersModule, ContentModule).

---

## 2. Module Architecture

### 2.1 Module Interface Pattern

All backup modules implement one of two interfaces:

```go
// For modules using GraphQL bulk operations
type BulkModule interface {
    Name() string
    Run(ctx context.Context, graphqlClient *shopify.GraphQLClient, outputDir string) (int, int64, error)
}

// For modules using REST API
type RESTModule interface {
    Name() string
    Run(ctx context.Context, restClient *shopify.RESTClient, outputDir string) (int, int64, error)
}
```

### 2.2 Module Classification

| Module | Primary API | Fallback API | Interface |
|--------|-------------|--------------|-----------|
| MetaobjectsModule | GraphQL Bulk | REST Pagination | BulkModule + RESTFallback |
| RedirectsModule | GraphQL Bulk | REST Pagination | BulkModule + RESTFallback |

---

## 3. MetaobjectsModule Design

### 3.1 Module Structure

```
backup/metaobjects.go
├── MetaobjectsModule (struct)
├── NewMetaobjectsModule() (constructor)
├── Name() (interface method)
├── Run() (main execution)
├── runBulk() (GraphQL bulk path)
├── runREST() (REST fallback path)
├── backupDefinitions() (definition backup)
├── writeByType() (per-type file writing)
└── sanitizeTypeName() (filename sanitization)
```

### 3.2 Data Types

```go
// MetaobjectDefinition represents a metaobject schema definition
type MetaobjectDefinition struct {
    ID              string                `json:"id"`
    Name            string                `json:"name"`
    Type            string                `json:"type"`
    FieldDefinitions []MetaobjectFieldDef  `json:"fieldDefinitions"`
}

type MetaobjectFieldDef struct {
    Key   string        `json:"key"`
    Name  string        `json:"name"`
    Type  FieldType     `json:"type"`
}

type FieldType struct {
    Name      string `json:"name"`
    Namespace string `json:"namespace"`
}

// MetaobjectEntry represents a metaobject entry
type MetaobjectEntry struct {
    ID          string              `json:"id"`
    Handle      string              `json:"handle"`
    DisplayName string              `json:"displayName"`
    Type        string              `json:"type"`
    Fields      []MetaobjectField   `json:"fields"`
    Capabilities *MetaobjectCaps    `json:"capabilities,omitempty"`
    UpdatedAt   string              `json:"updatedAt"`
    CreatedAt   string              `json:"createdAt"`
}

type MetaobjectField struct {
    Key      string      `json:"key"`
    Value    string      `json:"value"`
    Type     string      `json:"type"`
    JSONValue interface{} `json:"jsonValue,omitempty"`
}

type MetaobjectCaps struct {
    Publishable *PublishableStatus `json:"publishable,omitempty"`
}

type PublishableStatus struct {
    Status string `json:"status"`
}
```

### 3.3 GraphQL Bulk Query

```go
const metaobjectsQuery = `{
    metaobjects {
        edges {
            node {
                id
                handle
                displayName
                type
                fields {
                    key
                    value
                    type
                    jsonValue
                }
                capabilities {
                    publishable {
                        status
                    }
                }
                updatedAt
                createdAt
            }
        }
    }
}`
```

### 3.4 Definitions Query

```go
const metaobjectDefinitionsQuery = `{
    metaobjectDefinitions(first: 250) {
        edges {
            node {
                id
                name
                type
                fieldDefinitions {
                    key
                    name
                    type {
                        name
                        namespace
                    }
                }
            }
        }
    }
}`
```

### 3.5 Flow Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                    MetaobjectsModule.Run()                    │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
                   ┌─────────────────────┐
                   │ Create metaobjects/ │
                   │    subdirectory      │
                   └─────────────────────┘
                              │
                              ▼
                   ┌─────────────────────┐
                   │ backupDefinitions() │
                   └─────────────────────┘
                              │
                              ▼
                ┌─────────────────────────────┐
                │ Try runBulk() (GraphQL)      │
                └─────────────────────────────┘
                              │
                    ┌─────────┴─────────┐
                    │                   │
                    ▼                   ▼
              Success          ACCESS_DENIED
                    │                   │
                    │                   ▼
                    │          ┌───────────────┐
                    │          │ runREST()     │
                    │          └───────────────┘
                    │                   │
                    └─────────┬─────────┘
                              │
                              ▼
                   ┌─────────────────────┐
                   │   writeByType()     │
                   │  Group by 'type'    │
                   └─────────────────────┘
                              │
                    ┌─────────┴─────────┐
                    ▼                   ▼
            metaobject-       size_charts.json
            definitions.json  product_highlights.json
                              ...
                              │
                              ▼
                   ┌─────────────────────┐
                   │  Return count, size │
                   └─────────────────────┘
```

### 3.6 REST Fallback Pagination

```go
// REST API doesn't support metaobjects - this is GraphQL-only
// However, we can use pagination on the GraphQL query if bulk is denied
const metaobjectsPaginatedQuery = `query($first: Int!, $after: String) {
    metaobjects(first: $first, after: $after) {
        edges {
            node {
                id
                handle
                displayName
                type
                ...
            }
            cursor
        }
        pageInfo {
            hasNextPage
        }
    }
}`
```

---

## 4. RedirectsModule Design

### 4.1 Module Structure

```
backup/redirects.go
├── RedirectsModule (struct)
├── NewRedirectsModule() (constructor)
├── Name() (interface method)
├── Run() (main execution)
├── runBulk() (GraphQL bulk path)
└── runREST() (REST fallback path)
```

### 4.2 Data Types

```go
// URLRedirect represents a URL redirect
type URLRedirect struct {
    ID     string `json:"id"`
    Path   string `json:"path"`
    Target string `json:"target"`
}

// RESTRedirect represents the REST API format
type RESTRedirect struct {
    ID     int64  `json:"id"`
    Path   string `json:"path"`
    Target string `json:"target"`
}
```

### 4.3 GraphQL Bulk Query

```go
const urlRedirectsQuery = `{
    urlRedirects {
        edges {
            node {
                id
                path
                target
            }
        }
    }
}`
```

### 4.4 Flow Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                    RedirectsModule.Run()                     │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
                ┌─────────────────────────────┐
                │ Try runBulk() (GraphQL)      │
                └─────────────────────────────┘
                              │
                    ┌─────────┴─────────┐
                    │                   │
                    ▼                   ▼
              Success          ACCESS_DENIED
                    │                   │
                    │                   ▼
                    │          ┌───────────────┐
                    │          │ runREST()     │
                    │          └───────────────┘
                    │                   │
                    │                   ▼
                    │          ┌───────────────┐
                    │          │ Convert ID     │
                    │          │ to GID format  │
                    │          └───────────────┘
                    │                   │
                    └─────────┬─────────┘
                              │
                              ▼
                   ┌─────────────────────┐
                   │ Write url-          │
                   │ redirects.json []   │
                   └─────────────────────┘
                              │
                              ▼
                   ┌─────────────────────┐
                   │  Return count, size │
                   └─────────────────────┘
```

### 4.5 REST Pagination

```
GET /admin/api/2025-01/redirects.json?limit=250
GET /admin/api/2025-01/redirects.json?limit=250&since_id={last_id}
```

---

## 5. Output File Structure

### 5.1 Metaobjects Output

```
{BACKUP_DIR}/{YYYY-MM-DD}/
└── metaobjects/
    ├── metaobject-definitions.json
    ├── size_charts.json
    ├── product_highlights.json
    ├── app-product-highlights.json
    ├── shopify-color-pattern.json
    └── ...
```

### 5.2 Redirects Output

```
{BACKUP_DIR}/{YYYY-MM-DD}/
└── url-redirects.json
```

### 5.3 Filename Sanitization Rules

| Input Handle | Sanitized Filename |
|--------------|-------------------|
| `size_chart` | `size_chart.json` |
| `$app:product_highlight` | `app-product-highlight.json` |
| `shopify--color-pattern` | `shopify-color-pattern.json` |

```go
func sanitizeTypeName(typeName string) string {
    // Remove $ prefix
    typeName = strings.TrimPrefix(typeName, "$")
    // Replace : with -
    typeName = strings.ReplaceAll(typeName, ":", "-")
    // Replace -- with -
    for strings.Contains(typeName, "--") {
        typeName = strings.ReplaceAll(typeName, "--", "-")
    }
    return typeName + ".json"
}
```

---

## 6. Module Execution Order

```go
var expectedModules = []string{
    "products",
    "customers",
    "orders",
    "collections",
    "content",
    "metaobjects",
    "redirects",
}
```

**Rationale:**
- Products, Customers, Orders: Core transactional data (highest priority)
- Collections: Product-related but less critical
- Content: Store pages/blogs (important but not transactional)
- Metaobjects: Custom data structures (lower priority)
- Redirects: Navigation configuration (lowest priority)

---

## 7. Error Handling Strategy

### 7.1 MetaobjectsModule

| Error Type | Action | Fallback |
|------------|--------|----------|
| ACCESS_DENIED (bulk) | Log warning | Try REST pagination |
| Empty metaobjects | Write `[]` to each file | Continue |
| Directory creation fail | Return error | Fail module |
| JSONL parse fail | Return error | Fail module |
| REST pagination fail | Return error | Fail module |

### 7.2 RedirectsModule

| Error Type | Action | Fallback |
|------------|--------|----------|
| ACCESS_DENIED (bulk) | Log warning | Try REST API |
| Empty redirects | Write `[]` to file | Continue |
| REST pagination fail | Return error | Fail module |
| ID conversion fail | Return error | Fail module |

---

## 8. Integration Points

### 8.1 Main.go Updates

```go
// Add modules to expectedModules
var expectedModules = []string{
    "products", "customers", "orders", "collections",
    "content", "metaobjects", "redirects",
}

// Add switch cases in runModules()
switch moduleName {
case "metaobjects":
    metaobjectsMod := backup.NewMetaobjectsModule()
    count, fileSize, fallback, err = runMetaobjectsWithFallback(
        ctx, graphqlClient, restClient, metaobjectsMod, outputDir)

case "redirects":
    redirectsMod := backup.NewRedirectsModule()
    count, fileSize, fallback, err = runRedirectsWithFallback(
        ctx, graphqlClient, restClient, redirectsMod, outputDir)
}
```

### 8.2 Fallback Functions

```go
func runMetaobjectsWithFallback(
    ctx context.Context,
    graphqlClient *shopify.GraphQLClient,
    restClient *shopify.RESTClient,
    mod *backup.MetaobjectsModule,
    outputDir string,
) (int, int64, string, error) {
    count, size, err := mod.Run(ctx, graphqlClient, outputDir)
    if err != nil && isAccessDenied(err) {
        fmt.Printf("[metaobjects] Bulk operation access denied, falling back to pagination\n")
        count, size, err = mod.RunREST(ctx, graphqlClient, outputDir)
        if err != nil {
            return 0, 0, "", err
        }
        return count, size, "PAGINATION", nil
    }
    return count, size, "", err
}

func runRedirectsWithFallback(
    ctx context.Context,
    graphqlClient *shopify.GraphQLClient,
    restClient *shopify.RESTClient,
    mod *backup.RedirectsModule,
    outputDir string,
) (int, int64, string, error) {
    count, size, err := mod.Run(ctx, graphqlClient, outputDir)
    if err != nil && isAccessDenied(err) {
        fmt.Printf("[redirects] Bulk operation access denied, falling back to REST\n")
        count, size, err = mod.RunREST(ctx, restClient, outputDir)
        if err != nil {
            return 0, 0, "", err
        }
        return count, size, "REST", nil
    }
    return count, size, "", err
}
```

---

## 9. Testing Strategy

### 9.1 Unit Tests

```go
// backup/metaobjects_test.go
func TestSanitizeTypeName(t *testing.T)
func TestMetaobjectsModule_Run_Bulk(t *testing.T)
func TestMetaobjectsModule_Run_Empty(t *testing.T)
func TestWriteByType_Grouping(t *testing.T)

// backup/redirects_test.go
func TestRedirectsModule_Run_Bulk(t *testing.T)
func TestRedirectsModule_Run_Empty(t *testing.T)
func TestRedirectsModule_Run_REST(t *testing.T)
func TestConvertRESTIDToGID(t *testing.T)
```

### 9.2 Integration Tests

- Mock Shopify GraphQL client responses
- Mock Shopify REST client responses
- Test file creation and structure
- Test error handling paths

---

## 10. Dependencies

### 10.1 New Imports Required

```go
import (
    // Existing
    "context"
    "encoding/json"
    "fmt"
    "os"
    "strings"

    // From project
    "github.com/btafoya/goshopify-backup/jsonl"
    "github.com/btafoya/goshopify-backup/shopify"
)
```

### 10.2 No New External Dependencies

All functionality uses existing project packages.

---

## 11. Implementation Checklist

### MetaobjectsModule
- [ ] Create `backup/metaobjects.go`
- [ ] Define data types
- [ ] Implement `NewMetaobjectsModule()`
- [ ] Implement `Name()`
- [ ] Implement `Run()` with bulk support
- [ ] Implement `runBulk()`
- [ ] Implement `runREST()` (GraphQL pagination fallback)
- [ ] Implement `backupDefinitions()`
- [ ] Implement `writeByType()`
- [ ] Implement `sanitizeTypeName()`
- [ ] Create `backup/metaobjects_test.go`
- [ ] Update `main.go` expectedModules
- [ ] Add fallback function in `main.go`
- [ ] Add switch case in `main.go` runModules()

### RedirectsModule
- [ ] Create `backup/redirects.go`
- [ ] Define data types
- [ ] Implement `NewRedirectsModule()`
- [ ] Implement `Name()`
- [ ] Implement `Run()` with bulk support
- [ ] Implement `runBulk()`
- [ ] Implement `runREST()`
- [ ] Implement `convertRESTIDToGID()`
- [ ] Create `backup/redirects_test.go`
- [ ] Update `main.go` expectedModules
- [ ] Add fallback function in `main.go`
- [ ] Add switch case in `main.go` runModules()

---

## 12. Future Considerations

### 12.1 Restore Tool Compatibility

The JSON array format is designed for easy parsing by a future `goshopify-restore` CLI:

- Metaobject definitions can be recreated using `metaobjectDefinitionCreate` mutation
- Metaobject entries can be recreated using `metaobjectCreate` mutation
- Redirects can be recreated using `urlRedirectCreate` mutation or REST API

### 12.2 Delta Backup

Future enhancement to track changes and only backup modified entries:
- Use `updatedAt` field comparison
- Store last backup timestamp in status.json
- Query with `query: "updated_at:>last_backup_time"`

### 12.3 Reference Expansion

Currently, metaobject fields that reference other resources store only the GID. Future enhancement:
- Optionally expand references to include referenced object data
- Configurable via flag: `--expand-references`

---

## Appendix A: GraphQL Queries Reference

### A.1 Metaobjects Bulk Query

```graphql
mutation {
    bulkOperationRunQuery(
        query: """
        {
            metaobjects {
                edges {
                    node {
                        id
                        handle
                        displayName
                        type
                        fields {
                            key
                            value
                            type
                            jsonValue
                        }
                        capabilities {
                            publishable {
                                status
                            }
                        }
                        updatedAt
                        createdAt
                    }
                }
            }
        }
        """
    ) {
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

### A.2 URL Redirects Bulk Query

```graphql
mutation {
    bulkOperationRunQuery(
        query: """
        {
            urlRedirects {
                edges {
                    node {
                        id
                        path
                        target
                    }
                }
            }
        }
        """
    ) {
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

### A.3 Metaobject Definitions Query

```graphql
query {
    metaobjectDefinitions(first: 250) {
        edges {
            node {
                id
                name
                type
                fieldDefinitions {
                    key
                    name
                    type {
                        name
                        namespace
                    }
                }
            }
        }
    }
}
```

---

## Appendix B: File Format Examples

### B.1 metaobject-definitions.json

```json
[
    {
        "id": "gid://shopify/MetaobjectDefinition/123456789",
        "name": "Size Chart",
        "type": "size_chart",
        "fieldDefinitions": [
            {
                "key": "title",
                "name": "Title",
                "type": {
                    "name": "single_line_text_field",
                    "namespace": "shopify"
                }
            },
            {
                "key": "dimensions",
                "name": "Dimensions",
                "type": {
                    "name": "json",
                    "namespace": "shopify"
                }
            }
        ]
    }
]
```

### B.2 size_chart.json

```json
[
    {
        "id": "gid://shopify/Metaobject/987654321",
        "handle": "size-chart-m",
        "displayName": "Size Chart M",
        "type": "size_chart",
        "fields": [
            {
                "key": "title",
                "value": "Medium Size Chart",
                "type": "single_line_text_field"
            },
            {
                "key": "dimensions",
                "value": "{\"width\":20,\"length\":30}",
                "type": "json",
                "jsonValue": {
                    "width": 20,
                    "length": 30
                }
            }
        ],
        "capabilities": {
            "publishable": {
                "status": "ACTIVE"
            }
        },
        "updatedAt": "2026-04-03T00:00:00Z",
        "createdAt": "2026-01-01T00:00:00Z"
    }
]
```

### B.3 url-redirects.json

```json
[
    {
        "id": "gid://shopify/UrlRedirect/123456789",
        "path": "/old-product-page",
        "target": "/products/new-product"
    },
    {
        "id": "gid://shopify/UrlRedirect/987654321",
        "path": "/blog/legacy-post",
        "target": "/blogs/news/updated-post"
    }
]
```