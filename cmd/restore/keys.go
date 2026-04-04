package main

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Key bindings
const (
	KeyQuit      = "ctrl+c"
	KeyHelp      = "?"
	KeyHelpAlt   = "f1"
	KeyBack      = "esc"
	KeySelect    = "enter"
	KeyToggle    = " "
	KeySelectAll = "ctrl+a"
	KeySearch    = "/"
	KeyTab       = "tab"
	KeyUp        = "up"
	KeyDown      = "down"
	KeyUpVim     = "k"
	KeyDownVim   = "j"
)

// KeyMap maps keys to descriptions
var KeyMap = map[string]string{
	KeyQuit:      "Quit",
	KeyHelp:      "Show help",
	KeyHelpAlt:   "Show help",
	KeyBack:      "Go back",
	KeySelect:    "Confirm",
	KeyToggle:    "Toggle selection",
	KeySelectAll: "Select all",
	KeySearch:    "Search",
	KeyTab:       "Switch panels",
	KeyUp:        "Move up",
	KeyDown:      "Move down",
	KeyUpVim:     "Move up",
	KeyDownVim:   "Move down",
}

// KeyGroup groups related keys
type KeyGroup struct {
	Title string
	Keys  []KeyBinding
}

// KeyBinding represents a key and its description
type KeyBinding struct {
	Key         string
	Description string
}

// GlobalKeyBindings returns key bindings for global keys
func GlobalKeyBindings() []KeyBinding {
	return []KeyBinding{
		{Key: "?", Description: "Show help"},
		{Key: "q", Description: "Quit"},
		{Key: "Ctrl+C", Description: "Quit"},
	}
}

// NavigationKeyBindings returns key bindings for navigation
func NavigationKeyBindings() []KeyBinding {
	return []KeyBinding{
		{Key: "↑/k", Description: "Move up"},
		{Key: "↓/j", Description: "Move down"},
		{Key: "Enter", Description: "Select/Confirm"},
		{Key: "Esc", Description: "Go back"},
	}
}

// SelectionKeyBindings returns key bindings for item selection
func SelectionKeyBindings() []KeyBinding {
	return []KeyBinding{
		{Key: "Space", Description: "Toggle selection"},
		{Key: "Ctrl+A", Description: "Select all"},
		{Key: "/", Description: "Search"},
	}
}

// IsQuitKey checks if a key press is a quit key
func IsQuitKey(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "q", "ctrl+c":
		return true
	}
	return false
}

// IsBackKey checks if a key press is a back key
func IsBackKey(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "esc":
		return true
	}
	return false
}

// IsSelectKey checks if a key press is a select key
func IsSelectKey(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "enter":
		return true
	}
	return false
}

// IsUpKey checks if a key press is an up key
func IsUpKey(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "up", "k":
		return true
	}
	return false
}

// IsDownKey checks if a key press is a down key
func IsDownKey(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "down", "j":
		return true
	}
	return false
}

// IsToggleKey checks if a key press is a toggle key
func IsToggleKey(msg tea.KeyMsg) bool {
	switch msg.String() {
	case " ":
		return true
	}
	return false
}

// FormatKeyBindings formats a slice of key bindings for display
func FormatKeyBindings(bindings []KeyBinding) string {
	var result string
	for _, kb := range bindings {
		result += FormatKeyBinding(kb.Key, kb.Description) + "\n"
	}
	return result
}

// FormatKeyBinding formats a single key binding for display
func FormatKeyBinding(key, description string) string {
	return lipgloss.NewStyle().
		Width(20).
		Render(lipgloss.NewStyle().Foreground(colorInfo).Render(key) + " " +
			lipgloss.NewStyle().Foreground(colorDim).Render(description))
}

// HelpContent returns the help content
func HelpContent() string {
	return `
┌─────────────────────────────────────────────────────────────────┐
│  HELP - Shopify Restore CLI                                      │
└─────────────────────────────────────────────────────────────────┘

  GLOBAL SHORTCUTS
  ─────────────────
  ?, F1       Show this help screen
  q, Ctrl+C  Quit application
  Esc        Go back / Cancel

  NAVIGATION
  ───────────
  ↑, k       Move cursor up
  ↓, j       Move cursor down
  Tab        Switch panels / entities
  Enter      Select / Confirm

  SELECTION
  ──────────
  Space      Toggle item selection
  Ctrl+A     Select all items
  /          Start search/filter

  RESTORE ACTIONS
  ────────────────
  y, Y       Confirm restore
  n, N       Go back

  STATUS KEYS
  ───────────
  Esc        Abort restore (shows options)

  Press any key to close help
`
}