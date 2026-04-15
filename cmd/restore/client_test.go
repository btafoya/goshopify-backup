package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewShopifyClient(t *testing.T) {
	client := NewShopifyClient("https://test.myshopify.com", "shpat_test", "2025-07", false)
	if client.StoreURL != "https://test.myshopify.com" {
		t.Errorf("StoreURL = %q, want %q", client.StoreURL, "https://test.myshopify.com")
	}
	if client.AccessToken != "shpat_test" {
		t.Errorf("AccessToken = %q, want %q", client.AccessToken, "shpat_test")
	}
	if client.APIVersion != "2025-07" {
		t.Errorf("APIVersion = %q, want %q", client.APIVersion, "2025-07")
	}
	if client.DryRun {
		t.Error("DryRun should be false")
	}
	if client.RateLimiter == nil {
		t.Error("RateLimiter should not be nil")
	}
}

func TestNewShopifyClient_DryRun(t *testing.T) {
	client := NewShopifyClient("https://test.myshopify.com", "shpat_test", "2025-07", true)
	if !client.DryRun {
		t.Error("DryRun should be true")
	}
}

func TestHTTPError(t *testing.T) {
	err := &HTTPError{StatusCode: 429, Body: "rate limited"}
	if err.Error() != "HTTP 429: rate limited" {
		t.Errorf("Error() = %q, want %q", err.Error(), "HTTP 429: rate limited")
	}
}

func TestIsErrorRetryable_Nil(t *testing.T) {
	if IsErrorRetryable(nil) {
		t.Error("nil error should not be retryable")
	}
}

func TestIsErrorRetryable_HTTP429(t *testing.T) {
	err := &HTTPError{StatusCode: 429, Body: "rate limited"}
	if !IsErrorRetryable(err) {
		t.Error("429 should be retryable")
	}
}

func TestIsErrorRetryable_HTTP500(t *testing.T) {
	err := &HTTPError{StatusCode: 500, Body: "internal error"}
	if !IsErrorRetryable(err) {
		t.Error("500 should be retryable")
	}
}

func TestIsErrorRetryable_HTTP503(t *testing.T) {
	err := &HTTPError{StatusCode: 503, Body: "service unavailable"}
	if !IsErrorRetryable(err) {
		t.Error("503 should be retryable")
	}
}

func TestIsErrorRetryable_HTTP400(t *testing.T) {
	err := &HTTPError{StatusCode: 400, Body: "bad request"}
	if IsErrorRetryable(err) {
		t.Error("400 should not be retryable")
	}
}

func TestIsErrorRetryable_HTTP404(t *testing.T) {
	err := &HTTPError{StatusCode: 404, Body: "not found"}
	if IsErrorRetryable(err) {
		t.Error("404 should not be retryable")
	}
}

func TestRetry_Success(t *testing.T) {
	ctx := context.Background()
	callCount := 0
	err := Retry(ctx, 3, 10*time.Millisecond, func() error {
		callCount++
		return nil
	})
	if err != nil {
		t.Errorf("Retry() returned error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("fn called %d times, want 1", callCount)
	}
}

func TestRetry_RetryableError(t *testing.T) {
	ctx := context.Background()
	callCount := 0
	err := Retry(ctx, 3, 10*time.Millisecond, func() error {
		callCount++
		if callCount < 3 {
			return &HTTPError{StatusCode: 500, Body: "error"}
		}
		return nil
	})
	if err != nil {
		t.Errorf("Retry() returned error: %v", err)
	}
	if callCount != 3 {
		t.Errorf("fn called %d times, want 3", callCount)
	}
}

func TestRetry_NonRetryableError(t *testing.T) {
	ctx := context.Background()
	callCount := 0
	err := Retry(ctx, 3, 10*time.Millisecond, func() error {
		callCount++
		return &HTTPError{StatusCode: 400, Body: "bad request"}
	})
	if err == nil {
		t.Error("Retry() should return error for non-retryable")
	}
	if callCount != 1 {
		t.Errorf("fn called %d times, want 1 (non-retryable stops immediately)", callCount)
	}
}

func TestRetry_Exhausted(t *testing.T) {
	ctx := context.Background()
	callCount := 0
	err := Retry(ctx, 2, 10*time.Millisecond, func() error {
		callCount++
		return &HTTPError{StatusCode: 500, Body: "always fails"}
	})
	if err == nil {
		t.Error("Retry() should return error when retries exhausted")
	}
	// 0th attempt + 2 retries = 3 total calls
	if callCount != 3 {
		t.Errorf("fn called %d times, want 3", callCount)
	}
}

func TestRetry_Cancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := Retry(ctx, 3, 10*time.Millisecond, func() error {
		return &HTTPError{StatusCode: 500, Body: "error"}
	})
	if err == nil {
		t.Error("Retry() should return error on cancelled context")
	}
}

func TestDoGraphQL_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Shopify-Access-Token") != "shpat_test" {
			t.Error("Missing or wrong access token header")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{"productCreate":{"product":{"id":"gid://shopify/Product/123","handle":"test-product"}}}}`))
	}))
	defer server.Close()

	client := NewShopifyClient(server.URL, "shpat_test", "2025-07", false)
	client.HTTPClient = server.Client()

	resp, err := client.DoGraphQL(context.Background(), `{ query }`, nil)
	if err != nil {
		t.Fatalf("DoGraphQL() error: %v", err)
	}
	if resp == nil {
		t.Fatal("DoGraphQL() returned nil response")
	}
}

func TestDoGraphQL_GraphQLErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"errors":[{"message":"Field 'foo' not found","locations":[{"line":1,"column":5}]}]}`))
	}))
	defer server.Close()

	client := NewShopifyClient(server.URL, "shpat_test", "2025-07", false)
	client.HTTPClient = server.Client()

	_, err := client.DoGraphQL(context.Background(), `{ query }`, nil)
	if err == nil {
		t.Error("DoGraphQL() should return error for GraphQL errors")
	}
}

func TestDoWithRetry_429(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount < 2 {
			w.WriteHeader(429)
			w.Write([]byte(`{"errors":"rate limited"}`))
			return
		}
		w.WriteHeader(200)
		w.Write([]byte(`{"data":{}}`))
	}))
	defer server.Close()

	client := NewShopifyClient(server.URL, "shpat_test", "2025-07", false)
	client.HTTPClient = server.Client()

	_, err := client.DoWithRetry(context.Background(), Request{
		Method: "GET",
		Path:   "/test.json",
	})
	if err != nil {
		t.Errorf("DoWithRetry() error: %v", err)
	}
	if callCount < 2 {
		t.Errorf("Expected retry on 429, only called %d times", callCount)
	}
}