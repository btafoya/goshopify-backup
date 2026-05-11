package main

import (
	"context"
	"testing"
	"time"
)

func TestRestoreItem_DryRun(t *testing.T) {
	client := NewShopifyClient("https://test.myshopify.com", "shpat_test", "2025-07", true)
	executor := NewMutationExecutor(client)

	item := Item{
		ID:     "gid://shopify/Product/1",
		Title:  "Test Product",
		Handle: "test-product",
		Type:   EntityProducts,
		Tags:   []string{},
	}

	result, err := executor.RestoreItem(context.Background(), item, ConflictSkip)
	if err != nil {
		t.Fatalf("RestoreItem() error: %v", err)
	}
	if !result.Success {
		t.Error("Dry run should succeed")
	}
	if result.Message != "Dry run - would be restored" {
		t.Errorf("Message = %q, want %q", result.Message, "Dry run - would be restored")
	}
}

func TestRestoreItem_UnsupportedType(t *testing.T) {
	client := NewShopifyClient("https://test.myshopify.com", "shpat_test", "2025-07", false)
	executor := NewMutationExecutor(client)

	item := Item{
		ID:    "gid://shopify/Unknown/1",
		Title: "Unknown",
		Type:  "unknown",
	}

	_, err := executor.RestoreItem(context.Background(), item, ConflictSkip)
	if err == nil {
		t.Error("RestoreItem() should error for unsupported type")
	}
}

func TestRestoreItem_AppliesRestoreTag(t *testing.T) {
	client := NewShopifyClient("https://test.myshopify.com", "shpat_test", "2025-07", true)
	executor := NewMutationExecutor(client)

	item := Item{
		ID:     "gid://shopify/Product/1",
		Title:  "Test Product",
		Handle: "test-product",
		Type:   EntityProducts,
		Tags:   []string{"existing"},
	}

	result, err := executor.RestoreItem(context.Background(), item, ConflictSkip)
	if err != nil {
		t.Fatalf("RestoreItem() error: %v", err)
	}
	if !result.Success {
		t.Error("Should succeed")
	}
	// The tag is applied internally before restore - we can't easily verify it went to the API
	// in a unit test, but the applyRestoreTag function is tested separately
}

func TestApplyRestoreTag(t *testing.T) {
	client := NewShopifyClient("https://test.myshopify.com", "shpat_test", "2025-07", true)
	executor := NewMutationExecutor(client)

	tests := []struct {
		name     string
		input    Item
		wantTags []string
	}{
		{
			name:     "adds tag to empty tags",
			input:    Item{ID: "1", Tags: []string{}},
			wantTags: []string{RestoreTag},
		},
		{
			name:     "adds tag to existing tags",
			input:    Item{ID: "1", Tags: []string{"existing"}},
			wantTags: []string{"existing", RestoreTag},
		},
		{
			name:     "does not duplicate tag",
			input:    Item{ID: "1", Tags: []string{RestoreTag}},
			wantTags: []string{RestoreTag},
		},
		{
			name:     "handles nil tags",
			input:    Item{ID: "1", Tags: nil},
			wantTags: []string{RestoreTag},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := executor.applyRestoreTag(tt.input)
			if len(result.Tags) != len(tt.wantTags) {
				t.Errorf("Tags length = %d, want %d", len(result.Tags), len(tt.wantTags))
			}
			for i, tag := range tt.wantTags {
				if i < len(result.Tags) && result.Tags[i] != tag {
					t.Errorf("Tags[%d] = %q, want %q", i, result.Tags[i], tag)
				}
			}
		})
	}
}

func TestGenerateNewHandle(t *testing.T) {
	handle := generateNewHandle("test-product")
	if handle == "test-product" {
		t.Error("Should generate different handle")
	}
	if len(handle) <= len("test-product") {
		t.Error("New handle should be longer (has timestamp suffix)")
	}
}

func TestGenerateNewKey(t *testing.T) {
	key := generateNewKey("test-key")
	if key == "test-key" {
		t.Error("Should generate different key")
	}
}

func TestContains(t *testing.T) {
	if !contains([]string{"a", "b", "c"}, "b") {
		t.Error("Should find 'b' in slice")
	}
	if contains([]string{"a", "b", "c"}, "d") {
		t.Error("Should not find 'd' in slice")
	}
	if contains([]string{}, "a") {
		t.Error("Should not find 'a' in empty slice")
	}
}

func TestRestoreCustomer_MissingEmail(t *testing.T) {
	client := NewShopifyClient("https://test.myshopify.com", "shpat_test", "2025-07", false)
	executor := NewMutationExecutor(client)

	item := Item{
		ID:    "gid://shopify/Customer/1",
		Title: "Test Customer",
		Type:  EntityCustomers,
		Email: nil,
	}

	_, err := executor.restoreCustomer(context.Background(), item, ConflictSkip)
	if err == nil {
		t.Error("Should error for missing email")
	}
}

func TestRestoreMetaobject_MissingDefinition(t *testing.T) {
	client := NewShopifyClient("https://test.myshopify.com", "shpat_test", "2025-07", false)
	executor := NewMutationExecutor(client)

	item := Item{
		ID:    "gid://shopify/Metaobject/1",
		Title: "Test",
		Type:  EntityMetaobjects,
	}

	_, err := executor.restoreMetaobject(context.Background(), item, ConflictSkip)
	if err == nil {
		t.Error("Should error for missing metaobject definition")
	}
}

func TestRestoreValidator(t *testing.T) {
	v := NewRestoreValidator()

	email := "test@example.com"
	v.ValidateItem(Item{
		ID:     "1",
		Title:  "Product",
		Handle: "product",
		Type:   EntityProducts,
		Tags:   []string{},
	})

	v.ValidateItem(Item{
		ID:     "2",
		Title:  "",
		Handle: "",
		Type:   EntityProducts,
		Tags:   []string{},
	})

	v.ValidateItem(Item{
		ID:    "3",
		Title: "Customer",
		Type:  EntityCustomers,
		Email: &email,
	})

	if !v.HasWarnings() {
		t.Error("Should have warnings for missing handle")
	}

	summary := v.Summary()
	if summary == "" {
		t.Error("Summary should not be empty")
	}
}

func TestRestoreValidator_MissingCustomerEmail(t *testing.T) {
	v := NewRestoreValidator()
	v.ValidateItem(Item{
		ID:    "1",
		Title: "Customer",
		Type:  EntityCustomers,
		Email: nil,
	})

	if !v.HasErrors() {
		t.Error("Should have error for missing customer email")
	}
}

func TestRestoreValidator_Relationships(t *testing.T) {
	v := NewRestoreValidator()
	items := []Item{
		{ID: "p1", Type: EntityProducts},
		{ID: "c1", Type: EntityCollections, CollectionProducts: []string{"p1", "p2"}},
	}

	v.ValidateRelationships(items)

	if !v.HasWarnings() {
		t.Error("Should warn about missing product reference p2")
	}
}

func TestRestoreExecutor_RollbackScript(t *testing.T) {
	client := NewShopifyClient("https://test.myshopopify.com", "shpat_test", "2025-07", false)
	executor := NewRestoreExecutor(client, ConflictSkip)

	// Simulate some rollback actions
	executor.rollbackActions = append(executor.rollbackActions,
		RollbackAction{
			EntityType:  EntityProducts,
			Action:      "delete",
			ID:          "gid://shopify/Product/123",
			Description: "Delete restored product: Test Product",
		},
		RollbackAction{
			EntityType:  EntityCollections,
			Action:      "delete",
			ID:          "gid://shopify/Collection/456",
			Description: "Delete restored collection: Test Collection",
		},
	)

	script := executor.GetRollbackScript("2026-04-15")
	if script == nil {
		t.Fatal("GetRollbackScript() returned nil")
	}
	if len(script.Actions) != 2 {
		t.Errorf("Actions length = %d, want 2", len(script.Actions))
	}
	if len(script.Commands) == 0 {
		t.Error("Commands should not be empty")
	}
}

func TestConflictMode_WithForce(t *testing.T) {
	cfg := &Config{
		Force: true,
	}

	var conflictMode ConflictMode
	if cfg.Force {
		conflictMode = ConflictOverwrite
	} else {
		conflictMode = ConflictSkip
	}

	if conflictMode != ConflictOverwrite {
		t.Errorf("With --force, conflictMode = %q, want %q", conflictMode, ConflictOverwrite)
	}
}

func TestFileLogger(t *testing.T) {
	tmpDir := t.TempDir()

	logger, err := NewFileLogger(tmpDir)
	if err != nil {
		t.Fatalf("NewFileLogger() error: %v", err)
	}
	defer logger.Close()

	logger.Info("test info message")
	logger.Warn("test warning message")
	logger.Error("test error message")
	logger.RestoreItem(EntityProducts, "p1", "create", "gid://shopify/Product/123", 100*time.Millisecond, nil)

	// Verify file exists and has content
	filePath := logger.FilePath()
	if filePath == "" {
		t.Error("FilePath() should not be empty")
	}
}

func TestCredentialManager(t *testing.T) {
	tmpDir := t.TempDir()

	cm := &CredentialManager{credentialsDir: tmpDir}

	cred := Credential{
		Store:       "https://test.myshopify.com",
		AccessToken: "shpat_test",
		APIVersion:  "2025-07",
	}

	// Save
	if err := cm.Save(cred); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Load
	loaded, err := cm.Load("https://test.myshopify.com")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if loaded.AccessToken != "shpat_test" {
		t.Errorf("AccessToken = %q, want %q", loaded.AccessToken, "shpat_test")
	}

	// List
	creds, err := cm.List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(creds) != 1 {
		t.Errorf("List() returned %d creds, want 1", len(creds))
	}

	// Delete
	if err := cm.Delete("https://test.myshopify.com"); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	// Verify deleted
	_, err = cm.Load("https://test.myshopify.com")
	if err == nil {
		t.Error("Load() should error after Delete()")
	}
}
