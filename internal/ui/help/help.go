package help

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// KeyBinding represents a keyboard shortcut
type KeyBinding struct {
	Key         string
	Description string
}

// GetGlobalKeys returns global key bindings
func GetGlobalKeys() []KeyBinding {
	return []KeyBinding{
		{"?", "Toggle help"},
		{"q, Ctrl+C", "Quit application"},
		{"Esc/Enter", "Dismiss error"},
		{"Ctrl+K", "Open command palette"},
		{"Ctrl+P", "Quick query"},
		{"Tab", "Switch panel focus"},
		{"c", "Open connection dialog"},
		{"r, F5", "Refresh current view"},
	}
}

// GetConnectionKeys returns connection key bindings
func GetConnectionKeys() []KeyBinding {
	return []KeyBinding{
		{"d", "Disconnect"},
		{"Ctrl+R", "Reconnect"},
		{"Ctrl+D", "Show all connections"},
	}
}

// GetNavigationKeys returns navigation key bindings
func GetNavigationKeys() []KeyBinding {
	return []KeyBinding{
		{"↑/k", "Move up"},
		{"↓/j", "Move down"},
		{"←/h", "Collapse or move left"},
		{"→/l", "Expand or move right"},
		{"Enter", "Select item"},
		{"Backspace", "Go to parent"},
	}
}

// GetDataViewKeys returns data view key bindings
func GetDataViewKeys() []KeyBinding {
	return []KeyBinding{
		{"f", "Open filter builder"},
		{"Ctrl+F", "Quick filter from cell"},
		{"Ctrl+R", "Clear filter"},
		{"J", "Open JSONB viewer (on JSONB cell)"},
		{"s", "Toggle sort on column (ASC/DESC)"},
		{"S", "Toggle NULLS FIRST/LAST"},
		{"h/l", "Move column left/right"},
		{"H/L", "Jump scroll half screen"},
		{"0", "Jump to first column"},
		{"$", "Jump to last column"},
		{"/", "Open search (Tab to toggle mode)"},
		{"n/N", "Next/Previous search match"},
	}
}

// GetStructureViewKeys returns structure view key bindings
func GetStructureViewKeys() []KeyBinding {
	return []KeyBinding{
		{"1/2/3/4", "Switch tabs (Data/Columns/Constraints/Indexes)"},
		{"↑↓ or j/k", "Navigate rows"},
		{"y", "Copy name"},
		{"Y", "Copy definition"},
	}
}

// Render creates the help view
func Render(width, height int, theme lipgloss.Style) string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("62")).
		Padding(1, 0)

	sectionStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("75")).
		Padding(0, 0, 0, 2)

	keyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("220")).
		Width(20)

	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))

	var b strings.Builder

	// Title
	b.WriteString(titleStyle.Render("pgtui - Keyboard Shortcuts"))
	b.WriteString("\n\n")

	// Global keys
	b.WriteString(sectionStyle.Render("Global"))
	b.WriteString("\n")
	for _, kb := range GetGlobalKeys() {
		b.WriteString("  ")
		b.WriteString(keyStyle.Render(kb.Key))
		b.WriteString(descStyle.Render(kb.Description))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// Connection keys
	b.WriteString(sectionStyle.Render("Connection"))
	b.WriteString("\n")
	for _, kb := range GetConnectionKeys() {
		b.WriteString("  ")
		b.WriteString(keyStyle.Render(kb.Key))
		b.WriteString(descStyle.Render(kb.Description))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// Navigation keys
	b.WriteString(sectionStyle.Render("Navigation"))
	b.WriteString("\n")
	for _, kb := range GetNavigationKeys() {
		b.WriteString("  ")
		b.WriteString(keyStyle.Render(kb.Key))
		b.WriteString(descStyle.Render(kb.Description))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// Data view keys
	b.WriteString(sectionStyle.Render("Data View"))
	b.WriteString("\n")
	for _, kb := range GetDataViewKeys() {
		b.WriteString("  ")
		b.WriteString(keyStyle.Render(kb.Key))
		b.WriteString(descStyle.Render(kb.Description))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// Structure view keys
	b.WriteString(sectionStyle.Render("Structure View"))
	b.WriteString("\n")
	for _, kb := range GetStructureViewKeys() {
		b.WriteString("  ")
		b.WriteString(keyStyle.Render(kb.Key))
		b.WriteString(descStyle.Render(kb.Description))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	b.WriteString(lipgloss.NewStyle().Faint(true).Render("Press '?' or Esc to close help"))

	// Wrap in a box
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Width(width - 4).
		Height(height - 4)

	return boxStyle.Render(b.String())
}
