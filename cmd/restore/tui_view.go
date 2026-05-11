package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// viewRenderers holds view rendering methods
type viewRenderers struct{}

// renderBackupSelect renders the backup selection view
func (m Model) renderBackupSelect() string {
	var b strings.Builder

	// Instructions
	b.WriteString(lipgloss.NewStyle().
		Foreground(colorDim).
		Render("Use ↑/↓ to navigate, Enter to select, Q to quit\n\n"))

	// Backup list
	if len(m.backupList) == 0 {
		b.WriteString(styleError.Render("No backups found in " + m.backupDir))
		return b.String()
	}

	// Calculate visible range
	visibleCount := min(20, len(m.backupList))
	startIndex := max(0, m.backupIndex-5)
	endIndex := min(len(m.backupList), startIndex+visibleCount)

	// If near end, shift up
	if endIndex-m.backupIndex < 10 && endIndex < len(m.backupList) {
		startIndex = max(0, len(m.backupList)-visibleCount)
		endIndex = len(m.backupList)
	}

	for i := startIndex; i < endIndex; i++ {
		backup := m.backupList[i]
		cursor := " "
		if i == m.backupIndex {
			cursor = RenderInfo(">")
		}

		dateStr := backup.Date.Format("2006-01-02")
		status := RenderStatus(backup.Status.Modules["products"].Status)

		itemStyle := lipgloss.NewStyle()
		if i == m.backupIndex {
			itemStyle = itemStyle.Bold(true)
		}

		b.WriteString(fmt.Sprintf("%s %s  %s  %s  %s\n",
			cursor,
			itemStyle.Render(dateStr),
			itemStyle.Render(fmt.Sprintf("%d items", backup.Status.Modules["products"].Count)),
			itemStyle.Render(formatSize(backup.FileSize)),
			status,
		))
	}

	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().
		Foreground(colorDim).
		Render(fmt.Sprintf("%d backup(s) available, showing %d-%d",
			len(m.backupList), startIndex+1, endIndex)))

	return b.String()
}

// renderEntitySelect renders the entity selection view
func (m Model) renderEntitySelect() string {
	var b strings.Builder

	// Backup info
	b.WriteString(lipgloss.NewStyle().
		Foreground(colorDim).
		Render(fmt.Sprintf("Backup: %s\n\n", m.selectedDate)))

	// Entity list
	entityTypes := EntityTypes
	for _, entityType := range entityTypes {
		cursor := " "
		if entityType == m.activeEntity {
			cursor = RenderInfo(">")
		}

		displayName := EntityDisplayNames[entityType]

		entityStyle := lipgloss.NewStyle()
		if entityType == m.activeEntity {
			entityStyle = entityStyle.Foreground(colorSuccess).Bold(true)
		}

		b.WriteString(fmt.Sprintf("%s %s\n", cursor, entityStyle.Render(displayName)))
	}

	// Footer
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().
		Foreground(colorDim).
		Render("↑/↓: Select entity  Enter: View items  Esc: Back  Q: Quit"))

	return b.String()
}

// renderItemList renders the item list view
func (m Model) renderItemList() string {
	var b strings.Builder

	// Backup info
	b.WriteString(lipgloss.NewStyle().
		Foreground(colorDim).
		Render(fmt.Sprintf("Backup: %s\n\n", m.selectedDate)))

	// Get entity state
	state, ok := m.entityStates[m.activeEntity]
	if !ok || len(state.filtered) == 0 {
		b.WriteString(styleDim.Render("No items available."))
		return b.String()
	}

	// Selected count
	selectedCount := len(state.selected)
	b.WriteString(lipgloss.NewStyle().
		Foreground(colorInfo).
		Render(fmt.Sprintf("Selected: %d of %d items\n\n", selectedCount, len(state.items))))

	// Filter indicator
	if hasActiveFilters(state.filters) {
		b.WriteString(lipgloss.NewStyle().
			Foreground(colorWarning).
			Render(fmt.Sprintf("%s Active filters: %s", RenderInfo("◈"), formatFilterSummary(state.filters))))
		b.WriteString("\n\n")
	} else if state.filters.SearchText != "" {
		b.WriteString(lipgloss.NewStyle().
			Foreground(colorWarning).
			Render(fmt.Sprintf("Filter: %s\n\n", state.filters.SearchText)))
	}

	// Calculate visible range
	visibleCount := min(25, len(state.filtered))
	startIndex := max(0, state.cursor-10)
	endIndex := min(len(state.filtered), startIndex+visibleCount)

	// If near end, shift up
	if endIndex-state.cursor < 12 && endIndex < len(state.filtered) {
		startIndex = max(0, len(state.filtered)-visibleCount)
		endIndex = len(state.filtered)
	}

	// Item list
	for i := startIndex; i < endIndex; i++ {
		item := state.filtered[i]
		cursor := " "
		if i == state.cursor {
			cursor = RenderInfo(">")
		}

		checked := " "
		if state.selected[item.ID] {
			checked = RenderSuccess("✓")
		}

		itemStyle := lipgloss.NewStyle()
		if i == state.cursor {
			itemStyle = itemStyle.Bold(true)
		}

		b.WriteString(fmt.Sprintf("%s [%s] %s\n",
			cursor,
			itemStyle.Render(checked),
			itemStyle.Render(m.formatItem(item)),
		))
	}

	return b.String()
}

// renderConfirm renders the confirmation view
func (m Model) renderConfirm() string {
	var b strings.Builder

	// Summary
	totalItems := m.countSelectedItems()
	b.WriteString(lipgloss.NewStyle().
		Foreground(colorInfo).
		Render(fmt.Sprintf("About to restore %d item(s)\n\n", totalItems)))

	// Backup info
	b.WriteString(lipgloss.NewStyle().
		Foreground(colorDim).
		Render(fmt.Sprintf("Backup: %s\n", m.selectedDate)))

	if m.cfg.Store != "" {
		b.WriteString(lipgloss.NewStyle().
			Foreground(colorDim).
			Render(fmt.Sprintf("Target: %s\n", m.cfg.Store)))
	}

	if m.cfg.DryRun {
		b.WriteString(lipgloss.NewStyle().
			Foreground(colorWarning).
			Render("\nDRY-RUN MODE - No actual changes will be made"))
	} else {
		b.WriteString(lipgloss.NewStyle().
			Foreground(colorWarning).
			Render("\nThis will restore data to your Shopify store!"))
	}

	return b.String()
}

// renderProgress renders the progress view
func (m Model) renderProgress() string {
	var b strings.Builder

	// Header with progress bar
	percentage := 0
	if m.restoreProgress != nil && m.restoreProgress.TotalItems > 0 {
		percentage = (m.restoreProgress.CompletedItems * 100) / m.restoreProgress.TotalItems
	}

	b.WriteString(lipgloss.NewStyle().
		Foreground(colorTitle).
		Bold(true).
		Render(fmt.Sprintf("Restoring... [%d%%]\n\n", percentage)))

	// Current entity
	if m.restoreProgress != nil && m.restoreProgress.CurrentEntity != "" {
		b.WriteString(lipgloss.NewStyle().
			Foreground(colorInfo).
			Render(fmt.Sprintf("Current: %s", EntityDisplayNames[m.restoreProgress.CurrentEntity])))
	}

	// Current item
	if m.restoreProgress != nil && m.restoreProgress.CurrentItem != "" {
		b.WriteString(lipgloss.NewStyle().
			Foreground(colorDim).
			Render(fmt.Sprintf("\nItem: %s", m.restoreProgress.CurrentItem)))
	}

	// Progress bar
	if m.restoreProgress != nil {
		b.WriteString("\n\n")
		b.WriteString(RenderProgressBar(m.restoreProgress.CompletedItems, m.restoreProgress.TotalItems))
		b.WriteString(fmt.Sprintf(" %d/%d (%d%%)",
			m.restoreProgress.CompletedItems,
			m.restoreProgress.TotalItems,
			percentage))
	}

	// Stats
	if m.restoreProgress != nil {
		b.WriteString("\n\n")
		b.WriteString(fmt.Sprintf("  %s Completed: %d\n", RenderSuccess("✓"), m.restoreProgress.CompletedItems))
		b.WriteString(fmt.Sprintf("  %s Failed: %d\n", RenderError("✗"), m.restoreProgress.FailedItems))
		b.WriteString(fmt.Sprintf("  %s Skipped: %d\n", RenderDim("○"), m.restoreProgress.SkippedItems))
	}

	// Recent logs
	if m.restoreProgress != nil && len(m.restoreProgress.Logs) > 0 {
		b.WriteString("\n\nRecent activity:\n")
		logCount := min(5, len(m.restoreProgress.Logs))
		startIdx := len(m.restoreProgress.Logs) - logCount
		for i := startIdx; i < len(m.restoreProgress.Logs); i++ {
			log := m.restoreProgress.Logs[i]
			logStyle := styleDim
			if log.Level == "error" {
				logStyle = styleError
			} else if log.Level == "info" {
				logStyle = styleSuccess
			}

			b.WriteString(fmt.Sprintf("  %s\n", logStyle.Render(log.Message)))
		}
	}

	return b.String()
}

// renderComplete renders the completion view
func (m Model) renderComplete() string {
	var b strings.Builder

	// Summary
	successCount := 0
	failedCount := 0
	for _, result := range m.restoreResults {
		if result.Success {
			successCount++
		} else {
			failedCount++
		}
	}

	b.WriteString(lipgloss.NewStyle().
		Foreground(colorSuccess).
		Render(fmt.Sprintf("%s Successfully restored: %d item(s)",
			RenderSuccess("✓"), successCount)))

	if failedCount > 0 {
		b.WriteString(lipgloss.NewStyle().
			Foreground(colorError).
			Render(fmt.Sprintf("\n%s Failed: %d item(s)",
				RenderError("✗"), failedCount)))
	}

	// Rollback info
	if successCount > 0 && !m.cfg.DryRun {
		b.WriteString(lipgloss.NewStyle().
			Foreground(colorInfo).
			Render(fmt.Sprintf("\n\nRollback script saved to: %s/rollback_%s.sh",
				m.cfg.RollbackDir, m.selectedDate)))
	}

	return b.String()
}

// renderAbort renders the abort confirmation view
func (m Model) renderAbort() string {
	var b strings.Builder

	// Options
	b.WriteString(`
Choose an option:

  1. Resume    Save state and continue later
  2. Clean up  Remove partially restored data
  3. Leave     Leave partial data for manual inspection

`)

	return b.String()
}

// renderHeader renders a common header
func (m Model) renderHeader(title string) string {
	width := m.width
	if width == 0 {
		width = 80
	}

	headerStyle := lipgloss.NewStyle().
		Background(colorTitle).
		Foreground(lipgloss.Color("15")).
		Bold(true).
		Width(width).
		Padding(0, 2)

	return headerStyle.Render(" " + title + " ")
}

// formatItem formats an item for display
func (m Model) formatItem(item Item) string {
	var parts []string

	// Title
	if item.Title != "" {
		parts = append(parts, item.Title)
	}

	// Handle
	if item.Handle != "" {
		parts = append(parts, fmt.Sprintf("(%s)", item.Handle))
	}

	// Status
	if item.Status != "" {
		parts = append(parts, fmt.Sprintf("[%s]", item.Status))
	}

	// Price (for products)
	if item.Price != nil {
		parts = append(parts, fmt.Sprintf("$%s", *item.Price))
	}

	// Email (for customers)
	if item.Email != nil {
		parts = append(parts, fmt.Sprintf("<%s>", *item.Email))
	}

	// Order number (for orders)
	if item.OrderNumber != nil {
		parts = append(parts, fmt.Sprintf("#%s", *item.OrderNumber))
	}

	return strings.Join(parts, " ")
}

// countSelectedItems counts total selected items across all entity types
func (m Model) countSelectedItems() int {
	count := 0
	for _, state := range m.entityStates {
		count += len(state.selected)
	}
	return count
}

// Helper functions
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// formatSize formats a size in bytes to human readable format
func formatSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}

// renderView wraps content with header and status bar
func (m Model) renderView(title, content string) string {
	var b strings.Builder

	// Header
	b.WriteString(m.renderHeader(title))

	// Content
	b.WriteString(content)

	// Status bar footer
	b.WriteString("\n")
	m.statusBar.SetWidth(m.width)
	if m.width == 0 {
		m.statusBar.SetWidth(80)
	}
	b.WriteString(m.statusBar.View())

	return b.String()
}

// hasActiveFilters returns true if any filters are active
func hasActiveFilters(filters FilterCriteria) bool {
	return filters.SearchText != "" ||
		(filters.DateFrom != nil) ||
		(filters.DateTo != nil) ||
		len(filters.Statuses) > 0 ||
		len(filters.Tags) > 0
}

// formatFilterSummary returns a human-readable filter summary
func formatFilterSummary(filters FilterCriteria) string {
	var parts []string

	if filters.SearchText != "" {
		parts = append(parts, fmt.Sprintf("\"%s\"", filters.SearchText))
	}

	if len(filters.Statuses) > 0 {
		parts = append(parts, fmt.Sprintf("status=%s", strings.Join(filters.Statuses, ",")))
	}

	if len(filters.Tags) > 0 {
		parts = append(parts, fmt.Sprintf("tag=%s", strings.Join(filters.Tags, ",")))
	}

	if filters.DateFrom != nil {
		parts = append(parts, fmt.Sprintf("from=%s", filters.DateFrom.Format("2006-01-02")))
	}

	if filters.DateTo != nil {
		parts = append(parts, fmt.Sprintf("to=%s", filters.DateTo.Format("2006-01-02")))
	}

	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, ", ")
}
