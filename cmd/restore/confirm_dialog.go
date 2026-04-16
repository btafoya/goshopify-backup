package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ConfirmDialogModel represents a confirmation dialog
type ConfirmDialogModel struct {
	visible   bool
	title     string
	message   string
	items     []ConfirmItem
	selection int // 0 = yes, 1 = no
}

// ConfirmItem represents an item in the confirmation list
type ConfirmItem struct {
	ID       string
	Title    string
	Conflict string // conflict description if any
}

// NewConfirmDialog creates a new confirm dialog
func NewConfirmDialog() ConfirmDialogModel {
	return ConfirmDialogModel{
		visible:   false,
		selection: 0, // Default to yes
	}
}

// Show shows the dialog with given title and message
func (m *ConfirmDialogModel) Show(title, message string) {
	m.visible = true
	m.title = title
	m.message = message
	m.selection = 0
}

// AddItems adds items to confirm
func (m *ConfirmDialogModel) AddItems(items []ConfirmItem) {
	m.items = items
}

// Hide hides the dialog
func (m *ConfirmDialogModel) Hide() {
	m.visible = false
}

// IsVisible returns true if dialog is visible
func (m ConfirmDialogModel) IsVisible() bool {
	return m.visible
}

// Confirmed returns true if user confirmed (selected yes)
func (m ConfirmDialogModel) Confirmed() bool {
	return m.visible && m.selection == 0
}

// View renders the confirm dialog
func (m ConfirmDialogModel) View() string {
	if !m.visible {
		return ""
	}

	dialogStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorMuted).
		Padding(1, 2).
		Width(60)

	headerStyle := lipgloss.NewStyle().
		Foreground(colorWarning).
		Bold(true)

	yesStyle := lipgloss.NewStyle().
		Foreground(colorSuccess).
		Bold(true)

	noStyle := lipgloss.NewStyle().
		Foreground(colorError).
		Bold(true)

	content := m.renderDialogContent(headerStyle, yesStyle, noStyle)

	return dialogStyle.Render(content)
}

// renderDialogContent renders the dialog content
func (m ConfirmDialogModel) renderDialogContent(headerStyle, yesStyle, noStyle lipgloss.Style) string {
	var b strings.Builder

	// Header
	b.WriteString(headerStyle.Render(m.title))
	b.WriteString("\n\n")

	// Message
	b.WriteString(m.message)
	b.WriteString("\n\n")

	// Items if any
	if len(m.items) > 0 {
		displayCount := min(10, len(m.items))
		b.WriteString("Items to restore:\n")
		for i := 0; i < displayCount; i++ {
			item := m.items[i]
			prefix := "  "
			if item.Conflict != "" {
				prefix = RenderWarning("! ") // Conflict indicator
			}
			b.WriteString(fmt.Sprintf("%s%s\n", prefix, item.Title))
		}
		if len(m.items) > displayCount {
			b.WriteString(fmt.Sprintf("  ... and %d more\n", len(m.items)-displayCount))
		}
		b.WriteString("\n")
	}

	// Options
	yesCursor := " "
	noCursor := " "
	if m.selection == 0 {
		yesCursor = RenderInfo(">")
	} else {
		noCursor = RenderInfo(">")
	}

	b.WriteString(yesStyle.Render(fmt.Sprintf("%s Yes - Confirm restore", yesCursor)))
	b.WriteString("\n")
	b.WriteString(noStyle.Render(fmt.Sprintf("%s No  - Go back", noCursor)))
	b.WriteString("\n\n")

	// Footer
	b.WriteString(lipgloss.NewStyle().
		Foreground(colorDim).
		Render("←/→: Choose  Enter: Confirm  Esc: Cancel"))

	return b.String()
}

// MoveSelection moves the selection
func (m *ConfirmDialogModel) MoveSelection(direction string) {
	switch direction {
	case "left":
		m.selection = 0
	case "right":
		m.selection = 1
	}
}
