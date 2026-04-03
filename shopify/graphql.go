package shopify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
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
}

// NewGraphQLClient creates a new Shopify GraphQL client
func NewGraphQLClient(cfg *Config) *GraphQLClient {
	return &GraphQLClient{
		store:       cfg.Store,
		accessToken: cfg.AccessToken,
		apiVersion:  cfg.APIVersion,
		limiter:     cfg.Limiter,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
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

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Shopify-Access-Token", c.accessToken)

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
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
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

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Shopify-Access-Token", c.accessToken)

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