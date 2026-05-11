package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// StatusBarModel represents a status bar component
type StatusBarModel struct {
	leftText  string
	rightText string
	width     int
}

// NewStatusBar creates a new status bar
func NewStatusBar(left, right string) StatusBarModel {
	return StatusBarModel{
		leftText:  left,
		rightText: right,
	}
}

// SetLeft sets the left text
func (m *StatusBarModel) SetLeft(text string) {
	m.leftText = text
}

// SetRight sets the right text
func (m *StatusBarModel) SetRight(text string) {
	m.rightText = text
}

// SetWidth sets the width
func (m *StatusBarModel) SetWidth(width int) {
	m.width = width
}

// View renders the status bar
func (m StatusBarModel) View() string {
	if m.width == 0 {
		m.width = 80
	}

	// Calculate available width for right text
	leftLen := lipgloss.Width(m.leftText)
	rightLen := lipgloss.Width(m.rightText)
	separator := "  │  "
	separatorLen := lipgloss.Width(separator)

	leftStyle := lipgloss.NewStyle().
		Foreground(colorDim)

	rightStyle := lipgloss.NewStyle().
		Foreground(colorDim)

	availableWidth := m.width - leftLen - rightLen - separatorLen
	if availableWidth < 0 {
		availableWidth = 0
	}

	leftPart := leftStyle.Render(m.leftText)
	rightPart := rightStyle.Render(m.rightText)

	return lipgloss.JoinHorizontal(
		lipgloss.Left,
		leftPart,
		strings.Repeat(" ", availableWidth),
		separator,
		rightPart,
	)
}

// FormatStatus creates a status bar with backup and store info
func FormatStatus(backupDate, storeURL string, selectedCount, totalCount int) string {
	var parts []string

	if backupDate != "" {
		parts = append(parts, fmt.Sprintf("Backup: %s", backupDate))
	}

	if storeURL != "" {
		parts = append(parts, fmt.Sprintf("Store: %s", storeURL))
	}

	if totalCount > 0 {
		parts = append(parts, fmt.Sprintf("Selected: %d/%d", selectedCount, totalCount))
	}

	return strings.Join(parts, "  │  ")
}
