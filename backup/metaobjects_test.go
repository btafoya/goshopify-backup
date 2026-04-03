package backup

import (
	"testing"
)

func TestNewMetaobjectsModule(t *testing.T) {
	mod := NewMetaobjectsModule()
	if mod == nil {
		t.Fatal("NewMetaobjectsModule() returned nil")
	}
	if mod.Name() != "metaobjects" {
		t.Errorf("Name() = %v, want metaobjects", mod.Name())
	}
}

func TestSanitizeTypeName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple type",
			input:    "size_chart",
			expected: "size_chart.json",
		},
		{
			name:     "app prefix",
			input:    "$app:product_highlight",
			expected: "app-product_highlight.json",
		},
		{
			name:     "shopify prefix",
			input:    "shopify--color-pattern",
			expected: "shopify-color-pattern.json",
		},
		{
			name:     "double dash",
			input:    "test--name",
			expected: "test-name.json",
		},
		{
			name:     "multiple double dashes",
			input:    "$app:test--name",
			expected: "app-test-name.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeTypeName(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeTypeName(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseMetaobjectEntry(t *testing.T) {
	raw := map[string]interface{}{
		"id":          "gid://shopify/Metaobject/123",
		"handle":      "test-handle",
		"displayName": "Test Entry",
		"type":        "test_type",
		"fields": []interface{}{
			map[string]interface{}{
				"key":   "title",
				"value": "Test Title",
				"type":  "single_line_text_field",
			},
			map[string]interface{}{
				"key":       "data",
				"value":     `{"key": "value"}`,
				"type":      "json",
				"jsonValue": map[string]interface{}{"key": "value"},
			},
		},
		"capabilities": map[string]interface{}{
			"publishable": map[string]interface{}{
				"status": "ACTIVE",
			},
		},
		"updatedAt": "2026-04-03T00:00:00Z",
		"createdAt": "2026-01-01T00:00:00Z",
	}

	entry, ok := parseMetaobjectEntry(raw)
	if !ok {
		t.Fatal("parseMetaobjectEntry() returned false")
	}

	if entry.ID != "gid://shopify/Metaobject/123" {
		t.Errorf("ID = %v, want gid://shopify/Metaobject/123", entry.ID)
	}
	if entry.Handle != "test-handle" {
		t.Errorf("Handle = %v, want test-handle", entry.Handle)
	}
	if entry.Type != "test_type" {
		t.Errorf("Type = %v, want test_type", entry.Type)
	}

	if len(entry.Fields) != 2 {
		t.Fatalf("Fields count = %v, want 2", len(entry.Fields))
	}

	if entry.Fields[0].Key != "title" {
		t.Errorf("Fields[0].Key = %v, want title", entry.Fields[0].Key)
	}

	if entry.Capabilities == nil {
		t.Error("Capabilities should not be nil")
	}
	if entry.Capabilities.Publishable == nil {
		t.Error("Publishable capability should not be nil")
	}
	if entry.Capabilities.Publishable.Status != "ACTIVE" {
		t.Errorf("Publishable.Status = %v, want ACTIVE", entry.Capabilities.Publishable.Status)
	}
}

func TestParseMetaobjectEntry_MissingID(t *testing.T) {
	raw := map[string]interface{}{
		"handle": "test-handle",
		"type":   "test_type",
	}

	_, ok := parseMetaobjectEntry(raw)
	if ok {
		t.Error("parseMetaobjectEntry() should return false for missing ID")
	}
}

func TestParseMetaobjectEntry_MissingType(t *testing.T) {
	raw := map[string]interface{}{
		"id":     "gid://shopify/Metaobject/123",
		"handle": "test-handle",
	}

	_, ok := parseMetaobjectEntry(raw)
	if ok {
		t.Error("parseMetaobjectEntry() should return false for missing type")
	}
}
