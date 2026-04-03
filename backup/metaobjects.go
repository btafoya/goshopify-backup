package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/btafoya/goshopify-backup/jsonl"
	"github.com/btafoya/goshopify-backup/shopify"
)

// GraphQL query constants for metaobjects
const (
	metaobjectsQuery = `{
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

	metaobjectDefinitionsQuery = `{
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
							}
						}
					}
				}
			}
		}`
)

// Data types for metaobjects

// MetaobjectDefinition represents a metaobject schema definition
type MetaobjectDefinition struct {
	ID               string               `json:"id"`
	Name             string               `json:"name"`
	Type             string               `json:"type"`
	FieldDefinitions []MetaobjectFieldDef `json:"fieldDefinitions"`
}

// MetaobjectFieldDef represents a field definition in a metaobject
type MetaobjectFieldDef struct {
	Key  string    `json:"key"`
	Name string    `json:"name"`
	Type FieldType `json:"type"`
}

// FieldType represents the type of a metaobject field
type FieldType struct {
	Name string `json:"name"`
}

// MetaobjectEntry represents a metaobject entry
type MetaobjectEntry struct {
	ID           string            `json:"id"`
	Handle       string            `json:"handle"`
	DisplayName  string            `json:"displayName"`
	Type         string            `json:"type"`
	Fields       []MetaobjectField `json:"fields"`
	Capabilities *MetaobjectCaps   `json:"capabilities,omitempty"`
	UpdatedAt    string            `json:"updatedAt"`
}

// MetaobjectField represents a field in a metaobject entry
type MetaobjectField struct {
	Key       string      `json:"key"`
	Value     string      `json:"value"`
	Type      string      `json:"type"`
	JSONValue interface{} `json:"jsonValue,omitempty"`
}

// MetaobjectCaps represents capabilities of a metaobject
type MetaobjectCaps struct {
	Publishable *PublishableStatus `json:"publishable,omitempty"`
}

// PublishableStatus represents publishable capability status
type PublishableStatus struct {
	Status string `json:"status"`
}

// MetaobjectsModule backs up metaobject definitions and entries
type MetaobjectsModule struct {
	name string
}

// NewMetaobjectsModule creates a new metaobjects backup module
func NewMetaobjectsModule() *MetaobjectsModule {
	return &MetaobjectsModule{
		name: "metaobjects",
	}
}

// Name returns the module name
func (m *MetaobjectsModule) Name() string {
	return m.name
}

// Run executes the metaobjects backup using pagination (bulk requires type argument)
func (m *MetaobjectsModule) Run(ctx context.Context, graphqlClient *shopify.GraphQLClient, outputDir string) (int, int64, error) {
	// Create metaobjects subdirectory
	metaobjectsDir := filepath.Join(outputDir, "metaobjects")
	if err := os.MkdirAll(metaobjectsDir, 0755); err != nil {
		return 0, 0, fmt.Errorf("failed to create metaobjects directory: %w", err)
	}

	totalCount := 0
	totalSize := int64(0)

	// Backup metaobject definitions and get types
	defsCount, defsSize, types, err := m.backupDefinitions(ctx, graphqlClient, metaobjectsDir)
	if err != nil {
		return totalCount, totalSize, fmt.Errorf("failed to backup metaobject definitions: %w", err)
	}
	totalCount += defsCount
	totalSize += defsSize

	// Backup metaobject entries by type
	if len(types) > 0 {
		entriesCount, entriesSize, err := m.RunREST(ctx, graphqlClient, types, metaobjectsDir)
		if err != nil {
			return totalCount, totalSize, fmt.Errorf("failed to backup metaobject entries: %w", err)
		}
		totalCount += entriesCount
		totalSize += entriesSize
	}

	return totalCount, totalSize, nil
}

// runBulk executes bulk operation for metaobject entries
func (m *MetaobjectsModule) runBulk(ctx context.Context, graphqlClient *shopify.GraphQLClient, outputDir string) (int, int64, error) {
	// Submit bulk operation
	_, err := graphqlClient.SubmitBulkOperation(ctx, metaobjectsQuery)
	if err != nil {
		if isAccessDenied(err) {
			return 0, 0, &shopify.AccessDeniedError{Message: "Bulk operation access denied"}
		}
		return 0, 0, fmt.Errorf("failed to submit bulk operation: %w", err)
	}

	// Poll for completion
	downloadURL, err := graphqlClient.PollBulkOperation(ctx, 1, 600) // 10 minutes
	if err != nil {
		return 0, 0, fmt.Errorf("bulk operation polling failed: %w", err)
	}

	// Check if no data
	if downloadURL == "no-data" {
		// Write empty definitions file already handled, just return
		return 0, 0, nil
	}

	// Download JSONL
	jsonlReader, err := graphqlClient.DownloadJSONL(ctx, downloadURL)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to download JSONL: %w", err)
	}
	defer jsonlReader.Close()

	// Parse JSONL and group by type
	entriesByType := make(map[string][]MetaobjectEntry)
	parser := jsonl.NewParser(jsonlReader)
	entryCount := 0

	for parser.Scan() {
		var entry map[string]interface{}
		if err := parser.Decode(&entry); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to decode metaobject entry: %v\n", err)
			continue
		}

		// Convert to MetaobjectEntry
		metaobject, ok := parseMetaobjectEntry(entry)
		if !ok {
			continue
		}

		entriesByType[metaobject.Type] = append(entriesByType[metaobject.Type], metaobject)
		entryCount++
	}

	if err := parser.Err(); err != nil {
		return 0, 0, fmt.Errorf("JSONL parse error: %w", err)
	}

	// Write each type to its own file
	totalSize := int64(0)
	for typeName, entries := range entriesByType {
		jsonData, err := json.MarshalIndent(entries, "", "  ")
		if err != nil {
			return entryCount, totalSize, fmt.Errorf("failed to marshal entries for type %s: %w", typeName, err)
		}

		filename := sanitizeTypeName(typeName)
		outputPath := filepath.Join(outputDir, filename)
		if err := writeFile(outputPath, jsonData); err != nil {
			return entryCount, totalSize, fmt.Errorf("failed to write %s: %w", filename, err)
		}

		totalSize += int64(len(jsonData))
	}

	// If no entries, still create a marker
	if entryCount == 0 {
		fmt.Fprintf(os.Stderr, "Note: No metaobject entries found\n")
	}

	return entryCount, totalSize, nil
}

// RunREST executes GraphQL pagination for each metaobject type
func (m *MetaobjectsModule) RunREST(ctx context.Context, graphqlClient *shopify.GraphQLClient, types []string, outputDir string) (int, int64, error) {
	entriesByType := make(map[string][]MetaobjectEntry)
	entryCount := 0

	// Query each metaobject type separately
	for _, metaobjectType := range types {
		after := ""

		for {
			// Build query with type argument
			// Note: metaobjects returns nodes directly, not edges { node { ... } }
			queryTemplate := `query($first: Int!, $after: String, $type: String!) {
					metaobjects(type: $type, first: $first, after: $after) {
						nodes {
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
						}
						pageInfo {
							hasNextPage
							endCursor
						}
					}
				}`

			// Minify query: replace newlines with spaces, then tabs, then collapse double spaces
			queryStr := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(queryTemplate, "\n", " "), "\t", ""), "  ", " ")

			// Build query payload - handle null cursor for first page
			var queryWithVars string
			if after == "" {
				// First page - don't include after variable (use null)
				queryWithVars = fmt.Sprintf(`{"query":"%s","variables":{"first":250,"type":%q}}`,
					queryStr, metaobjectType)
			} else {
				// Subsequent pages - include cursor
				queryWithVars = fmt.Sprintf(`{"query":"%s","variables":{"first":250,"after":%q,"type":%q}}`,
					queryStr, after, metaobjectType)
			}

			var result struct {
				Data struct {
					Metaobjects struct {
						Nodes []map[string]interface{} `json:"nodes"`
						PageInfo struct {
							HasNextPage bool   `json:"hasNextPage"`
							EndCursor   string `json:"endCursor"`
						} `json:"pageInfo"`
					} `json:"metaobjects"`
				} `json:"data"`
			}

			data, err := graphqlClient.Query(ctx, queryWithVars)
			if err != nil {
				return entryCount, 0, fmt.Errorf("failed to execute paginated query for type %s: %w", metaobjectType, err)
			}

			if err := json.Unmarshal(data, &result); err != nil {
				return entryCount, 0, fmt.Errorf("failed to unmarshal paginated response for type %s: %w", metaobjectType, err)
			}

			// Process entries for this type
			for _, node := range result.Data.Metaobjects.Nodes {
				metaobject, ok := parseMetaobjectEntry(node)
				if !ok {
					continue
				}
				entriesByType[metaobject.Type] = append(entriesByType[metaobject.Type], metaobject)
				entryCount++
			}

			// Check if more pages
			if !result.Data.Metaobjects.PageInfo.HasNextPage || len(result.Data.Metaobjects.Nodes) == 0 {
				break
			}

			// Use endCursor from pageInfo for next page
			after = result.Data.Metaobjects.PageInfo.EndCursor
		}
	}

	// Write each type to its own file
	totalSize := int64(0)
	for typeName, entries := range entriesByType {
		jsonData, err := json.MarshalIndent(entries, "", "  ")
		if err != nil {
			return entryCount, totalSize, fmt.Errorf("failed to marshal entries for type %s: %w", typeName, err)
		}

		filename := sanitizeTypeName(typeName)
		outputPath := filepath.Join(outputDir, filename)
		if err := writeFile(outputPath, jsonData); err != nil {
			return entryCount, totalSize, fmt.Errorf("failed to write %s: %w", filename, err)
		}

		totalSize += int64(len(jsonData))
	}

	// Log summary
	if len(types) > 0 {
		fmt.Printf("[metaobjects] Backed up %d entries across %d types\n", entryCount, len(types))
	}

	return entryCount, totalSize, nil
}

// backupDefinitions fetches and backs up metaobject definitions
// Returns the list of metaobject types for entry backup
func (m *MetaobjectsModule) backupDefinitions(ctx context.Context, graphqlClient *shopify.GraphQLClient, outputDir string) (int, int64, []string, error) {
	// Minify query: replace newlines with spaces, then tabs, then collapse double spaces
	minifiedQuery := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(metaobjectDefinitionsQuery, "\n", " "), "\t", ""), "  ", " ")
	data, err := graphqlClient.Query(ctx, fmt.Sprintf(`{"query":"%s"}`, minifiedQuery))
	if err != nil {
		return 0, 0, nil, fmt.Errorf("failed to query metaobject definitions: %w", err)
	}

	var result struct {
		Data struct {
			MetaobjectDefinitions struct {
				Edges []struct {
					Node MetaobjectDefinition `json:"node"`
				} `json:"edges"`
			} `json:"metaobjectDefinitions"`
		} `json:"data"`
	}

	if err := json.Unmarshal(data, &result); err != nil {
		return 0, 0, nil, fmt.Errorf("failed to unmarshal definitions: %w", err)
	}

	definitions := make([]MetaobjectDefinition, len(result.Data.MetaobjectDefinitions.Edges))
	types := make([]string, 0, len(result.Data.MetaobjectDefinitions.Edges))
	for i, edge := range result.Data.MetaobjectDefinitions.Edges {
		definitions[i] = edge.Node
		types = append(types, edge.Node.Type)
	}

	jsonData, err := json.MarshalIndent(definitions, "", "  ")
	if err != nil {
		return len(definitions), 0, types, fmt.Errorf("failed to marshal definitions: %w", err)
	}

	outputPath := filepath.Join(outputDir, "metaobject-definitions.json")
	if err := writeFile(outputPath, jsonData); err != nil {
		return len(definitions), 0, types, fmt.Errorf("failed to write definitions: %w", err)
	}

	return len(definitions), int64(len(jsonData)), types, nil
}

// parseMetaobjectEntry converts a raw map to MetaobjectEntry
func parseMetaobjectEntry(raw map[string]interface{}) (MetaobjectEntry, bool) {
	id, ok := raw["id"].(string)
	if !ok {
		return MetaobjectEntry{}, false
	}

	handle, _ := raw["handle"].(string)
	displayName, _ := raw["displayName"].(string)
	mtype, ok := raw["type"].(string)
	if !ok {
		return MetaobjectEntry{}, false
	}
	updatedAt, _ := raw["updatedAt"].(string)

	entry := MetaobjectEntry{
		ID:          id,
		Handle:      handle,
		DisplayName: displayName,
		Type:        mtype,
		UpdatedAt:   updatedAt,
	}

	// Parse fields
	if fieldsRaw, ok := raw["fields"].([]interface{}); ok {
		for _, f := range fieldsRaw {
			if fieldMap, ok := f.(map[string]interface{}); ok {
				field := MetaobjectField{
					Key:   getString(fieldMap, "key"),
					Value: getString(fieldMap, "value"),
					Type:  getString(fieldMap, "type"),
				}
				if jsonVal, ok := fieldMap["jsonValue"]; ok && jsonVal != nil {
					field.JSONValue = jsonVal
				}
				entry.Fields = append(entry.Fields, field)
			}
		}
	}

	// Parse capabilities
	if capsRaw, ok := raw["capabilities"].(map[string]interface{}); ok {
		if pubRaw, ok := capsRaw["publishable"].(map[string]interface{}); ok {
			entry.Capabilities = &MetaobjectCaps{
				Publishable: &PublishableStatus{
					Status: getString(pubRaw, "status"),
				},
			}
		}
	}

	return entry, true
}

// getString safely gets a string from a map
func getString(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if s, ok := val.(string); ok {
			return s
		}
	}
	return ""
}

// sanitizeTypeName converts a metaobject type to a safe filename
func sanitizeTypeName(typeName string) string {
	// Remove $ prefix
	typeName = strings.TrimPrefix(typeName, "$")
	// Replace : with -
	typeName = strings.ReplaceAll(typeName, ":", "-")
	// Replace -- with - (repeat until no more)
	for strings.Contains(typeName, "--") {
		typeName = strings.ReplaceAll(typeName, "--", "-")
	}
	return typeName + ".json"
}