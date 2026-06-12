package components

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
	"github.com/pgplex/pgtui/internal/ui/theme"
)

// Zone ID for error overlay dismiss button
const (
	ZoneErrorDismiss = "error-dismiss"
)

// CloseErrorOverlayMsg is sent when error overlay should close
type CloseErrorOverlayMsg struct{}

// ErrorOverlay represents an error message overlay
type ErrorOverlay struct {
	Title   string
	Message string
	Width   int
	Height  int
	Theme   theme.Theme
}

// NewErrorOverlay creates a new error overlay
func NewErrorOverlay(th theme.Theme) *ErrorOverlay {
	return &ErrorOverlay{
		Theme:  th,
		Width:  60,
		Height: 15,
	}
}

// SetError sets the error title and message
func (e *ErrorOverlay) SetError(title, message string) {
	e.Title = title
	e.Message = message
}

// View renders the error overlay
func (e *ErrorOverlay) View() string {
	if e.Width <= 0 || e.Height <= 0 {
		return ""
	}

	// Title style with error color
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(e.Theme.Error).
		Padding(0, 1)

	// Message style - use MaxWidth to constrain, not Width
	messageStyle := lipgloss.NewStyle().
		Foreground(e.Theme.Foreground).
		Padding(1, 2).
		MaxWidth(e.Width - 8) // Account for border (2) + padding (4) + margin (2)

	// Footer style (dimmed)
	footerStyle := lipgloss.NewStyle().
		Faint(true).
		Foreground(e.Theme.Foreground).
		Align(lipgloss.Center).
		MaxWidth(e.Width - 8)

	// Build content
	var content strings.Builder

	// Title
	content.WriteString(titleStyle.Render("Error: " + e.Title))
	content.WriteString("\n\n")

	// Message - wrap text to fit width
	wrappedMessage := wrapText(e.Message, e.Width-12) // More conservative wrapping
	content.WriteString(messageStyle.Render(wrappedMessage))
	content.WriteString("\n")

	// Footer with clickable dismiss text
	dismissText := footerStyle.Render("Press Enter or Esc to dismiss")
	content.WriteString(zone.Mark(ZoneErrorDismiss, dismissText))

	// Box style with error border - don't set Width, let it size naturally
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(e.Theme.Error).
		Padding(1, 2).
		MaxWidth(e.Width).
		Background(e.Theme.Background)

	return boxStyle.Render(content.String())
}

// HandleMouseClick handles mouse click events
// Returns true if click was handled (clicking anywhere on overlay dismisses it)
func (e *ErrorOverlay) HandleMouseClick(msg tea.MouseMsg) (handled bool, cmd tea.Cmd) {
	// Only handle left click press events
	if msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress {
		return false, nil
	}

	// Check if clicked on dismiss zone (or anywhere on overlay to dismiss)
	if zone.Get(ZoneErrorDismiss).InBounds(msg) {
		return true, func() tea.Msg {
			return CloseErrorOverlayMsg{}
		}
	}

	// Any click on overlay area could dismiss - handled by app.go checking overlay bounds
	return false, nil
}

// wrapText wraps text to fit within the specified width
func wrapText(text string, width int) string {
	if width <= 0 {
		return text
	}

	lines := strings.Split(text, "\n")
	var wrapped []string

	for _, line := range lines {
		if len(line) <= width {
			wrapped = append(wrapped, line)
			continue
		}

		// Wrap long lines
		words := strings.Fields(line)
		if len(words) == 0 {
			wrapped = append(wrapped, line)
			continue
		}

		currentLine := words[0]
		for _, word := range words[1:] {
			if len(currentLine)+1+len(word) <= width {
				currentLine += " " + word
			} else {
				wrapped = append(wrapped, currentLine)
				currentLine = word
			}
		}
		wrapped = append(wrapped, currentLine)
	}

	return strings.Join(wrapped, "\n")
}
