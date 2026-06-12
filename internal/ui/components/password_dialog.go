package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
	"github.com/pgplex/pgtui/internal/ui/theme"
)

// Zone IDs for password dialog
const (
	ZonePasswordSubmit = "password-submit"
	ZonePasswordCancel = "password-cancel"
)

// PasswordSubmitMsg is sent when password is submitted
type PasswordSubmitMsg struct {
	Password string
}

// PasswordCancelMsg is sent when password dialog is cancelled
type PasswordCancelMsg struct{}

// PasswordDialog represents a password input dialog
type PasswordDialog struct {
	Title       string
	Description string
	Width       int
	Height      int
	Theme       theme.Theme

	input    textinput.Model
	host     string
	port     int
	database string
	user     string
}

// NewPasswordDialog creates a new password dialog
func NewPasswordDialog(th theme.Theme) *PasswordDialog {
	input := textinput.New()
	input.Placeholder = "Enter password"
	input.EchoMode = textinput.EchoPassword
	input.EchoCharacter = '•'
	input.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#cba6f7"))
	input.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#cdd6f4"))
	input.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#f38ba8"))
	input.CharLimit = 256
	input.Width = 40
	input.Focus()

	return &PasswordDialog{
		Theme:  th,
		Width:  50,
		Height: 12,
		input:  input,
	}
}

// SetConnectionInfo sets the connection info to display
func (p *PasswordDialog) SetConnectionInfo(host string, port int, database, user string) {
	p.host = host
	p.port = port
	p.database = database
	p.user = user
	p.Title = "Password Required"
	// Description will be built in View() with proper formatting
	p.Description = ""
	p.input.SetValue("")
	p.input.Focus()
}

// Init initializes the password dialog
func (p *PasswordDialog) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles messages
func (p *PasswordDialog) Update(msg tea.Msg) (*PasswordDialog, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			return p, func() tea.Msg {
				return PasswordSubmitMsg{Password: p.input.Value()}
			}
		case "esc":
			return p, func() tea.Msg {
				return PasswordCancelMsg{}
			}
		}
	}

	p.input, cmd = p.input.Update(msg)
	return p, cmd
}

// View renders the password dialog
func (p *PasswordDialog) View() string {
	if p.Width <= 0 || p.Height <= 0 {
		return ""
	}

	// Fixed dialog width for consistent layout
	dialogWidth := 56
	contentWidth := dialogWidth - 6 // account for border and padding

	// Update input width to match content
	p.input.Width = contentWidth - 4

	// Styles
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(p.Theme.Info)

	labelStyle := lipgloss.NewStyle().
		Foreground(p.Theme.Metadata)

	valueStyle := lipgloss.NewStyle().
		Foreground(p.Theme.Foreground)

	inputLabelStyle := lipgloss.NewStyle().
		Foreground(p.Theme.Info)

	hintStyle := lipgloss.NewStyle().
		Faint(true).
		Foreground(p.Theme.Metadata)

	// Build content with fixed width centering
	lineStyle := lipgloss.NewStyle().Width(contentWidth)

	var lines []string

	// Title (centered)
	lines = append(lines, lineStyle.Align(lipgloss.Center).Render(titleStyle.Render(p.Title)))
	lines = append(lines, "")

	// Connection info (two lines)
	userLine := labelStyle.Render("User: ") + valueStyle.Render(p.user)
	hostLine := labelStyle.Render("Host: ") + valueStyle.Render(fmt.Sprintf("%s:%d/%s", p.host, p.port, p.database))
	lines = append(lines, lineStyle.Render(userLine))
	lines = append(lines, lineStyle.Render(hostLine))
	lines = append(lines, "")

	// Password input
	lines = append(lines, lineStyle.Render(inputLabelStyle.Render("Password:")))
	lines = append(lines, lineStyle.Render(" "+p.input.View()))
	lines = append(lines, "")

	// Footer with buttons (centered)
	submitBtn := zone.Mark(ZonePasswordSubmit, "[Enter] Submit")
	cancelBtn := zone.Mark(ZonePasswordCancel, "[Esc] Cancel")
	footer := hintStyle.Render(submitBtn + "    " + cancelBtn)
	lines = append(lines, lineStyle.Align(lipgloss.Center).Render(footer))

	content := strings.Join(lines, "\n")

	// Box style
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.Theme.BorderFocused).
		Padding(1, 2).
		Width(dialogWidth)

	return boxStyle.Render(content)
}

// HandleMouseClick handles mouse click events
func (p *PasswordDialog) HandleMouseClick(msg tea.MouseMsg) (handled bool, cmd tea.Cmd) {
	if msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress {
		return false, nil
	}

	if zone.Get(ZonePasswordSubmit).InBounds(msg) {
		return true, func() tea.Msg {
			return PasswordSubmitMsg{Password: p.input.Value()}
		}
	}

	if zone.Get(ZonePasswordCancel).InBounds(msg) {
		return true, func() tea.Msg {
			return PasswordCancelMsg{}
		}
	}

	return false, nil
}

// GetPassword returns the entered password
func (p *PasswordDialog) GetPassword() string {
	return p.input.Value()
}

// GetConnectionInfo returns the connection info
func (p *PasswordDialog) GetConnectionInfo() (host string, port int, database, user string) {
	return p.host, p.port, p.database, p.user
}
