package main

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/charmbracelet/log"
)

// RestoreExecutor manages the restore process
type RestoreExecutor struct {
	client         *ShopifyClient
	mutation       *MutationExecutor
	progress       *RestoreProgress
	progressChan   chan *RestoreProgress
	resultChan     chan *RestoreResult
	logger         *log.Logger
	conflictMode   ConflictMode
	cancelFunc     context.CancelFunc
	mu             sync.Mutex
}

// NewRestoreExecutor creates a new restore executor
func NewRestoreExecutor(client *ShopifyClient, conflictMode ConflictMode) *RestoreExecutor {
	mutation := NewMutationExecutor(client)
	return &RestoreExecutor{
		client:       client,
		mutation:     mutation,
		progressChan: make(chan *RestoreProgress, 100),
		resultChan:   make(chan *RestoreResult, 100),
		logger:       log.New(os.Stderr),
		conflictMode: conflictMode,
	}
}

// ExecuteRestore executes the restore process
func (e *RestoreExecutor) ExecuteRestore(ctx context.Context, items []Item) ([]RestoreResult, error) {
	ctx, cancel := context.WithCancel(ctx)
	e.cancelFunc = cancel
	defer cancel()

	// Initialize progress
	e.progress = &RestoreProgress{
		TotalItems:   len(items),
		StartTime:    time.Now(),
		Status:       "running",
		CurrentItems: make(map[string]bool),
	}

	// Send initial progress
	e.progressChan <- e.progress

	// Create worker pool
	numWorkers := 5 // Concurrent workers
	itemChan := make(chan Item, numWorkers*2)

	var wg sync.WaitGroup
	results := make([]RestoreResult, 0, len(items))
	resultsMu := sync.Mutex{}

	// Start workers
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go e.worker(ctx, &wg, itemChan, &resultsMu, &results)
	}

	// Send items to workers
	go func() {
		for _, item := range items {
			select {
			case itemChan <- item:
			case <-ctx.Done():
				return
			}
		}
		close(itemChan)
	}()

	// Wait for completion
	wg.Wait()

	// Final progress update
	e.progress.Status = "completed"
	e.progress.CompletedAt = time.Now()
	e.progress.Duration = e.progress.CompletedAt.Sub(e.progress.StartTime)
	e.progressChan <- e.progress

	close(e.progressChan)
	close(e.resultChan)

	return results, nil
}

// worker processes restore items
func (e *RestoreExecutor) worker(ctx context.Context, wg *sync.WaitGroup, itemChan <-chan Item, resultsMu *sync.Mutex, results *[]RestoreResult) {
	defer wg.Done()

	for item := range itemChan {
		select {
		case <-ctx.Done():
			return
		default:
			e.processItem(ctx, item, resultsMu, results)
		}
	}
}

// processItem processes a single item
func (e *RestoreExecutor) processItem(ctx context.Context, item Item, resultsMu *sync.Mutex, results *[]RestoreResult) {
	// Update progress
	e.mu.Lock()
	e.progress.CurrentEntity = item.Type
	e.progress.CurrentItem = item.Title
	e.progress.CurrentItems[item.ID] = true
	e.mu.Unlock()

	// Execute restore
	result, err := e.mutation.RestoreItem(ctx, item, e.conflictMode)
	if err != nil {
		result.Error = err.Error()
		e.logger.Errorf("Failed to restore %s: %v", item.ID, err)
	}

	// Update progress
	e.mu.Lock()
	if result.Success {
		e.progress.CompletedItems++
	} else {
		e.progress.FailedItems++
		e.progress.Logs = append(e.progress.Logs, LogEntry{
			Level:     "error",
			Message:   fmt.Sprintf("Failed to restore %s: %s", item.Title, result.Message),
			Timestamp: time.Now(),
		})
	}
	e.mu.Unlock()

	// Send progress update
	e.progressChan <- e.progress

	// Send result
	e.resultChan <- result

	// Store result
	resultsMu.Lock()
	*results = append(*results, *result)
	resultsMu.Unlock()
}

// GetProgressChan returns the progress channel
func (e *RestoreExecutor) GetProgressChan() <-chan *RestoreProgress {
	return e.progressChan
}

// GetResultChan returns the result channel
func (e *RestoreExecutor) GetResultChan() <-chan *RestoreResult {
	return e.resultChan
}

// GetProgress returns the current progress
func (e *RestoreExecutor) GetProgress() *RestoreProgress {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.progress
}

// Cancel cancels the restore operation
func (e *RestoreExecutor) Cancel() {
	if e.cancelFunc != nil {
		e.cancelFunc()
	}
}

// Pause pauses the restore operation
func (e *RestoreExecutor) Pause() {
	// Implementation depends on worker pool design
}

// Resume resumes the restore operation
func (e *RestoreExecutor) Resume() {
	// Implementation depends on worker pool design
}

// GetRollbackScript generates a rollback script
func (e *RestoreExecutor) GetRollbackScript(backupDate string) *RollbackScript {
	e.mu.Lock()
	defer e.mu.Unlock()

	script := &RollbackScript{
		BackupDate:   backupDate,
		CreatedAt:    time.Now(),
		Instructions: []string{},
		Commands:     []string{},
	}

	// Add header
	script.Instructions = append(script.Instructions,
		fmt.Sprintf("# Rollback script for restore from %s", backupDate),
		fmt.Sprintf("# Generated on: %s", script.CreatedAt.Format(time.RFC3339)),
		"",
		"# WARNING: This script will DELETE the restored items from your store.",
		"# Review carefully before running.",
		"",
	)

	// Add commands for each restored item
	// This would need to collect the restored IDs during the restore process

	script.Instructions = append(script.Instructions,
		"# Run this script to rollback the restore:",
		"#   chmod +x rollback.sh",
		"#   ./rollback.sh",
	)

	return script
}