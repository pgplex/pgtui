package components

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
	"github.com/pgplex/pgtui/internal/models"
	"github.com/pgplex/pgtui/internal/search"
	"github.com/pgplex/pgtui/internal/ui/theme"
)

// Zone ID prefixes for mouse click handling
const (
	ZoneCommandPaletteItemPrefix = "cmd-palette-item-"
)

// CloseCommandPaletteMsg is sent when the command palette should close
type CloseCommandPaletteMsg struct{}

// PaletteMode represents the current mode of the command palette
type PaletteMode int

const (
	PaletteModeDefault  PaletteMode = iota // Commands + Tables/Views
	PaletteModeCommands                    // Only commands (> prefix)
	PaletteModeTables                      // Only tables/views (@ prefix)
	PaletteModeHistory                     // Only history (# prefix)
)

// CommandPalette provides fuzzy search over commands, tables, and history
type CommandPalette struct {
	Input    string
	Query    string // The actual search query (without prefix)
	Mode     PaletteMode

	// Data sources
	Commands []models.Command // Built-in commands
	Tables   []models.Command // Tables and views
	History  []models.Command // Query history

	// Filtered results
	Filtered     []models.Command
	Selected     int
	ScrollOffset int // For scrolling when results exceed visible area

	// Dimensions
	Width  int
	Height int
	Theme  theme.Theme
}

// NewCommandPalette creates a new command palette
func NewCommandPalette(th theme.Theme) *CommandPalette {
	return &CommandPalette{
		Input:    "",
		Query:    "",
		Mode:     PaletteModeDefault,
		Commands: []models.Command{},
		Tables:   []models.Command{},
		History:  []models.Command{},
		Filtered: []models.Command{},
		Selected: 0,
		Width:    80,
		Height:   20,
		Theme:    th,
	}
}

// SetCommands updates the available commands
func (cp *CommandPalette) SetCommands(commands []models.Command) {
	cp.Commands = commands
	cp.Filter()
}

// SetTables updates the available tables/views
func (cp *CommandPalette) SetTables(tables []models.Command) {
	cp.Tables = tables
	cp.Filter()
}

// SetHistory updates the query history
func (cp *CommandPalette) SetHistory(history []models.Command) {
	cp.History = history
	cp.Filter()
}

// Reset clears the input and resets the palette state
func (cp *CommandPalette) Reset() {
	cp.Input = ""
	cp.Query = ""
	cp.Mode = PaletteModeDefault
	cp.Selected = 0
	cp.ScrollOffset = 0
	cp.Filter()
}

// parseInput parses the input to detect mode prefix and extract query
func (cp *CommandPalette) parseInput() {
	if len(cp.Input) == 0 {
		cp.Mode = PaletteModeDefault
		cp.Query = ""
		return
	}

	switch cp.Input[0] {
	case '>':
		cp.Mode = PaletteModeCommands
		cp.Query = strings.TrimSpace(cp.Input[1:])
	case '@':
		cp.Mode = PaletteModeTables
		cp.Query = strings.TrimSpace(cp.Input[1:])
	case '#':
		cp.Mode = PaletteModeHistory
		cp.Query = strings.TrimSpace(cp.Input[1:])
	default:
		cp.Mode = PaletteModeDefault
		cp.Query = cp.Input
	}
}

// Update handles keyboard input for the command palette
func (cp *CommandPalette) Update(msg tea.KeyMsg) (*CommandPalette, tea.Cmd) {
	maxVisible := 8 // Max visible results

	switch msg.String() {
	case "up", "ctrl+p":
		if cp.Selected > 0 {
			cp.Selected--
			// Scroll up if needed
			if cp.Selected < cp.ScrollOffset {
				cp.ScrollOffset = cp.Selected
			}
		}
		return cp, nil

	case "down", "ctrl+n":
		if cp.Selected < len(cp.Filtered)-1 {
			cp.Selected++
			// Scroll down if needed
			if cp.Selected >= cp.ScrollOffset+maxVisible {
				cp.ScrollOffset = cp.Selected - maxVisible + 1
			}
		}
		return cp, nil

	case "enter":
		if cp.Selected < len(cp.Filtered) && cp.Selected >= 0 {
			cmd := cp.Filtered[cp.Selected]
			if cmd.Action != nil {
				return cp, func() tea.Msg {
					result := cmd.Action()
					// Also close the palette
					return result
				}
			}
		}
		return cp, func() tea.Msg {
			return CloseCommandPaletteMsg{}
		}

	case "esc", "ctrl+c":
		return cp, func() tea.Msg {
			return CloseCommandPaletteMsg{}
		}

	case "backspace":
		if len(cp.Input) > 0 {
			cp.Input = cp.Input[:len(cp.Input)-1]
			cp.parseInput()
			cp.Filter()
		}
		return cp, nil

	default:
		key := msg.String()
		// Only accept single printable characters
		if len(key) == 1 && key[0] >= 32 && key[0] <= 126 {
			cp.Input += key
			cp.parseInput()
			cp.Filter()
		}
		return cp, nil
	}
}

// HandleMouseWheel handles mouse wheel events for scrolling
// Returns true if the event was handled
func (cp *CommandPalette) HandleMouseWheel(msg tea.MouseMsg) bool {
	maxVisible := 8
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		// Scroll viewport up (like lazygit)
		cp.ScrollOffset -= 3
		if cp.ScrollOffset < 0 {
			cp.ScrollOffset = 0
		}
		// Keep selection within visible range
		if cp.Selected >= cp.ScrollOffset+maxVisible {
			cp.Selected = cp.ScrollOffset + maxVisible - 1
		}
		if cp.Selected < cp.ScrollOffset {
			cp.Selected = cp.ScrollOffset
		}
		return true
	case tea.MouseButtonWheelDown:
		// Scroll viewport down (like lazygit)
		maxScrollOffset := len(cp.Filtered) - maxVisible
		if maxScrollOffset < 0 {
			maxScrollOffset = 0
		}
		cp.ScrollOffset += 3
		if cp.ScrollOffset > maxScrollOffset {
			cp.ScrollOffset = maxScrollOffset
		}
		// Keep selection within visible range
		if cp.Selected < cp.ScrollOffset {
			cp.Selected = cp.ScrollOffset
		}
		if cp.Selected >= cp.ScrollOffset+maxVisible {
			cp.Selected = cp.ScrollOffset + maxVisible - 1
		}
		// Bounds check
		if cp.Selected >= len(cp.Filtered) {
			cp.Selected = len(cp.Filtered) - 1
		}
		if cp.Selected < 0 {
			cp.Selected = 0
		}
		return true
	}
	return false
}

// HandleMouseClick handles mouse click events
// Returns true if click was handled, and a command if an item was selected
func (cp *CommandPalette) HandleMouseClick(msg tea.MouseMsg) (handled bool, cmd tea.Cmd) {
	// Only handle left click press events
	if msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress {
		return false, nil
	}

	// Check each visible item zone
	maxResults := 8
	startIdx := cp.ScrollOffset
	endIdx := cp.ScrollOffset + maxResults
	if endIdx > len(cp.Filtered) {
		endIdx = len(cp.Filtered)
	}

	for i := startIdx; i < endIdx; i++ {
		zoneID := fmt.Sprintf("%s%d", ZoneCommandPaletteItemPrefix, i)
		if zone.Get(zoneID).InBounds(msg) {
			// Double-click or click on already selected item executes
			if i == cp.Selected {
				if cp.Selected < len(cp.Filtered) {
					cmdItem := cp.Filtered[cp.Selected]
					if cmdItem.Action != nil {
						return true, func() tea.Msg {
							return cmdItem.Action()
						}
					}
				}
				return true, func() tea.Msg {
					return CloseCommandPaletteMsg{}
				}
			}
			// First click selects
			cp.Selected = i
			return true, nil
		}
	}

	return false, nil
}

// Filter filters commands based on input and mode, updates the filtered list
func (cp *CommandPalette) Filter() {
	// Determine which data sources to search based on mode
	var sources [][]models.Command

	switch cp.Mode {
	case PaletteModeCommands:
		sources = [][]models.Command{cp.Commands}
	case PaletteModeTables:
		sources = [][]models.Command{cp.Tables}
	case PaletteModeHistory:
		sources = [][]models.Command{cp.History}
	default: // PaletteModeDefault - Commands + Tables
		sources = [][]models.Command{cp.Commands, cp.Tables}
	}

	// If no query, show all items from selected sources
	if cp.Query == "" {
		var all []models.Command
		for _, source := range sources {
			all = append(all, source...)
		}
		cp.Filtered = all
		cp.Selected = 0
		return
	}

	filtered := []models.Command{}

	for _, source := range sources {
		for _, cmd := range source {
			// Try fuzzy matching on label first
			matchLabel := search.FuzzyMatch(cp.Query, cmd.Label)
			matchDesc := search.FuzzyMatch(cp.Query, cmd.Description)

			// Check tags
			var matchTag search.Match
			for _, tag := range cmd.Tags {
				tagMatch := search.FuzzyMatch(cp.Query, tag)
				if tagMatch.Matched && tagMatch.Score > matchTag.Score {
					matchTag = tagMatch
				}
			}

			// Use best match
			bestScore := 0
			matched := false

			if matchLabel.Matched {
				bestScore = matchLabel.Score + 50 // Bonus for label match
				matched = true
			}
			if matchDesc.Matched && matchDesc.Score > bestScore {
				bestScore = matchDesc.Score + 25 // Bonus for description match
				matched = true
			}
			if matchTag.Matched && matchTag.Score > bestScore {
				bestScore = matchTag.Score + 10 // Bonus for tag match
				matched = true
			}

			if matched {
				cmdCopy := cmd
				cmdCopy.Score = bestScore
				filtered = append(filtered, cmdCopy)
			}
		}
	}

	// Sort by score (descending)
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Score > filtered[j].Score
	})

	cp.Filtered = filtered
	cp.Selected = 0
	cp.ScrollOffset = 0
}

// GetSelectedCommand returns the currently selected command, or nil if none
func (cp *CommandPalette) GetSelectedCommand() *models.Command {
	if cp.Selected >= 0 && cp.Selected < len(cp.Filtered) {
		return &cp.Filtered[cp.Selected]
	}
	return nil
}

// getPlaceholder returns the placeholder text based on current mode
func (cp *CommandPalette) getPlaceholder() string {
	switch cp.Mode {
	case PaletteModeCommands:
		return "Search commands..."
	case PaletteModeTables:
		return "Search tables and views..."
	case PaletteModeHistory:
		return "Search query history..."
	default:
		return "Search commands and tables..."
	}
}

// getModePrefix returns the display prefix for the current mode
func (cp *CommandPalette) getModePrefix() string {
	switch cp.Mode {
	case PaletteModeCommands:
		return "> "
	case PaletteModeTables:
		return "@ "
	case PaletteModeHistory:
		return "# "
	default:
		return ""
	}
}

// View renders the command palette
func (cp *CommandPalette) View() string {
	// Input box
	inputStyle := lipgloss.NewStyle().
		Foreground(cp.Theme.Foreground).
		Background(cp.Theme.Selection).
		Padding(0, 1).
		Width(cp.Width - 4)

	cursor := lipgloss.NewStyle().
		Foreground(cp.Theme.Cursor).
		Render("█")

	// Build input display
	var inputContent string
	if cp.Input == "" {
		// Show placeholder
		placeholder := lipgloss.NewStyle().
			Foreground(cp.Theme.Comment).
			Render(cp.getPlaceholder())
		inputContent = placeholder + cursor
	} else {
		// Show prefix in color if present
		prefix := cp.getModePrefix()
		if prefix != "" {
			prefixStyled := lipgloss.NewStyle().
				Foreground(cp.Theme.BorderFocused).
				Bold(true).
				Render(prefix)
			inputContent = prefixStyled + cp.Query + cursor
		} else {
			inputContent = cp.Input + cursor
		}
	}

	input := inputStyle.Render(inputContent)

	// Separator
	separator := lipgloss.NewStyle().
		Foreground(cp.Theme.Border).
		Render(strings.Repeat("─", cp.Width-4))

	// Results list with scrolling
	maxResults := 8
	results := []string{}

	// Calculate visible range based on scroll offset
	startIdx := cp.ScrollOffset
	endIdx := cp.ScrollOffset + maxResults
	if endIdx > len(cp.Filtered) {
		endIdx = len(cp.Filtered)
	}

	for i := startIdx; i < endIdx; i++ {
		cmd := cp.Filtered[i]
		isSelected := i == cp.Selected

		// Build content with styled parts
		var content string

		// Icon with color (unless selected, then use selection foreground)
		if cmd.Icon != "" {
			iconStyle := lipgloss.NewStyle().Foreground(cp.Theme.Info)
			if isSelected {
				iconStyle = lipgloss.NewStyle().Foreground(cp.Theme.Background)
			}
			content = iconStyle.Render(cmd.Icon) + " "
		}

		// Label
		labelStyle := lipgloss.NewStyle()
		if isSelected {
			labelStyle = labelStyle.Bold(true)
		}
		content += labelStyle.Render(cmd.Label)

		// Description
		if cmd.Description != "" {
			descStyle := lipgloss.NewStyle().Foreground(cp.Theme.Metadata)
			if isSelected {
				descStyle = lipgloss.NewStyle().Foreground(cp.Theme.Background)
			}
			content += descStyle.Render(" - " + cmd.Description)
		}

		// Line style (background for selection)
		lineStyle := lipgloss.NewStyle().
			Padding(0, 1).
			Width(cp.Width - 4)

		if isSelected {
			lineStyle = lineStyle.Background(cp.Theme.BorderFocused)
		}

		line := lineStyle.Render(content)
		// Wrap with zone mark for mouse click detection
		zoneID := fmt.Sprintf("%s%d", ZoneCommandPaletteItemPrefix, i)
		results = append(results, zone.Mark(zoneID, line))
	}

	// Empty state
	if len(cp.Filtered) == 0 {
		emptyStyle := lipgloss.NewStyle().
			Foreground(cp.Theme.Comment).
			Italic(true).
			Padding(0, 1).
			Width(cp.Width - 4).
			Align(lipgloss.Center)
		results = append(results, emptyStyle.Render("No results found"))
	}

	// Pad results to fixed height to prevent layout jumping
	emptyLineStyle := lipgloss.NewStyle().
		Width(cp.Width - 4).
		Padding(0, 1)
	for len(results) < maxResults {
		results = append(results, emptyLineStyle.Render(""))
	}

	// Mode hints at bottom - keyboard shortcut style
	bracketStyle := lipgloss.NewStyle().
		Foreground(cp.Theme.Border)

	keyStyle := lipgloss.NewStyle().
		Foreground(cp.Theme.BorderFocused).
		Bold(true)

	labelStyle := lipgloss.NewStyle().
		Foreground(cp.Theme.Comment)

	// Build hint items: [>] Commands  [@] Tables  [#] History
	cmdHint := bracketStyle.Render("[") + keyStyle.Render(">") + bracketStyle.Render("]") +
		labelStyle.Render(" Commands")

	tableHint := bracketStyle.Render("[") + keyStyle.Render("@") + bracketStyle.Render("]") +
		labelStyle.Render(" Tables")

	historyHint := bracketStyle.Render("[") + keyStyle.Render("#") + bracketStyle.Render("]") +
		labelStyle.Render(" History")

	hints := cmdHint + labelStyle.Render("   ") + tableHint + labelStyle.Render("   ") + historyHint

	hintLine := lipgloss.NewStyle().
		Width(cp.Width - 4).
		Padding(0, 1).
		Render(hints)

	// Bottom separator
	bottomSeparator := lipgloss.NewStyle().
		Foreground(cp.Theme.Border).
		Render(strings.Repeat("─", cp.Width-4))

	// Combine
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		input,
		separator,
		lipgloss.JoinVertical(lipgloss.Left, results...),
		bottomSeparator,
		hintLine,
	)

	// Box
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(cp.Theme.BorderFocused).
		Padding(1, 2).
		Width(cp.Width)

	return boxStyle.Render(content)
}
