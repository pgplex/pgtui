package components

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/pgplex/pgtui/internal/ui/theme"
)

// SearchInputMsg is sent when search should be executed
type SearchInputMsg struct {
	Query string
	Mode  string // "local" or "table"
}

// CloseSearchMsg is sent when search should be closed
type CloseSearchMsg struct{}

// SearchInput provides a search input box
type SearchInput struct {
	Input   textinput.Model
	Mode    string // "local" or "table"
	Theme   theme.Theme
	Width   int
	Visible bool
}

// NewSearchInput creates a new search input
func NewSearchInput(th theme.Theme) *SearchInput {
	ti := textinput.New()
	ti.Placeholder = "Search..."
	ti.Focus()
	ti.CharLimit = 256
	ti.Width = 40

	return &SearchInput{
		Input: ti,
		Mode:  "local",
		Theme: th,
	}
}

// ToggleMode switches between local and table search
func (s *SearchInput) ToggleMode() {
	if s.Mode == "local" {
		s.Mode = "table"
	} else {
		s.Mode = "local"
	}
}

// Reset clears the search input
func (s *SearchInput) Reset() {
	s.Input.SetValue("")
	s.Mode = "local"
}

// Update handles messages
func (s *SearchInput) Update(msg tea.Msg) (*SearchInput, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			s.ToggleMode()
			return s, nil
		case "enter":
			query := s.Input.Value()
			if query != "" {
				return s, func() tea.Msg {
					return SearchInputMsg{Query: query, Mode: s.Mode}
				}
			}
			return s, nil
		case "esc":
			return s, func() tea.Msg {
				return CloseSearchMsg{}
			}
		}
	}

	var cmd tea.Cmd
	s.Input, cmd = s.Input.Update(msg)
	return s, cmd
}

// View renders the search input
func (s *SearchInput) View() string {
	modeIndicator := "[Local]"
	modeColor := lipgloss.Color("#a6e3a1") // Green for local
	if s.Mode == "table" {
		modeIndicator = "[Table]"
		modeColor = lipgloss.Color("#89b4fa") // Blue for table
	}

	modeStyle := lipgloss.NewStyle().
		Foreground(modeColor).
		Bold(true)

	// Calculate input width
	inputWidth := s.Width - 20 // Reserve space for mode indicator and icon
	if inputWidth < 20 {
		inputWidth = 20
	}
	s.Input.Width = inputWidth

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(s.Theme.BorderFocused).
		Padding(0, 1).
		Width(s.Width)

	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6c7086")).
		Italic(true)

	content := modeStyle.Render(modeIndicator) + " 🔍 " + s.Input.View()
	helpText := helpStyle.Render("Tab: toggle mode │ Enter: search │ Esc: close")

	return boxStyle.Render(content + "\n" + helpText)
}
