package main

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// FilterBarModel represents a filter input component
type FilterBarModel struct {
	textInput textinput.Model
	active    bool
	visible   bool
}

// NewFilterBar creates a new filter bar
func NewFilterBar() FilterBarModel {
	ti := textinput.New()
	ti.Placeholder = "Filter..."
	ti.CharLimit = 50
	ti.PromptStyle = lipgloss.NewStyle().
		Foreground(colorDim).
		Padding(0, 1)
	ti.TextStyle = lipgloss.NewStyle().
		Foreground(colorInfo)
	ti.Width = 30

	return FilterBarModel{
		textInput: ti,
		visible:   true,
	}
}

// Init initializes the filter bar
func (m FilterBarModel) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles messages
func (m FilterBarModel) Update(msg tea.Msg) (FilterBarModel, tea.Cmd) {
	var cmd tea.Cmd

	if m.active {
		m.textInput, cmd = m.textInput.Update(msg)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "/":
			m.active = true
			m.textInput.Reset()
			m.textInput.Focus()
			return m, textinput.Blink
		case "esc":
			if m.active {
				m.active = false
				m.textInput.Blur()
				m.textInput.Reset()
				return m, nil
			}
		case "enter":
			if m.active {
				m.active = false
				m.textInput.Blur()
				return m, func() tea.Msg {
					return filterTextChangedMsg{text: m.textInput.Value()}
				}
			}
		}
	}

	return m, cmd
}

// View renders the filter bar
func (m FilterBarModel) View() string {
	if !m.visible {
		return ""
	}

	var cursor string
	if m.active {
		cursor = RenderInfo("▶")
	} else {
		cursor = styleDim.Render("/")
	}

	return cursor + " " + m.textInput.View()
}

// Value returns the current filter value
func (m FilterBarModel) Value() string {
	return m.textInput.Value()
}

// Reset resets the filter bar
func (m *FilterBarModel) Reset() {
	m.textInput.Reset()
	m.active = false
	m.textInput.Blur()
}

// Activate activates the filter bar
func (m *FilterBarModel) Activate() {
	m.active = true
	m.textInput.Focus()
}

// Deactivate deactivates the filter bar
func (m *FilterBarModel) Deactivate() {
	m.active = false
	m.textInput.Blur()
}

// IsActive returns true if filter bar is active
func (m FilterBarModel) IsActive() bool {
	return m.active
}

// SetVisible sets the visibility of the filter bar
func (m *FilterBarModel) SetVisible(visible bool) {
	m.visible = visible
}
