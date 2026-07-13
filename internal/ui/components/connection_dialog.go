package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
	"github.com/pgplex/pgtui/internal/models"
	"github.com/pgplex/pgtui/internal/ui/theme"
)

// ConnectionDialog represents a connection dialog
type ConnectionDialog struct {
	Width               int
	Height              int
	Theme               theme.Theme
	DiscoveredInstances []models.DiscoveredInstance
	HistoryEntries      []models.ConnectionHistoryEntry
	ManualMode          bool
	SelectedIndex       int
	InHistorySection    bool // true = selecting in history, false = selecting in discovered

	// Search
	SearchMode  bool // true = user is typing in search box
	searchInput textinput.Model

	// Text input fields for manual mode
	inputs     []textinput.Model
	focusIndex int
	cursorMode cursor.Mode
}

const (
	hostField = iota
	portField
	databaseField
	userField
	passwordField
)

// Zone IDs for mouse click handling
const (
	ZoneHistoryPrefix    = "conn-history-"
	ZoneDiscoveredPrefix = "conn-discovered-"
	ZoneSearchBox        = "conn-search"
	ZoneManualField      = "conn-manual-field-"
)

// NewConnectionDialog creates a new connection dialog
func NewConnectionDialog(th theme.Theme) *ConnectionDialog {
	// Create text inputs for each field
	inputs := make([]textinput.Model, 5)

	// Host input
	inputs[hostField] = textinput.New()
	inputs[hostField].Placeholder = "localhost"
	inputs[hostField].Focus()
	inputs[hostField].PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#cba6f7"))
	inputs[hostField].TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#cdd6f4"))
	inputs[hostField].Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#f38ba8"))
	inputs[hostField].CharLimit = 100
	inputs[hostField].Width = 40

	// Port input
	inputs[portField] = textinput.New()
	inputs[portField].Placeholder = "5432"
	inputs[portField].SetValue("5432")
	inputs[portField].PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#cba6f7"))
	inputs[portField].TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#cdd6f4"))
	inputs[portField].Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#f38ba8"))
	inputs[portField].CharLimit = 5
	inputs[portField].Width = 40

	// Database input
	inputs[databaseField] = textinput.New()
	inputs[databaseField].Placeholder = "postgres"
	inputs[databaseField].PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#cba6f7"))
	inputs[databaseField].TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#cdd6f4"))
	inputs[databaseField].Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#f38ba8"))
	inputs[databaseField].CharLimit = 100
	inputs[databaseField].Width = 40

	// User input
	inputs[userField] = textinput.New()
	inputs[userField].Placeholder = "postgres"
	inputs[userField].PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#cba6f7"))
	inputs[userField].TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#cdd6f4"))
	inputs[userField].Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#f38ba8"))
	inputs[userField].CharLimit = 100
	inputs[userField].Width = 40

	// Password input
	inputs[passwordField] = textinput.New()
	inputs[passwordField].Placeholder = ""
	inputs[passwordField].EchoMode = textinput.EchoPassword
	inputs[passwordField].EchoCharacter = '•'
	inputs[passwordField].PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#cba6f7"))
	inputs[passwordField].TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#cdd6f4"))
	inputs[passwordField].Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#f38ba8"))
	inputs[passwordField].CharLimit = 100
	inputs[passwordField].Width = 40

	// Create search input (width will be set dynamically in View)
	searchInput := textinput.New()
	searchInput.Placeholder = "Search for connection..."
	searchInput.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#89b4fa"))
	searchInput.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#cdd6f4"))
	searchInput.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#f38ba8"))
	searchInput.CharLimit = 100
	searchInput.Width = 50 // Initial width, will be adjusted dynamically

	return &ConnectionDialog{
		inputs:           inputs,
		focusIndex:       0,
		cursorMode:       cursor.CursorBlink,
		Theme:            th,
		searchInput:      searchInput,
		InHistorySection: true, // Start in history section
	}
}

// Init initializes the connection dialog (required for tea.Model)
func (c *ConnectionDialog) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles messages for the connection dialog
func (c *ConnectionDialog) Update(msg tea.Msg) (*ConnectionDialog, tea.Cmd) {
	var cmd tea.Cmd

	// Handle search mode
	if c.SearchMode {
		c.searchInput, cmd = c.searchInput.Update(msg)
		return c, cmd
	}

	// Handle manual mode
	if c.ManualMode {
		c.inputs[c.focusIndex], cmd = c.inputs[c.focusIndex].Update(msg)
		return c, cmd
	}

	return c, nil
}

// View renders the connection dialog
func (c *ConnectionDialog) View() string {
	if c.Width <= 0 || c.Height <= 0 {
		return ""
	}

	// Define container style
	containerStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#cba6f7")).
		Padding(1, 2)

	// Calculate max content width using GetHorizontalFrameSize (the correct way!)
	maxWidth := 80 // Standard terminal width
	if c.Width < maxWidth {
		maxWidth = c.Width
	}
	contentWidth := maxWidth - containerStyle.GetHorizontalFrameSize()

	// Render content with calculated width
	var content string
	if c.ManualMode {
		content = c.renderManualMode(contentWidth)
	} else {
		content = c.renderDiscoveryMode(contentWidth)
	}

	return lipgloss.Place(
		c.Width,
		c.Height,
		lipgloss.Center,
		lipgloss.Center,
		containerStyle.Render(content),
	)
}

func (c *ConnectionDialog) renderDiscoveryMode(contentWidth int) string {
	var sections []string

	// Title
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#cba6f7"))
	sections = append(sections, titleStyle.Render("🔌 Open Connection"))
	sections = append(sections, "")

	// Search box - calculate width using GetHorizontalFrameSize
	searchBoxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#89b4fa")).
		Padding(0, 1)

	// Set search input width based on available content width
	searchInputWidth := contentWidth - searchBoxStyle.GetHorizontalFrameSize() - 4 // 4 for emoji + margins
	if searchInputWidth > 0 && searchInputWidth != c.searchInput.Width {
		c.searchInput.Width = searchInputWidth
	}

	// Wrap search box with zone for click detection
	searchBox := searchBoxStyle.Render("🔍 " + c.searchInput.View())
	sections = append(sections, zone.Mark(ZoneSearchBox, searchBox))
	sections = append(sections, "")

	// History section header
	historyHeaderStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#a6adc8")).
		Bold(true)
	sections = append(sections, historyHeaderStyle.Render("Recent Connections"))

	// History entries (filtered by search)
	filteredHistory := c.GetFilteredHistory()
	if len(filteredHistory) == 0 {
		emptyStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6c7086")).
			Italic(true).
			PaddingLeft(2)
		if c.searchInput.Value() != "" {
			sections = append(sections, emptyStyle.Render("No matches"))
		} else {
			sections = append(sections, emptyStyle.Render("No history yet"))
		}
	} else {
		historyCount := 0
		for i, entry := range filteredHistory {
			if historyCount >= 5 {
				break // Limit to 5 history items
			}

			itemStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#cdd6f4")).
				PaddingLeft(2).
				Width(contentWidth) // Full width for better click area

			// Check if this item is selected and we're in history section
			if c.InHistorySection && i == c.SelectedIndex {
				itemStyle = itemStyle.
					Foreground(lipgloss.Color("#1e1e2e")).
					Background(lipgloss.Color("#a6e3a1")).
					Bold(true).
					PaddingLeft(1)
			}

			// Format: name (local)
			metaStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#6c7086"))
			line := fmt.Sprintf("%s  %s",
				entry.Name,
				metaStyle.Render("(local)"),
			)
			// Wrap with zone for click detection
			zoneID := fmt.Sprintf("%s%d", ZoneHistoryPrefix, i)
			sections = append(sections, zone.Mark(zoneID, itemStyle.Render(line)))
			historyCount++
		}
	}

	sections = append(sections, "")

	// Discovered section header
	discoveredHeaderStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#a6adc8")).
		Bold(true)
	sections = append(sections, discoveredHeaderStyle.Render("Discovered"))

	// Discovered instances (filtered by search)
	filteredDiscovered := c.GetFilteredDiscovered()
	if len(filteredDiscovered) == 0 {
		emptyStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6c7086")).
			Italic(true).
			PaddingLeft(2)
		if c.searchInput.Value() != "" {
			sections = append(sections, emptyStyle.Render("No matches"))
		} else if len(c.DiscoveredInstances) == 0 {
			sections = append(sections, emptyStyle.Render("Searching..."))
		} else {
			sections = append(sections, emptyStyle.Render("No matches"))
		}
	} else {
		discoveredCount := 0
		for i, instance := range filteredDiscovered {
			if discoveredCount >= 3 {
				break // Limit to 3 discovered items
			}

			itemStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#cdd6f4")).
				PaddingLeft(2).
				Width(contentWidth) // Full width for better click area

			// Check if this item is selected and we're in discovered section
			if !c.InHistorySection && i == c.SelectedIndex {
				itemStyle = itemStyle.
					Foreground(lipgloss.Color("#1e1e2e")).
					Background(lipgloss.Color("#a6e3a1")).
					Bold(true).
					PaddingLeft(1)
			}

			sourceStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#6c7086"))
			line := fmt.Sprintf("%s  %s",
				instance.DisplayTarget(),
				sourceStyle.Render(fmt.Sprintf("(%s)", instance.Source.String())),
			)
			// Wrap with zone for click detection
			zoneID := fmt.Sprintf("%s%d", ZoneDiscoveredPrefix, i)
			sections = append(sections, zone.Mark(zoneID, itemStyle.Render(line)))
			discoveredCount++
		}
	}

	sections = append(sections, "")

	// Instructions (keep under 68 chars)
	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6c7086"))
	if c.SearchMode {
		sections = append(sections, helpStyle.Render("Type to search │ Enter: Apply │ Esc: Clear & Exit"))
	} else {
		sections = append(sections, helpStyle.Render("↑↓: Navigate │ /: Search │ m: Manual │ Enter: Connect"))
	}

	return strings.Join(sections, "\n")
}

func (c *ConnectionDialog) renderManualMode(contentWidth int) string {
	var sections []string

	// Title
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#cba6f7")).
		MarginBottom(1)
	sections = append(sections, titleStyle.Render("🔧 Manual Connection"))

	// Form fields
	fieldLabels := []string{"Host:", "Port:", "Database:", "User:", "Password:"}

	for i, label := range fieldLabels {
		labelStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#a6adc8")).
			Width(10).
			Align(lipgloss.Right)

		// Add focus indicator
		focusIndicator := "  "
		if i == c.focusIndex {
			focusIndicator = "▸ "
		}

		fieldLine := fmt.Sprintf("%s%s %s",
			focusIndicator,
			labelStyle.Render(label),
			c.inputs[i].View(),
		)
		sections = append(sections, fieldLine)
	}

	sections = append(sections, "")

	// Instructions - shorter to fit within MaxWidth
	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6c7086"))
	sections = append(sections, helpStyle.Render("Tab: Next  │  Enter: Connect  │  Ctrl+D: Back  │  Esc: Cancel"))

	return strings.Join(sections, "\n")
}

// NextInput focuses the next input field
func (c *ConnectionDialog) NextInput() {
	c.inputs[c.focusIndex].Blur()
	c.focusIndex = (c.focusIndex + 1) % len(c.inputs)
	c.inputs[c.focusIndex].Focus()
}

// PrevInput focuses the previous input field
func (c *ConnectionDialog) PrevInput() {
	c.inputs[c.focusIndex].Blur()
	c.focusIndex--
	if c.focusIndex < 0 {
		c.focusIndex = len(c.inputs) - 1
	}
	c.inputs[c.focusIndex].Focus()
}

// MoveSelection moves the selection up or down in discovery mode
func (c *ConnectionDialog) MoveSelection(delta int) {
	if c.ManualMode {
		if delta > 0 {
			c.NextInput()
		} else {
			c.PrevInput()
		}
		return
	}

	// Get the list size based on current section (using filtered lists)
	listSize := 0
	if c.InHistorySection {
		listSize = len(c.GetFilteredHistory())
		if listSize > 5 {
			listSize = 5 // Limit to 5 displayed history items
		}
	} else {
		listSize = len(c.GetFilteredDiscovered())
		if listSize > 3 {
			listSize = 3 // Limit to 3 displayed discovered items
		}
	}

	if listSize == 0 {
		c.SelectedIndex = 0
		return
	}

	c.SelectedIndex += delta
	if c.SelectedIndex < 0 {
		// Moving up past the top
		if c.InHistorySection {
			// At top of history, stay there
			c.SelectedIndex = 0
		} else {
			// At top of discovered, move back to history (bottom)
			c.InHistorySection = true
			historySize := len(c.HistoryEntries)
			if historySize > 5 {
				historySize = 5
			}
			if historySize > 0 {
				c.SelectedIndex = historySize - 1
			} else {
				c.SelectedIndex = 0
			}
		}
	} else if c.SelectedIndex >= listSize {
		// Moving down past the bottom
		if c.InHistorySection {
			// At bottom of history, move to discovered (top)
			c.InHistorySection = false
			c.SelectedIndex = 0
		} else {
			// At bottom of discovered, stay there
			c.SelectedIndex = listSize - 1
		}
	}
}

// SwitchSection switches between history and discovered sections
func (c *ConnectionDialog) SwitchSection() {
	c.InHistorySection = !c.InHistorySection
	c.SelectedIndex = 0 // Reset selection when switching sections
}

// ToggleMode switches between discovery and manual mode
func (c *ConnectionDialog) ToggleMode() {
	c.ManualMode = !c.ManualMode
	if c.ManualMode {
		// Focus first input when entering manual mode
		c.focusIndex = 0
		c.inputs[c.focusIndex].Focus()
	} else {
		// Blur all inputs when leaving manual mode
		for i := range c.inputs {
			c.inputs[i].Blur()
		}
	}
}

// EnterSearchMode enables search mode and focuses the search input
func (c *ConnectionDialog) EnterSearchMode() {
	c.SearchMode = true
	c.searchInput.Focus()
}

// ExitSearchMode disables search mode and clears/keeps the search
func (c *ConnectionDialog) ExitSearchMode(clearSearch bool) {
	c.SearchMode = false
	c.searchInput.Blur()
	if clearSearch {
		c.searchInput.SetValue("")
	}
	// Reset selection to first item
	c.SelectedIndex = 0
	c.InHistorySection = true
}

// GetFilteredHistory returns history entries matching the search query
func (c *ConnectionDialog) GetFilteredHistory() []models.ConnectionHistoryEntry {
	query := strings.ToLower(strings.TrimSpace(c.searchInput.Value()))
	if query == "" {
		return c.HistoryEntries
	}

	var filtered []models.ConnectionHistoryEntry
	for _, entry := range c.HistoryEntries {
		// Search in name, host, database, and user
		if strings.Contains(strings.ToLower(entry.Name), query) ||
			strings.Contains(strings.ToLower(entry.Host), query) ||
			strings.Contains(strings.ToLower(entry.Database), query) ||
			strings.Contains(strings.ToLower(entry.User), query) {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

// GetFilteredDiscovered returns discovered instances matching the search query
func (c *ConnectionDialog) GetFilteredDiscovered() []models.DiscoveredInstance {
	query := strings.ToLower(strings.TrimSpace(c.searchInput.Value()))
	if query == "" {
		return c.DiscoveredInstances
	}

	var filtered []models.DiscoveredInstance
	for _, instance := range c.DiscoveredInstances {
		// Search in host and source
		if strings.Contains(strings.ToLower(instance.Host), query) ||
			strings.Contains(strings.ToLower(instance.Source.String()), query) {
			filtered = append(filtered, instance)
		}
	}
	return filtered
}

// GetSelectedInstance returns the currently selected instance
func (c *ConnectionDialog) GetSelectedInstance() *models.DiscoveredInstance {
	if c.ManualMode || c.InHistorySection {
		return nil
	}
	filtered := c.GetFilteredDiscovered()
	if c.SelectedIndex < 0 || c.SelectedIndex >= len(filtered) {
		return nil
	}
	return &filtered[c.SelectedIndex]
}

// GetSelectedHistory returns the currently selected history entry
func (c *ConnectionDialog) GetSelectedHistory() *models.ConnectionHistoryEntry {
	if c.ManualMode || !c.InHistorySection {
		return nil
	}
	filtered := c.GetFilteredHistory()
	if c.SelectedIndex < 0 || c.SelectedIndex >= len(filtered) {
		return nil
	}
	return &filtered[c.SelectedIndex]
}

// GetManualConfig returns the manual connection config if valid, or error
func (c *ConnectionDialog) GetManualConfig() (models.ConnectionConfig, error) {
	host := strings.TrimSpace(c.inputs[hostField].Value())
	port := strings.TrimSpace(c.inputs[portField].Value())
	database := strings.TrimSpace(c.inputs[databaseField].Value())
	user := strings.TrimSpace(c.inputs[userField].Value())
	password := c.inputs[passwordField].Value()

	// Use placeholder values as defaults when fields are empty
	if host == "" {
		host = c.inputs[hostField].Placeholder
	}
	if port == "" {
		port = c.inputs[portField].Placeholder
	}
	if database == "" {
		database = c.inputs[databaseField].Placeholder
	}
	if user == "" {
		user = c.inputs[userField].Placeholder
	}

	// Validate required fields after applying defaults
	if host == "" {
		return models.ConnectionConfig{}, fmt.Errorf("host is required")
	}
	if user == "" {
		return models.ConnectionConfig{}, fmt.Errorf("user is required")
	}
	if database == "" {
		return models.ConnectionConfig{}, fmt.Errorf("database is required")
	}

	return models.ConnectionConfig{
		Host:     host,
		Port:     mustParseInt(port, 5432),
		Database: database,
		User:     user,
		Password: password,
		SSLMode:  "prefer",
	}, nil
}

// SetDiscoveredInstances updates the list of discovered instances
func (c *ConnectionDialog) SetDiscoveredInstances(instances []models.DiscoveredInstance) {
	c.DiscoveredInstances = instances
	if !c.InHistorySection && c.SelectedIndex >= len(instances) {
		c.SelectedIndex = 0
	}
}

// SetHistoryEntries updates the list of connection history entries
func (c *ConnectionDialog) SetHistoryEntries(entries []models.ConnectionHistoryEntry) {
	c.HistoryEntries = entries
	if c.InHistorySection && c.SelectedIndex >= len(entries) {
		c.SelectedIndex = 0
	}
}

func mustParseInt(s string, defaultVal int) int {
	var result int
	if _, err := fmt.Sscanf(s, "%d", &result); err != nil {
		return defaultVal
	}
	return result
}
