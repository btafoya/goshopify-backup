package shopify

import (
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
