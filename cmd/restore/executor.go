package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	completedIDs   map[string]bool     // Tracks completed item IDs for resume (O1)
	rollbackActions []RollbackAction   // Tracks actions for rollback script (O2)
	rollbackMu     sync.Mutex
	stateFile      string              // Path to .restore_state.json
	paused         bool
	pauseMu        sync.Mutex
}

// NewRestoreExecutor creates a new restore executor
func NewRestoreExecutor(client *ShopifyClient, conflictMode ConflictMode) *RestoreExecutor {
	mutation := NewMutationExecutor(client)
	return &RestoreExecutor{
		client:          client,
		mutation:        mutation,
		progressChan:    make(chan *RestoreProgress, 100),
		resultChan:      make(chan *RestoreResult, 100),
		logger:          log.New(os.Stderr),
		conflictMode:    conflictMode,
		completedIDs:    make(map[string]bool),
		rollbackActions: make([]RollbackAction, 0),
	}
}

// SetStateFile sets the path for resume state persistence
func (e *RestoreExecutor) SetStateFile(path string) {
	e.stateFile = path
}

// ExecuteRestore executes the restore process
func (e *RestoreExecutor) ExecuteRestore(ctx context.Context, items []Item) ([]RestoreResult, error) {
	ctx, cancel := context.WithCancel(ctx)
	e.cancelFunc = cancel
	defer cancel()

	// Load resume state if state file exists (O1)
	resumeSet := make(map[string]bool)
	if e.stateFile != "" {
		if state, err := e.loadResumeState(); err == nil && state != nil {
			for _, ci := range state.CompletedItems {
				resumeSet[ci.SourceID] = true
				e.completedIDs[ci.SourceID] = true
			}
			e.logger.Infof("Resuming restore: %d items already completed", len(resumeSet))
		}
	}

	// Initialize progress
	skipped := len(resumeSet)
	e.progress = &RestoreProgress{
		TotalItems:     len(items),
		CompletedItems: skipped,
		SkippedItems:   skipped,
		StartTime:      time.Now(),
		Status:         "running",
		CurrentItems:   make(map[string]bool),
	}

	e.progressChan <- e.progress

	// Create worker pool
	numWorkers := MaxConcurrentUploads
	itemChan := make(chan Item, numWorkers*2)

	var wg sync.WaitGroup
	results := make([]RestoreResult, 0, len(items))
	resultsMu := sync.Mutex{}

	// Start workers
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go e.worker(ctx, &wg, itemChan, &resultsMu, &results, resumeSet)
	}

	// Send items to workers (skip already completed for resume)
	go func() {
		for _, item := range items {
			if resumeSet[item.ID] {
				continue // Already completed in previous run
			}
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

	// Clean up state file on successful completion
	if e.stateFile != "" {
		os.Remove(e.stateFile)
	}

	return results, nil
}

// worker processes restore items
func (e *RestoreExecutor) worker(ctx context.Context, wg *sync.WaitGroup, itemChan <-chan Item, resultsMu *sync.Mutex, results *[]RestoreResult, resumeSet map[string]bool) {
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
	// Check pause state
	e.pauseMu.Lock()
	paused := e.paused
	e.pauseMu.Unlock()
	if paused {
		// Wait until unpaused
		for {
			e.pauseMu.Lock()
			p := e.paused
			e.pauseMu.Unlock()
			if !p {
				break
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
			}
		}
	}

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

	// Update progress and tracking
	e.mu.Lock()
	if result.Success {
		e.progress.CompletedItems++
		e.completedIDs[item.ID] = true

		// Record rollback action (O2)
		if result.RestoredID != "" {
			e.rollbackMu.Lock()
			e.rollbackActions = append(e.rollbackActions, RollbackAction{
				EntityType:  item.Type,
				Action:      "delete",
				ID:          result.RestoredID,
				Description: fmt.Sprintf("Delete restored %s: %s", item.Type, item.Title),
			})
			e.rollbackMu.Unlock()
		}
	} else {
		e.progress.FailedItems++
		e.progress.Logs = append(e.progress.Logs, LogEntry{
			Level:     "error",
			Message:   fmt.Sprintf("Failed to restore %s: %s", item.Title, result.Message),
			Timestamp: time.Now(),
		})
	}
	e.mu.Unlock()

	// Save resume state after each item (O1)
	if e.stateFile != "" {
		e.saveResumeState(item, result)
	}

	// Send progress update
	e.progressChan <- e.progress

	// Send result
	e.resultChan <- result

	// Store result
	resultsMu.Lock()
	*results = append(*results, *result)
	resultsMu.Unlock()
}

// --- Resume capability (O1) ---

// saveResumeState persists restore state after each item
func (e *RestoreExecutor) saveResumeState(item Item, result *RestoreResult) {
	state := &RestoreState{
		StartedAt:    e.progress.StartTime,
		BackupDate:   "",
		TargetStore:  e.client.StoreURL,
		CompletedItems: []CompletedItem{},
		FailedItems:  []FailedItem{},
	}

	// Load existing state
	if existing, err := e.loadResumeState(); err == nil && existing != nil {
		state = existing
	}

	if result.Success {
		state.CompletedItems = append(state.CompletedItems, CompletedItem{
			EntityType:  item.Type,
			SourceID:    item.ID,
			TargetID:    result.RestoredID,
			CompletedAt: time.Now(),
		})
	} else {
		state.FailedItems = append(state.FailedItems, FailedItem{
			EntityType: item.Type,
			SourceID:   item.ID,
			Error:      result.Message,
			FailedAt:   time.Now(),
		})
	}

	state.Progress = ProgressState{
		Total:     e.progress.TotalItems,
		Completed: e.progress.CompletedItems,
		Failed:    e.progress.FailedItems,
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return
	}

	os.WriteFile(e.stateFile, data, 0600)
}

// loadResumeState reads the resume state file
func (e *RestoreExecutor) loadResumeState() (*RestoreState, error) {
	if e.stateFile == "" {
		return nil, fmt.Errorf("no state file configured")
	}

	data, err := os.ReadFile(e.stateFile)
	if err != nil {
		return nil, err
	}

	var state RestoreState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	return &state, nil
}

// Pause pauses the restore operation (O1)
func (e *RestoreExecutor) Pause() {
	e.pauseMu.Lock()
	defer e.pauseMu.Unlock()
	e.paused = true
	e.logger.Infof("Restore paused")
}

// Resume resumes the restore operation (O1)
func (e *RestoreExecutor) Resume() {
	e.pauseMu.Lock()
	defer e.pauseMu.Unlock()
	e.paused = false
	e.logger.Infof("Restore resumed")
}

// --- Rollback script generation (O2) ---

// GetRollbackScript generates a rollback script with actual delete commands
func (e *RestoreExecutor) GetRollbackScript(backupDate string) *RollbackScript {
	e.rollbackMu.Lock()
	defer e.rollbackMu.Unlock()

	script := &RollbackScript{
		BackupDate:   backupDate,
		CreatedAt:    time.Now(),
		Instructions: []string{},
		Commands:     []string{},
		Actions:      make([]RollbackAction, len(e.rollbackActions)),
	}

	copy(script.Actions, e.rollbackActions)

	// Generate shell commands from rollback actions
	script.Commands = append(script.Commands,
		"#!/bin/bash",
		fmt.Sprintf("# Rollback script for restore from %s", backupDate),
		fmt.Sprintf("# Generated on: %s", script.CreatedAt.Format(time.RFC3339)),
		"",
		"# WARNING: This script will DELETE the restored items from your store.",
		"# Review carefully before running.",
		"",
		"set -e",
		"",
		fmt.Sprintf("STORE=\"%s\"", e.client.StoreURL),
		fmt.Sprintf("TOKEN=\"${SHOPIFY_ACCESS_TOKEN:-}\""),
		"if [ -z \"$TOKEN\" ]; then echo \"Error: SHOPIFY_ACCESS_TOKEN not set\"; exit 1; fi",
		"",
		"echo \"Starting rollback...\"",
		"",
	)

	for _, action := range e.rollbackActions {
		switch action.Action {
		case "delete":
			// Generate curl command for deletion via GraphQL
			script.Commands = append(script.Commands,
				fmt.Sprintf("# %s", action.Description),
				fmt.Sprintf("echo \"Deleting %s %s...\"", action.EntityType, action.ID),
			)

			var deleteMutation string
			switch action.EntityType {
			case EntityProducts:
				deleteMutation = `{"query":"mutation{productDelete(input:{id:\"` + action.ID + `\"}){deletedId userErrors{message}}}"}`
			case EntityCollections:
				deleteMutation = `{"query":"mutation{collectionDelete(input:{id:\"` + action.ID + `\"}){deletedId userErrors{message}}}"}`
			case EntityMetaobjects:
				deleteMutation = `{"query":"mutation{metaobjectDelete(id:\"` + action.ID + `\"){deletedId userErrors{message}}}"}`
			default:
				deleteMutation = `{"query":"mutation{metaobjectDelete(id:\"` + action.ID + `\"){deletedId userErrors{message}}}"}`
			}

			script.Commands = append(script.Commands,
				fmt.Sprintf("curl -s -X POST \"${STORE}/admin/api/%s/graphql.json\" \\", e.client.APIVersion),
				"  -H \"X-Shopify-Access-Token: ${TOKEN}\" \\",
				"  -H \"Content-Type: application/json\" \\",
				fmt.Sprintf("  -d '%s'", deleteMutation),
				"echo \"\"",
				"",
			)
		}
	}

	script.Commands = append(script.Commands,
		"echo \"Rollback complete.\"",
	)

	script.Instructions = append(script.Instructions,
		"To rollback this restore:",
		"  1. Set SHOPIFY_ACCESS_TOKEN environment variable",
		"  2. Review the generated commands",
		"  3. Run: chmod +x rollback.sh && ./rollback.sh",
	)

	return script
}

// WriteRollbackScript writes the rollback script to disk (O2)
func (e *RestoreExecutor) WriteRollbackScript(rollbackDir, backupDate string) (string, error) {
	script := e.GetRollbackScript(backupDate)

	filename := fmt.Sprintf(RollbackFile, backupDate)
	filePath := filepath.Join(rollbackDir, filename)

	var content string
	for _, cmd := range script.Commands {
		content += cmd + "\n"
	}

	if err := os.WriteFile(filePath, []byte(content), 0755); err != nil {
		return "", fmt.Errorf("write rollback script: %w", err)
	}

	return filePath, nil
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