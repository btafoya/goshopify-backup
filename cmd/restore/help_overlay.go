package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// HelpOverlayModel represents the help overlay
type HelpOverlayModel struct {
	visible bool
	width   int
	height  int
}

// NewHelpOverlay creates a new help overlay
func NewHelpOverlay() HelpOverlayModel {
	return HelpOverlayModel{
		visible: false,
	}
}

// Show shows the help overlay
func (m *HelpOverlayModel) Show() {
	m.visible = true
}

// Hide hides the help overlay
func (m *HelpOverlayModel) Hide() {
	m.visible = false
}

// IsVisible returns true if the overlay is visible
func (m HelpOverlayModel) IsVisible() bool {
	return m.visible
}

// View renders the help overlay
func (m HelpOverlayModel) View() string {
	if !m.visible {
		return ""
	}

	dialogStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorMuted).
		Padding(1, 2).
		Width(80)

	content := m.renderHelpContent()

	return dialogStyle.Render(content)
}

// renderHelpContent renders the help content
func (m HelpOverlayModel) renderHelpContent() string {
	var b strings.Builder

	headerStyle := lipgloss.NewStyle().
		Foreground(colorTitle).
		Bold(true)

	sectionStyle := lipgloss.NewStyle().
		Foreground(colorInfo).
		Bold(true)

	// Header
	b.WriteString(headerStyle.Render("Keyboard Shortcuts\n\n"))

	// Sections
	b.WriteString(sectionStyle.Render("Navigation\n"))
	b.WriteString("  ↑/k, ↓/j     Move up/down\n")
	b.WriteString("  Enter        Select/Confirm\n")
	b.WriteString("  Esc          Go back / Cancel\n")
	b.WriteString("  Tab          Switch panels\n\n")

	b.WriteString(sectionStyle.Render("Selection\n"))
	b.WriteString("  Space        Toggle selection\n")
	b.WriteString("  Ctrl+A       Select all items\n")
	b.WriteString("  /            Search filter\n")
	b.WriteString("  F            Advanced filters\n\n")

	b.WriteString(sectionStyle.Render("Actions\n"))
	b.WriteString("  Y            Confirm (yes)\n")
	b.WriteString("  N            Decline (no)\n\n")

	b.WriteString(sectionStyle.Render("Restore\n"))
	b.WriteString("  Esc          Abort restore\n")
	b.WriteString("  q, Ctrl+C    Quit\n\n")

	// Footer
	b.WriteString(lipgloss.NewStyle().
		Foreground(colorDim).
		Render("Press any key to close"))

	return b.String()
}
