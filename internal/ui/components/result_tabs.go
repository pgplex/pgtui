package components

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
	"github.com/pgplex/pgtui/internal/models"
	"github.com/pgplex/pgtui/internal/ui/theme"
)

// Zone ID prefixes for mouse click handling
const (
	ZoneResultTabPrefix = "result-tab-"
)

const MaxResultTabs = 10

// Pre-compiled regex patterns for performance
var (
	dashCommentRe  = regexp.MustCompile(`^\s*--\s*(.+)$`)
	blockCommentRe = regexp.MustCompile(`^\s*/\*\s*(.+?)\s*\*/`)
	fromRe         = regexp.MustCompile(`(?i)\bFROM\s+([a-zA-Z_][a-zA-Z0-9_.]*)(?:\s+(?:AS\s+)?[a-zA-Z_][a-zA-Z0-9_]*)?`)
	updateRe       = regexp.MustCompile(`(?i)\bUPDATE\s+([a-zA-Z_][a-zA-Z0-9_.]*)`)
	deleteRe       = regexp.MustCompile(`(?i)\bDELETE\s+FROM\s+([a-zA-Z_][a-zA-Z0-9_.]*)`)
	insertRe       = regexp.MustCompile(`(?i)\bINSERT\s+INTO\s+([a-zA-Z_][a-zA-Z0-9_.]*)`)
)

// TabType represents the type of content in a tab
type TabType int

const (
	TabTypeQueryResult TabType = iota // SQL query result
	TabTypeTableData                  // Table/View data from tree selection
	TabTypeCodeEditor                 // Function, Sequence, etc. (code/DDL display)
)

// ResultTab represents a single query result tab
type ResultTab struct {
	ID          int
	Title       string
	SQL         string
	Result      models.QueryResult
	CreatedAt   time.Time
	TableView   *TableView
	IsPending   bool // true if query is still executing
	IsCancelled bool // true if query was cancelled

	// Tab type and additional content
	Type       TabType
	CodeEditor *CodeEditor    // For code/DDL display tabs
	Structure  *StructureView // For table data tabs

	// Identifier for deduplication (e.g., "schema.table" or "schema.function")
	ObjectID string
}

// ResultTabs manages multiple query result tabs
type ResultTabs struct {
	tabs      []*ResultTab
	activeIdx int
	nextID    int
	Theme     theme.Theme

	// Pending execution state
	pendingSQL       string
	pendingStartTime time.Time
}

// NewResultTabs creates a new result tabs manager
func NewResultTabs(th theme.Theme) *ResultTabs {
	return &ResultTabs{
		tabs:      []*ResultTab{},
		activeIdx: 0,
		nextID:    1,
		Theme:     th,
	}
}

// StartPendingQuery creates a pending tab for an executing query
func (rt *ResultTabs) StartPendingQuery(sql string) {
	rt.pendingSQL = sql
	rt.pendingStartTime = time.Now()

	// Create pending tab
	tab := &ResultTab{
		ID:        rt.nextID,
		Title:     "Executing...",
		SQL:       sql,
		CreatedAt: time.Now(),
		IsPending: true,
	}
	rt.nextID++

	// Insert pending tab at the beginning (leftmost position)
	rt.tabs = append([]*ResultTab{tab}, rt.tabs...)

	// Remove oldest (rightmost) if exceeding max
	if len(rt.tabs) > MaxResultTabs {
		rt.tabs = rt.tabs[:MaxResultTabs]
	}

	// Set pending tab as active
	rt.activeIdx = 0
}

// CompletePendingQuery completes the pending query with results
func (rt *ResultTabs) CompletePendingQuery(sql string, result models.QueryResult) {
	// Find and update the pending tab
	for i, tab := range rt.tabs {
		if tab.IsPending && tab.SQL == sql {
			// Create TableView for results
			tableView := NewTableView(rt.Theme)
			tableView.SetData(result.Columns, result.Rows, len(result.Rows))

			tab.Title = rt.generateTitle(sql, result)
			tab.Result = result
			tab.TableView = tableView
			tab.IsPending = false

			// Make sure this tab is active
			rt.activeIdx = i
			break
		}
	}

	// Clear pending state
	rt.pendingSQL = ""
}

// CancelPendingQuery marks the pending tab as cancelled
func (rt *ResultTabs) CancelPendingQuery() {
	// Find and mark the pending tab as cancelled
	for _, tab := range rt.tabs {
		if tab.IsPending {
			tab.IsPending = false
			tab.IsCancelled = true
			tab.Title = "Cancelled"
			break
		}
	}

	// Clear pending state
	rt.pendingSQL = ""
}

// HasPendingQuery returns true if there's a pending query
func (rt *ResultTabs) HasPendingQuery() bool {
	return rt.pendingSQL != ""
}

// GetPendingElapsed returns the elapsed time for the pending query
func (rt *ResultTabs) GetPendingElapsed() time.Duration {
	if rt.pendingSQL == "" {
		return 0
	}
	return time.Since(rt.pendingStartTime)
}

// AddResult adds a new query result as a tab (newest appears on the left)
func (rt *ResultTabs) AddResult(sql string, result models.QueryResult) {
	// Create TableView for this result
	tableView := NewTableView(rt.Theme)
	tableView.SetData(result.Columns, result.Rows, len(result.Rows))

	tab := &ResultTab{
		ID:        rt.nextID,
		Title:     rt.generateTitle(sql, result),
		SQL:       sql,
		Result:    result,
		CreatedAt: time.Now(),
		TableView: tableView,
		Type:      TabTypeQueryResult,
	}
	rt.nextID++

	// Insert new tab at the beginning (leftmost position)
	rt.tabs = append([]*ResultTab{tab}, rt.tabs...)

	// Remove oldest (rightmost) if exceeding max
	if len(rt.tabs) > MaxResultTabs {
		rt.tabs = rt.tabs[:MaxResultTabs]
	}

	// Set new tab as active (index 0 = leftmost)
	rt.activeIdx = 0
}

// AddTableData adds a table/view data tab (from tree selection)
// If a tab for the same objectID exists, it becomes active instead of creating a new tab
func (rt *ResultTabs) AddTableData(objectID, title string, structure *StructureView) {
	// Check if tab for this object already exists
	for i, tab := range rt.tabs {
		if tab.ObjectID == objectID && tab.Type == TabTypeTableData {
			// Tab exists, just activate it
			rt.activeIdx = i
			return
		}
	}

	tab := &ResultTab{
		ID:        rt.nextID,
		Title:     title,
		CreatedAt: time.Now(),
		Type:      TabTypeTableData,
		Structure: structure,
		ObjectID:  objectID,
	}
	rt.nextID++

	// Insert new tab at the beginning (leftmost position)
	rt.tabs = append([]*ResultTab{tab}, rt.tabs...)

	// Remove oldest (rightmost) if exceeding max
	if len(rt.tabs) > MaxResultTabs {
		rt.tabs = rt.tabs[:MaxResultTabs]
	}

	// Set new tab as active (index 0 = leftmost)
	rt.activeIdx = 0
}

// AddCodeEditor adds a code/DDL display tab (for functions, sequences, etc.)
// If a tab for the same objectID exists, it becomes active instead of creating a new tab
func (rt *ResultTabs) AddCodeEditor(objectID, title string, codeEditor *CodeEditor) {
	// Check if tab for this object already exists
	for i, tab := range rt.tabs {
		if tab.ObjectID == objectID && tab.Type == TabTypeCodeEditor {
			// Tab exists, just activate it
			rt.activeIdx = i
			return
		}
	}

	tab := &ResultTab{
		ID:         rt.nextID,
		Title:      title,
		CreatedAt:  time.Now(),
		Type:       TabTypeCodeEditor,
		CodeEditor: codeEditor,
		ObjectID:   objectID,
	}
	rt.nextID++

	// Insert new tab at the beginning (leftmost position)
	rt.tabs = append([]*ResultTab{tab}, rt.tabs...)

	// Remove oldest (rightmost) if exceeding max
	if len(rt.tabs) > MaxResultTabs {
		rt.tabs = rt.tabs[:MaxResultTabs]
	}

	// Set new tab as active (index 0 = leftmost)
	rt.activeIdx = 0
}

// CloseActiveTab closes the currently active tab
func (rt *ResultTabs) CloseActiveTab() {
	if len(rt.tabs) == 0 {
		return
	}

	// Remove the active tab
	rt.tabs = append(rt.tabs[:rt.activeIdx], rt.tabs[rt.activeIdx+1:]...)

	// Adjust active index
	if rt.activeIdx >= len(rt.tabs) && len(rt.tabs) > 0 {
		rt.activeIdx = len(rt.tabs) - 1
	}
}

// GetActiveStructureView returns the StructureView of the active tab (if it's a table data tab)
func (rt *ResultTabs) GetActiveStructureView() *StructureView {
	tab := rt.GetActiveTab()
	if tab == nil || tab.Type != TabTypeTableData {
		return nil
	}
	return tab.Structure
}

// GetActiveCodeEditor returns the CodeEditor of the active tab (if it's a code editor tab)
func (rt *ResultTabs) GetActiveCodeEditor() *CodeEditor {
	tab := rt.GetActiveTab()
	if tab == nil || tab.Type != TabTypeCodeEditor {
		return nil
	}
	return tab.CodeEditor
}

// generateTitle generates a smart title for the tab
func (rt *ResultTabs) generateTitle(sql string, result models.QueryResult) string {
	// Check for custom comment title
	if title := rt.extractCommentTitle(sql); title != "" {
		return title
	}

	// Extract table name from SQL
	if tableName := rt.extractTableName(sql); tableName != "" {
		return tableName
	}

	// Fallback to truncated SQL
	cleaned := strings.TrimSpace(sql)
	cleaned = strings.ReplaceAll(cleaned, "\n", " ")
	if len(cleaned) > 20 {
		cleaned = cleaned[:17] + "..."
	}
	return cleaned
}

// extractCommentTitle extracts title from SQL comment (-- title or /* title */)
func (rt *ResultTabs) extractCommentTitle(sql string) string {
	// Match -- comment at start
	lines := strings.Split(sql, "\n")
	if len(lines) > 0 {
		if matches := dashCommentRe.FindStringSubmatch(lines[0]); len(matches) > 1 {
			return strings.TrimSpace(matches[1])
		}
	}

	// Match /* comment */ at start
	if matches := blockCommentRe.FindStringSubmatch(sql); len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	return ""
}

// extractTableName extracts the main table name from SQL
func (rt *ResultTabs) extractTableName(sql string) string {
	upperSQL := strings.ToUpper(sql)

	// SELECT ... FROM table
	if matches := fromRe.FindStringSubmatch(sql); len(matches) > 1 {
		tableName := matches[1]
		// Check for JOIN
		if strings.Contains(upperSQL, "JOIN") {
			return tableName + "(+)"
		}
		return tableName
	}

	// UPDATE table
	if matches := updateRe.FindStringSubmatch(sql); len(matches) > 1 {
		return "UPDATE " + matches[1]
	}

	// DELETE FROM table
	if matches := deleteRe.FindStringSubmatch(sql); len(matches) > 1 {
		return "DELETE " + matches[1]
	}

	// INSERT INTO table
	if matches := insertRe.FindStringSubmatch(sql); len(matches) > 1 {
		return "INSERT " + matches[1]
	}

	return ""
}

// GetActiveTab returns the currently active tab
func (rt *ResultTabs) GetActiveTab() *ResultTab {
	if len(rt.tabs) == 0 || rt.activeIdx < 0 || rt.activeIdx >= len(rt.tabs) {
		return nil
	}
	return rt.tabs[rt.activeIdx]
}

// GetActiveTableView returns the TableView of the active tab
func (rt *ResultTabs) GetActiveTableView() *TableView {
	tab := rt.GetActiveTab()
	if tab == nil {
		return nil
	}
	// For TableData tabs, get table view from structure view
	if tab.Type == TabTypeTableData && tab.Structure != nil {
		return tab.Structure.GetActiveTableView()
	}
	return tab.TableView
}

// GetActiveSQL returns the SQL of the active tab
func (rt *ResultTabs) GetActiveSQL() string {
	tab := rt.GetActiveTab()
	if tab == nil {
		return ""
	}
	return tab.SQL
}

// NextTab switches to the next tab
func (rt *ResultTabs) NextTab() {
	if len(rt.tabs) > 0 {
		rt.activeIdx = (rt.activeIdx + 1) % len(rt.tabs)
	}
}

// PrevTab switches to the previous tab
func (rt *ResultTabs) PrevTab() {
	if len(rt.tabs) > 0 {
		rt.activeIdx = (rt.activeIdx - 1 + len(rt.tabs)) % len(rt.tabs)
	}
}

// TabCount returns the number of tabs
func (rt *ResultTabs) TabCount() int {
	return len(rt.tabs)
}

// GetAllTabs returns all tabs for iteration
func (rt *ResultTabs) GetAllTabs() []*ResultTab {
	return rt.tabs
}

// GetTabByObjectID returns a tab by its object ID
func (rt *ResultTabs) GetTabByObjectID(objectID string) *ResultTab {
	for _, tab := range rt.tabs {
		if tab.ObjectID == objectID {
			return tab
		}
	}
	return nil
}

// HasTabs returns whether there are any tabs
func (rt *ResultTabs) HasTabs() bool {
	return len(rt.tabs) > 0
}

// SetActiveTab sets the active tab by index (for mouse click)
func (rt *ResultTabs) SetActiveTab(index int) {
	if index < 0 || index >= len(rt.tabs) {
		return
	}
	rt.activeIdx = index
}

// RenderTabBar renders the tab bar
func (rt *ResultTabs) RenderTabBar(width int) string {
	if len(rt.tabs) == 0 {
		return ""
	}

	var tabViews []string

	for i, tab := range rt.tabs {
		// Generate label based on tab type
		var label string
		switch tab.Type {
		case TabTypeQueryResult:
			// Format: [index] title (rows)
			rowCount := len(tab.Result.Rows)
			rowStr := fmt.Sprintf("%d rows", rowCount)
			if rowCount == 1 {
				rowStr = "1 row"
			}
			label = fmt.Sprintf("[%d] %s (%s)", i+1, tab.Title, rowStr)
		case TabTypeTableData:
			// Format: [index] ▦ title
			label = fmt.Sprintf("[%d] ▦ %s", i+1, tab.Title)
		case TabTypeCodeEditor:
			// Format: [index] ƒ title
			label = fmt.Sprintf("[%d] ƒ %s", i+1, tab.Title)
		default:
			label = fmt.Sprintf("[%d] %s", i+1, tab.Title)
		}

		// Truncate if too long
		maxLabelLen := width / MaxResultTabs
		if maxLabelLen < 15 {
			maxLabelLen = 15
		}
		if len(label) > maxLabelLen {
			// Try without row count for query results
			if tab.Type == TabTypeQueryResult {
				label = fmt.Sprintf("[%d] %s", i+1, tab.Title)
			}
			if len(label) > maxLabelLen {
				label = label[:maxLabelLen-3] + "..."
			}
		}

		var style lipgloss.Style
		if i == rt.activeIdx {
			style = lipgloss.NewStyle().
				Foreground(rt.Theme.Background).
				Background(rt.Theme.Info).
				Bold(true).
				Padding(0, 1)
		} else {
			style = lipgloss.NewStyle().
				Foreground(rt.Theme.Foreground).
				Background(rt.Theme.Selection).
				Padding(0, 1)
		}

		// Wrap each tab with zone mark for click detection
		zoneID := fmt.Sprintf("%s%d", ZoneResultTabPrefix, i)
		tabViews = append(tabViews, zone.Mark(zoneID, style.Render(label)))
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, tabViews...)
}
