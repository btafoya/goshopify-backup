package main

import "time"

// Config holds all configuration from environment variables
type Config struct {
	Store         string
	AccessToken   string
	ClientID      string
	ClientSecret  string
	APIVersion    string
	BackupDir     string
	RetentionDays int
	Force         bool
	PollTimeout   time.Duration
}

// BackupStatus is written to status.json
type BackupStatus struct {
	StartedAt   time.Time               `json:"startedAt"`
	CompletedAt time.Time               `json:"completedAt,omitempty"`
	Duration    string                  `json:"duration,omitempty"`
	Modules     map[string]ModuleStatus `json:"modules"`
	TotalSize   int64                   `json:"totalSize,omitempty"`
}

// ModuleStatus represents the status of a single backup module
type ModuleStatus struct {
	Status      string    `json:"status"` // "pending", "running", "completed", "failed"
	StartedAt   time.Time `json:"startedAt"`
	CompletedAt time.Time `json:"completedAt,omitempty"`
	Count       int       `json:"count"`
	Error       string    `json:"error,omitempty"`
	FileSize    int64     `json:"fileSize,omitempty"`
	Fallback    string    `json:"fallback,omitempty"` // "REST" if bulk operation fell back
}

// LockFile represents the lock file content
type LockFile struct {
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"startedAt"`
}

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
	FileSize    int64  `json:"fileSize"`
	ObjectCount int64  `json:"objectCount"`
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
