package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/btafoya/goshopify-restore/backup"
	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"
)

// Model is the main Bubbletea model with state machine
type Model struct {
	// State determines which view is active
	state State

	// Configuration
	cfg *Config

	// Backup selection
	backupDir    string
	selectedDate string
	backupList   []BackupInfo
	backupIndex  int

	// Entity selection
	activeEntity EntityType
	entityStates map[EntityType]EntityState

	// Restore state
	selectedItems    map[EntityType]map[string]Item
	previewChanges   []PreviewChange
	restoreResults   []RestoreResult
	restoreProgress  *RestoreProgress
	restoreStateFile string

	// Error handling
	errorMsg string

	// Abort options
	abortOption string // "resume", "cleanup", "leave"

	// Screen dimensions
	width  int
	height int

	// Sub-models for Bubbles components
	filterBar     FilterBarModel
	statusBar     StatusBarModel
	filterDialog  FilterDialogModel

	// Quit flag
	quit bool
}

// EntityState represents the state of an entity type
type EntityState struct {
	items    []Item
	filtered []Item
	selected map[string]bool
	cursor   int
	scroll   int
	filters  FilterCriteria
}

// InitialModel creates the initial TUI model
func InitialModel(cfg *Config) (*Model, error) {
	m := &Model{
		cfg:          cfg,
		state:        StateConfig,
		backupDir:    cfg.BackupDir,
		entityStates: make(map[EntityType]EntityState),
		selectedItems: make(map[EntityType]map[string]Item),
		filterBar:    NewFilterBar(),
		statusBar:    NewStatusBar("", ""),
		filterDialog: NewFilterDialog(),
	}

	// Initialize entity states
	for _, entityType := range EntityTypes {
		m.entityStates[entityType] = EntityState{
			selected: make(map[string]bool),
		}
	}

	// If backup date is specified, skip to entity select
	if cfg.BackupDate != "" {
		m.selectedDate = cfg.BackupDate
		m.state = StateEntitySelect
		m.activeEntity = EntityProducts
	}

	return m, nil
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	// Start by loading backups if no specific date selected
	if m.state == StateConfig {
		return loadBackupsCmd(m.backupDir)
	}
	return nil
}

// Update handles messages
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	// Update sub-models
	m.filterBar, cmd = m.filterBar.Update(msg)
	if cmd != nil {
		return m, cmd
	}

	// Update filter dialog if visible
	m.filterDialog, cmd = m.filterDialog.Update(msg)
	if m.filterDialog.IsVisible() && cmd != nil {
		return m, cmd
	}
	if m.filterDialog.IsVisible() {
		return m, nil // Dialog consumes all input when visible
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyMsg(msg)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case errorMsg:
		m.errorMsg = string(msg)
		m.state = StateError
		return m, nil
	case backupsLoadedMsg:
		m.backupList = msg.backups
		if len(m.backupList) > 0 {
			m.state = StateBackupSelect
		} else {
			m.errorMsg = "No backups found in " + m.backupDir
			m.state = StateError
		}
		return m, nil
	case restoreProgressMsg:
		if m.restoreProgress == nil {
			m.restoreProgress = &RestoreProgress{}
		}
		m.restoreProgress = msg.progress
		return m, nil
	case restoreCompleteMsg:
		m.restoreResults = msg.results
		m.state = StateComplete
		return m, nil
		case previewGeneratedMsg:
			m.previewChanges = msg.changes
			m.state = StatePreview
			return m, nil
		case itemsLoadedMsg:
			state := m.entityStates[msg.entityType]
			state.items = msg.items
			state.filtered = make([]Item, len(msg.items))
			copy(state.filtered, msg.items)
			state.cursor = 0
			state.scroll = 0
			m.entityStates[msg.entityType] = state
			return m, nil

	case filterTextChangedMsg:
		m.applyFilter(msg.text)
		return m, nil
	}

	return m, nil
}

// View renders the TUI
func (m Model) View() string {
	switch m.state {
	case StateConfig:
		return m.configView()
	case StateBackupSelect:
		return m.backupSelectView()
	case StateEntitySelect:
		return m.entitySelectView()
	case StateItemSelect:
		return m.itemSelectView()
	case StatePreview:
		return m.previewView()
	case StateConfirm:
		return m.confirmView()
	case StateRunning:
		return m.runningView()
	case StateComplete:
		return m.completeView()
	case StateError:
		return m.errorView()
	case StateAbort:
		return m.abortView()
	default:
		return "Unknown state"
	}
}

// handleKeyMsg handles keyboard input
func (m Model) handleKeyMsg(msg tea.KeyMsg) (Model, tea.Cmd) {
	// Global keys
	if m.state == StateComplete || m.state == StateError {
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			m.quit = true
			return m, tea.Quit
		}
	}

	// State-specific handling
	switch m.state {
	case StateConfig:
		return m.handleConfigKey(msg)
	case StateBackupSelect:
		return m.handleBackupSelectKey(msg)
	case StateEntitySelect:
		return m.handleEntitySelectKey(msg)
	case StateItemSelect:
		return m.handleItemSelectKey(msg)
	case StatePreview:
		return m.handlePreviewKey(msg)
	case StateConfirm:
		return m.handleConfirmKey(msg)
	case StateRunning:
		return m.handleRunningKey(msg)
	case StateAbort:
		return m.handleAbortKey(msg)
	}

	return m, nil
}

// handleConfigKey handles keys in config state
func (m Model) handleConfigKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		m.quit = true
		return m, tea.Quit
	case "enter":
		m.state = StateItemSelect
	}
	return m, nil
}

// handleBackupSelectKey handles keys in backup select state
func (m Model) handleBackupSelectKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		m.quit = true
		return m, tea.Quit
	case "up", "k":
		if m.backupIndex > 0 {
			m.backupIndex--
		}
	case "down", "j":
		if m.backupIndex < len(m.backupList)-1 {
			m.backupIndex++
		}
	case "enter":
		m.state = StateItemSelect
	}
	return m, nil
}

// handleEntitySelectKey handles keys in entity select state
func (m Model) handleEntitySelectKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	entityTypes := EntityTypes
	currentIndex := 0
	for i, et := range entityTypes {
		if et == m.activeEntity {
			currentIndex = i
			break
		}
	}

	switch msg.String() {
	case "q", "ctrl+c":
		m.quit = true
		return m, tea.Quit
	case "up", "k":
		if currentIndex > 0 {
			m.activeEntity = entityTypes[currentIndex-1]
		}
	case "down", "j":
		if currentIndex < len(entityTypes)-1 {
			m.activeEntity = entityTypes[currentIndex+1]
		}
	case "enter":
		m.state = StateItemSelect
		return m, loadItemsCmd(m.backupDir, m.selectedDate, m.activeEntity)
	case "esc":
		m.state = StateBackupSelect
	}
	return m, nil
}

// handleItemSelectKey handles keys in item select state
func (m Model) handleItemSelectKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	state := m.entityStates[m.activeEntity]

	switch msg.String() {
	case "q", "ctrl+c":
		m.quit = true
		return m, tea.Quit
	case "up", "k":
		if state.cursor > 0 {
			state.cursor--
		}
	case "down", "j":
		if state.cursor < len(state.filtered)-1 {
			state.cursor++
		}
	case " ":
		if len(state.filtered) > 0 {
			id := state.filtered[state.cursor].ID
			if state.selected[id] {
				delete(state.selected, id)
			} else {
				state.selected[id] = true
			}
		}
	case "ctrl+a":
		for _, item := range state.filtered {
			state.selected[item.ID] = true
		}
		case "enter":
			m.state = StatePreview
			return m, generatePreviewCmd

	case "/":
		m.filterBar.Activate()
		return m, nil
	case "f", "F":
		// Open advanced filter dialog
		state := m.entityStates[m.activeEntity]
		m.filterDialog.SetCriteria(state.filters)
		m.filterDialog.Open()
		return m, nil
	case "esc":
		m.state = StateEntitySelect
	}

	m.entityStates[m.activeEntity] = state
	return m, nil
}

// handlePreviewKey handles keys in preview state
func (m Model) handlePreviewKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		m.quit = true
		return m, tea.Quit
	case "enter":
		m.state = StateConfirm
	case "esc":
		m.state = StateItemSelect
	}
	return m, nil
}


// handleConfirmKey handles keys in confirm state
func (m Model) handleConfirmKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		m.quit = true
		return m, tea.Quit
	case "y", "Y":
		m.state = StateRunning
		return m, startRestoreCmd(m)
	case "n", "N":
		m.state = StateItemSelect
	case "esc":
		m.state = StatePreview
	}
	return m, nil
}

// handleRunningKey handles keys in running state
func (m Model) handleRunningKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.state = StateAbort
	}
	return m, nil
}

// handleAbortKey handles keys in abort state
func (m Model) handleAbortKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "1":
		m.abortOption = "resume"
		m.state = StateItemSelect
	case "2":
		m.abortOption = "cleanup"
		m.quit = true
		return m, tea.Quit
	case "3":
		m.abortOption = "leave"
		m.quit = true
		return m, tea.Quit
	case "q", "ctrl+c", "esc":
		m.state = StateRunning
	}
	return m, nil
}

// applyFilter applies the filter text to the current entity's items
func (m *Model) applyFilter(filterText string) {
	state, exists := m.entityStates[m.activeEntity]
	if !exists {
		return
	}

	state.filters.SearchText = filterText
	m.applyAdvancedFilters(&state)
	m.entityStates[m.activeEntity] = state
}

// applyAdvancedFilters applies all filter criteria to items
func (m *Model) applyAdvancedFilters(state *EntityState) {
	criteria := state.filters

	var filtered []Item
	for _, item := range state.items {
		if m.matchesFilter(item, criteria) {
			filtered = append(filtered, item)
		}
	}

	state.filtered = filtered

	// Reset cursor if needed
	if state.cursor >= len(state.filtered) {
		state.cursor = max(0, len(state.filtered)-1)
	}
}

// matchesFilter returns true if the item matches the filter criteria
func (m *Model) matchesFilter(item Item, criteria FilterCriteria) bool {
	// Search text filter
	if criteria.SearchText != "" {
		searchText := strings.ToLower(criteria.SearchText)
		if !strings.Contains(strings.ToLower(item.Title), searchText) &&
			!strings.Contains(strings.ToLower(item.Handle), searchText) &&
			!strings.Contains(strings.ToLower(item.ID), searchText) {
			return false
		}
	}

	// Status filter
	if len(criteria.Statuses) > 0 {
		matchesStatus := false
		for _, status := range criteria.Statuses {
			if item.Status == status {
				matchesStatus = true
				break
			}
		}
		if !matchesStatus {
			return false
		}
	}

	// Tag filter
	if len(criteria.Tags) > 0 {
		matchesTag := false
		for _, tag := range criteria.Tags {
			for _, itemTag := range item.Tags {
				if strings.EqualFold(itemTag, tag) {
					matchesTag = true
					break
				}
			}
			if matchesTag {
				break
			}
		}
		if !matchesTag {
			return false
		}
	}

	// Date range filter
	if criteria.DateFrom != nil || criteria.DateTo != nil {
		if item.CreatedAt.IsZero() {
			return false
		}

		if criteria.DateFrom != nil && item.CreatedAt.Before(*criteria.DateFrom) {
			return false
		}

		if criteria.DateTo != nil && item.CreatedAt.After(*criteria.DateTo) {
			return false
		}
	}

	return true
}

// overlayDialog overlays a dialog on top of content
func overlayDialog(content, dialog string) string {
	contentLines := strings.Split(content, "\n")
	dialogLines := strings.Split(dialog, "\n")

	// Find center position
	contentHeight := len(contentLines)
	dialogHeight := len(dialogLines)
	dialogWidth := 0
	for _, line := range dialogLines {
		if len(line) > dialogWidth {
			dialogWidth = len(line)
		}
	}

	startY := (contentHeight - dialogHeight) / 2
	if startY < 0 {
		startY = 0
	}
	if startY+dialogHeight > contentHeight {
		startY = contentHeight - dialogHeight
	}

	// Create output
	result := make([]string, len(contentLines))
	copy(result, contentLines)

	// Overlay dialog
	for i, line := range dialogLines {
		if startY+i < len(result) {
			// Calculate horizontal position
			originalLine := result[startY+i]
			contentWidth := len(originalLine)
			startX := (contentWidth - dialogWidth) / 2
			if startX < 0 {
				startX = 0
			}

			// Center the dialog line
			padding := strings.Repeat(" ", startX)
			result[startY+i] = originalLine + "\n" + padding + line
		}
	}

	return strings.Join(result, "\n")
}

// View methods for each state

func (m Model) configView() string {
	return "Loading configuration..."
}

func (m Model) backupSelectView() string {
	if len(m.backupList) == 0 {
		return "No backups available."
	}
	m.statusBar.SetLeft(fmt.Sprintf("%d backup(s) available", len(m.backupList)))
	m.statusBar.SetRight("↑/↓: Navigate  Enter: Select  Q: Quit")
	return m.renderView("Select Backup", m.renderBackupSelect())
}

func (m Model) entitySelectView() string {
	m.statusBar.SetLeft(fmt.Sprintf("Backup: %s", m.selectedDate))
	m.statusBar.SetRight("↑/↓: Select  Enter: View items  Esc: Back  Q: Quit")
	return m.renderView("Select Entity Type", m.renderEntitySelect())
}

func (m Model) itemSelectView() string {
	state, ok := m.entityStates[m.activeEntity]
	selectedCount := 0
	totalCount := 0
	if ok {
		selectedCount = len(state.selected)
		totalCount = len(state.items)
	}
	m.statusBar.SetLeft(fmt.Sprintf("%s: %d/%d selected", EntityDisplayNames[m.activeEntity], selectedCount, totalCount))
	m.statusBar.SetRight("Space: Toggle  Ctrl+A: All  Enter: Preview  /: Search  F: Advanced  Esc: Back  Q: Quit")

	baseView := m.renderView(fmt.Sprintf("%s - Select Items", EntityDisplayNames[m.activeEntity]), m.renderItemList())

	// Overlay filter dialog if visible
	if m.filterDialog.IsVisible() {
		return overlayDialog(baseView, m.filterDialog.View())
	}

	return baseView
}

func (m Model) previewView() string {
	var b strings.Builder

	// Summary
	totalItems := m.countSelectedItems()
	b.WriteString(lipgloss.NewStyle().
		Foreground(colorInfo).
		Render(fmt.Sprintf("Preview: %d item(s) selected for restore\n\n", totalItems)))

	// Group by entity type
	entityGroups := make(map[EntityType][]Item)
	for entityType, state := range m.entityStates {
		for id := range state.selected {
			for _, item := range state.items {
				if item.ID == id {
					entityGroups[entityType] = append(entityGroups[entityType], item)
					break
				}
			}
		}
	}

	// Show items by entity type
	for entityType, items := range entityGroups {
		b.WriteString(lipgloss.NewStyle().
			Bold(true).
			Render(EntityDisplayNames[entityType]))
		b.WriteString(fmt.Sprintf(" (%d items)\n", len(items)))

		// Show up to 5 items
		showCount := min(5, len(items))
		for i := 0; i < showCount; i++ {
			b.WriteString(fmt.Sprintf("  • %s\n", items[i].Title))
		}
		if len(items) > 5 {
			b.WriteString(fmt.Sprintf("  ... and %d more\n", len(items)-5))
		}
		b.WriteString("\n")
	}

	// Target info
	if m.cfg.Store != "" {
		b.WriteString(lipgloss.NewStyle().
			Foreground(colorDim).
			Render(fmt.Sprintf("Target: %s\n", m.cfg.Store)))
	}

	if m.cfg.DryRun {
		b.WriteString(lipgloss.NewStyle().
			Foreground(colorWarning).
			Render("\nDRY-RUN MODE - No actual changes will be made"))
	}

	return m.renderView("Preview Changes", b.String())
}

func (m Model) confirmView() string {
	totalItems := m.countSelectedItems()
	m.statusBar.SetLeft(fmt.Sprintf("About to restore %d item(s)", totalItems))
	m.statusBar.SetRight("Y: Confirm  N: Back  Q: Quit")
	return m.renderView("Confirm Restore", m.renderConfirm())
}

func (m Model) runningView() string {
	if m.restoreProgress != nil {
		m.statusBar.SetLeft(fmt.Sprintf("Restoring: %d/%d", m.restoreProgress.CompletedItems, m.restoreProgress.TotalItems))
	}
	m.statusBar.SetRight("Esc: Abort")
	return m.renderProgress() // Progress view has its own header
}

func (m Model) completeView() string {
	m.statusBar.SetLeft("Restore complete")
	m.statusBar.SetRight("Q: Quit")
	return m.renderView("Restore Complete", m.renderComplete())
}

func (m Model) errorView() string {
	m.statusBar.SetLeft("Error")
	m.statusBar.SetRight("Q: Quit")
	if m.errorMsg != "" {
		return m.renderView("Error", "Error: "+m.errorMsg)
	}
	return m.renderView("Error", "An unknown error occurred.")
}

func (m Model) abortView() string {
	m.statusBar.SetLeft("Restore interrupted")
	m.statusBar.SetRight("1: Resume  2: Cleanup  3: Leave  Esc: Continue")
	return m.renderView("Restore Interrupted", m.renderAbort())
}

// Commands

type errorMsg string

type backupsLoadedMsg struct {
	backups []BackupInfo
}

type restoreProgressMsg struct {
	progress *RestoreProgress
}

type restoreCompleteMsg struct {
	results []RestoreResult
}

type filterTextChangedMsg struct {
	text string
}

type itemsLoadedMsg struct {
	entityType EntityType
	items      []Item
}

type previewGeneratedMsg struct {
	changes []PreviewChange
}

func loadBackupsCmd(dir string) tea.Cmd {
	return func() tea.Msg {
		loader := backup.NewLoader(dir)
		backupLoaderBackups, err := loader.ListBackups()
		if err != nil {
			return errorMsg(fmt.Sprintf("Failed to load backups: %v", err))
		}

		// Convert backup.BackupInfo to main.BackupInfo
		backups := make([]BackupInfo, len(backupLoaderBackups))
		for i, b := range backupLoaderBackups {
			backups[i] = BackupInfo{
				Date:     b.Date,
				Path:     b.Path,
				Status:   convertBackupStatus(b.Status),
				FileSize: b.FileSize,
			}
		}

		return backupsLoadedMsg{backups: backups}
	}
}

// convertBackupStatus converts backup.BackupStatus to main.BackupStatus
func convertBackupStatus(status *backup.BackupStatus) *BackupStatus {
	if status == nil {
		return nil
	}
	return &BackupStatus{
		StartedAt:   status.StartedAt,
		CompletedAt: status.CompletedAt,
		Duration:    status.Duration,
		Modules:     convertModuleStatuses(status.Modules),
		TotalSize:   status.TotalSize,
	}
}

// convertModuleStatuses converts module status map
func convertModuleStatuses(modules map[string]backup.ModuleStatus) map[string]ModuleStatus {
	result := make(map[string]ModuleStatus)
	for k, v := range modules {
		result[k] = ModuleStatus{
			Status:      v.Status,
			StartedAt:   v.StartedAt,
			CompletedAt: v.CompletedAt,
			Count:       v.Count,
			Error:       v.Error,
			FileSize:    v.FileSize,
		}
	}
	return result
}

// loadItemsCmd loads items for a specific entity type from backup
func loadItemsCmd(backupDir, date string, entityType EntityType) tea.Cmd {
	return func() tea.Msg {
		loader := backup.NewLoader(backupDir)
		backupItems, err := loader.LoadEntity(date, backup.EntityType(entityType))
		if err != nil {
			return errorMsg(fmt.Sprintf("Failed to load %s: %v", entityType, err))
		}

		// Convert backup.Item to main.Item with Type field
		items := make([]Item, len(backupItems))
		for i, bi := range backupItems {
			items[i] = convertBackupItem(bi, entityType)
		}

		return itemsLoadedMsg{
			entityType: entityType,
			items:      items,
		}
	}
}

// convertBackupItem converts a backup.Item to main.Item with Type
func convertBackupItem(bi backup.Item, entityType EntityType) Item {
	return Item{
		ID:         bi.ID,
		Title:      bi.Title,
		Handle:     bi.Handle,
		CreatedAt:  bi.CreatedAt,
		UpdatedAt:  bi.UpdatedAt,
		Status:     bi.Status,
		Tags:       bi.Tags,
		CustomData: bi.CustomData,
		Type:       entityType,
		Price:      bi.Price,
		VariantCount: bi.VariantCount,
		Email:      bi.Email,
		OrderNumber: bi.OrderNumber,
		FinancialStatus: bi.FinancialStatus,
		FulfillmentStatus: bi.FulfillmentStatus,
		ProductsCount: bi.ProductsCount,
	}
}

var generatePreviewCmd tea.Cmd = func() tea.Msg {
	// Preview is generated by checking what items are selected
	// For now, just transition to preview state
	// In a real implementation, this would check for conflicts
	return previewGeneratedMsg{changes: []PreviewChange{}}
}

func startRestoreCmd(m Model) tea.Cmd {
	return func() tea.Msg {
		// Collect all selected items
		var selectedItems []Item
		for entityType, state := range m.entityStates {
			for id := range state.selected {
				for _, item := range state.items {
					if item.ID == id {
						item.Type = entityType
						selectedItems = append(selectedItems, item)
						break
					}
				}
			}
		}

		if len(selectedItems) == 0 {
			return errorMsg("No items selected for restore")
		}

		// In dry run mode, just return success
		if m.cfg.DryRun {
			results := make([]RestoreResult, len(selectedItems))
			for i, item := range selectedItems {
				results[i] = RestoreResult{
					EntityType: item.Type,
					ItemID:     item.ID,
					Success:    true,
					Message:    "Dry run - would be restored",
				}
			}
			return restoreCompleteMsg{results: results}
		}

		// Real restore execution
		client := NewShopifyClient(m.cfg.Store, m.cfg.AccessToken, m.cfg.APIVersion, m.cfg.DryRun)
		executor := NewRestoreExecutor(client, ConflictSkip)

		ctx := context.Background()
		results, err := executor.ExecuteRestore(ctx, selectedItems)
		if err != nil {
			return errorMsg(fmt.Sprintf("Restore failed: %v", err))
		}

		return restoreCompleteMsg{results: results}
	}
}