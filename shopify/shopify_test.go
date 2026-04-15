package shopify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewRateLimiter(t *testing.T) {
	rl := NewRateLimiter(40)
	if rl == nil {
		t.Fatal("NewRateLimiter() returned nil")
	}

	if rl.interval != 25*time.Millisecond {
		t.Errorf("interval = %v, want 25ms", rl.interval)
	}
}

func TestRateLimiter_Wait(t *testing.T) {
	rl := NewRateLimiter(1000) // 1000 requests per second = 1ms interval

	start := time.Now()
	rl.Wait()
	elapsed := time.Since(start)

	if elapsed > 10*time.Millisecond {
		t.Errorf("First Wait() took too long: %v", elapsed)
	}

	// Second wait should respect rate limit
	start = time.Now()
	rl.Wait()
	elapsed = time.Since(start)

	if elapsed < 500*time.Microsecond {
		t.Errorf("Second Wait() too fast: %v", elapsed)
	}
	if elapsed > 2*time.Millisecond {
		t.Errorf("Second Wait() too slow: %v", elapsed)
	}
}

func TestRateLimiter_Concurrent(t *testing.T) {
	rl := NewRateLimiter(10) // 10 requests per second = 100ms interval

	// Spawn multiple goroutines to test thread safety
	done := make(chan bool, 5)
	for i := 0; i < 5; i++ {
		go func() {
			rl.Wait()
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 5; i++ {
		<-done
	}
}

func TestNewGraphQLClient(t *testing.T) {
	cfg := &Config{
		Store:       "https://test.myshopify.com",
		AccessToken: "test_token",
		APIVersion:  "2025-01",
		Limiter:     NewRateLimiter(40),
	}

	client := NewGraphQLClient(cfg)
	if client == nil {
		t.Fatal("NewGraphQLClient() returned nil")
	}

	if client.store != "https://test.myshopify.com" {
		t.Errorf("store = %v, want https://test.myshopify.com", client.store)
	}
	if client.accessToken != "test_token" {
		t.Errorf("accessToken = %v, want test_token", client.accessToken)
	}
	if client.apiVersion != "2025-01" {
		t.Errorf("apiVersion = %v, want 2025-01", client.apiVersion)
	}
}

func TestNewRESTClient(t *testing.T) {
	cfg := &Config{
		Store:       "https://test.myshopify.com",
		AccessToken: "test_token",
		APIVersion:  "2025-01",
		Limiter:     NewRateLimiter(40),
	}

	client := NewRESTClient(cfg)
	if client == nil {
		t.Fatal("NewRESTClient() returned nil")
	}

	if client.store != "https://test.myshopify.com" {
		t.Errorf("store = %v, want https://test.myshopify.com", client.store)
	}
}

func TestBulkOperationStatus_String(t *testing.T) {
	tests := []struct {
		status BulkOperationStatus
		want   string
	}{
		{StatusCreated, "CREATED"},
		{StatusRunning, "RUNNING"},
		{StatusCompleted, "COMPLETED"},
		{StatusFailed, "FAILED"},
		{StatusCanceled, "CANCELED"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if string(tt.status) != tt.want {
				t.Errorf("BulkOperationStatus = %v, want %v", tt.status, tt.want)
			}
		})
	}
}

func TestAccessDeniedError(t *testing.T) {
	err := &AccessDeniedError{Message: "bulk operation not allowed"}
	got := err.Error()
	want := "ACCESS_DENIED: bulk operation not allowed"
	if got != want {
		t.Errorf("Error() = %v, want %v", got, want)
	}
}

func TestConfig(t *testing.T) {
	cfg := &Config{
		Store:       "https://test.myshopify.com",
		AccessToken: "test_token",
		APIVersion:  "2025-01",
		Limiter:     NewRateLimiter(40),
	}

	if cfg.Store != "https://test.myshopify.com" {
		t.Errorf("Store = %v", cfg.Store)
	}
	if cfg.Limiter == nil {
		t.Error("Limiter should not be nil")
	}
}

func TestGraphQLError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  GraphQLError
		want string
	}{
		{
			name: "simple message",
			err:  GraphQLError{Message: "Throttled"},
			want: "graphql error: Throttled",
		},
		{
			name: "with path",
			err: GraphQLError{
				Message: "Field 'foo' not found",
				Path:    []interface{}{"query", "foo"},
			},
			want: "graphql error: Field 'foo' not found (path: [query foo])",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGraphQLErrorGroup_Error(t *testing.T) {
	tests := []struct {
		name string
		err  *GraphQLErrorGroup
		want string
	}{
		{
			name: "single error",
			err:  &GraphQLErrorGroup{Errors: []GraphQLError{{Message: "cost exceeded"}}},
			want: "graphql error: cost exceeded",
		},
		{
			name: "multiple errors",
			err: &GraphQLErrorGroup{Errors: []GraphQLError{
				{Message: "error one"},
				{Message: "error two"},
			}},
			want: "graphql errors (2): graphql error: error one; graphql error: error two",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGraphQLResponse_Unmarshal(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantErrors  int
		wantErrMsg  string
		wantDataStr string
	}{
		{
			name:        "error response with null data",
			input:       `{"data": null, "errors": [{"message": "Throttled"}]}`,
			wantErrors:  1,
			wantErrMsg:  "Throttled",
			wantDataStr: "null",
		},
		{
			name:        "successful response with no errors",
			input:       `{"data": {"metaobjectDefinitions": {"edges": []}}}`,
			wantErrors:  0,
			wantDataStr: `{"metaobjectDefinitions": {"edges": []}}`,
		},
		{
			name:        "multiple errors",
			input:       `{"data": null, "errors": [{"message": "err1"}, {"message": "err2"}]}`,
			wantErrors:  2,
			wantErrMsg:  "err1",
			wantDataStr: "null",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resp GraphQLResponse
			if err := json.Unmarshal([]byte(tt.input), &resp); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}
			if len(resp.Errors) != tt.wantErrors {
				t.Errorf("Errors length = %d, want %d", len(resp.Errors), tt.wantErrors)
			}
			if tt.wantErrors > 0 && resp.Errors[0].Message != tt.wantErrMsg {
				t.Errorf("Errors[0].Message = %q, want %q", resp.Errors[0].Message, tt.wantErrMsg)
			}
			if string(resp.Data) != tt.wantDataStr {
				t.Errorf("Data = %q, want %q", string(resp.Data), tt.wantDataStr)
			}
		})
	}
}

func TestQuery_GraphQLErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"data": null, "errors": [{"message": "Throttled"}]}`)
	}))
	defer server.Close()

	cfg := &Config{
		Store:       server.URL,
		AccessToken: "test",
		APIVersion:  "2025-01",
		Limiter:     NewRateLimiter(1000),
	}
	client := NewGraphQLClient(cfg)

	data, err := client.Query(context.Background(), `{"query": "{ test }"}`)
	if err == nil {
		t.Fatal("Query() should return error when GraphQL errors present")
	}

	var errGroup *GraphQLErrorGroup
	if !errors.As(err, &errGroup) {
		t.Errorf("error type = %T, want *GraphQLErrorGroup", err)
	}

	// Query() returns the full response body, so data contains the complete JSON
	if len(data) == 0 {
		t.Error("data should not be empty")
	}
}

func TestQuery_SuccessResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"data": {"test": "value"}, "extensions": {"cost": {"requestedQueryCost": 1}}}`)
	}))
	defer server.Close()

	cfg := &Config{
		Store:       server.URL,
		AccessToken: "test",
		APIVersion:  "2025-01",
		Limiter:     NewRateLimiter(1000),
	}
	client := NewGraphQLClient(cfg)

	data, err := client.Query(context.Background(), `{"query": "{ test }"}`)
	if err != nil {
		t.Fatalf("Query() returned unexpected error: %v", err)
	}

	// Query() returns the full response body
	var parsed struct {
		Data struct {
			Test string `json:"test"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	if parsed.Data.Test != "value" {
		t.Errorf("data.test = %q, want 'value'", parsed.Data.Test)
	}
}

func TestQuery_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "Internal Server Error")
	}))
	defer server.Close()

	cfg := &Config{
		Store:       server.URL,
		AccessToken: "test",
		APIVersion:  "2025-01",
		Limiter:     NewRateLimiter(1000),
	}
	client := NewGraphQLClient(cfg)

	_, err := client.Query(context.Background(), `{"query": "{ test }"}`)
	if err == nil {
		t.Fatal("Query() should return error on HTTP 500")
	}
}

func TestQuery_AccessDeniedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"data": null, "errors": [{"message": "Access denied for metaobjectDefinitions field. Requires read_metaobject_definitions access scope.", "path": ["query", "metaobjectDefinitions"]}]}`)
	}))
	defer server.Close()

	cfg := &Config{
		Store:       server.URL,
		AccessToken: "test",
		APIVersion:  "2025-01",
		Limiter:     NewRateLimiter(1000),
	}
	client := NewGraphQLClient(cfg)

	_, err := client.Query(context.Background(), `{"query": "{ metaobjectDefinitions { edges { node { id } } } }"}`)
	if err == nil {
		t.Fatal("Query() should return error for access denied")
	}

	var errGroup *GraphQLErrorGroup
	if !errors.As(err, &errGroup) {
		t.Errorf("error type = %T, want *GraphQLErrorGroup", err)
	}
}
