package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Styles for the TUI
var (
	// Colors
	colorTitle    = lipgloss.Color("13")  // Purple
	colorSubtitle = lipgloss.Color("8")   // Gray
	colorSuccess  = lipgloss.Color("2")   // Green
	colorError    = lipgloss.Color("1")   // Red
	colorWarning  = lipgloss.Color("3")   // Yellow
	colorInfo     = lipgloss.Color("6")   // Cyan
	colorDim      = lipgloss.Color("245") // Dim gray
	colorMuted    = lipgloss.Color("241") // Muted gray

	// Styles
	styleTitle = lipgloss.NewStyle().
		Foreground(colorTitle).
		Bold(true)

	styleSubtitle = lipgloss.NewStyle().
		Foreground(colorSubtitle)

	styleSuccess = lipgloss.NewStyle().
		Foreground(colorSuccess)

	styleError = lipgloss.NewStyle().
		Foreground(colorError)

	styleWarning = lipgloss.NewStyle().
		Foreground(colorWarning)

	styleInfo = lipgloss.NewStyle().
		Foreground(colorInfo)

	styleDim = lipgloss.NewStyle().
		Foreground(colorDim)

	styleMuted = lipgloss.NewStyle().
		Foreground(colorMuted)

	// Border styles
	styleBorder = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorMuted)

	styleBorderStyle = lipgloss.Border{
		Top:         "─",
		Bottom:      "─",
		Left:        "│",
		Right:       "│",
		TopLeft:     "┌",
		TopRight:    "┐",
		BottomLeft:  "└",
		BottomRight: "┘",
	}

	styleDoubleBorder = lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(colorMuted)

	// Header style
	styleHeader = lipgloss.NewStyle().
		Background(colorTitle).
		Foreground(lipgloss.Color("15")).
		Bold(true).
		Padding(0, 1)

	// Footer style
	styleFooter = lipgloss.NewStyle().
		Foreground(colorDim).
		Padding(0, 1)

	// Cursor style
	styleCursor = lipgloss.NewStyle().
		Foreground(colorInfo).
		Bold(true)

	// Selected style
	styleSelected = lipgloss.NewStyle().
		Foreground(colorSuccess)

	// Focused style
	styleFocused = lipgloss.NewStyle().
		Foreground(colorSuccess).
		Bold(true)

	// Card style
	styleCard = lipgloss.NewStyle().
		Border(styleBorderStyle).
		BorderForeground(colorDim).
		Padding(1)

	// Progress bar styles
	styleProgressBar = lipgloss.NewStyle().
		Width(40)

	styleProgressFill = lipgloss.NewStyle().
		Background(colorSuccess)

	styleProgressEmpty = lipgloss.NewStyle().
		Background(colorDim)

	// Status styles
	styleStatusRunning = lipgloss.NewStyle().
		Foreground(colorWarning)

	styleStatusCompleted = lipgloss.NewStyle().
		Foreground(colorSuccess)

	styleStatusFailed = lipgloss.NewStyle().
		Foreground(colorError)

	styleStatusPending = lipgloss.NewStyle().
		Foreground(colorDim)
)

// RenderTitle renders a title with the global title style
func RenderTitle(text string) string {
	return styleTitle.Render(text)
}

// RenderSubtitle renders a subtitle with the global subtitle style
func RenderSubtitle(text string) string {
	return styleSubtitle.Render(text)
}

// RenderSuccess renders success text
func RenderSuccess(text string) string {
	return styleSuccess.Render(text)
}

// RenderError renders error text
func RenderError(text string) string {
	return styleError.Render(text)
}

// RenderWarning renders warning text
func RenderWarning(text string) string {
	return styleWarning.Render(text)
}

// RenderInfo renders info text
func RenderInfo(text string) string {
	return styleInfo.Render(text)
}

// RenderDim renders dimmed text
func RenderDim(text string) string {
	return styleDim.Render(text)
}

// RenderCard renders text in a card style
func RenderCard(text string) string {
	return styleCard.Render(text)
}

// RenderProgressBar renders a progress bar
func RenderProgressBar(current, total int) string {
	if total == 0 {
		return styleProgressEmpty.Render(strings.Repeat("░", 40))
	}

	percentage := float64(current) / float64(total)
	filled := int(percentage * 40)
	empty := 40 - filled

	return styleProgressFill.Render(strings.Repeat("█", filled)) +
		styleProgressEmpty.Render(strings.Repeat("░", empty))
}

// RenderStatus renders a status with appropriate color
func RenderStatus(status string) string {
	switch status {
	case "completed":
		return styleStatusCompleted.Render("✓ " + status)
	case "running":
		return styleStatusRunning.Render("→ " + status)
	case "failed":
		return styleStatusFailed.Render("✗ " + status)
	default:
		return styleStatusPending.Render("○ " + status)
	}
}