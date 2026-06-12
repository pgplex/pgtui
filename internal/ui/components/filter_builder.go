package components

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
	"github.com/pgplex/pgtui/internal/filter"
	"github.com/pgplex/pgtui/internal/models"
	"github.com/pgplex/pgtui/internal/ui/theme"
)

// Zone ID prefixes for mouse click handling
const (
	ZoneFilterConditionPrefix = "filter-cond-"
	ZoneFilterOperatorPrefix  = "filter-op-"
)

// ApplyFilterMsg is sent when a filter should be applied
type ApplyFilterMsg struct {
	Filter models.Filter
}

// CloseFilterBuilderMsg is sent when the filter builder should close
type CloseFilterBuilderMsg struct{}

// FilterBuilder provides an interactive UI for building SQL filters
type FilterBuilder struct {
	Width   int
	Height  int
	Theme   theme.Theme
	builder *filter.Builder

	// State
	columns         []models.ColumnInfo
	filter          models.Filter
	currentIndex    int    // Index in conditions list
	editMode        string // "", "column", "operator", "value"
	columnInput     string
	operatorIndex   int
	valueInput      string
	jsonbPath       string // For JSONB path input (e.g., $.user.name)
	validationError string

	// UI elements
	selectedColumn models.ColumnInfo
	availableOps   []models.FilterOperator
	previewSQL     string
}

// NewFilterBuilder creates a new filter builder
func NewFilterBuilder(th theme.Theme) *FilterBuilder {
	return &FilterBuilder{
		Width:   80,
		Height:  30,
		Theme:   th,
		builder: filter.NewBuilder(),
		filter: models.Filter{
			RootGroup: models.FilterGroup{
				Conditions: []models.FilterCondition{},
				Logic:      "AND",
				Groups:     []models.FilterGroup{},
			},
		},
		editMode: "",
	}
}

// SetColumns updates the available columns for filtering
func (fb *FilterBuilder) SetColumns(columns []models.ColumnInfo) {
	fb.columns = columns
}

// SetTable sets the table being filtered
func (fb *FilterBuilder) SetTable(schema, table string) {
	fb.filter.Schema = schema
	fb.filter.TableName = table
}

// Update handles keyboard input
func (fb *FilterBuilder) Update(msg tea.KeyMsg) (*FilterBuilder, tea.Cmd) {
	switch fb.editMode {
	case "":
		return fb.handleNavigationMode(msg)
	case "column":
		return fb.handleColumnMode(msg)
	case "operator":
		return fb.handleOperatorMode(msg)
	case "value":
		return fb.handleValueMode(msg)
	}
	return fb, nil
}

// handleNavigationMode handles keys in navigation mode
func (fb *FilterBuilder) handleNavigationMode(msg tea.KeyMsg) (*FilterBuilder, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if fb.currentIndex > 0 {
			fb.currentIndex--
		}
	case "down", "j":
		if fb.currentIndex < len(fb.filter.RootGroup.Conditions) {
			fb.currentIndex++
		}
	case "a", "n":
		// Add new condition
		fb.editMode = "column"
		fb.columnInput = ""
	case "d", "x":
		// Delete current condition
		if fb.currentIndex < len(fb.filter.RootGroup.Conditions) {
			fb.filter.RootGroup.Conditions = append(
				fb.filter.RootGroup.Conditions[:fb.currentIndex],
				fb.filter.RootGroup.Conditions[fb.currentIndex+1:]...,
			)
			if fb.currentIndex > 0 && fb.currentIndex >= len(fb.filter.RootGroup.Conditions) {
				fb.currentIndex--
			}
			fb.updatePreview()
		}
	case "enter":
		// Validate and apply filter
		if err := fb.builder.Validate(fb.filter); err != nil {
			fb.validationError = err.Error()
			return fb, nil
		}

		fb.validationError = ""
		return fb, func() tea.Msg {
			return ApplyFilterMsg{Filter: fb.filter}
		}
	case "esc":
		return fb, func() tea.Msg {
			return CloseFilterBuilderMsg{}
		}
	}
	return fb, nil
}

// handleColumnMode handles column selection
func (fb *FilterBuilder) handleColumnMode(msg tea.KeyMsg) (*FilterBuilder, tea.Cmd) {
	switch msg.String() {
	case "esc":
		fb.editMode = ""
		fb.columnInput = ""
		fb.validationError = ""
	case "enter":
		// Find matching column
		for _, col := range fb.columns {
			if strings.EqualFold(col.Name, fb.columnInput) {
				fb.selectedColumn = col
				fb.availableOps = filter.GetOperatorsForType(col.DataType)
				fb.editMode = "operator"
				fb.operatorIndex = 0
				fb.validationError = ""
				return fb, nil
			}
		}
		// No match, show error and stay in column mode
		fb.validationError = fmt.Sprintf("Column '%s' not found", fb.columnInput)
	case "backspace":
		if len(fb.columnInput) > 0 {
			fb.columnInput = fb.columnInput[:len(fb.columnInput)-1]
		}
	default:
		if len(msg.String()) == 1 {
			fb.columnInput += msg.String()
		}
	}
	return fb, nil
}

// handleOperatorMode handles operator selection
func (fb *FilterBuilder) handleOperatorMode(msg tea.KeyMsg) (*FilterBuilder, tea.Cmd) {
	switch msg.String() {
	case "esc":
		fb.editMode = "column"
	case "up", "k":
		if fb.operatorIndex > 0 {
			fb.operatorIndex--
		}
	case "down", "j":
		if fb.operatorIndex < len(fb.availableOps)-1 {
			fb.operatorIndex++
		}
	case "enter":
		// Check if operator needs a value
		selectedOp := fb.availableOps[fb.operatorIndex]
		if selectedOp == models.OpIsNull || selectedOp == models.OpIsNotNull {
			// No value needed, add condition immediately
			fb.filter.RootGroup.Conditions = append(fb.filter.RootGroup.Conditions, models.FilterCondition{
				Column:   fb.selectedColumn.Name,
				Operator: selectedOp,
				Value:    nil,
				Type:     fb.selectedColumn.DataType,
			})
			fb.editMode = ""
			fb.updatePreview()
		} else {
			fb.editMode = "value"
			fb.valueInput = ""
		}
	}
	return fb, nil
}

// handleValueMode handles value input
func (fb *FilterBuilder) handleValueMode(msg tea.KeyMsg) (*FilterBuilder, tea.Cmd) {
	switch msg.String() {
	case "esc":
		fb.editMode = "operator"
		fb.valueInput = ""
		fb.jsonbPath = ""
	case "enter":
		// Check if this is a JSONB operator and handle path extraction
		selectedOp := fb.availableOps[fb.operatorIndex]
		if selectedOp == models.OpContains ||
		   selectedOp == models.OpContainedBy ||
		   selectedOp == models.OpHasKey {
			// For JSONB operators, check if value looks like a path
			if strings.HasPrefix(fb.valueInput, "$.") {
				// This is a path, store it separately
				fb.jsonbPath = fb.valueInput
			}
		}

		// Add condition
		fb.filter.RootGroup.Conditions = append(fb.filter.RootGroup.Conditions, models.FilterCondition{
			Column:   fb.selectedColumn.Name,
			Operator: fb.availableOps[fb.operatorIndex],
			Value:    fb.valueInput,
			Type:     fb.selectedColumn.DataType,
		})
		fb.editMode = ""
		fb.valueInput = ""
		fb.jsonbPath = ""
		fb.updatePreview()
	case "backspace":
		if len(fb.valueInput) > 0 {
			fb.valueInput = fb.valueInput[:len(fb.valueInput)-1]
		}
	default:
		if len(msg.String()) == 1 {
			fb.valueInput += msg.String()
		}
	}
	return fb, nil
}

// updatePreview updates the SQL preview
func (fb *FilterBuilder) updatePreview() {
	whereClause, _, err := fb.builder.BuildWhere(fb.filter)
	if err != nil {
		fb.previewSQL = fmt.Sprintf("Error: %s", err.Error())
	} else {
		if whereClause == "" {
			fb.previewSQL = fmt.Sprintf(`SELECT * FROM "%s"."%s"`, fb.filter.Schema, fb.filter.TableName)
		} else {
			fb.previewSQL = fmt.Sprintf(`SELECT * FROM "%s"."%s" %s`, fb.filter.Schema, fb.filter.TableName, whereClause)
		}
	}
}

// HandleMouseWheel handles mouse wheel events for scrolling
// Returns true if the event was handled
func (fb *FilterBuilder) HandleMouseWheel(msg tea.MouseMsg) bool {
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		switch fb.editMode {
		case "":
			// Navigation mode - scroll conditions
			if fb.currentIndex > 0 {
				fb.currentIndex--
			}
		case "operator":
			// Operator mode - scroll operators
			if fb.operatorIndex > 0 {
				fb.operatorIndex--
			}
		}
		return true
	case tea.MouseButtonWheelDown:
		switch fb.editMode {
		case "":
			// Navigation mode - scroll conditions
			if fb.currentIndex < len(fb.filter.RootGroup.Conditions) {
				fb.currentIndex++
			}
		case "operator":
			// Operator mode - scroll operators
			if fb.operatorIndex < len(fb.availableOps)-1 {
				fb.operatorIndex++
			}
		}
		return true
	}
	return false
}

// HandleMouseClick handles mouse click events
// Returns true if click was handled, and a command if needed
func (fb *FilterBuilder) HandleMouseClick(msg tea.MouseMsg) (handled bool, cmd tea.Cmd) {
	// Only handle left click press events
	if msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress {
		return false, nil
	}

	// In navigation mode, check for condition clicks
	if fb.editMode == "" {
		for i := 0; i < len(fb.filter.RootGroup.Conditions); i++ {
			zoneID := fmt.Sprintf("%s%d", ZoneFilterConditionPrefix, i)
			if zone.Get(zoneID).InBounds(msg) {
				fb.currentIndex = i
				return true, nil
			}
		}
	}

	// In operator mode, check for operator clicks
	if fb.editMode == "operator" {
		for i := 0; i < len(fb.availableOps); i++ {
			zoneID := fmt.Sprintf("%s%d", ZoneFilterOperatorPrefix, i)
			if zone.Get(zoneID).InBounds(msg) {
				// If clicking on already selected, confirm selection
				if i == fb.operatorIndex {
					selectedOp := fb.availableOps[fb.operatorIndex]
					if selectedOp == models.OpIsNull || selectedOp == models.OpIsNotNull {
						// No value needed, add condition immediately
						fb.filter.RootGroup.Conditions = append(fb.filter.RootGroup.Conditions, models.FilterCondition{
							Column:   fb.selectedColumn.Name,
							Operator: selectedOp,
							Value:    nil,
							Type:     fb.selectedColumn.DataType,
						})
						fb.editMode = ""
						fb.updatePreview()
					} else {
						fb.editMode = "value"
						fb.valueInput = ""
					}
				} else {
					fb.operatorIndex = i
				}
				return true, nil
			}
		}
	}

	return false, nil
}

// View renders the filter builder
func (fb *FilterBuilder) View() string {
	var sections []string

	// Title
	titleStyle := lipgloss.NewStyle().
		Foreground(fb.Theme.Foreground).
		Background(fb.Theme.Info).
		Padding(0, 1).
		Bold(true)
	sections = append(sections, titleStyle.Render("Filter Builder"))

	// Instructions based on mode
	instructionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#a6adc8")). // Subtext0 from Catppuccin
		Padding(0, 1)

	var instructions string
	switch fb.editMode {
	case "column":
		instructions = "Type column name, Enter to confirm, Esc to cancel"
	case "operator":
		instructions = "↑↓ Select operator, Enter to confirm, Esc to go back"
	case "value":
		instructions = "Type value, Enter to confirm, Esc to go back"
	default:
		instructions = "a=Add n=New d=Delete Enter=Apply Esc=Cancel"
	}
	sections = append(sections, instructionStyle.Render(instructions))

	// Show validation error if present
	if fb.validationError != "" {
		errorStyle := lipgloss.NewStyle().
			Foreground(fb.Theme.Error).
			Padding(0, 1).
			Bold(true)
		sections = append(sections, errorStyle.Render("Warning: "+fb.validationError))
	}

	// Conditions list
	if len(fb.filter.RootGroup.Conditions) > 0 {
		sections = append(sections, "\nConditions:")
		for i, cond := range fb.filter.RootGroup.Conditions {
			condStr := fmt.Sprintf("%s %s %v", cond.Column, cond.Operator, cond.Value)
			if cond.Operator == models.OpIsNull || cond.Operator == models.OpIsNotNull {
				condStr = fmt.Sprintf("%s %s", cond.Column, cond.Operator)
			}

			style := lipgloss.NewStyle().Padding(0, 1)
			if i == fb.currentIndex && fb.editMode == "" {
				style = style.Background(fb.Theme.Selection).Foreground(fb.Theme.Foreground)
			}
			line := style.Render(fmt.Sprintf(" %d. %s", i+1, condStr))
			// Wrap with zone mark for mouse click
			zoneID := fmt.Sprintf("%s%d", ZoneFilterConditionPrefix, i)
			sections = append(sections, zone.Mark(zoneID, line))
		}
	}

	// Edit area
	if fb.editMode != "" {
		sections = append(sections, "\n")
		switch fb.editMode {
		case "column":
			sections = append(sections, fmt.Sprintf("Column: %s_", fb.columnInput))
		case "operator":
			sections = append(sections, fmt.Sprintf("Column: %s", fb.selectedColumn.Name))
			sections = append(sections, "Select operator:")
			for i, op := range fb.availableOps {
				style := lipgloss.NewStyle().Padding(0, 1)
				if i == fb.operatorIndex {
					style = style.Background(fb.Theme.Selection).Foreground(fb.Theme.Foreground)
				}
				line := style.Render(fmt.Sprintf("  %s", op))
				// Wrap with zone mark for mouse click
				zoneID := fmt.Sprintf("%s%d", ZoneFilterOperatorPrefix, i)
				sections = append(sections, zone.Mark(zoneID, line))
			}
		case "value":
			sections = append(sections, fmt.Sprintf("Column: %s %s", fb.selectedColumn.Name, fb.availableOps[fb.operatorIndex]))
			sections = append(sections, fmt.Sprintf("Value: %s_", fb.valueInput))
		}
	}

	// SQL Preview
	if fb.previewSQL != "" {
		sections = append(sections, "\nSQL Preview:")
		previewStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6c7086")). // Overlay0 from Catppuccin
			Background(fb.Theme.Background).
			Padding(0, 1).
			Italic(true)
		sections = append(sections, previewStyle.Render(fb.previewSQL))
	}

	content := strings.Join(sections, "\n")

	// Container
	containerStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(fb.Theme.Border).
		Background(fb.Theme.Background).
		Foreground(fb.Theme.Foreground).
		Width(fb.Width).
		Height(fb.Height).
		Padding(1)

	return containerStyle.Render(content)
}
