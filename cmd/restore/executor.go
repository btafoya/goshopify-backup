package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/log"
)

// RestoreExecutor manages the restore process
type RestoreExecutor struct {
	client          *ShopifyClient
	mutation        *MutationExecutor
	progress        *RestoreProgress
	progressChan    chan *RestoreProgress
	resultChan      chan *RestoreResult
	logger          *log.Logger
	conflictMode    ConflictMode
	cancelFunc      context.CancelFunc
	mu              sync.Mutex
	completedIDs    map[string]bool  // Tracks completed item IDs for resume (O1)
	rollbackActions []RollbackAction // Tracks actions for rollback script (O2)
	rollbackMu      sync.Mutex
	stateFile       string // Path to .restore_state.json
	paused          bool
	pauseMu         sync.Mutex
	done            chan struct{}   // Closed when ExecuteRestore finishes
	results         []RestoreResult // Final results after execution
	execErr         error           // Final error after execution
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
	e.done = make(chan struct{})
	defer close(e.done)

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

	select {
	case e.progressChan <- e.progress:
	default:
	}

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
	// Metaobject definitions must be processed before entries so that
	// the target store has the definition when entries reference it.
	go func() {
		definitions, otherItems := partitionMetaobjectDefinitions(items)
		// Send definitions first
		for _, item := range definitions {
			if resumeSet[item.ID] {
				continue
			}
			select {
			case itemChan <- item:
			case <-ctx.Done():
				return
			}
		}
		// Then send all other items (including metaobject entries)
		for _, item := range otherItems {
			if resumeSet[item.ID] {
				continue
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

	close(e.progressChan)
	close(e.resultChan)

	// Clean up state file on successful completion
	if e.stateFile != "" {
		os.Remove(e.stateFile)
	}

	e.results = results
	e.execErr = nil
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

	// Send progress update (non-blocking — TUI polls via GetProgress)
	select {
	case e.progressChan <- e.progress:
	default:
	}

	// Send result (non-blocking — results collected in local slice)
	select {
	case e.resultChan <- result:
	default:
	}

	// Store result
	resultsMu.Lock()
	*results = append(*results, *result)
	resultsMu.Unlock()
}

// --- Resume capability (O1) ---

// saveResumeState persists restore state after each item
func (e *RestoreExecutor) saveResumeState(item Item, result *RestoreResult) {
	state := &RestoreState{
		StartedAt:      e.progress.StartTime,
		BackupDate:     "",
		TargetStore:    e.client.StoreURL,
		CompletedItems: []CompletedItem{},
		FailedItems:    []FailedItem{},
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
		"",
		"# Get access token: either directly or via client credentials",
		"TOKEN=\"${SHOPIFY_ACCESS_TOKEN:-}\"",
		"if [ -z \"$TOKEN\" ]; then",
		"  CLIENT_ID=\"${SHOPIFY_CLIENT_ID:-}\"",
		"  CLIENT_SECRET=\"${SHOPIFY_SECRET:-}\"",
		"  if [ -z \"$CLIENT_ID\" ] || [ -z \"$CLIENT_SECRET\" ]; then",
		"    echo \"Error: SHOPIFY_ACCESS_TOKEN or both SHOPIFY_CLIENT_ID and SHOPIFY_SECRET must be set\"",
		"    exit 1",
		"  fi",
		"  TOKEN=$(curl -s -X POST \"${STORE}/admin/oauth/access_token\" \\",
		"    -H \"Content-Type: application/x-www-form-urlencoded\" \\",
		"    -d \"grant_type=client_credentials&client_id=${CLIENT_ID}&client_secret=${CLIENT_SECRET}\" \\",
		"    | jq -r '.access_token' 2>/dev/null)",
		"  if [ -z \"$TOKEN\" ]; then echo \"Error: Failed to obtain access token via client credentials\"; exit 1; fi",
		"fi",
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
			case EntityPages:
				deleteMutation = "" // Pages use REST DELETE
			default:
				deleteMutation = `{"query":"mutation{metaobjectDelete(id:\"` + action.ID + `\"){deletedId userErrors{message}}}"}`
			}

			if action.EntityType == EntityThemes {
				// Themes use the Shopify CLI for deletion.
				// Action ID is "gid://shopify/OnlineStoreTheme/{id}" — extract the numeric tail.
				numericID := action.ID
				if idx := strings.LastIndex(action.ID, "/"); idx >= 0 {
					numericID = action.ID[idx+1:]
				}
				script.Commands = append(script.Commands,
					"SHOPIFY_CLI_THEME_TOKEN=\"${TOKEN}\" \\",
					"SHOPIFY_FLAG_PASSWORD=\"${TOKEN}\" \\",
					fmt.Sprintf("shopify theme delete --force --theme %s --store \"${STORE}\" --no-color", numericID),
					"echo \"\"",
					"",
				)
			} else if action.EntityType == EntityPages {
				// Pages use REST API for deletion
				script.Commands = append(script.Commands,
					fmt.Sprintf("curl -s -X DELETE \"${STORE}/admin/api/%s/pages/%s.json\" \\", e.client.APIVersion, action.ID),
					"  -H \"X-Shopify-Access-Token: ${TOKEN}\" \\",
					"  -H \"Content-Type: application/json\"",
					"echo \"\"",
					"",
				)
			} else {
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
	}

	script.Commands = append(script.Commands,
		"echo \"Rollback complete.\"",
	)

	script.Instructions = append(script.Instructions,
		"To rollback this restore:",
		"  1. Set SHOPIFY_ACCESS_TOKEN (or SHOPIFY_CLIENT_ID + SHOPIFY_SECRET) environment variable",
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

// GetProgress returns a snapshot of the current progress (safe for concurrent access)
func (e *RestoreExecutor) GetProgress() *RestoreProgress {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.progress == nil {
		return &RestoreProgress{}
	}
	p := *e.progress
	return &p
}

// Cancel cancels the restore operation
func (e *RestoreExecutor) Cancel() {
	if e.cancelFunc != nil {
		e.cancelFunc()
	}
}

// Done returns a channel that is closed when ExecuteRestore finishes
func (e *RestoreExecutor) Done() <-chan struct{} {
	return e.done
}

// GetResults returns the final results and error after execution completes
func (e *RestoreExecutor) GetResults() ([]RestoreResult, error) {
	return e.results, e.execErr
}

// partitionMetaobjectDefinitions separates metaobject definition items from other items
// so definitions can be processed first during restore.
func partitionMetaobjectDefinitions(items []Item) (definitions, other []Item) {
	for _, item := range items {
		if item.Type == EntityMetaobjects && item.CustomData != nil {
			if isDef, _ := item.CustomData["isDefinition"].(bool); isDef {
				definitions = append(definitions, item)
				continue
			}
		}
		other = append(other, item)
	}
	return
}
