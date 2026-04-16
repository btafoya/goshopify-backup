package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/log"
)

// ShopifyClient handles API communication with Shopify
type ShopifyClient struct {
	StoreURL    string
	APIVersion  string
	HTTPClient  *http.Client
	RateLimiter *RateLimiter
	logger      *log.Logger
	DryRun      bool

	// Authentication: either AccessToken (direct) or ClientID+ClientSecret (OAuth)
	AccessToken  string
	ClientID     string
	ClientSecret string

	// Token cache for client credentials flow
	tokenMu     sync.Mutex
	cachedToken string
	tokenExpiry time.Time
}

// RateLimiter implements a leaky bucket rate limiter
type RateLimiter struct {
	mu             sync.Mutex
	tokens         int
	maxTokens      int
	lastRefill     time.Time
	refillInterval time.Duration
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

// NewShopifyClient creates a new Shopify client with a direct access token
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

// NewShopifyClientWithCredentials creates a client that authenticates via client credentials
func NewShopifyClientWithCredentials(storeURL, clientID, clientSecret, apiVersion string, dryRun bool) *ShopifyClient {
	return &ShopifyClient{
		StoreURL:     storeURL,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		APIVersion:   apiVersion,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		RateLimiter: NewRateLimiter(40),
		logger:      log.New(os.Stderr),
		DryRun:      dryRun,
	}
}

// Authenticate exchanges client credentials for an access token via the
// Shopify OAuth client_credentials grant, then stores the token for API calls.
// If an AccessToken was already provided directly, this is a no-op.
func (c *ShopifyClient) Authenticate(ctx context.Context) error {
	// Direct access token already set — nothing to do
	if c.AccessToken != "" {
		return nil
	}

	// Must have client credentials if no direct token
	if c.ClientID == "" || c.ClientSecret == "" {
		return fmt.Errorf("authentication requires either SHOPIFY_ACCESS_TOKEN or both SHOPIFY_CLIENT_ID and SHOPIFY_SECRET")
	}

	// Check cached token — refresh 60 seconds before expiry
	c.tokenMu.Lock()
	if c.cachedToken != "" && time.Now().Before(c.tokenExpiry.Add(-TokenRefreshBuffer)) {
		c.AccessToken = c.cachedToken
		c.tokenMu.Unlock()
		return nil
	}
	c.tokenMu.Unlock()

	// Exchange client credentials for access token
	token, expiresIn, err := c.fetchAccessToken(ctx)
	if err != nil {
		return fmt.Errorf("client credentials authentication failed: %w", err)
	}

	c.tokenMu.Lock()
	c.cachedToken = token
	c.tokenExpiry = time.Now().Add(time.Duration(expiresIn) * time.Second)
	c.AccessToken = token
	c.tokenMu.Unlock()

	c.logger.Infof("Authenticated via client credentials (token expires in %ds)", expiresIn)
	return nil
}

// oauthTokenResponse represents the response from the OAuth token endpoint
type oauthTokenResponse struct {
	AccessToken string `json:"access_token"`
	Scope       string `json:"scope"`
	ExpiresIn   int    `json:"expires_in"`
}

// fetchAccessToken POSTs to the Shopify OAuth endpoint to exchange client
// credentials for a short-lived access token.
func (c *ShopifyClient) fetchAccessToken(ctx context.Context) (string, int, error) {
	// Build the token URL from the store URL
	storeBase := strings.TrimSuffix(c.StoreURL, "/")
	tokenURL := fmt.Sprintf("%s/admin/oauth/access_token", storeBase)

	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", c.ClientID)
	data.Set("client_secret", c.ClientSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", 0, fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, fmt.Errorf("read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("token request returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp oauthTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", 0, fmt.Errorf("parse token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return "", 0, fmt.Errorf("token response missing access_token")
	}

	return tokenResp.AccessToken, tokenResp.ExpiresIn, nil
}

// Request represents an API request
type Request struct {
	Method      string
	Path        string
	Body        interface{}
	QueryParams map[string]string
}

// HTTPError wraps an error with an HTTP status code
type HTTPError struct {
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Body)
}

// Response represents an API response
type Response struct {
	StatusCode int
	Body       []byte
	Headers    http.Header
}

// Do executes a single HTTP request (no retry). Use DoWithRetry for automatic retries.
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

	// Return HTTPError for retryable status codes so Retry() can detect them
	if resp.StatusCode == 429 || resp.StatusCode >= 500 {
		return nil, &HTTPError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}

	return &Response{
		StatusCode: resp.StatusCode,
		Body:       respBody,
		Headers:    resp.Header,
	}, nil
}

// DoWithRetry executes an HTTP request with automatic retry on 429/5xx/network errors.
func (c *ShopifyClient) DoWithRetry(ctx context.Context, req Request) (*Response, error) {
	var resp *Response
	err := Retry(ctx, RestoreRetryCount, RestoreRetryDelay, func() error {
		var reqErr error
		resp, reqErr = c.Do(ctx, req)
		if reqErr == nil {
			return nil
		}
		// Handle 429 with Retry-After header
		if httpErr, ok := reqErr.(*HTTPError); ok && httpErr.StatusCode == 429 {
			// The Retry-After header is in the response, but we lost it in HTTPError.
			// Use a minimum 2-second wait for rate limits.
			c.logger.Warnf("Rate limited (429), backing off before retry")
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(RestoreRetryDelay):
			}
		}
		return reqErr
	})
	return resp, err
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

// DoGraphQL executes a GraphQL request with automatic retry on 429/5xx.
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

	resp, err := c.DoWithRetry(ctx, req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &HTTPError{StatusCode: resp.StatusCode, Body: string(resp.Body)}
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
	if httpErr, ok := err.(*HTTPError); ok {
		code := httpErr.StatusCode
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
