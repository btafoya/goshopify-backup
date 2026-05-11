package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/btafoya/goshopify-backup/jsonl"
	"github.com/btafoya/goshopify-backup/shopify"
)

// GraphQL query constant for URL redirects
const (
	urlRedirectsQuery = `{
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
)

// Data types for URL redirects

// URLRedirect represents a URL redirect
type URLRedirect struct {
	ID     string `json:"id"`
	Path   string `json:"path"`
	Target string `json:"target"`
}

// RESTRedirect represents the REST API format for redirects
type RESTRedirect struct {
	ID     int64  `json:"id"`
	Path   string `json:"path"`
	Target string `json:"target"`
}

// RedirectsModule backs up URL redirects
type RedirectsModule struct {
	name string
}

// NewRedirectsModule creates a new redirects backup module
func NewRedirectsModule() *RedirectsModule {
	return &RedirectsModule{
		name: "redirects",
	}
}

// Name returns the module name
func (m *RedirectsModule) Name() string {
	return m.name
}

// Run executes the redirects backup using bulk operations
func (m *RedirectsModule) Run(ctx context.Context, graphqlClient *shopify.GraphQLClient, outputDir string) (int, int64, error) {
	// Submit bulk operation
	_, err := graphqlClient.SubmitBulkOperation(ctx, urlRedirectsQuery)
	if err != nil {
		if isAccessDenied(err) {
			return 0, 0, &shopify.AccessDeniedError{Message: "Bulk operation access denied"}
		}
		return 0, 0, fmt.Errorf("failed to submit bulk operation: %w", err)
	}

	// Poll for completion
	downloadURL, err := graphqlClient.PollBulkOperation(ctx, 1*time.Second, 10*time.Minute)
	if err != nil {
		return 0, 0, fmt.Errorf("bulk operation polling failed: %w", err)
	}

	// Check if no data
	if downloadURL == "no-data" {
		// Write empty array
		emptyData := []byte("[]")
		outputPath := outputDir + "/url-redirects.json"
		if err := writeFile(outputPath, emptyData); err != nil {
			return 0, int64(2), fmt.Errorf("failed to write url-redirects.json: %w", err)
		}
		return 0, int64(2), nil
	}

	// Download JSONL
	jsonlReader, err := graphqlClient.DownloadJSONL(ctx, downloadURL)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to download JSONL: %w", err)
	}
	defer jsonlReader.Close()

	// Parse JSONL
	redirects := make([]URLRedirect, 0)
	parser := jsonl.NewParser(jsonlReader)

	for parser.Scan() {
		var raw map[string]interface{}
		if err := parser.Decode(&raw); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to decode redirect entry: %v\n", err)
			continue
		}

		redirect, ok := parseURLRedirect(raw)
		if !ok {
			continue
		}

		redirects = append(redirects, redirect)
	}

	if err := parser.Err(); err != nil {
		return 0, 0, fmt.Errorf("JSONL parse error: %w", err)
	}

	// Write to JSON file
	jsonData, err := json.MarshalIndent(redirects, "", "  ")
	if err != nil {
		return len(redirects), 0, fmt.Errorf("failed to marshal redirects: %w", err)
	}

	outputPath := outputDir + "/url-redirects.json"
	if err := writeFile(outputPath, jsonData); err != nil {
		return len(redirects), 0, fmt.Errorf("failed to write url-redirects.json: %w", err)
	}

	return len(redirects), int64(len(jsonData)), nil
}

// RunREST executes REST API fallback for URL redirects
func (m *RedirectsModule) RunREST(ctx context.Context, restClient *shopify.RESTClient, outputDir string) (int, int64, error) {
	var result struct {
		Redirects []RESTRedirect `json:"redirects"`
	}

	_, err := restClient.GetPages(ctx, "/redirects.json", &result)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to fetch redirects: %w", err)
	}

	// Convert to URLRedirect format with GID
	redirects := make([]URLRedirect, len(result.Redirects))
	for i, restRedirect := range result.Redirects {
		redirects[i] = URLRedirect{
			ID:     convertRESTIDToGID(restRedirect.ID),
			Path:   restRedirect.Path,
			Target: restRedirect.Target,
		}
	}

	// Write to JSON file
	jsonData, err := json.MarshalIndent(redirects, "", "  ")
	if err != nil {
		return len(redirects), 0, fmt.Errorf("failed to marshal redirects: %w", err)
	}

	outputPath := outputDir + "/url-redirects.json"
	if err := writeFile(outputPath, jsonData); err != nil {
		return len(redirects), 0, fmt.Errorf("failed to write url-redirects.json: %w", err)
	}

	return len(redirects), int64(len(jsonData)), nil
}

// parseURLRedirect converts a raw map to URLRedirect
func parseURLRedirect(raw map[string]interface{}) (URLRedirect, bool) {
	id, ok := raw["id"].(string)
	if !ok {
		return URLRedirect{}, false
	}

	path, ok := raw["path"].(string)
	if !ok {
		return URLRedirect{}, false
	}

	target, ok := raw["target"].(string)
	if !ok {
		return URLRedirect{}, false
	}

	return URLRedirect{
		ID:     id,
		Path:   path,
		Target: target,
	}, true
}

// convertRESTIDToGID converts a REST numeric ID to Shopify GID format
func convertRESTIDToGID(id int64) string {
	return fmt.Sprintf("gid://shopify/UrlRedirect/%d", id)
}

// ParseRedirectID extracts numeric ID from a GID string
func ParseRedirectID(gid string) (int64, error) {
	// Expected format: gid://shopify/UrlRedirect/12345
	parts := strings.Split(gid, "/")
	if len(parts) < 5 {
		return 0, fmt.Errorf("invalid GID format: %s", gid)
	}

	idStr := parts[len(parts)-1]
	return strconv.ParseInt(idStr, 10, 64)
}
