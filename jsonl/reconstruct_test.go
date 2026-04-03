package jsonl

import (
	"testing"
)

func TestReconstructBulkData_SimpleOrder(t *testing.T) {
	records := []map[string]interface{}{
		{
			"id":         "gid://shopify/Order/123",
			"__typename": "Order",
			"name":       "#1234",
		},
		{
			"id":         "gid://shopify/LineItem/1",
			"__parentId": "gid://shopify/Order/123",
			"__typename": "LineItem",
			"title":      "Test Product",
			"quantity":   2,
		},
		{
			"id":         "gid://shopify/LineItem/2",
			"__parentId": "gid://shopify/Order/123",
			"__typename": "LineItem",
			"title":      "Test Product 2",
			"quantity":   1,
		},
	}

	result, err := ReconstructBulkData(records)
	if err != nil {
		t.Fatalf("ReconstructBulkData() error = %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("got %d root objects, want 1", len(result))
	}

	order := result[0]
	if order["id"] != "gid://shopify/Order/123" {
		t.Errorf("Order id = %v, want gid://shopify/Order/123", order["id"])
	}

	// Check that line items are attached
	lineItems, ok := order["lineItems"].([]map[string]interface{})
	if !ok {
		t.Fatal("lineItems not found or wrong type")
	}

	if len(lineItems) != 2 {
		t.Fatalf("got %d line items, want 2", len(lineItems))
	}

	if lineItems[0]["title"] != "Test Product" {
		t.Errorf("First line item title = %v, want Test Product", lineItems[0]["title"])
	}
}

func TestReconstructBulkData_ProductWithVariants(t *testing.T) {
	records := []map[string]interface{}{
		{
			"id":         "gid://shopify/Product/1",
			"__typename": "Product",
			"title":      "Test Product",
		},
		{
			"id":         "gid://shopify/ProductVariant/1",
			"__parentId": "gid://shopify/Product/1",
			"__typename": "ProductVariant",
			"title":      "Default Title",
		},
		{
			"id":         "gid://shopify/ProductVariant/2",
			"__parentId": "gid://shopify/Product/1",
			"__typename": "ProductVariant",
			"title":      "Variant 2",
		},
		{
			"id":         "gid://shopify/ProductImage/1",
			"__parentId": "gid://shopify/Product/1",
			"__typename": "ProductImage",
			"src":        "https://example.com/image.jpg",
		},
	}

	result, err := ReconstructBulkData(records)
	if err != nil {
		t.Fatalf("ReconstructBulkData() error = %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("got %d root objects, want 1", len(result))
	}

	product := result[0]
	if product["title"] != "Test Product" {
		t.Errorf("Product title = %v, want Test Product", product["title"])
	}

	// Check variants
	variants, ok := product["variants"].([]map[string]interface{})
	if !ok {
		t.Fatal("variants not found or wrong type")
	}

	if len(variants) != 2 {
		t.Fatalf("got %d variants, want 2", len(variants))
	}

	// Check images
	images, ok := product["images"].([]map[string]interface{})
	if !ok {
		t.Fatal("images not found or wrong type")
	}

	if len(images) != 1 {
		t.Fatalf("got %d images, want 1", len(images))
	}

	if images[0]["src"] != "https://example.com/image.jpg" {
		t.Errorf("Image src = %v, want https://example.com/image.jpg", images[0]["src"])
	}
}

func TestReconstructBulkData_CustomerWithAddress(t *testing.T) {
	records := []map[string]interface{}{
		{
			"id":         "gid://shopify/Customer/1",
			"__typename": "Customer",
			"email":      "test@example.com",
		},
		{
			"id":         "gid://shopify/MailingAddress/1",
			"__parentId": "gid://shopify/Customer/1",
			"__typename": "MailingAddress",
			"address1":   "123 Test St",
			"city":       "Test City",
		},
	}

	result, err := ReconstructBulkData(records)
	if err != nil {
		t.Fatalf("ReconstructBulkData() error = %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("got %d root objects, want 1", len(result))
	}

	customer := result[0]
	if customer["email"] != "test@example.com" {
		t.Errorf("Customer email = %v, want test@example.com", customer["email"])
	}

	// Check addresses
	addresses, ok := customer["addresses"].([]map[string]interface{})
	if !ok {
		t.Fatal("addresses not found or wrong type")
	}

	if len(addresses) != 1 {
		t.Fatalf("got %d addresses, want 1", len(addresses))
	}

	if addresses[0]["address1"] != "123 Test St" {
		t.Errorf("Address address1 = %v, want 123 Test St", addresses[0]["address1"])
	}
}

func TestReconstructBulkData_EmptyRecords(t *testing.T) {
	records := []map[string]interface{}{}

	result, err := ReconstructBulkData(records)
	if err != nil {
		t.Fatalf("ReconstructBulkData() error = %v", err)
	}

	if len(result) != 0 {
		t.Errorf("got %d root objects, want 0", len(result))
	}
}

func TestReconstructBulkData_OrphanChild(t *testing.T) {
	records := []map[string]interface{}{
		{
			"id":         "gid://shopify/Order/123",
			"__typename": "Order",
		},
		{
			"id":         "gid://shopify/LineItem/1",
			"__parentId": "gid://shopify/Order/999", // Non-existent parent
			"__typename": "LineItem",
		},
	}

	result, err := ReconstructBulkData(records)
	if err != nil {
		t.Fatalf("ReconstructBulkData() error = %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("got %d root objects, want 1 (orphan child should be ignored)", len(result))
	}
}

func TestNormalizeJSONL(t *testing.T) {
	data := []map[string]interface{}{
		{
			"id":         "gid://shopify/Order/123",
			"__typename": "Order",
			"name":       "#1234",
		},
	}

	result, err := NormalizeJSONL(data)
	if err != nil {
		t.Fatalf("NormalizeJSONL() error = %v", err)
	}

	if len(result) == 0 {
		t.Fatal("NormalizeJSONL() returned empty result")
	}

	resultStr := string(result)
	if containsSubstring(resultStr, `"__typename"`) {
		t.Error("NormalizeJSONL() should remove __typename field")
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && findString(s, substr)
}

func findString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
