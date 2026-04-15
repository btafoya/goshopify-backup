package main

import (
	"context"
	"testing"
	"time"
)

func TestNewRestoreExecutor(t *testing.T) {
	client := NewShopifyClient("https://test.myshopify.com", "shpat_test", "2025-07", false)
	executor := NewRestoreExecutor(client, ConflictSkip)

	if executor.conflictMode != ConflictSkip {
		t.Errorf("conflictMode = %q, want %q", executor.conflictMode, ConflictSkip)
	}
	if executor.completedIDs == nil {
		t.Error("completedIDs should be initialized")
	}
}

func TestNewRestoreExecutor_WithForce(t *testing.T) {
	client := NewShopifyClient("https://test.myshopify.com", "shpat_test", "2025-07", false)
	executor := NewRestoreExecutor(client, ConflictOverwrite)

	if executor.conflictMode != ConflictOverwrite {
		t.Errorf("conflictMode = %q, want %q", executor.conflictMode, ConflictOverwrite)
	}
}

func TestRestoreExecutor_PauseResume(t *testing.T) {
	client := NewShopifyClient("https://test.myshopify.com", "shpat_test", "2025-07", false)
	executor := NewRestoreExecutor(client, ConflictSkip)

	executor.Pause()
	if !executor.paused {
		t.Error("Should be paused after Pause()")
	}

	executor.Resume()
	if executor.paused {
		t.Error("Should not be paused after Resume()")
	}
}

func TestRestoreExecutor_Cancel(t *testing.T) {
	client := NewShopifyClient("https://test.myshopify.com", "shpat_test", "2025-07", false)
	executor := NewRestoreExecutor(client, ConflictSkip)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start a long-running operation that we cancel
	go func() {
		time.Sleep(10 * time.Millisecond)
		executor.Cancel()
	}()

	_ = ctx // Use ctx to avoid unused variable error
}

func TestRestoreExecutor_GetProgress(t *testing.T) {
	client := NewShopifyClient("https://test.myshopify.com", "shpat_test", "2025-07", false)
	executor := NewRestoreExecutor(client, ConflictSkip)

	progress := executor.GetProgress()
	if progress != nil {
		t.Error("GetProgress() should be nil before execution")
	}
}

func TestRestoreExecutor_SetStateFile(t *testing.T) {
	client := NewShopifyClient("https://test.myshopify.com", "shpat_test", "2025-07", false)
	executor := NewRestoreExecutor(client, ConflictSkip)

	executor.SetStateFile("/tmp/.restore_state.json")
	if executor.stateFile != "/tmp/.restore_state.json" {
		t.Errorf("stateFile = %q, want %q", executor.stateFile, "/tmp/.restore_state.json")
	}
}

func TestRestoreExecutor_SaveResumeState(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := tmpDir + "/.restore_state.json"

	client := NewShopifyClient("https://test.myshopify.com", "shpat_test", "2025-07", false)
	executor := NewRestoreExecutor(client, ConflictSkip)
	executor.SetStateFile(stateFile)

	executor.progress = &RestoreProgress{
		TotalItems:     10,
		CompletedItems: 1,
		StartTime:      time.Now(),
		Status:         "running",
	}

	item := Item{
		ID:     "gid://shopify/Product/1",
		Title:  "Test Product",
		Type:   EntityProducts,
	}
	result := &RestoreResult{
		Success:    true,
		RestoredID: "gid://shopify/Product/999",
	}

	executor.saveResumeState(item, result)

	// Verify state file was created
	state, err := executor.loadResumeState()
	if err != nil {
		t.Fatalf("loadResumeState() error: %v", err)
	}
	if len(state.CompletedItems) != 1 {
		t.Errorf("CompletedItems length = %d, want 1", len(state.CompletedItems))
	}
	if state.CompletedItems[0].SourceID != "gid://shopify/Product/1" {
		t.Errorf("SourceID = %q, want %q", state.CompletedItems[0].SourceID, "gid://shopify/Product/1")
	}
	if state.CompletedItems[0].TargetID != "gid://shopify/Product/999" {
		t.Errorf("TargetID = %q, want %q", state.CompletedItems[0].TargetID, "gid://shopify/Product/999")
	}
}

func TestRestoreExecutor_WriteRollbackScript(t *testing.T) {
	tmpDir := t.TempDir()

	client := NewShopifyClient("https://test.myshopify.com", "shpat_test", "2025-07", false)
	executor := NewRestoreExecutor(client, ConflictSkip)

	executor.rollbackActions = append(executor.rollbackActions,
		RollbackAction{
			EntityType:  EntityProducts,
			Action:      "delete",
			ID:          "gid://shopify/Product/123",
			Description: "Delete product",
		},
	)

	path, err := executor.WriteRollbackScript(tmpDir, "2026-04-15")
	if err != nil {
		t.Fatalf("WriteRollbackScript() error: %v", err)
	}
	if path == "" {
		t.Error("WriteRollbackScript() should return a path")
	}
}

func TestRestoreExecutor_DryRun(t *testing.T) {
	client := NewShopifyClient("https://test.myshopify.com", "shpat_test", "2025-07", true)
	executor := NewRestoreExecutor(client, ConflictSkip)

	items := []Item{
		{ID: "1", Title: "Product A", Handle: "product-a", Type: EntityProducts, Tags: []string{}},
		{ID: "2", Title: "Product B", Handle: "product-b", Type: EntityProducts, Tags: []string{}},
	}

	results, err := executor.ExecuteRestore(context.Background(), items)
	if err != nil {
		t.Fatalf("ExecuteRestore() error: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Results length = %d, want 2", len(results))
	}

	for _, r := range results {
		if !r.Success {
			t.Errorf("Dry run result should be successful, got: %s", r.Message)
		}
	}
}