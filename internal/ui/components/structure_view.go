package components

import (
	"context"
	"fmt"
	"strings"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
	"github.com/pgplex/pgtui/internal/db/connection"
	"github.com/pgplex/pgtui/internal/db/metadata"
	"github.com/pgplex/pgtui/internal/models"
	"github.com/pgplex/pgtui/internal/ui/theme"
)

// Zone ID prefixes for mouse click handling
const (
	ZoneStructureTabPrefix = "structure-tab-"
)

// StructureView is a tabbed container for viewing table structure
type StructureView struct {
	Width  int
	Height int
	Theme  theme.Theme

	// Current active tab (0=Data, 1=Columns, 2=Constraints, 3=Indexes)
	activeTab int

	// Tab views - all using TableView for consistent UI
	tableView       *TableView // For Data tab
	columnsTable    *TableView // For Columns tab
	constraintsTable *TableView // For Constraints tab
	indexesTable    *TableView // For Indexes tab

	// Raw data for copy operations
	columnsData     []models.ColumnDetail
	constraintsData []models.Constraint
	indexesData     []models.IndexInfo

	// Table info
	schema string
	table  string
	pool   *connection.Pool

	// Status
	loading        bool
	metadataLoaded bool // Whether columns/constraints/indexes have been loaded
	errorMessage   string
}

// NewStructureView creates a new structure view
func NewStructureView(th theme.Theme, tableView *TableView) *StructureView {
	return &StructureView{
		Theme:            th,
		activeTab:        0, // Start with Data tab
		tableView:        tableView,
		columnsTable:     NewTableView(th),
		constraintsTable: NewTableView(th),
		indexesTable:     NewTableView(th),
	}
}

// HasTableLoaded checks if structure data has been loaded for the given table
func (sv *StructureView) HasTableLoaded(schema, table string) bool {
	return sv.schema == schema && sv.table == table
}

// SetTable sets the current table and loads structure data
func (sv *StructureView) SetTable(ctx context.Context, pool *connection.Pool, schema, table string) error {
	sv.schema = schema
	sv.table = table
	sv.pool = pool
	sv.loading = true
	sv.errorMessage = ""

	// Load columns
	columns, err := metadata.GetColumnDetails(ctx, pool, schema, table)
	if err != nil {
		sv.errorMessage = fmt.Sprintf("Failed to load columns: %v", err)
		sv.loading = false
		return err
	}
	sv.columnsData = columns
	sv.setColumnsTableData(columns)

	// Load constraints
	constraints, err := metadata.GetConstraints(ctx, pool, schema, table)
	if err != nil {
		sv.errorMessage = fmt.Sprintf("Failed to load constraints: %v", err)
		sv.loading = false
		return err
	}
	sv.constraintsData = constraints
	sv.setConstraintsTableData(constraints)

	// Load indexes
	indexes, err := metadata.GetIndexes(ctx, pool, schema, table)
	if err != nil {
		sv.errorMessage = fmt.Sprintf("Failed to load indexes: %v", err)
		sv.loading = false
		return err
	}
	sv.indexesData = indexes
	sv.setIndexesTableData(indexes)

	sv.loading = false
	return nil
}

// setColumnsTableData converts column details to TableView format
func (sv *StructureView) setColumnsTableData(columns []models.ColumnDetail) {
	headers := []string{"Name", "Type", "Nullable", "Default", "Constraints", "Comment"}
	rows := make([][]string, len(columns))

	for i, col := range columns {
		// Format constraint markers
		constraints := sv.formatColumnConstraints(col)
		nullable := "NO"
		if col.IsNullable {
			nullable = "YES"
		}

		rows[i] = []string{
			col.Name,
			col.DataType,
			nullable,
			col.DefaultValue,
			constraints,
			col.Comment,
		}
	}

	sv.columnsTable.SetData(headers, rows, len(rows))
}

func (sv *StructureView) formatColumnConstraints(col models.ColumnDetail) string {
	markers := []string{}
	if col.IsPrimaryKey {
		markers = append(markers, "PK")
	}
	if col.IsForeignKey {
		markers = append(markers, "FK")
	}
	if col.IsUnique {
		markers = append(markers, "UQ")
	}
	if col.HasCheck {
		markers = append(markers, "CK")
	}
	if len(markers) == 0 {
		return "-"
	}
	return strings.Join(markers, ", ")
}

// setConstraintsTableData converts constraints to TableView format
func (sv *StructureView) setConstraintsTableData(constraints []models.Constraint) {
	headers := []string{"Type", "Name", "Columns", "Definition", "Description"}
	rows := make([][]string, len(constraints))

	for i, con := range constraints {
		typeLabel := metadata.FormatConstraintType(con.Type)
		columnsStr := strings.Join(con.Columns, ", ")
		definition := sv.formatConstraintDefinition(con)
		description := sv.formatConstraintDescription(con)

		rows[i] = []string{
			typeLabel,
			con.Name,
			columnsStr,
			definition,
			description,
		}
	}

	sv.constraintsTable.SetData(headers, rows, len(rows))
}

func (sv *StructureView) formatConstraintDefinition(con models.Constraint) string {
	if con.Type == "f" && con.ForeignTable != "" {
		fkCols := strings.Join(con.ForeignCols, ", ")
		return fmt.Sprintf("→ %s(%s)", con.ForeignTable, fkCols)
	}
	return con.Definition
}

func (sv *StructureView) formatConstraintDescription(con models.Constraint) string {
	switch con.Type {
	case "p":
		return "Primary key constraint"
	case "f":
		return fmt.Sprintf("References %s", con.ForeignTable)
	case "u":
		return "Unique constraint"
	case "c":
		return "Check constraint"
	default:
		return "-"
	}
}

// setIndexesTableData converts indexes to TableView format
func (sv *StructureView) setIndexesTableData(indexes []models.IndexInfo) {
	headers := []string{"Name", "Type", "Columns", "Properties", "Size", "Definition"}
	rows := make([][]string, len(indexes))

	for i, idx := range indexes {
		columnsStr := strings.Join(idx.Columns, ", ")
		properties := sv.formatIndexProperties(idx)
		sizeStr := metadata.FormatSize(idx.Size)

		rows[i] = []string{
			idx.Name,
			idx.Type,
			columnsStr,
			properties,
			sizeStr,
			idx.Definition,
		}
	}

	sv.indexesTable.SetData(headers, rows, len(rows))
}

func (sv *StructureView) formatIndexProperties(idx models.IndexInfo) string {
	props := []string{}
	if idx.IsPrimary {
		props = append(props, "PK")
	}
	if idx.IsUnique {
		props = append(props, "UQ")
	}
	if idx.IsPartial {
		props = append(props, "Partial")
	}
	if len(props) == 0 {
		return "-"
	}
	return strings.Join(props, ", ")
}

// NeedsMetadata returns true if structure metadata has not been loaded yet
func (sv *StructureView) NeedsMetadata() bool {
	return !sv.metadataLoaded && !sv.loading
}

// SetMetadata populates the columns/constraints/indexes tables with pre-loaded data
func (sv *StructureView) SetMetadata(columns []models.ColumnDetail, constraints []models.Constraint, indexes []models.IndexInfo) {
	sv.columnsData = columns
	sv.setColumnsTableData(columns)
	sv.constraintsData = constraints
	sv.setConstraintsTableData(constraints)
	sv.indexesData = indexes
	sv.setIndexesTableData(indexes)
	sv.metadataLoaded = true
	sv.loading = false
}

// SetMetadataLoading marks that metadata is being loaded
func (sv *StructureView) SetMetadataLoading() {
	sv.loading = true
}

// SwitchTab switches to a specific tab
func (sv *StructureView) SwitchTab(tabIndex int) {
	if tabIndex >= 0 && tabIndex <= 3 {
		sv.activeTab = tabIndex
	}
}

// HandleMouseClick handles mouse click events on the tab bar
// Returns (handled, tabIndex) where tabIndex is the clicked tab (-1 if not a tab click)
func (sv *StructureView) HandleMouseClick(msg tea.MouseMsg) (bool, int) {
	// Only handle left click press events
	if msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress {
		return false, -1
	}

	// Check each tab zone
	for i := 0; i <= 3; i++ {
		zoneID := fmt.Sprintf("%s%d", ZoneStructureTabPrefix, i)
		if zone.Get(zoneID).InBounds(msg) {
			sv.SwitchTab(i)
			return true, i
		}
	}

	return false, -1
}

// Update handles keyboard input
func (sv *StructureView) Update(msg tea.KeyMsg) {
	if sv.activeTab == 0 {
		// Data tab - handled by app.go with existing table view
		return
	}

	// Get current table view for the active structure tab
	currentTable := sv.getCurrentTableView()
	if currentTable == nil {
		return
	}

	// Handle navigation keys for structure tabs
	switch msg.String() {
	case "up", "k":
		currentTable.MoveSelection(-1)
	case "down", "j":
		currentTable.MoveSelection(1)
	case "left", "h":
		currentTable.MoveSelectionHorizontal(-1)
	case "right", "l":
		currentTable.MoveSelectionHorizontal(1)
	case "H":
		currentTable.JumpScrollHorizontal(-1)
	case "L":
		currentTable.JumpScrollHorizontal(1)
	case "0":
		currentTable.JumpToFirstColumn()
	case "$":
		currentTable.JumpToLastColumn()
	}
}

// getCurrentTableView returns the TableView for the current structure tab (internal use)
func (sv *StructureView) getCurrentTableView() *TableView {
	switch sv.activeTab {
	case 1:
		return sv.columnsTable
	case 2:
		return sv.constraintsTable
	case 3:
		return sv.indexesTable
	default:
		return nil
	}
}

// GetActiveTableView returns the TableView for the current tab (including Data tab)
// Returns the main tableView for Data tab (0), or structure tables for other tabs
func (sv *StructureView) GetActiveTableView() *TableView {
	switch sv.activeTab {
	case 0:
		return sv.tableView
	case 1:
		return sv.columnsTable
	case 2:
		return sv.constraintsTable
	case 3:
		return sv.indexesTable
	default:
		return sv.tableView
	}
}

// GetTableView returns the main data TableView (for tab 0)
func (sv *StructureView) GetTableView() *TableView {
	return sv.tableView
}

// View renders the structure view
func (sv *StructureView) View() string {
	var b strings.Builder

	// Render tab bar
	b.WriteString(sv.renderTabBar())
	b.WriteString("\n")

	// Calculate content height (subtract tab bar)
	contentHeight := sv.Height - 1

	// Update view dimensions for all TableViews
	sv.tableView.Width = sv.Width
	sv.tableView.Height = contentHeight
	sv.columnsTable.Width = sv.Width
	sv.columnsTable.Height = contentHeight
	sv.constraintsTable.Width = sv.Width
	sv.constraintsTable.Height = contentHeight
	sv.indexesTable.Width = sv.Width
	sv.indexesTable.Height = contentHeight

	// Render active tab content
	switch sv.activeTab {
	case 0:
		b.WriteString(sv.tableView.View())
	case 1, 2, 3:
		// Show loading state only for metadata tabs
		if sv.loading {
			b.WriteString(lipgloss.NewStyle().
				Foreground(sv.Theme.Metadata).
				Render("Loading structure..."))
		} else if sv.errorMessage != "" {
			b.WriteString(lipgloss.NewStyle().
				Foreground(sv.Theme.Error).
				Render(sv.errorMessage))
		} else {
			switch sv.activeTab {
			case 1:
				b.WriteString(sv.columnsTable.View())
			case 2:
				b.WriteString(sv.constraintsTable.View())
			case 3:
				b.WriteString(sv.indexesTable.View())
			}
		}
	default:
		b.WriteString("Unknown tab")
	}

	return b.String()
}

func (sv *StructureView) renderTabBar() string {
	tabs := []struct {
		index int
		label string
	}{
		{0, "Data"},
		{1, "Columns"},
		{2, "Constraints"},
		{3, "Indexes"},
	}

	var parts []string

	for i, tab := range tabs {
		var tabContent string
		if tab.index == sv.activeTab {
			// Active tab - with blue indicator and background
			indicatorStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#89b4fa")). // Blue
				Bold(true)

			tabStyle := lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#cdd6f4")). // Text
				Background(lipgloss.Color("#313244")). // Surface0
				Padding(0, 1)

			tabContent = indicatorStyle.Render("▌") + tabStyle.Render(tab.label)
		} else {
			// Inactive tab
			tabStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#6c7086")). // Overlay0
				Padding(0, 1)

			tabContent = tabStyle.Render(tab.label)
		}

		// Wrap with zone mark for mouse click
		zoneID := fmt.Sprintf("%s%d", ZoneStructureTabPrefix, tab.index)
		parts = append(parts, zone.Mark(zoneID, tabContent))

		// Add separator between tabs (except after last)
		if i < len(tabs)-1 {
			separator := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#45475a")). // Surface1
				Render(" │ ")
			parts = append(parts, separator)
		}
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

// CopyCurrentName copies the name of the selected item
func (sv *StructureView) CopyCurrentName() string {
	var name string
	switch sv.activeTab {
	case 1:
		if col := sv.getSelectedColumn(); col != nil {
			name = col.Name
		}
	case 2:
		if con := sv.getSelectedConstraint(); con != nil {
			name = con.Name
		}
	case 3:
		if idx := sv.getSelectedIndex(); idx != nil {
			name = idx.Name
		}
	}

	if name != "" {
		_ = clipboard.WriteAll(name)
		return fmt.Sprintf("✓ Copied: %s", name)
	}
	return ""
}

// CopyCurrentDefinition copies the full definition of the selected item
func (sv *StructureView) CopyCurrentDefinition() string {
	var definition string
	switch sv.activeTab {
	case 1:
		if col := sv.getSelectedColumn(); col != nil {
			definition = fmt.Sprintf("%s %s %s DEFAULT %s",
				col.Name, col.DataType,
				map[bool]string{true: "NULL", false: "NOT NULL"}[col.IsNullable],
				col.DefaultValue)
		}
	case 2:
		if con := sv.getSelectedConstraint(); con != nil {
			definition = con.Definition
		}
	case 3:
		if idx := sv.getSelectedIndex(); idx != nil {
			definition = idx.Definition
		}
	}

	if definition != "" {
		_ = clipboard.WriteAll(definition)
		preview := definition
		if len(preview) > 50 {
			preview = preview[:50] + "..."
		}
		return fmt.Sprintf("✓ Copied: %s", preview)
	}
	return ""
}

// getSelectedColumn returns the currently selected column from raw data
func (sv *StructureView) getSelectedColumn() *models.ColumnDetail {
	idx := sv.columnsTable.SelectedRow
	if idx < 0 || idx >= len(sv.columnsData) {
		return nil
	}
	return &sv.columnsData[idx]
}

// getSelectedConstraint returns the currently selected constraint from raw data
func (sv *StructureView) getSelectedConstraint() *models.Constraint {
	idx := sv.constraintsTable.SelectedRow
	if idx < 0 || idx >= len(sv.constraintsData) {
		return nil
	}
	return &sv.constraintsData[idx]
}

// getSelectedIndex returns the currently selected index from raw data
func (sv *StructureView) getSelectedIndex() *models.IndexInfo {
	idx := sv.indexesTable.SelectedRow
	if idx < 0 || idx >= len(sv.indexesData) {
		return nil
	}
	return &sv.indexesData[idx]
}
