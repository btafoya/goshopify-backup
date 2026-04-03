package shopify

import (
	"sync"
	"time"
)

// RateLimiter implements a leaky bucket algorithm for rate limiting
// Uses sync.Mutex to ensure thread-safe interval enforcement
type RateLimiter struct {
	mu       sync.Mutex
	lastTime time.Time
	interval time.Duration
}

// NewRateLimiter creates a RateLimiter with the specified requests-per-second
func NewRateLimiter(requestsPerSecond int) *RateLimiter {
	return &RateLimiter{
		interval: time.Second / time.Duration(requestsPerSecond),
		lastTime: time.Now().Add(-time.Second), // Allow first request immediately
	}
}

// Wait blocks until enough time has passed since the last call
func (r *RateLimiter) Wait() {
	r.mu.Lock()
	defer r.mu.Unlock()

	elapsed := time.Since(r.lastTime)
	if elapsed < r.interval {
		time.Sleep(r.interval - elapsed)
	}
	r.lastTime = time.Now()
}

// Config holds shared configuration for Shopify clients
type Config struct {
	Store       string
	AccessToken string
	APIVersion  string
	Limiter     *RateLimiter
}
