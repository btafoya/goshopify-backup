package main

import (
	"fmt"
)

// ConflictMode determines how to handle conflicts
type ConflictMode string

const (
	ConflictSkip      ConflictMode = "skip"      // Skip conflicting items
	ConflictOverwrite ConflictMode = "overwrite" // Overwrite existing items
	ConflictRename    ConflictMode = "rename"    // Rename to avoid conflicts
)

// ConflictResolution represents a resolved conflict
type ConflictResolution struct {
	ItemID       string
	ConflictType string
	Resolution   ConflictMode
	OriginalID   string
	NewID        string
	Message      string
}

// ConflictResolver handles conflict detection and resolution
type ConflictResolver struct {
	resolutions []ConflictResolution
}

// NewConflictResolver creates a new conflict resolver
func NewConflictResolver() *ConflictResolver {
	return &ConflictResolver{
		resolutions: make([]ConflictResolution, 0),
	}
}

// ResolveConflict resolves a conflict based on the mode
func (r *ConflictResolver) ResolveConflict(item Item, existingID string, mode ConflictMode) ConflictResolution {
	resolution := ConflictResolution{
		ItemID:       item.ID,
		OriginalID:   existingID,
		Resolution:   mode,
		ConflictType: item.Type.String(),
	}

	switch mode {
	case ConflictSkip:
		resolution.NewID = existingID
		resolution.Message = fmt.Sprintf("Skipped %s (ID: %s) - already exists", item.Title, item.ID)

	case ConflictOverwrite:
		resolution.NewID = existingID // Will be overwritten
		resolution.Message = fmt.Sprintf("Overwriting %s (ID: %s)", item.Title, item.ID)

	case ConflictRename:
		newHandle := generateNewHandle(item.Handle)
		resolution.Message = fmt.Sprintf("Renamed %s handle from '%s' to '%s'", item.Title, item.Handle, newHandle)
	}

	r.resolutions = append(r.resolutions, resolution)
	return resolution
}

// GetResolutions returns all conflict resolutions
func (r *ConflictResolver) GetResolutions() []ConflictResolution {
	return r.resolutions
}

// GetResolutionForItem returns the resolution for a specific item
func (r *ConflictResolver) GetResolutionForItem(itemID string) *ConflictResolution {
	for _, res := range r.resolutions {
		if res.ItemID == itemID {
			return &res
		}
	}
	return nil
}

// Clear clears all resolutions
func (r *ConflictResolver) Clear() {
	r.resolutions = make([]ConflictResolution, 0)
}

// GetSummary returns a summary of resolutions
func (r *ConflictResolver) GetSummary() map[ConflictMode]int {
	summary := make(map[ConflictMode]int)
	for _, res := range r.resolutions {
		summary[res.Resolution]++
	}
	return summary
}
