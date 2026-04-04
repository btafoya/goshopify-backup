package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

// FilterDialogModel represents the advanced filter dialog
type FilterDialogModel struct {
	visible    bool
	selection  int  // Currently selected filter option
	cursor     int  // For multi-option filters
	width      int
	height     int

	// Filter values
	searchText  string
	statusFilter string
	tagFilter   string
	dateFrom    string
	dateTo      string

	// Text inputs for value entry
	textInput   textinput.Model
	inputMode   bool // true when entering text

	// Viewport for scrollable content
	viewport viewport.Model
}

// FilterOptions represents available filter options
type FilterOption struct {
	Key         string
	Label       string
	Description string
	Active      bool
	Type        string // "toggle", "text", "date", "select"
	Value       string
	Options     []string // For select type
}

// NewFilterDialog creates a new filter dialog
func NewFilterDialog() FilterDialogModel {
	ti := textinput.New()
	ti.Placeholder = "Enter value..."
	ti.CharLimit = 50
	ti.PromptStyle = lipgloss.NewStyle().
		Foreground(colorDim).
		Padding(0, 1)
	ti.TextStyle = lipgloss.NewStyle().
		Foreground(colorInfo)

	vp := viewport.New(0, 0)

	return FilterDialogModel{
		textInput: ti,
		viewport:  vp,
	}
}

// Init initializes the filter dialog
func (m FilterDialogModel) Init() tea.Cmd {
	return nil
}

// Update handles messages
func (m FilterDialogModel) Update(msg tea.Msg) (FilterDialogModel, tea.Cmd) {
	var cmd tea.Cmd

	// Handle input mode separately
	if m.inputMode {
		m.textInput, cmd = m.textInput.Update(msg)
		return m.handleInputMode(msg, cmd)
	}

	// Handle viewport scroll
	m.viewport, cmd = m.viewport.Update(msg)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width - 8
		m.viewport.Height = m.height - 10
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			m.visible = false
			return m, nil
		case "up", "k":
			if m.selection > 0 {
				m.selection--
			}
		case "down", "j":
			maxSelection := len(m.filterOptions()) - 1
			if m.selection < maxSelection {
				m.selection++
			}
		case "enter", " ":
			return m.handleSelection()
		}
	}

	return m, cmd
}

// handleInputMode handles text input mode
func (m FilterDialogModel) handleInputMode(msg tea.Msg, cmd tea.Cmd) (FilterDialogModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			m.saveInput()
			m.inputMode = false
			m.textInput.Reset()
		case "esc":
			m.inputMode = false
			m.textInput.Reset()
		}
	}
	return m, cmd
}

// handleSelection handles option selection
func (m FilterDialogModel) handleSelection() (FilterDialogModel, tea.Cmd) {
	opts := m.filterOptions()
	if m.selection >= len(opts) {
		return m, nil
	}

	opt := opts[m.selection]

	switch opt.Type {
	case "text":
		m.inputMode = true
		m.textInput.SetValue(opt.Value)
		m.textInput.Focus()
	case "toggle":
		m.toggleFilter(opt.Key)
	case "select":
		m.cursor++
		if m.cursor >= len(opt.Options) {
			m.cursor = 0
		}
		m.setFilterValue(opt.Key, opt.Options[m.cursor])
	case "date":
		m.inputMode = true
		m.textInput.SetValue(opt.Value)
		m.textInput.Placeholder = "YYYY-MM-DD"
		m.textInput.Focus()
	}

	return m, nil
}

// saveInput saves the current text input
func (m *FilterDialogModel) saveInput() {
	opts := m.filterOptions()
	if m.selection >= len(opts) {
		return
	}

	opt := opts[m.selection]
	m.setFilterValue(opt.Key, m.textInput.Value())
}

// toggleFilter toggles a boolean filter
func (m *FilterDialogModel) toggleFilter(key string) {
	switch key {
	case "status-active":
		if m.statusFilter == "active" {
			m.statusFilter = ""
		} else {
			m.statusFilter = "active"
		}
	case "status-archived":
		if m.statusFilter == "archived" {
			m.statusFilter = ""
		} else {
			m.statusFilter = "archived"
		}
	}
}

// setFilterValue sets a filter value
func (m *FilterDialogModel) setFilterValue(key, value string) {
	switch key {
	case "search":
		m.searchText = value
	case "tag":
		m.tagFilter = value
	case "date-from":
		m.dateFrom = value
	case "date-to":
		m.dateTo = value
	}
}

// filterOptions returns available filter options
func (m FilterDialogModel) filterOptions() []FilterOption {
	return []FilterOption{
		{
			Key:         "search",
			Label:       "Search Text",
			Description: "Filter by title, handle, or ID",
			Active:      m.searchText != "",
			Type:        "text",
			Value:       m.searchText,
		},
		{
			Key:         "status-active",
			Label:       "Status: Active",
			Description: "Show only active items",
			Active:      m.statusFilter == "active",
			Type:        "toggle",
		},
		{
			Key:         "status-archived",
			Label:       "Status: Archived",
			Description: "Show only archived items",
			Active:      m.statusFilter == "archived",
			Type:        "toggle",
		},
		{
			Key:         "tag",
			Label:       "Tag",
			Description: "Filter by tag",
			Active:      m.tagFilter != "",
			Type:        "text",
			Value:       m.tagFilter,
		},
		{
			Key:         "date-from",
			Label:       "Date From",
			Description: "Items created after this date",
			Active:      m.dateFrom != "",
			Type:        "date",
			Value:       m.dateFrom,
		},
		{
			Key:         "date-to",
			Label:       "Date To",
			Description: "Items created before this date",
			Active:      m.dateTo != "",
			Type:        "date",
			Value:       m.dateTo,
		},
	}
}

// GetCriteria returns the current filter criteria
func (m FilterDialogModel) GetCriteria() FilterCriteria {
	criteria := FilterCriteria{
		SearchText: m.searchText,
	}

	if m.tagFilter != "" {
		criteria.Tags = []string{m.tagFilter}
	}

	if m.statusFilter != "" {
		criteria.Statuses = []string{m.statusFilter}
	}

	if m.dateFrom != "" {
		if t, err := time.Parse("2006-01-02", m.dateFrom); err == nil {
			criteria.DateFrom = &t
		}
	}

	if m.dateTo != "" {
		if t, err := time.Parse("2006-01-02", m.dateTo); err == nil {
			criteria.DateTo = &t
		}
	}

	return criteria
}

// SetCriteria sets the filter criteria
func (m *FilterDialogModel) SetCriteria(criteria FilterCriteria) {
	m.searchText = criteria.SearchText

	if len(criteria.Tags) > 0 {
		m.tagFilter = criteria.Tags[0]
	} else {
		m.tagFilter = ""
	}

	if len(criteria.Statuses) > 0 {
		m.statusFilter = criteria.Statuses[0]
	} else {
		m.statusFilter = ""
	}

	if criteria.DateFrom != nil {
		m.dateFrom = criteria.DateFrom.Format("2006-01-02")
	} else {
		m.dateFrom = ""
	}

	if criteria.DateTo != nil {
		m.dateTo = criteria.DateTo.Format("2006-01-02")
	} else {
		m.dateTo = ""
	}
}

// Reset resets all filters
func (m *FilterDialogModel) Reset() {
	m.searchText = ""
	m.statusFilter = ""
	m.tagFilter = ""
	m.dateFrom = ""
	m.dateTo = ""
	m.textInput.Reset()
	m.inputMode = false
}

// HasActiveFilters returns true if any filters are active
func (m FilterDialogModel) HasActiveFilters() bool {
	return m.searchText != "" || m.statusFilter != "" ||
		m.tagFilter != "" || m.dateFrom != "" || m.dateTo != ""
}

// View renders the filter dialog
func (m FilterDialogModel) View() string {
	if !m.visible {
		return ""
	}

	// Dialog box
	dialogStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorMuted).
		Padding(1, 2)

	content := m.renderDialogContent()

	return dialogStyle.Render(content)
}

// renderDialogContent renders the dialog content
func (m FilterDialogModel) renderDialogContent() string {
	var b strings.Builder

	// Header
	headerStyle := lipgloss.NewStyle().
		Foreground(colorTitle).
		Bold(true)
	b.WriteString(headerStyle.Render("Advanced Filters\n\n"))

	if m.inputMode {
		// Input mode view
		b.WriteString(fmt.Sprintf("%s %s", "Enter value:", m.textInput.View()))
		b.WriteString("\n\n")
		b.WriteString(lipgloss.NewStyle().
			Foreground(colorDim).
			Render("Enter: Save  Esc: Cancel"))
	} else {
		// Filter options list
		opts := m.filterOptions()
		for i, opt := range opts {
			cursor := " "
			if i == m.selection {
				cursor = RenderInfo(">")
			}

			// Active indicator
			active := ""
			if opt.Active {
				active = RenderSuccess("✓")
			} else {
				active = styleDim.Render("○")
			}

			// Value display
			valueDisplay := ""
			if opt.Value != "" {
				valueDisplay = fmt.Sprintf(": %s", RenderInfo(opt.Value))
			}

			lineStyle := lipgloss.NewStyle()
			if i == m.selection {
				lineStyle = lineStyle.Bold(true)
			}

			b.WriteString(fmt.Sprintf("%s [%s] %s%s\n",
				lineStyle.Render(cursor),
				active,
				lineStyle.Render(opt.Label),
				valueDisplay,
			))

			if opt.Description != "" {
				b.WriteString(lipgloss.NewStyle().
					Foreground(colorDim).
					Render(fmt.Sprintf("    %s\n", opt.Description)))
			}
		}
	}

	// Footer
	b.WriteString("\n")
	if m.HasActiveFilters() {
		b.WriteString(lipgloss.NewStyle().
			Foreground(colorWarning).
			Render("Press R to reset all filters  "))
	}
	b.WriteString(lipgloss.NewStyle().
		Foreground(colorDim).
		Render("Esc: Close  Enter/Space: Edit"))

	return b.String()
}

// Open opens the dialog
func (m *FilterDialogModel) Open() {
	m.visible = true
	m.selection = 0
}

// Close closes the dialog
func (m *FilterDialogModel) Close() {
	m.visible = false
}

// IsVisible returns true if dialog is visible
func (m FilterDialogModel) IsVisible() bool {
	return m.visible
}

// InInputMode returns true if in text input mode
func (m FilterDialogModel) InInputMode() bool {
	return m.inputMode
}