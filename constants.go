package main

import "time"

const (
	// API
	APIVersion = "2025-01"

	// Rate Limiting
	RequestsPerSecond  = 40
	MinRequestInterval = 25 * time.Millisecond // 1000ms / 40 = 25ms

	// Retry
	RetryCount     = 3
	RetryBaseDelay = 2 * time.Second
	RetryMaxDelay  = 30 * time.Second

	// Bulk Operations
	PollInterval = 1 * time.Second
	PollTimeout  = 10 * time.Minute

	// Image Download
	ImageConcurrency = 10
	ImageMaxRetries  = 3

	// Status Writer
	StatusFlushInterval = 5 * time.Second

	// Retention
	MaxRetentionDays = 3650

	// Lock
	StaleLockDuration = 24 * time.Hour

	// Output
	DateFormat = "2006-01-02"
)

// AllowedDomains for bulk operation JSONL download URLs
var AllowedDomains = []string{
	"storage.shopifycloud.com",
	"shopify.com",
}

// Exit codes
const (
	ExitSuccess       = 0
	ExitBackupFailed  = 1
	ExitConfigError   = 2
	ExitConcurrentRun = 3
)
