package components

import (
	"encoding/json"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/pgplex/pgtui/internal/jsonb"
	"github.com/pgplex/pgtui/internal/ui/theme"
)

// PreviewPane displays full content for truncated values
type PreviewPane struct {
	Width     int
	MaxHeight int    // Maximum height (screen 1/3)
	Content   string // Raw content to display
	Title     string // Title (column name or JSON path)

	// Visibility state
	Visible       bool // Whether pane should be shown
	ForceHidden   bool // User manually hid the pane (overrides auto-show)
	IsTruncated   bool // Whether content was truncated in parent view

	// Scrolling
	scrollY       int
	contentLines  []string // Formatted content split into lines

	// Styling
	Theme theme.Theme
	style lipgloss.Style
}

// NewPreviewPane creates a new preview pane
func NewPreviewPane(th theme.Theme) *PreviewPane {
	return &PreviewPane{
		Width:       80,
		MaxHeight:   10,
		Theme:       th,
		ForceHidden: true, // Default to hidden, user must press 'p' to show
		style: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(th.Border).
			Padding(0, 1),
	}
}

// SetContent sets the content to display
// isTruncated indicates whether the content was truncated in the parent view
func (p *PreviewPane) SetContent(content, title string, isTruncated bool) {
	// Skip if content hasn't changed (performance optimization)
	if p.Content == content && p.Title == title {
		return
	}

	p.Content = content
	p.Title = title
	p.IsTruncated = isTruncated
	p.scrollY = 0
	p.contentLines = nil // Clear cached lines, will be formatted on demand
}

// formatContent formats the raw content for display
func (p *PreviewPane) formatContent() {
	if p.Content == "" {
		p.contentLines = []string{}
		return
	}

	// Calculate available width for content
	contentWidth := p.Width - p.style.GetHorizontalFrameSize()
	if contentWidth < 10 {
		contentWidth = 10
	}

	// Try to format as JSON if it looks like JSONB
	formatted := p.Content
	if jsonb.IsJSONB(p.Content) {
		var parsed interface{}
		if err := json.Unmarshal([]byte(p.Content), &parsed); err == nil {
			if pretty, err := json.MarshalIndent(parsed, "", "  "); err == nil {
				formatted = string(pretty)
			}
		}
	}

	// Wrap lines to fit width
	p.contentLines = p.wrapText(formatted, contentWidth)
}

// wrapText wraps text to fit within maxWidth
func (p *PreviewPane) wrapText(text string, maxWidth int) []string {
	var result []string
	lines := strings.Split(text, "\n")

	for _, line := range lines {
		if runewidth.StringWidth(line) <= maxWidth {
			result = append(result, line)
			continue
		}

		// Wrap long lines
		current := ""
		currentWidth := 0
		for _, r := range line {
			rWidth := runewidth.RuneWidth(r)
			if currentWidth+rWidth > maxWidth {
				result = append(result, current)
				current = string(r)
				currentWidth = rWidth
			} else {
				current += string(r)
				currentWidth += rWidth
			}
		}
		if current != "" {
			result = append(result, current)
		}
	}

	return result
}

// Toggle toggles the preview pane visibility
func (p *PreviewPane) Toggle() {
	if p.Visible {
		p.Visible = false
		p.ForceHidden = true
		p.contentLines = nil // Clear formatted content for performance
	} else {
		// Show if we have content
		if p.Content != "" && p.Content != "NULL" {
			p.Visible = true
			p.ForceHidden = false
			p.formatContent()
		}
	}
}

// Height returns the rendered height including borders
// Returns 0 if not visible, otherwise returns MaxHeight for consistent layout
func (p *PreviewPane) Height() int {
	if !p.Visible {
		return 0
	}
	// Always return MaxHeight for consistent layout
	return p.MaxHeight
}

// IsScrollable returns true if content exceeds visible area
func (p *PreviewPane) IsScrollable() bool {
	maxContentHeight := p.MaxHeight - p.style.GetVerticalFrameSize()
	return len(p.contentLines) > maxContentHeight
}

// ScrollUp scrolls content up
func (p *PreviewPane) ScrollUp() {
	if p.scrollY > 0 {
		p.scrollY--
	}
}

// ScrollDown scrolls content down
func (p *PreviewPane) ScrollDown() {
	maxContentHeight := p.MaxHeight - p.style.GetVerticalFrameSize()
	maxContentLines := maxContentHeight - 2 // -2 for header and footer
	if maxContentLines < 1 {
		maxContentLines = 1
	}
	maxScroll := len(p.contentLines) - maxContentLines
	if maxScroll < 0 {
		maxScroll = 0
	}
	if p.scrollY < maxScroll {
		p.scrollY++
	}
}

// GetContent returns the raw content for copying
func (p *PreviewPane) GetContent() string {
	return p.Content
}

// CopyContent copies the preview content to clipboard
func (p *PreviewPane) CopyContent() error {
	return clipboard.WriteAll(p.Content)
}

// View renders the preview pane
func (p *PreviewPane) View() string {
	if !p.Visible {
		return ""
	}

	// Lazy format if needed
	if p.contentLines == nil && p.Content != "" {
		p.formatContent()
	}

	// Calculate dimensions - contentWidth is the space inside borders/padding
	contentWidth := p.Width - p.style.GetHorizontalFrameSize()
	if contentWidth < 10 {
		contentWidth = 10
	}
	maxContentHeight := p.MaxHeight - p.style.GetVerticalFrameSize()
	if maxContentHeight < 1 {
		maxContentHeight = 1
	}

	// Build header
	titleStyle := lipgloss.NewStyle().
		Foreground(p.Theme.Info).
		Bold(true)

	headerText := "Preview"
	if p.Title != "" {
		headerText = "Preview: " + p.Title
	}
	// Truncate header if too long
	if runewidth.StringWidth(headerText) > contentWidth {
		headerText = runewidth.Truncate(headerText, contentWidth-3, "...")
	}
	header := titleStyle.Render(headerText)

	// Get visible content lines
	startLine := p.scrollY
	endLine := startLine + maxContentHeight - 2 // -2 for header and footer
	if endLine > len(p.contentLines) {
		endLine = len(p.contentLines)
	}

	// Helper to pad line to exact width
	padLine := func(line string, width int) string {
		lineWidth := runewidth.StringWidth(line)
		if lineWidth >= width {
			return line
		}
		return line + strings.Repeat(" ", width-lineWidth)
	}

	var contentParts []string

	// Add header (left-aligned, padded to width)
	contentParts = append(contentParts, padLine(header, contentWidth))

	// Add content lines (left-aligned, padded to width)
	contentStyle := lipgloss.NewStyle().Foreground(p.Theme.Foreground)
	for i := startLine; i < endLine; i++ {
		line := p.contentLines[i]
		// Truncate line if too long
		if runewidth.StringWidth(line) > contentWidth {
			line = runewidth.Truncate(line, contentWidth-3, "...")
		}
		// Pad to full width and apply style
		paddedLine := padLine(line, contentWidth)
		contentParts = append(contentParts, contentStyle.Render(paddedLine))
	}

	// Build help text - use abbreviated form for narrow panes
	helpParts := []string{}
	if p.IsScrollable() {
		helpParts = append(helpParts, "J/K:scroll")
	}
	helpParts = append(helpParts, "y:copy", "p:close")

	// Add JSONB hint if content is JSON
	if jsonb.IsJSONB(p.Content) {
		helpParts = append(helpParts, "v:tree")
	}

	helpText := strings.Join(helpParts, " ")

	// Truncate help text if too long
	if runewidth.StringWidth(helpText) > contentWidth {
		helpText = runewidth.Truncate(helpText, contentWidth, "")
	}

	// Build footer with right-aligned help (manually pad on left)
	helpWidth := runewidth.StringWidth(helpText)
	leftPad := contentWidth - helpWidth
	if leftPad < 0 {
		leftPad = 0
	}
	helpStyle := lipgloss.NewStyle().
		Foreground(p.Theme.Metadata).
		Italic(true)
	footer := strings.Repeat(" ", leftPad) + helpStyle.Render(helpText)
	contentParts = append(contentParts, footer)

	// Join content
	content := strings.Join(contentParts, "\n")

	// Apply container style - don't set Width, let content determine size
	innerHeight := p.MaxHeight - p.style.GetVerticalFrameSize()
	if innerHeight < 3 {
		innerHeight = 3
	}

	containerStyle := p.style.
		Height(innerHeight)

	return containerStyle.Render(content)
}
