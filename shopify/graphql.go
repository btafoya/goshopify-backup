package shopify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/btafoya/goshopify-backup/pkg/auth"
)

// GraphQL response types
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

type CurrentBulkOperationResponse struct {
	Data struct {
		CurrentBulkOperation *BulkOperation `json:"currentBulkOperation"`
	} `json:"data"`
	Errors []GraphQLError `json:"errors,omitempty"`
}

type BulkOperation struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	CompletedAt string `json:"completedAt"`
	ErrorCode   string `json:"errorCode"`
	FileSize    string `json:"fileSize"`    // Changed from int64 to string for API compatibility
	ObjectCount string `json:"objectCount"` // Changed from int64 to string for API compatibility
	URL         string `json:"url"`
}

type UserError struct {
	Field   []string `json:"field"`
	Message string   `json:"message"`
}

// GraphQLError represents a top-level error in a GraphQL response.
// Shopify returns these for throttling/cost errors, access denied, and
// query validation failures. They are distinct from userErrors which
// appear inside mutation data.
type GraphQLError struct {
	Message   string `json:"message"`
	Locations []struct {
		Line   int `json:"line"`
		Column int `json:"column"`
	} `json:"locations"`
	Path []interface{} `json:"path,omitempty"`
}

// Error implements the error interface for GraphQLError
func (e GraphQLError) Error() string {
	if len(e.Path) > 0 {
		return fmt.Sprintf("graphql error: %s (path: %v)", e.Message, e.Path)
	}
	return fmt.Sprintf("graphql error: %s", e.Message)
}

// GraphQLResponse is a minimal envelope for checking top-level errors
// in a raw GraphQL response before callers unmarshal the data field
// into their own structs.
type GraphQLResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []GraphQLError  `json:"errors,omitempty"`
}

// GraphQLErrorGroup wraps multiple GraphQL errors into a single error.
type GraphQLErrorGroup struct {
	Errors []GraphQLError
}

// Error implements the error interface for GraphQLErrorGroup
func (g *GraphQLErrorGroup) Error() string {
	if len(g.Errors) == 1 {
		return g.Errors[0].Error()
	}
	msgs := make([]string, len(g.Errors))
	for i, e := range g.Errors {
		msgs[i] = e.Error()
	}
	return fmt.Sprintf("graphql errors (%d): %s", len(g.Errors), strings.Join(msgs, "; "))
}

// BulkOperationStatus represents the status of a bulk operation
type BulkOperationStatus string

const (
	StatusCreated   BulkOperationStatus = "CREATED"
	StatusRunning   BulkOperationStatus = "RUNNING"
	StatusCompleted BulkOperationStatus = "COMPLETED"
	StatusFailed    BulkOperationStatus = "FAILED"
	StatusCanceled  BulkOperationStatus = "CANCELED"
)

// AccessDeniedError indicates GraphQL bulk operation access was denied
type AccessDeniedError struct {
	Message string
}

func (e *AccessDeniedError) Error() string {
	return "ACCESS_DENIED: " + e.Message
}

// GraphQLClient wraps hasura/go-graphql-client for Shopify GraphQL API
type GraphQLClient struct {
	store       string
	accessToken string
	apiVersion  string
	limiter     *RateLimiter
	client      *http.Client
	auth        *auth.Authenticator
}

// NewGraphQLClient creates a new Shopify GraphQL client
func NewGraphQLClient(cfg *Config) *GraphQLClient {
	return &GraphQLClient{
		store:       cfg.Store,
		accessToken: cfg.AccessToken,
		apiVersion:  cfg.APIVersion,
		limiter:     cfg.Limiter,
		auth:        cfg.Authenticator,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// token returns the current access token, refreshing via the Authenticator
// when configured.
func (c *GraphQLClient) token(ctx context.Context) (string, error) {
	if c.auth != nil {
		return c.auth.EnsureToken(ctx)
	}
	return c.accessToken, nil
}

// endpoint returns the GraphQL API endpoint URL
func (c *GraphQLClient) endpoint() string {
	return fmt.Sprintf("%s/admin/api/%s/graphql.json", c.store, c.apiVersion)
}

// SubmitBulkOperation submits a bulk operation and returns the operation ID
// Uses bulkOperationRunQuery mutation
func (c *GraphQLClient) SubmitBulkOperation(ctx context.Context, query string) (string, error) {
	c.limiter.Wait()

	mutation := fmt.Sprintf(`mutation bulkOperationRunQuery($query: String!, $groupObjects: Boolean!) {
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
	}`)

	variables := map[string]interface{}{
		"query":        query,
		"groupObjects": false,
	}

	body := map[string]interface{}{
		"query":     mutation,
		"variables": variables,
	}

	reqBody, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.endpoint(), strings.NewReader(string(reqBody)))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	token, err := c.token(ctx)
	if err != nil {
		return "", fmt.Errorf("auth: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Shopify-Access-Token", token)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
			return "", &AccessDeniedError{Message: fmt.Sprintf("status %d: %s", resp.StatusCode, string(body))}
		}
		return "", fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data struct {
			BulkOperationRunQuery struct {
				BulkOperation struct {
					ID     string `json:"id"`
					Status string `json:"status"`
				} `json:"bulkOperation"`
				UserErrors []struct {
					Field   []string `json:"field"`
					Message string   `json:"message"`
				} `json:"userErrors"`
			} `json:"bulkOperationRunQuery"`
		} `json:"data"`
		Errors []GraphQLError `json:"errors,omitempty"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if len(result.Errors) > 0 {
		return "", &GraphQLErrorGroup{Errors: result.Errors}
	}

	if len(result.Data.BulkOperationRunQuery.UserErrors) > 0 {
		errMsg := result.Data.BulkOperationRunQuery.UserErrors[0].Message
		return "", fmt.Errorf("bulk operation error: %s", errMsg)
	}

	return result.Data.BulkOperationRunQuery.BulkOperation.ID, nil
}

// PollBulkOperation polls currentBulkOperation until COMPLETED, FAILED, or timeout
// Returns the result URL for JSONL download
func (c *GraphQLClient) PollBulkOperation(ctx context.Context, pollInterval, timeout time.Duration) (string, error) {
	query := `query {
		currentBulkOperation {
			id
			status
			completedAt
			errorCode
			fileSize
			objectCount
			url
		}
	}`

	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			return "", fmt.Errorf("polling timeout after %v", timeout)
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		c.limiter.Wait()

		body := map[string]interface{}{"query": query}
		reqBody, err := json.Marshal(body)
		if err != nil {
			return "", fmt.Errorf("failed to marshal request: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, "POST", c.endpoint(), strings.NewReader(string(reqBody)))
		if err != nil {
			return "", fmt.Errorf("failed to create request: %w", err)
		}

		token, err := c.token(ctx)
		if err != nil {
			return "", fmt.Errorf("auth: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Shopify-Access-Token", token)

		resp, err := c.client.Do(req)
		if err != nil {
			return "", fmt.Errorf("request failed: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
		}

		var result CurrentBulkOperationResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			return "", fmt.Errorf("failed to decode response: %w", err)
		}
		resp.Body.Close()

		if len(result.Errors) > 0 {
			return "", &GraphQLErrorGroup{Errors: result.Errors}
		}

		op := result.Data.CurrentBulkOperation
		if op == nil {
			return "", fmt.Errorf("no current bulk operation")
		}

		status := BulkOperationStatus(op.Status)
		switch status {
		case StatusCompleted:
			if op.URL == "" {
				// Operation completed but no data - return empty result
				return "no-data", nil
			}
			return op.URL, nil
		case StatusFailed:
			return "", fmt.Errorf("bulk operation failed: %s", op.ErrorCode)
		case StatusCanceled:
			return "", fmt.Errorf("bulk operation canceled")
		case StatusCreated, StatusRunning:
			time.Sleep(pollInterval)
		default:
			return "", fmt.Errorf("unknown bulk operation status: %s", op.Status)
		}
	}
}

// DownloadJSONL downloads the JSONL file from the bulk operation result URL
func (c *GraphQLClient) DownloadJSONL(ctx context.Context, url string) (io.ReadCloser, error) {
	c.limiter.Wait()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	return resp.Body, nil
}

// Query executes a generic GraphQL query with the provided JSON payload
// Used for non-bulk operations like metaobject definitions
func (c *GraphQLClient) Query(ctx context.Context, payload string) ([]byte, error) {
	c.limiter.Wait()

	req, err := http.NewRequestWithContext(ctx, "POST", c.endpoint(), strings.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	token, err := c.token(ctx)
	if err != nil {
		return nil, fmt.Errorf("auth: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Shopify-Access-Token", token)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check for GraphQL-level errors (Shopify returns HTTP 200 with errors)
	var gqlResp GraphQLResponse
	if err := json.Unmarshal(body, &gqlResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal graphql response: %w", err)
	}

	if len(gqlResp.Errors) > 0 {
		return body, &GraphQLErrorGroup{Errors: gqlResp.Errors}
	}

	return body, nil
}
