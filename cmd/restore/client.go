package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/charmbracelet/log"
)

// ShopifyClient handles API communication with Shopify
type ShopifyClient struct {
	StoreURL     string
	AccessToken  string
	APIVersion   string
	HTTPClient   *http.Client
	RateLimiter  *RateLimiter
	logger       *log.Logger
	DryRun       bool
}

// RateLimiter implements a leaky bucket rate limiter
type RateLimiter struct {
	mu              sync.Mutex
	tokens          int
	maxTokens       int
	lastRefill      time.Time
	refillInterval  time.Duration
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(reqPerSec int) *RateLimiter {
	return &RateLimiter{
		maxTokens:      reqPerSec,
		tokens:         reqPerSec,
		lastRefill:     time.Now(),
		refillInterval: time.Second / time.Duration(reqPerSec),
	}
}

// Wait waits for a token to be available
func (r *RateLimiter) Wait(ctx context.Context) error {
	for {
		r.mu.Lock()
		now := time.Now()

		// Refill tokens
		elapsed := now.Sub(r.lastRefill)
		tokensToAdd := int(elapsed / r.refillInterval)
		if tokensToAdd > 0 {
			r.tokens = min(r.tokens+tokensToAdd, r.maxTokens)
			r.lastRefill = now
		}

		if r.tokens > 0 {
			r.tokens--
			r.mu.Unlock()
			return nil
		}

		r.mu.Unlock()

		// Wait for refill
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(r.refillInterval):
			continue
		}
	}
}

// NewShopifyClient creates a new Shopify client
func NewShopifyClient(storeURL, accessToken, apiVersion string, dryRun bool) *ShopifyClient {
	return &ShopifyClient{
		StoreURL:    storeURL,
		AccessToken: accessToken,
		APIVersion:  apiVersion,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		RateLimiter: NewRateLimiter(40), // Shopify limit: 40 req/sec
		logger:      log.New(os.Stderr),
		DryRun:      dryRun,
	}
}

// Request represents an API request
type Request struct {
	Method      string
	Path        string
	Body        interface{}
	QueryParams map[string]string
}

// Response represents an API response
type Response struct {
	StatusCode int
	Body       []byte
	Headers    http.Header
}

// Do executes an HTTP request
func (c *ShopifyClient) Do(ctx context.Context, req Request) (*Response, error) {
	if err := c.RateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit wait: %w", err)
	}

	var bodyReader io.Reader
	if req.Body != nil {
		bodyBytes, err := json.Marshal(req.Body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	url := fmt.Sprintf("%s/admin/api/%s%s", c.StoreURL, c.APIVersion, req.Path)

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("X-Shopify-Access-Token", c.AccessToken)
	httpReq.Header.Set("Content-Type", "application/json")

	if req.QueryParams != nil {
		q := httpReq.URL.Query()
		for k, v := range req.QueryParams {
			q.Add(k, v)
		}
		httpReq.URL.RawQuery = q.Encode()
	}

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	// Check for rate limit
	if resp.StatusCode == 429 {
		retryAfter := resp.Header.Get("Retry-After")
		if retryAfter != "" {
			c.logger.Warnf("Rate limited, retry after: %s", retryAfter)
		}
	}

	return &Response{
		StatusCode: resp.StatusCode,
		Body:       respBody,
		Headers:    resp.Header,
	}, nil
}

// GraphQLRequest represents a GraphQL request
type GraphQLRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

// GraphQLResponse represents a GraphQL response
type GraphQLResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []GraphQLError  `json:"errors,omitempty"`
}

// GraphQLError represents a GraphQL error
type GraphQLError struct {
	Message   string `json:"message"`
	Locations []struct {
		Line   int `json:"line"`
		Column int `json:"column"`
	} `json:"locations"`
	Path []interface{} `json:"path,omitempty"`
}

// DoGraphQL executes a GraphQL request
func (c *ShopifyClient) DoGraphQL(ctx context.Context, query string, variables map[string]interface{}) (*GraphQLResponse, error) {
	reqBody := GraphQLRequest{
		Query:     query,
		Variables: variables,
	}

	req := Request{
		Method: "POST",
		Path:   "/graphql.json",
		Body:   reqBody,
	}

	resp, err := c.Do(ctx, req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("graphql request failed: %d - %s", resp.StatusCode, string(resp.Body))
	}

	var graphqlResp GraphQLResponse
	if err := json.Unmarshal(resp.Body, &graphqlResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if len(graphqlResp.Errors) > 0 {
		return nil, fmt.Errorf("graphql errors: %v", graphqlResp.Errors)
	}

	return &graphqlResp, nil
}

// IsErrorRetryable checks if an error is retryable
func IsErrorRetryable(err error) bool {
	if err == nil {
		return false
	}

	// Network errors are retryable
	if netErr, ok := err.(interface{ Timeout() bool }); ok && netErr.Timeout() {
		return true
	}

	// HTTP 429, 5xx errors are retryable
	var httpErr interface{ StatusCode() int }
	if ok := false; ok {
		code := httpErr.StatusCode()
		return code == 429 || code >= 500
	}

	return false
}

// Retry executes a function with retry logic
func Retry(ctx context.Context, maxRetries int, delay time.Duration, fn func() error) error {
	var lastErr error

	for i := 0; i <= maxRetries; i++ {
		if i > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
				delay *= 2 // Exponential backoff
			}
		}

		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err

		if !IsErrorRetryable(err) {
			return err
		}
	}

	return lastErr
}