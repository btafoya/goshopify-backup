package backup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewLoader(t *testing.T) {
	loader := NewLoader("/backups/shopify")
	if loader.backupDir != "/backups/shopify" {
		t.Errorf("backupDir = %q, want %q", loader.backupDir, "/backups/shopify")
	}
}

func TestListBackups_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	loader := NewLoader(tmpDir)

	backups, err := loader.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups() error: %v", err)
	}
	if len(backups) != 0 {
		t.Errorf("ListBackups() returned %d backups, want 0", len(backups))
	}
}

func TestListBackups_WithValidBackups(t *testing.T) {
	tmpDir := t.TempDir()

	// Create backup directories with status.json
	for _, date := range []string{"2026-04-15", "2026-04-14", "2026-04-13"} {
		backupPath := filepath.Join(tmpDir, date)
		os.MkdirAll(backupPath, 0755)

		status := BackupStatus{
			StartedAt:   time.Now(),
			CompletedAt: time.Now(),
			Duration:    "5m",
			Modules:     map[string]ModuleStatus{},
		}
		data, _ := json.Marshal(status)
		os.WriteFile(filepath.Join(backupPath, "status.json"), data, 0644)
	}

	loader := NewLoader(tmpDir)
	backups, err := loader.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups() error: %v", err)
	}
	if len(backups) != 3 {
		t.Errorf("ListBackups() returned %d backups, want 3", len(backups))
	}

	// Should be sorted newest first
	if !backups[0].Date.After(backups[1].Date) {
		t.Error("Backups should be sorted newest first")
	}
}

func TestListBackups_SkipsNonDateDirs(t *testing.T) {
	tmpDir := t.TempDir()

	// Create valid and invalid directories
	os.MkdirAll(filepath.Join(tmpDir, "2026-04-15"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "not-a-date"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "random_dir"), 0755)

	// Add status.json to valid one
	status := BackupStatus{StartedAt: time.Now(), CompletedAt: time.Now()}
	data, _ := json.Marshal(status)
	os.WriteFile(filepath.Join(tmpDir, "2026-04-15", "status.json"), data, 0644)

	loader := NewLoader(tmpDir)
	backups, err := loader.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups() error: %v", err)
	}
	if len(backups) != 1 {
		t.Errorf("ListBackups() returned %d backups, want 1", len(backups))
	}
}

func TestLoadEntity_Products(t *testing.T) {
	tmpDir := t.TempDir()
	dateDir := filepath.Join(tmpDir, "2026-04-15")
	os.MkdirAll(dateDir, 0755)

	products := []Product{
		{
			ID:          "gid://shopify/Product/1",
			Title:       "Test Product",
			Handle:      "test-product",
			Description: "A test product",
			Vendor:      "Test Vendor",
			ProductType: "Shoes",
			Tags:        []string{"tag1", "tag2"},
			Status:      "active",
			Variants: []ProductVariant{
				{ID: "v1", Title: "Default", Price: "29.99", SKU: "SKU-1"},
			},
			Images: []ProductImage{
				{ID: "img1", Src: "https://cdn.shopify.com/test.jpg", AltText: "Test Image"},
			},
			Metafields: []Metafield{
				{ID: "mf1", Namespace: "custom", Key: "size", Value: "large", Type: "single_line_text_field"},
			},
		},
	}

	data, _ := json.Marshal(products)
	os.WriteFile(filepath.Join(dateDir, "products.json"), data, 0644)

	loader := NewLoader(tmpDir)
	items, err := loader.LoadEntity("2026-04-15", EntityProducts)
	if err != nil {
		t.Fatalf("LoadEntity() error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("LoadEntity() returned %d items, want 1", len(items))
	}

	item := items[0]
	if item.ID != "gid://shopify/Product/1" {
		t.Errorf("ID = %q, want %q", item.ID, "gid://shopify/Product/1")
	}
	if item.Title != "Test Product" {
		t.Errorf("Title = %q, want %q", item.Title, "Test Product")
	}
	if item.Price == nil || *item.Price != "29.99" {
		t.Errorf("Price = %v, want 29.99", item.Price)
	}
	if item.VariantCount == nil || *item.VariantCount != 1 {
		t.Errorf("VariantCount = %v, want 1", item.VariantCount)
	}
	// Verify rich data is preserved
	if len(item.Variants) != 1 {
		t.Errorf("Variants length = %d, want 1", len(item.Variants))
	}
	if len(item.Metafields) != 1 {
		t.Errorf("Metafields length = %d, want 1", len(item.Metafields))
	}
	if len(item.Images) != 1 {
		t.Errorf("Images length = %d, want 1", len(item.Images))
	}
}

func TestLoadEntity_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	dateDir := filepath.Join(tmpDir, "2026-04-15")
	os.MkdirAll(dateDir, 0755)

	loader := NewLoader(tmpDir)
	items, err := loader.LoadEntity("2026-04-15", EntityProducts)
	if err != nil {
		t.Fatalf("LoadEntity() error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("LoadEntity() returned %d items, want 0 for missing file", len(items))
	}
}

func TestLoadEntity_Customers(t *testing.T) {
	tmpDir := t.TempDir()
	dateDir := filepath.Join(tmpDir, "2026-04-15")
	os.MkdirAll(dateDir, 0755)

	customers := []Customer{
		{
			ID:        "gid://shopify/Customer/1",
			Email:     "test@example.com",
			FirstName: "John",
			LastName:  "Doe",
			Phone:     "555-1234",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			State:     "enabled",
			Addresses: []CustomerAddress{
				{ID: "a1", Address1: "123 Main St", City: "Springfield", Province: "IL", Country: "US", Zip: "62701"},
			},
			Metafields: []Metafield{
				{ID: "mf1", Namespace: "custom", Key: "loyalty", Value: "gold", Type: "single_line_text_field"},
			},
		},
	}

	data, _ := json.Marshal(customers)
	os.WriteFile(filepath.Join(dateDir, "customers.json"), data, 0644)

	loader := NewLoader(tmpDir)
	items, err := loader.LoadEntity("2026-04-15", EntityCustomers)
	if err != nil {
		t.Fatalf("LoadEntity() error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("LoadEntity() returned %d items, want 1", len(items))
	}

	item := items[0]
	if item.Email == nil || *item.Email != "test@example.com" {
		t.Errorf("Email = %v, want test@example.com", item.Email)
	}
	if len(item.Addresses) != 1 {
		t.Errorf("Addresses length = %d, want 1", len(item.Addresses))
	}
	if len(item.Metafields) != 1 {
		t.Errorf("Metafields length = %d, want 1", len(item.Metafields))
	}
}

func TestLoadEntity_Collections(t *testing.T) {
	tmpDir := t.TempDir()
	dateDir := filepath.Join(tmpDir, "2026-04-15")
	os.MkdirAll(dateDir, 0755)

	collections := []Collection{
		{
			ID:          "gid://shopify/Collection/1",
			Title:       "Summer Collection",
			Handle:      "summer-collection",
			Description: "Summer items",
			Products: []CollectionProduct{
				{ID: "gid://shopify/Product/1"},
				{ID: "gid://shopify/Product/2"},
			},
			Metafields: []Metafield{
				{ID: "mf1", Namespace: "custom", Key: "season", Value: "summer", Type: "single_line_text_field"},
			},
		},
	}

	data, _ := json.Marshal(collections)
	os.WriteFile(filepath.Join(dateDir, "collections.json"), data, 0644)

	loader := NewLoader(tmpDir)
	items, err := loader.LoadEntity("2026-04-15", EntityCollections)
	if err != nil {
		t.Fatalf("LoadEntity() error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("LoadEntity() returned %d items, want 1", len(items))
	}

	item := items[0]
	if len(item.CollectionProducts) != 2 {
		t.Errorf("CollectionProducts length = %d, want 2", len(item.CollectionProducts))
	}
	if len(item.Metafields) != 1 {
		t.Errorf("Metafields length = %d, want 1", len(item.Metafields))
	}
}

func TestLoadEntity_Metaobjects(t *testing.T) {
	tmpDir := t.TempDir()
	dateDir := filepath.Join(tmpDir, "2026-04-15")
	metaDir := filepath.Join(dateDir, "metaobjects")
	os.MkdirAll(metaDir, 0755)

	// Write definitions
	defs := []MetaobjectDefinition{
		{ID: "def1", Type: "custom_type", Name: "Custom Type", FieldDefinitions: []FieldDefinition{}},
	}
	defData, _ := json.Marshal(defs)
	os.WriteFile(filepath.Join(metaDir, "metaobject-definitions.json"), defData, 0644)

	// Write entries
	entries := []MetaobjectEntry{
		{ID: "e1", Handle: "entry-1", Type: "custom_type", Fields: []MetaobjectField{{Key: "name", Value: "Test"}}},
	}
	entryData, _ := json.Marshal(entries)
	os.WriteFile(filepath.Join(metaDir, "custom_type.json"), entryData, 0644)

	loader := NewLoader(tmpDir)
	items, err := loader.LoadEntity("2026-04-15", EntityMetaobjects)
	if err != nil {
		t.Fatalf("LoadEntity() error: %v", err)
	}

	// Should have 1 definition + 1 entry = 2 items
	if len(items) != 2 {
		t.Errorf("LoadEntity() returned %d items, want 2", len(items))
	}
}

func TestLoadEntity_UnsupportedType(t *testing.T) {
	tmpDir := t.TempDir()
	loader := NewLoader(tmpDir)

	_, err := loader.LoadEntity("2026-04-15", "pages")
	if err == nil {
		t.Error("LoadEntity() should error for unsupported type")
	}
}

func TestProductToItem(t *testing.T) {
	p := Product{
		ID:          "1",
		Title:       "Test",
		Handle:      "test",
		Description: "Desc",
		Vendor:      "V",
		ProductType: "Type",
		Tags:        []string{"a"},
		Status:      "active",
		Variants: []ProductVariant{
			{ID: "v1", Title: "V1", Price: "10.00"},
			{ID: "v2", Title: "V2", Price: "20.00"},
		},
		Metafields: []Metafield{
			{Namespace: "custom", Key: "test", Value: "val", Type: "string"},
		},
	}

	item := p.ToItem()
	if item.ID != "1" {
		t.Errorf("ID = %q, want %q", item.ID, "1")
	}
	if item.Price == nil || *item.Price != "10.00" {
		t.Errorf("Price = %v, want 10.00 (first variant)", item.Price)
	}
	if item.VariantCount == nil || *item.VariantCount != 2 {
		t.Errorf("VariantCount = %v, want 2", item.VariantCount)
	}
	if len(item.Variants) != 2 {
		t.Errorf("Variants length = %d, want 2", len(item.Variants))
	}
	if len(item.Metafields) != 1 {
		t.Errorf("Metafields length = %d, want 1", len(item.Metafields))
	}
}

func TestCustomerToItem(t *testing.T) {
	c := Customer{
		ID:        "1",
		Email:     "test@test.com",
		FirstName: "John",
		LastName:  "Doe",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		State:     "enabled",
		Addresses: []CustomerAddress{
			{ID: "a1", Address1: "123 St", City: "NYC", Country: "US", Zip: "10001"},
		},
		Metafields: []Metafield{
			{Namespace: "custom", Key: "tier", Value: "gold", Type: "string"},
		},
	}

	item := c.ToItem()
	if item.Email == nil || *item.Email != "test@test.com" {
		t.Errorf("Email = %v, want test@test.com", item.Email)
	}
	if len(item.Addresses) != 1 {
		t.Errorf("Addresses length = %d, want 1", len(item.Addresses))
	}
	if len(item.Metafields) != 1 {
		t.Errorf("Metafields length = %d, want 1", len(item.Metafields))
	}
}

func TestCollectionToItem(t *testing.T) {
	c := Collection{
		ID:     "1",
		Title:  "Test",
		Handle: "test",
		Products: []CollectionProduct{
			{ID: "p1"},
			{ID: "p2"},
		},
		Metafields: []Metafield{
			{Namespace: "custom", Key: "season", Value: "summer", Type: "string"},
		},
	}

	item := c.ToItem()
	if item.ProductsCount == nil || *item.ProductsCount != 2 {
		t.Errorf("ProductsCount = %v, want 2", item.ProductsCount)
	}
	if len(item.CollectionProducts) != 2 {
		t.Errorf("CollectionProducts length = %d, want 2", len(item.CollectionProducts))
	}
	if len(item.Metafields) != 1 {
		t.Errorf("Metafields length = %d, want 1", len(item.Metafields))
	}
}