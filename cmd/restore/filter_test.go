package main

import (
	"testing"
	"time"
)

func TestFilterCriteriaMatch(t *testing.T) {
	item := Item{
		ID:        "123",
		Title:     "Test Product",
		Handle:    "test-product",
		CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Status:    "active",
		Tags:      []string{"tag1", "tag2"},
	}

	tests := []struct {
		name     string
		criteria FilterCriteria
		want     bool
	}{
		{
			name:     "no filters",
			criteria: FilterCriteria{},
			want:     true,
		},
		{
			name: "search text matches title",
			criteria: FilterCriteria{
				SearchText: "test",
			},
			want: true,
		},
		{
			name: "search text matches handle",
			criteria: FilterCriteria{
				SearchText: "product",
			},
			want: true,
		},
		{
			name: "search text no match",
			criteria: FilterCriteria{
				SearchText: "xyz",
			},
			want: false,
		},
		{
			name: "status matches",
			criteria: FilterCriteria{
				Statuses: []string{"active"},
			},
			want: true,
		},
		{
			name: "status no match",
			criteria: FilterCriteria{
				Statuses: []string{"archived"},
			},
			want: false,
		},
		{
			name: "tag matches",
			criteria: FilterCriteria{
				Tags: []string{"tag1"},
			},
			want: true,
		},
		{
			name: "tag no match",
			criteria: FilterCriteria{
				Tags: []string{"tag3"},
			},
			want: false,
		},
		{
			name: "date range matches",
			criteria: FilterCriteria{
				DateFrom: func() *time.Time { t := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC); return &t }(),
				DateTo:   func() *time.Time { t := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC); return &t }(),
			},
			want: true,
		},
		{
			name: "date range before",
			criteria: FilterCriteria{
				DateFrom: func() *time.Time { t := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC); return &t }(),
				DateTo:   func() *time.Time { t := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC); return &t }(),
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := &Model{}
			got := model.matchesFilter(item, tt.criteria)
			if got != tt.want {
				t.Errorf("matchesFilter() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConflictResolution(t *testing.T) {
	resolver := NewConflictResolver()

	item := Item{
		ID:    "123",
		Title: "Test Product",
	}

	// Test skip mode
	res := resolver.ResolveConflict(item, "existing-id", ConflictSkip)
	if res.Resolution != ConflictSkip {
		t.Errorf("Expected ConflictSkip, got %v", res.Resolution)
	}
	if res.NewID != "existing-id" {
		t.Errorf("Expected existing-id, got %v", res.NewID)
	}

	// Test overwrite mode
	res = resolver.ResolveConflict(item, "existing-id", ConflictOverwrite)
	if res.Resolution != ConflictOverwrite {
		t.Errorf("Expected ConflictOverwrite, got %v", res.Resolution)
	}

	// Test rename mode
	res = resolver.ResolveConflict(item, "existing-id", ConflictRename)
	if res.Resolution != ConflictRename {
		t.Errorf("Expected ConflictRename, got %v", res.Resolution)
	}
}

func TestRateLimiter(t *testing.T) {
	limiter := NewRateLimiter(10)

	// Initial state should have max tokens
	if limiter.tokens != 10 {
		t.Errorf("Expected 10 tokens, got %d", limiter.tokens)
	}

	// Consume all tokens
	for i := 0; i < 10; i++ {
		if limiter.tokens == 0 {
			t.Errorf("Ran out of tokens too early at iteration %d", i)
		}
		limiter.tokens--
	}

	if limiter.tokens != 0 {
		t.Errorf("Expected 0 tokens, got %d", limiter.tokens)
	}
}

func TestEntityDisplayNames(t *testing.T) {
	tests := []struct {
		entityType EntityType
		want       string
	}{
		{EntityProducts, "Products"},
		{EntityCustomers, "Customers"},
		{EntityOrders, "Orders"},
		{EntityCollections, "Collections"},
		{EntityMetaobjects, "Metaobjects"},
	}

	for _, tt := range tests {
		t.Run(tt.entityType.String(), func(t *testing.T) {
			if got := EntityDisplayNames[tt.entityType]; got != tt.want {
				t.Errorf("EntityDisplayNames[%v] = %v, want %v", tt.entityType, got, tt.want)
			}
		})
	}
}

func TestStateString(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{StateConfig, "Config"},
		{StateBackupSelect, "Backup Select"},
		{StateEntitySelect, "Entity Select"},
		{StateItemSelect, "Item Select"},
		{StatePreview, "Preview"},
		{StateConfirm, "Confirm"},
		{StateRunning, "Running"},
		{StateComplete, "Complete"},
		{StateError, "Error"},
		{StateAbort, "Abort"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.state.String(); got != tt.want {
				t.Errorf("State.String() = %v, want %v", got, tt.want)
			}
		})
	}
}
