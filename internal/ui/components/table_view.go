package components

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
	"github.com/mattn/go-runewidth"
	"github.com/pgplex/pgtui/internal/jsonb"
	"github.com/pgplex/pgtui/internal/ui/theme"
)

// Zone ID prefixes for mouse click handling
const (
	ZoneTableRowPrefix  = "table-row-"
	ZoneTableCellPrefix = "table-cell-" // Format: table-cell-{row}-{col}
)

// TableView displays table data with virtual scrolling
type TableView struct {
	Columns      []string
	Rows         [][]string
	Width        int
	Height       int
	Style        lipgloss.Style
	Theme        theme.Theme // Color theme
	Focused      bool        // Whether this component has focus

	// Virtual scrolling state
	TopRow       int
	VisibleRows  int
	SelectedRow  int
	SelectedCol  int // Currently selected column
	TotalRows    int

	// Column widths (calculated)
	ColumnWidths []int

	// Sort state
	SortColumn    int    // -1 means no sort, otherwise index of sorted column
	SortDirection string // "ASC" or "DESC"
	NullsFirst    bool   // true = NULLS FIRST, false = NULLS LAST (default)

	// Horizontal scrolling state
	LeftColOffset int // First visible column index
	VisibleCols   int // Number of columns that fit in current width

	// Search state
	SearchActive bool
	SearchMode   string     // "local" or "table"
	SearchQuery  string
	Matches      []MatchPos // List of match positions
	CurrentMatch int        // Index in Matches

	// Preview pane for truncated content
	PreviewPane *PreviewPane

	// Line number display
	ShowLineNumbers bool // Whether to show line numbers (default true)
	RelativeNumbers bool // Whether to use relative line numbers (default false)

	// Vim motion state
	PendingCount     string    // Number prefix buffer (e.g., "42")
	PendingCountTime time.Time // Last input time for timeout
	PendingG         bool      // Waiting for second 'g' in 'gg'

	// Loading state
	IsLoading     bool           // True when first loading table data
	IsPaginating  bool           // True when loading more rows (pagination)
	LoadingStart  time.Time      // When loading started
	Spinner       *spinner.Model // Shared spinner instance

	// Pin state
	PinnedRows    []int      // Indices of pinned rows
	PinnedData    [][]string // Data copy of pinned rows
	MaxPinnedRows int        // Maximum number of pinned rows (default 5)

	// Prefetch state
	IsPrefetching     bool // Whether a prefetch is in progress
	PrefetchThreshold int  // Distance from end to trigger prefetch

	// Cached styles for performance (avoid recreating on every render)
	cachedStyles *tableViewStyles
}

// tableViewStyles holds pre-computed styles for TableView rendering
type tableViewStyles struct {
	headerBg         lipgloss.Style
	headerText       lipgloss.Style // Bold + foreground color for header text
	headerLineNum    lipgloss.Style
	headerSep        lipgloss.Style
	separator        lipgloss.Style
	separatorHeader  lipgloss.Style
	border           lipgloss.Style
	selectedCell     lipgloss.Style
	currentMatch     lipgloss.Style
	otherMatch       lipgloss.Style
	selectedRow      lipgloss.Style
	normal           lipgloss.Style
	lineNumNormal    lipgloss.Style
	lineNumSelected  lipgloss.Style
	lineNumRelative  lipgloss.Style
	status           lipgloss.Style
	containerNormal  lipgloss.Style // Container border style when not focused
	containerFocused lipgloss.Style // Container border style when focused
	pinnedRow        lipgloss.Style
	pinnedMarker     lipgloss.Style
	pinnedSep        lipgloss.Style
}

// MatchPos represents a search match position
type MatchPos struct {
	Row int
	Col int
}

// NewTableView creates a new table view with theme
func NewTableView(th theme.Theme) *TableView {
	tv := &TableView{
		Columns:         []string{},
		Rows:            [][]string{},
		ColumnWidths:    []int{},
		Theme:           th,
		SortColumn:      -1,
		SortDirection:   "ASC",
		NullsFirst:      false,
		PreviewPane:     NewPreviewPane(th),
		ShowLineNumbers: true,  // Default to showing line numbers
		RelativeNumbers: false, // Default to absolute line numbers
		MaxPinnedRows:     5,
		PinnedRows:        []int{},
		PinnedData:        [][]string{},
		PrefetchThreshold: 50,
	}
	tv.initStyles()
	return tv
}

// initStyles initializes cached styles for rendering performance
func (tv *TableView) initStyles() {
	tv.cachedStyles = &tableViewStyles{
		headerBg: lipgloss.NewStyle().Background(tv.Theme.Selection),
		headerText: lipgloss.NewStyle().
			Bold(true).
			Foreground(tv.Theme.TableHeader),
		headerLineNum: lipgloss.NewStyle().
			Background(tv.Theme.Selection).
			Foreground(tv.Theme.Metadata),
		headerSep: lipgloss.NewStyle().
			Background(tv.Theme.Selection).
			Foreground(tv.Theme.Border),
		separator: lipgloss.NewStyle().
			Foreground(tv.Theme.Border),
		separatorHeader: lipgloss.NewStyle().
			Foreground(tv.Theme.Border).
			Background(tv.Theme.Selection),
		border: lipgloss.NewStyle().Foreground(tv.Theme.Border),
		selectedCell: lipgloss.NewStyle().
			Background(tv.Theme.BorderFocused).
			Foreground(tv.Theme.Background).
			Bold(true),
		currentMatch: lipgloss.NewStyle().
			Background(lipgloss.Color("#f9e2af")). // Yellow
			Foreground(lipgloss.Color("#1e1e2e")). // Dark
			Bold(true),
		otherMatch: lipgloss.NewStyle().
			Background(lipgloss.Color("#585b70")). // Surface2
			Foreground(tv.Theme.Foreground),
		selectedRow: lipgloss.NewStyle().
			Background(tv.Theme.Selection).
			Foreground(tv.Theme.Foreground),
		normal: lipgloss.NewStyle(),
		lineNumNormal: lipgloss.NewStyle().
			Foreground(tv.Theme.Metadata),
		lineNumSelected: lipgloss.NewStyle().
			Foreground(tv.Theme.Info).
			Bold(true),
		lineNumRelative: lipgloss.NewStyle().
			Foreground(tv.Theme.Comment),
		status: lipgloss.NewStyle().
			Foreground(tv.Theme.Metadata).
			Italic(true),
		containerNormal: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(tv.Theme.Border),
		containerFocused: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(tv.Theme.BorderFocused),
		pinnedRow: lipgloss.NewStyle().
			Background(tv.Theme.Selection).
			Foreground(tv.Theme.Info),
		pinnedMarker: lipgloss.NewStyle().
			Foreground(tv.Theme.Warning).
			Bold(true),
		pinnedSep: lipgloss.NewStyle().
			Foreground(tv.Theme.Border),
	}
}

// SetData sets the table data
func (tv *TableView) SetData(columns []string, rows [][]string, totalRows int) {
	tv.Columns = columns
	tv.Rows = rows
	tv.TotalRows = totalRows
	tv.calculateColumnWidths()
}

// getLineNumberDigits returns the number of digits needed for line numbers
func (tv *TableView) getLineNumberDigits() int {
	maxRow := tv.TotalRows
	if maxRow < len(tv.Rows) {
		maxRow = len(tv.Rows)
	}
	if maxRow == 0 {
		maxRow = 1
	}
	digits := len(fmt.Sprintf("%d", maxRow))
	if digits < 2 {
		digits = 2 // Minimum 2 digits
	}
	return digits
}

// getLineNumberWidth returns the width needed for line number column
func (tv *TableView) getLineNumberWidth() int {
	if !tv.ShowLineNumbers {
		return 0
	}
	// Width = digits + 1 space + separator "│ "
	return tv.getLineNumberDigits() + 3
}

// renderLineNumber renders the line number for a row
func (tv *TableView) renderLineNumber(rowIndex int, isSelected bool) string {
	if !tv.ShowLineNumbers {
		return ""
	}

	// Calculate the display number
	var displayNum int
	if tv.RelativeNumbers && !isSelected {
		// Relative mode: show distance from selected row
		displayNum = rowIndex - tv.SelectedRow
		if displayNum < 0 {
			displayNum = -displayNum
		}
	} else {
		// Absolute mode or selected row in relative mode
		displayNum = rowIndex + 1 // 1-indexed
	}

	digits := tv.getLineNumberDigits()

	// Format the number right-aligned
	numStr := fmt.Sprintf("%*d", digits, displayNum)

	// Use cached styles based on selection
	var style lipgloss.Style
	if isSelected {
		// Current line: highlighted
		style = tv.cachedStyles.lineNumSelected
	} else if tv.RelativeNumbers {
		// Relative numbers: use comment color
		style = tv.cachedStyles.lineNumRelative
	} else {
		// Other lines: dimmed
		style = tv.cachedStyles.lineNumNormal
	}

	return style.Render(numStr) + tv.cachedStyles.separator.Render(" │ ")
}

// ToggleRelativeNumbers toggles between absolute and relative line numbers
func (tv *TableView) ToggleRelativeNumbers() {
	tv.RelativeNumbers = !tv.RelativeNumbers
}

// calculateColumnWidths calculates optimal column widths
func (tv *TableView) calculateColumnWidths() {
	if len(tv.Columns) == 0 {
		return
	}

	numColumns := len(tv.Columns)
	tv.ColumnWidths = make([]int, numColumns)

	// Step 1: Calculate desired widths based on content
	desiredWidths := make([]int, numColumns)

	// Start with column header lengths (add 4 chars for sort indicator space)
	for i, col := range tv.Columns {
		desiredWidths[i] = runewidth.StringWidth(col) + 4
	}

	// Step 2: Apply constraints (min/max width per column)
	maxWidth := 50

	// Check row data - sample first 100 rows for performance
	// (checking all rows is O(rows*cols) which is too slow for large tables)
	sampleSize := 100
	if sampleSize > len(tv.Rows) {
		sampleSize = len(tv.Rows)
	}
	// Only check first N bytes of each cell since we cap width at maxWidth anyway
	// This avoids O(n) runewidth.StringWidth on multi-MB cells
	maxCheckLen := maxWidth * 4 // Buffer for multi-byte chars
	for i := 0; i < sampleSize; i++ {
		row := tv.Rows[i]
		for j, cell := range row {
			if j < numColumns {
				// Truncate before measuring to avoid processing huge strings
				checkCell := cell
				if len(checkCell) > maxCheckLen {
					checkCell = checkCell[:maxCheckLen]
				}
				cellLen := runewidth.StringWidth(checkCell)
				if cellLen > desiredWidths[j] {
					desiredWidths[j] = cellLen
				}
			}
		}
	}
	minWidth := 10

	for i, w := range desiredWidths {
		if w > maxWidth {
			w = maxWidth
		}
		if w < minWidth {
			w = minWidth
		}
		tv.ColumnWidths[i] = w
	}
}

// calculateVisibleCols calculates how many columns fit in the given width
func (tv *TableView) calculateVisibleCols(width int) {
	if len(tv.ColumnWidths) == 0 {
		tv.VisibleCols = 0
		return
	}

	// Reserve space for edge indicators (2 chars each side) and line numbers
	availableWidth := width - 4 - tv.getLineNumberWidth()

	// Count columns that fit starting from LeftColOffset
	totalWidth := 0
	count := 0
	for i := tv.LeftColOffset; i < len(tv.ColumnWidths); i++ {
		colWidth := tv.ColumnWidths[i]
		separatorWidth := 0
		if count > 0 {
			separatorWidth = 3 // " │ "
		}

		if totalWidth+colWidth+separatorWidth > availableWidth {
			break
		}
		totalWidth += colWidth + separatorWidth
		count++
	}

	if count < 1 && len(tv.ColumnWidths) > 0 {
		count = 1 // Always show at least one column
	}
	tv.VisibleCols = count
}

// tableLoadingState returns the loading state view for initial table data load
func (tv *TableView) tableLoadingState(width, height int) string {
	spinnerView := ""
	if tv.Spinner != nil {
		spinnerView = tv.Spinner.View() + " "
	}

	elapsed := time.Since(tv.LoadingStart)
	elapsedStr := fmt.Sprintf("(%.1fs)", elapsed.Seconds())

	loadingStyle := lipgloss.NewStyle().
		Foreground(tv.Theme.Foreground)

	elapsedStyle := lipgloss.NewStyle().
		Foreground(tv.Theme.Metadata)

	cancelHint := lipgloss.NewStyle().
		Foreground(tv.Theme.Border).
		Render("Press Esc to cancel")

	content := lipgloss.JoinVertical(lipgloss.Center,
		"",
		spinnerView+loadingStyle.Render("Loading table data...")+elapsedStyle.Render(" "+elapsedStr),
		"",
		cancelHint,
	)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}

// View renders the table
func (tv *TableView) View() string {
	// Select container style based on focus
	containerStyle := tv.cachedStyles.containerNormal
	if tv.Focused {
		containerStyle = tv.cachedStyles.containerFocused
	}

	// Calculate content dimensions (inside border)
	// tv.Width/tv.Height are the total available space including border
	contentWidth := tv.Width - containerStyle.GetHorizontalFrameSize()
	contentHeight := tv.Height - containerStyle.GetVerticalFrameSize()

	// Show loading state for initial table load
	if tv.IsLoading {
		loadingContent := tv.tableLoadingState(contentWidth, contentHeight)
		return containerStyle.Width(contentWidth).Height(contentHeight).Render(loadingContent)
	}

	if len(tv.Columns) == 0 {
		return containerStyle.Width(contentWidth).Height(contentHeight).Render("No data")
	}

	// Calculate visible columns for horizontal scrolling
	tv.calculateVisibleCols(contentWidth)

	var b strings.Builder

	// Determine edge indicators
	leftIndicator := "  " // 2 spaces placeholder
	if tv.LeftColOffset > 0 {
		leftIndicator = "◀ "
	}
	rightIndicator := "  "
	if tv.LeftColOffset+tv.VisibleCols < len(tv.Columns) {
		rightIndicator = " ▶"
	}

	// Get line number column width
	lineNumWidth := tv.getLineNumberWidth()

	// Render header with line number column styled to match header background
	if tv.ShowLineNumbers {
		// Use "#" as header for line number column
		numColWidth := lineNumWidth - 3 // subtract " │ "
		headerLineNum := fmt.Sprintf("%*s", numColWidth, "#")
		b.WriteString(tv.cachedStyles.headerLineNum.Render(headerLineNum))
		b.WriteString(tv.cachedStyles.headerSep.Render(" │ "))
	}
	// Left indicator with header background
	b.WriteString(tv.cachedStyles.headerBg.Render(leftIndicator))
	b.WriteString(tv.renderHeader())
	b.WriteString(tv.cachedStyles.headerBg.Render(rightIndicator))
	b.WriteString("\n")

	// Render separator (extend through line number column)
	lineNumSep := strings.Repeat("─", lineNumWidth)
	b.WriteString(tv.cachedStyles.border.Render(lineNumSep))
	b.WriteString(tv.cachedStyles.border.Render("──")) // Align with left indicator
	b.WriteString(tv.renderSeparator())
	b.WriteString(tv.cachedStyles.border.Render("──"))
	b.WriteString("\n")

	// Render pinned rows (if any)
	if len(tv.PinnedRows) > 0 {
		b.WriteString(tv.renderPinnedRows())
	}

	// Calculate how many rows we can show
	// contentHeight is the content area height (inside border)
	// Subtract 3 for header + separator + status line
	pinnedHeight := len(tv.PinnedRows)
	if pinnedHeight > 0 {
		pinnedHeight += 1 // Add 1 for pinned separator
	}
	tv.VisibleRows = contentHeight - 3 - pinnedHeight
	if tv.VisibleRows < 1 {
		tv.VisibleRows = 1
	}

	// Render visible rows
	endRow := tv.TopRow + tv.VisibleRows
	if endRow > len(tv.Rows) {
		endRow = len(tv.Rows)
	}

	for i := tv.TopRow; i < endRow; i++ {
		isSelected := i == tv.SelectedRow
		visibleRowIndex := i - tv.TopRow

		// Build row content
		rowContent := tv.renderLineNumber(i, isSelected) +
			leftIndicator +
			tv.renderRow(tv.Rows[i], isSelected, i, visibleRowIndex) +
			rightIndicator

		// Wrap with zone.Mark for mouse click support
		// Use visibleRowIndex (0, 1, 2...) so app.go can calculate actual row as TopRow + visibleRowIndex
		rowZoneID := fmt.Sprintf("%s%d", ZoneTableRowPrefix, visibleRowIndex)
		b.WriteString(zone.Mark(rowZoneID, rowContent))

		if i < endRow-1 {
			b.WriteString("\n")
		}
	}

	// Render status
	b.WriteString("\n")
	b.WriteString(tv.renderStatus())

	// Render content and wrap in container with border
	return containerStyle.Width(contentWidth).Height(contentHeight).Render(b.String())
}

func (tv *TableView) renderHeader() string {
	var b strings.Builder
	b.Grow(tv.VisibleCols * 60)

	// Use cached separator style
	separator := tv.cachedStyles.separatorHeader.Render(" │ ")

	// Only render visible columns
	endCol := tv.LeftColOffset + tv.VisibleCols
	if endCol > len(tv.Columns) {
		endCol = len(tv.Columns)
	}

	colIndex := 0
	for i := tv.LeftColOffset; i < endCol; i++ {
		col := tv.Columns[i]
		width := tv.ColumnWidths[i]
		if width <= 0 {
			continue
		}

		// Add sort indicator if this column is sorted
		displayCol := col
		if i == tv.SortColumn {
			if tv.SortDirection == "ASC" {
				if tv.NullsFirst {
					displayCol = col + " ↑ⁿ"
				} else {
					displayCol = col + " ↑"
				}
			} else {
				if tv.NullsFirst {
					displayCol = col + " ↓ⁿ"
				} else {
					displayCol = col + " ↓"
				}
			}
		}

		// Use runewidth.Truncate for proper truncation
		truncated := runewidth.Truncate(displayCol, width, "…")

		// Render cell with cached header background style and width control
		renderedCell := tv.cachedStyles.headerBg.Width(width).MaxWidth(width).Inline(true).Render(truncated)

		// Add separator before cell (except first)
		if colIndex > 0 {
			b.WriteString(separator)
		}
		b.WriteString(renderedCell)
		colIndex++
	}

	// Apply cached header text style to the entire row
	return tv.cachedStyles.headerText.Render(b.String())
}

func (tv *TableView) renderSeparator() string {
	// Calculate total width of visible columns only
	totalWidth := 0
	endCol := tv.LeftColOffset + tv.VisibleCols
	if endCol > len(tv.ColumnWidths) {
		endCol = len(tv.ColumnWidths)
	}

	for i := tv.LeftColOffset; i < endCol; i++ {
		totalWidth += tv.ColumnWidths[i]
	}

	// Add width for separators: 3 chars (" │ ") * (number of separators)
	visibleCount := endCol - tv.LeftColOffset
	if visibleCount > 1 {
		totalWidth += 3 * (visibleCount - 1)
	}

	// Use cached border style
	return tv.cachedStyles.border.Render(strings.Repeat("─", totalWidth))
}

func (tv *TableView) renderRow(row []string, selected bool, rowIndex int, visibleRowIndex int) string {
	// Pre-allocate builder with estimated capacity
	var b strings.Builder
	b.Grow(tv.VisibleCols * 60) // Estimate ~60 chars per cell

	// Use cached separator style
	separator := tv.cachedStyles.separator.Render(" │ ")

	// Only render visible columns
	endCol := tv.LeftColOffset + tv.VisibleCols
	if endCol > len(tv.ColumnWidths) {
		endCol = len(tv.ColumnWidths)
	}

	visibleColIndex := 0
	for i := tv.LeftColOffset; i < endCol; i++ {
		if i >= len(row) || i >= len(tv.ColumnWidths) {
			break
		}
		width := tv.ColumnWidths[i]
		if width <= 0 {
			continue
		}

		value := row[i]

		// CRITICAL: Truncate FIRST before any string processing!
		// Cells can contain megabytes of data (e.g., JSONB columns)
		// Processing the full string is O(n) and extremely slow
		maxProcessLen := width * 4 // Allow some buffer for multi-byte chars
		if len(value) > maxProcessLen {
			value = value[:maxProcessLen]
		}

		// Replace newlines with spaces to keep content on single line
		cellValue := strings.ReplaceAll(value, "\n", " ")
		cellValue = strings.ReplaceAll(cellValue, "\r", "")

		// Check if this looks like JSONB and format for display
		if jsonb.IsJSONB(cellValue) {
			cellValue = jsonb.Truncate(cellValue, 50)
		}

		// Use runewidth.Truncate for proper truncation (handles multibyte chars)
		truncated := runewidth.Truncate(cellValue, width, "…")

		// Determine cell style based on selection and search
		// Priority: selected cell > current match > other matches > selected row > normal
		var cellStyle lipgloss.Style
		if selected && i == tv.SelectedCol {
			cellStyle = tv.cachedStyles.selectedCell
		} else if tv.IsCurrentMatch(rowIndex, i) {
			cellStyle = tv.cachedStyles.currentMatch
		} else if tv.IsMatch(rowIndex, i) {
			cellStyle = tv.cachedStyles.otherMatch
		} else if selected {
			cellStyle = tv.cachedStyles.selectedRow
		} else {
			cellStyle = tv.cachedStyles.normal
		}

		// Render with lipgloss width control for proper padding
		renderedCell := cellStyle.Width(width).MaxWidth(width).Inline(true).Render(truncated)

		// Add separator before cell (except first)
		if visibleColIndex > 0 {
			b.WriteString(separator)
		}

		// Wrap cell with zone.Mark for mouse click support
		// Use visible indices so app.go can calculate actual as TopRow + visibleRow, LeftColOffset + visibleCol
		cellZoneID := fmt.Sprintf("%s%d-%d", ZoneTableCellPrefix, visibleRowIndex, visibleColIndex)
		b.WriteString(zone.Mark(cellZoneID, renderedCell))

		visibleColIndex++
	}

	return b.String()
}

// renderPinnedRows renders the pinned rows section
func (tv *TableView) renderPinnedRows() string {
	if len(tv.PinnedRows) == 0 {
		return ""
	}

	var b strings.Builder
	lineNumWidth := tv.getLineNumberWidth()

	for i, rowIdx := range tv.PinnedRows {
		if i >= len(tv.PinnedData) {
			continue
		}

		isSelected := rowIdx == tv.SelectedRow

		// Render pin marker + line number
		// Pin marker replaces first char of padding to keep alignment
		if tv.ShowLineNumbers {
			digits := tv.getLineNumberDigits()

			// Format: "*" + number with (digits-1) width = total digits chars
			marker := tv.cachedStyles.pinnedMarker.Render("*")
			numStr := fmt.Sprintf("%*d", digits-1, rowIdx+1)
			b.WriteString(marker)
			b.WriteString(tv.cachedStyles.lineNumNormal.Render(numStr))
			b.WriteString(tv.cachedStyles.separator.Render(" │ "))
		}

		// Left indicator placeholder
		b.WriteString("  ")

		// Render the row cells
		b.WriteString(tv.renderPinnedRowCells(tv.PinnedData[i], isSelected, rowIdx))

		// Right indicator placeholder
		b.WriteString("  ")
		b.WriteString("\n")
	}

	// Pinned separator (dashed line)
	sepLine := strings.Repeat("─", lineNumWidth+2)
	for i := tv.LeftColOffset; i < tv.LeftColOffset+tv.VisibleCols && i < len(tv.ColumnWidths); i++ {
		if i > tv.LeftColOffset {
			sepLine += "─┼─"
		}
		sepLine += strings.Repeat("─", tv.ColumnWidths[i])
	}
	sepLine += "────"
	b.WriteString(tv.cachedStyles.pinnedSep.Render(sepLine))
	b.WriteString("\n")

	return b.String()
}

// renderPinnedRowCells renders cells for a single pinned row
func (tv *TableView) renderPinnedRowCells(row []string, selected bool, rowIndex int) string {
	var b strings.Builder
	separator := tv.cachedStyles.separator.Render(" │ ")

	endCol := tv.LeftColOffset + tv.VisibleCols
	if endCol > len(tv.ColumnWidths) {
		endCol = len(tv.ColumnWidths)
	}

	visibleColIndex := 0
	for i := tv.LeftColOffset; i < endCol; i++ {
		if i >= len(row) || i >= len(tv.ColumnWidths) {
			break
		}
		width := tv.ColumnWidths[i]
		if width <= 0 {
			continue
		}

		value := row[i]
		maxProcessLen := width * 4
		if len(value) > maxProcessLen {
			value = value[:maxProcessLen]
		}

		cellValue := strings.ReplaceAll(value, "\n", " ")
		cellValue = strings.ReplaceAll(cellValue, "\r", "")
		truncated := runewidth.Truncate(cellValue, width, "…")

		var cellStyle lipgloss.Style
		if selected && i == tv.SelectedCol {
			cellStyle = tv.cachedStyles.selectedCell
		} else {
			cellStyle = tv.cachedStyles.pinnedRow
		}

		renderedCell := cellStyle.Width(width).MaxWidth(width).Inline(true).Render(truncated)

		if visibleColIndex > 0 {
			b.WriteString(separator)
		}
		b.WriteString(renderedCell)
		visibleColIndex++
	}

	return b.String()
}

func (tv *TableView) renderStatus() string {
	// Show pagination loading indicator
	if tv.IsPaginating {
		spinnerView := ""
		if tv.Spinner != nil {
			spinnerView = tv.Spinner.View() + " "
		}
		paginatingText := spinnerView + "Loading..."
		return tv.cachedStyles.status.Render(paginatingText)
	}

	endRow := tv.TopRow + tv.VisibleRows
	if endRow > len(tv.Rows) {
		endRow = len(tv.Rows)
	}

	// Search match info
	matchInfo := ""
	if tv.SearchActive && len(tv.Matches) > 0 {
		matchInfo = fmt.Sprintf("Match %d of %d │ ", tv.CurrentMatch+1, len(tv.Matches))
	}

	// Column info for horizontal scrolling
	colInfo := ""
	if len(tv.Columns) > tv.VisibleCols {
		endCol := tv.LeftColOffset + tv.VisibleCols
		if endCol > len(tv.Columns) {
			endCol = len(tv.Columns)
		}
		colInfo = fmt.Sprintf("Cols %d-%d of %d │ ", tv.LeftColOffset+1, endCol, len(tv.Columns))
	}

	// Pinned rows info
	pinnedInfo := ""
	if len(tv.PinnedRows) > 0 {
		pinnedInfo = fmt.Sprintf("%d pinned │ ", len(tv.PinnedRows))
	}

	showing := fmt.Sprintf(" 󰈙 %s%s%s%d-%d of %d rows", matchInfo, colInfo, pinnedInfo, tv.TopRow+1, endRow, tv.TotalRows)
	return tv.cachedStyles.status.Render(showing)
}

// MoveSelection moves the selection up or down
func (tv *TableView) MoveSelection(delta int) {
	tv.SelectedRow += delta

	// Bounds checking
	if tv.SelectedRow < 0 {
		tv.SelectedRow = 0
	}
	if tv.SelectedRow >= len(tv.Rows) {
		tv.SelectedRow = len(tv.Rows) - 1
	}

	// Adjust visible window if needed
	if tv.SelectedRow < tv.TopRow {
		tv.TopRow = tv.SelectedRow
	}
	if tv.SelectedRow >= tv.TopRow+tv.VisibleRows {
		tv.TopRow = tv.SelectedRow - tv.VisibleRows + 1
	}

	// Update preview pane only if visible
	if tv.PreviewPane != nil && tv.PreviewPane.Visible {
		tv.UpdatePreviewPane()
	}
}

// ScrollViewport scrolls the viewport without changing selection (like lazygit)
// Returns true if we might need to load more data (scrolled near the end)
func (tv *TableView) ScrollViewport(delta int) bool {
	if len(tv.Rows) == 0 {
		return false
	}

	// Calculate new TopRow
	newTopRow := tv.TopRow + delta

	// Bounds checking for TopRow
	if newTopRow < 0 {
		newTopRow = 0
	}
	maxTopRow := len(tv.Rows) - tv.VisibleRows
	if maxTopRow < 0 {
		maxTopRow = 0
	}
	if newTopRow > maxTopRow {
		newTopRow = maxTopRow
	}

	tv.TopRow = newTopRow

	// Adjust selection to stay within visible range
	if tv.SelectedRow < tv.TopRow {
		tv.SelectedRow = tv.TopRow
	}
	if tv.SelectedRow >= tv.TopRow+tv.VisibleRows {
		tv.SelectedRow = tv.TopRow + tv.VisibleRows - 1
	}

	// Update preview pane only if visible
	if tv.PreviewPane != nil && tv.PreviewPane.Visible {
		tv.UpdatePreviewPane()
	}

	// Return true if scrolled near bottom (for lazy loading)
	return tv.TopRow+tv.VisibleRows >= len(tv.Rows)-10
}

// SetSelectedRow sets the selection to a specific row (for mouse click)
func (tv *TableView) SetSelectedRow(row int) {
	// Bounds checking
	if row < 0 {
		row = 0
	}
	if row >= len(tv.Rows) {
		row = len(tv.Rows) - 1
	}
	if row < 0 {
		return // No rows
	}

	tv.SelectedRow = row

	// Adjust visible window if needed
	if tv.SelectedRow < tv.TopRow {
		tv.TopRow = tv.SelectedRow
	}
	if tv.SelectedRow >= tv.TopRow+tv.VisibleRows {
		tv.TopRow = tv.SelectedRow - tv.VisibleRows + 1
	}

	// Update preview pane only if visible
	if tv.PreviewPane != nil && tv.PreviewPane.Visible {
		tv.UpdatePreviewPane()
	}
}

// PageUp/PageDown
func (tv *TableView) PageUp() {
	tv.SelectedRow -= tv.VisibleRows
	if tv.SelectedRow < 0 {
		tv.SelectedRow = 0
	}
	tv.TopRow = tv.SelectedRow
}

func (tv *TableView) PageDown() {
	tv.SelectedRow += tv.VisibleRows
	if tv.SelectedRow >= len(tv.Rows) {
		tv.SelectedRow = len(tv.Rows) - 1
	}
	tv.TopRow = tv.SelectedRow
	if tv.TopRow+tv.VisibleRows > len(tv.Rows) {
		tv.TopRow = len(tv.Rows) - tv.VisibleRows
		if tv.TopRow < 0 {
			tv.TopRow = 0
		}
	}
}

// GetSelectedCell returns the currently selected row and column indices
func (tv *TableView) GetSelectedCell() (row int, col int) {
	return tv.SelectedRow, tv.SelectedCol
}

// MoveSelectionHorizontal moves the selected column left or right with auto-scroll
func (tv *TableView) MoveSelectionHorizontal(delta int) {
	tv.SelectedCol += delta

	// Bounds checking
	if tv.SelectedCol < 0 {
		tv.SelectedCol = 0
	}
	if tv.SelectedCol >= len(tv.Columns) {
		tv.SelectedCol = len(tv.Columns) - 1
	}

	// Auto-scroll to keep selected column visible
	if tv.SelectedCol < tv.LeftColOffset {
		tv.LeftColOffset = tv.SelectedCol
	}
	if tv.SelectedCol >= tv.LeftColOffset+tv.VisibleCols {
		tv.LeftColOffset = tv.SelectedCol - tv.VisibleCols + 1
	}

	// Bounds check LeftColOffset
	if tv.LeftColOffset < 0 {
		tv.LeftColOffset = 0
	}
	maxOffset := len(tv.Columns) - tv.VisibleCols
	if maxOffset < 0 {
		maxOffset = 0
	}
	if tv.LeftColOffset > maxOffset {
		tv.LeftColOffset = maxOffset
	}

	// Update preview pane
	tv.UpdatePreviewPane()
}

// JumpScrollHorizontal scrolls horizontally by half the visible columns
func (tv *TableView) JumpScrollHorizontal(delta int) {
	jumpAmount := tv.VisibleCols / 2
	if jumpAmount < 1 {
		jumpAmount = 1
	}

	tv.SelectedCol += delta * jumpAmount

	// Bounds checking
	if tv.SelectedCol < 0 {
		tv.SelectedCol = 0
	}
	if tv.SelectedCol >= len(tv.Columns) {
		tv.SelectedCol = len(tv.Columns) - 1
	}

	// Update scroll position via MoveSelectionHorizontal's auto-scroll
	tv.MoveSelectionHorizontal(0)
}

// JumpToFirstColumn jumps to the first column
func (tv *TableView) JumpToFirstColumn() {
	tv.SelectedCol = 0
	tv.LeftColOffset = 0
}

// JumpToLastColumn jumps to the last column
func (tv *TableView) JumpToLastColumn() {
	if len(tv.Columns) > 0 {
		tv.SelectedCol = len(tv.Columns) - 1
		// Scroll to show last column
		maxOffset := len(tv.Columns) - tv.VisibleCols
		if maxOffset < 0 {
			maxOffset = 0
		}
		tv.LeftColOffset = maxOffset
	}
}

// ToggleSort toggles sorting on the currently selected column
func (tv *TableView) ToggleSort() {
	if tv.SortColumn == tv.SelectedCol {
		// Same column - toggle direction
		if tv.SortDirection == "ASC" {
			tv.SortDirection = "DESC"
		} else {
			tv.SortDirection = "ASC"
		}
	} else {
		// New column - start with ASC
		tv.SortColumn = tv.SelectedCol
		tv.SortDirection = "ASC"
	}
}

// ToggleNullsFirst toggles NULLS FIRST/LAST for current sort
func (tv *TableView) ToggleNullsFirst() {
	tv.NullsFirst = !tv.NullsFirst
}

// GetSortColumn returns the current sort column name, or empty string if no sort
func (tv *TableView) GetSortColumn() string {
	if tv.SortColumn < 0 || tv.SortColumn >= len(tv.Columns) {
		return ""
	}
	return tv.Columns[tv.SortColumn]
}

// GetSortDirection returns the current sort direction
func (tv *TableView) GetSortDirection() string {
	return tv.SortDirection
}

// GetNullsFirst returns whether NULLS FIRST is enabled
func (tv *TableView) GetNullsFirst() bool {
	return tv.NullsFirst
}

// ReverseSortDirection reverses the current sort direction
// Returns true if there was an active sort to reverse
func (tv *TableView) ReverseSortDirection() bool {
	if tv.SortColumn < 0 {
		return false
	}
	if tv.SortDirection == "ASC" {
		tv.SortDirection = "DESC"
	} else {
		tv.SortDirection = "ASC"
	}
	return true
}

// ClearSort clears the current sort
func (tv *TableView) ClearSort() {
	tv.SortColumn = -1
	tv.SortDirection = "ASC"
	tv.NullsFirst = false
}

// SearchLocal searches only loaded data
func (tv *TableView) SearchLocal(query string) {
	tv.SearchQuery = query
	tv.SearchMode = "local"
	tv.Matches = nil
	tv.CurrentMatch = 0

	if query == "" {
		tv.SearchActive = false
		return
	}

	tv.SearchActive = true
	queryLower := strings.ToLower(query)

	for rowIdx, row := range tv.Rows {
		for colIdx, cell := range row {
			if strings.Contains(strings.ToLower(cell), queryLower) {
				tv.Matches = append(tv.Matches, MatchPos{Row: rowIdx, Col: colIdx})
			}
		}
	}

	if len(tv.Matches) > 0 {
		tv.jumpToMatch(0)
	}
}

// SetSearchResults sets search results from table search
func (tv *TableView) SetSearchResults(query string, matches []MatchPos) {
	tv.SearchQuery = query
	tv.SearchMode = "table"
	tv.Matches = matches
	tv.CurrentMatch = 0
	tv.SearchActive = len(matches) > 0

	if len(matches) > 0 {
		tv.jumpToMatch(0)
	}
}

// jumpToMatch jumps to match at given index
func (tv *TableView) jumpToMatch(idx int) {
	if idx < 0 || idx >= len(tv.Matches) {
		return
	}

	tv.CurrentMatch = idx
	match := tv.Matches[idx]

	// Move selection to match
	tv.SelectedRow = match.Row
	tv.SelectedCol = match.Col

	// Scroll to show match (vertical)
	if tv.SelectedRow < tv.TopRow {
		tv.TopRow = tv.SelectedRow
	}
	if tv.SelectedRow >= tv.TopRow+tv.VisibleRows {
		tv.TopRow = tv.SelectedRow - tv.VisibleRows + 1
	}

	// Horizontal scroll via MoveSelectionHorizontal's auto-scroll
	tv.MoveSelectionHorizontal(0)
}

// NextMatch jumps to next match
func (tv *TableView) NextMatch() {
	if len(tv.Matches) == 0 {
		return
	}
	nextIdx := (tv.CurrentMatch + 1) % len(tv.Matches)
	tv.jumpToMatch(nextIdx)
}

// PrevMatch jumps to previous match
func (tv *TableView) PrevMatch() {
	if len(tv.Matches) == 0 {
		return
	}
	prevIdx := tv.CurrentMatch - 1
	if prevIdx < 0 {
		prevIdx = len(tv.Matches) - 1
	}
	tv.jumpToMatch(prevIdx)
}

// ClearSearch clears search state
func (tv *TableView) ClearSearch() {
	tv.SearchActive = false
	tv.SearchQuery = ""
	tv.Matches = nil
	tv.CurrentMatch = 0
}

// IsMatch checks if a cell is a match
func (tv *TableView) IsMatch(row, col int) bool {
	for _, m := range tv.Matches {
		if m.Row == row && m.Col == col {
			return true
		}
	}
	return false
}

// IsCurrentMatch checks if a cell is the current match
func (tv *TableView) IsCurrentMatch(row, col int) bool {
	if tv.CurrentMatch < 0 || tv.CurrentMatch >= len(tv.Matches) {
		return false
	}
	m := tv.Matches[tv.CurrentMatch]
	return m.Row == row && m.Col == col
}

// GetMatchInfo returns current match info for status bar
func (tv *TableView) GetMatchInfo() (current int, total int) {
	if !tv.SearchActive || len(tv.Matches) == 0 {
		return 0, 0
	}
	return tv.CurrentMatch + 1, len(tv.Matches)
}

// IsCellTruncated checks if the currently selected cell content is truncated
func (tv *TableView) IsCellTruncated() bool {
	if tv.SelectedRow < 0 || tv.SelectedRow >= len(tv.Rows) {
		return false
	}
	if tv.SelectedCol < 0 || tv.SelectedCol >= len(tv.ColumnWidths) {
		return false
	}
	if tv.SelectedCol >= len(tv.Rows[tv.SelectedRow]) {
		return false
	}

	cellValue := tv.Rows[tv.SelectedRow][tv.SelectedCol]
	colWidth := tv.ColumnWidths[tv.SelectedCol]

	// Check if cell content width exceeds column width
	return runewidth.StringWidth(cellValue) > colWidth
}

// GetSelectedCellContent returns the full content of the selected cell
func (tv *TableView) GetSelectedCellContent() string {
	if tv.SelectedRow < 0 || tv.SelectedRow >= len(tv.Rows) {
		return ""
	}
	if tv.SelectedCol < 0 || tv.SelectedCol >= len(tv.Rows[tv.SelectedRow]) {
		return ""
	}
	return tv.Rows[tv.SelectedRow][tv.SelectedCol]
}

// GetSelectedColumnName returns the name of the currently selected column
func (tv *TableView) GetSelectedColumnName() string {
	if tv.SelectedCol < 0 || tv.SelectedCol >= len(tv.Columns) {
		return ""
	}
	return tv.Columns[tv.SelectedCol]
}

// UpdatePreviewPane updates the preview pane with current selection
func (tv *TableView) UpdatePreviewPane() {
	if tv.PreviewPane == nil {
		return
	}

	content := tv.GetSelectedCellContent()
	title := tv.GetSelectedColumnName()
	isTruncated := tv.IsCellTruncated()

	tv.PreviewPane.SetContent(content, title, isTruncated)
}

// SetPreviewPaneDimensions sets the dimensions for the preview pane
func (tv *TableView) SetPreviewPaneDimensions(width, maxHeight int) {
	if tv.PreviewPane != nil {
		tv.PreviewPane.Width = width
		tv.PreviewPane.MaxHeight = maxHeight
	}
}

// TogglePreviewPane toggles the preview pane visibility
func (tv *TableView) TogglePreviewPane() {
	if tv.PreviewPane != nil {
		// Update content before toggling (so it has latest selection)
		tv.UpdatePreviewPane()
		tv.PreviewPane.Toggle()
	}
}

// GetPreviewPaneHeight returns the current preview pane height
func (tv *TableView) GetPreviewPaneHeight() int {
	if tv.PreviewPane != nil {
		return tv.PreviewPane.Height()
	}
	return 0
}

// === Vim Motion Support ===

const vimMotionTimeout = 1500 * time.Millisecond

// HandleVimMotion handles vim-style motion input
// Returns true if the key was handled, false otherwise
func (tv *TableView) HandleVimMotion(key string) bool {
	now := time.Now()

	// Check for timeout - clear pending state if too much time has passed
	if tv.PendingCount != "" || tv.PendingG {
		if now.Sub(tv.PendingCountTime) > vimMotionTimeout {
			tv.ClearVimMotion()
		}
	}

	// Handle number input (0-9)
	if len(key) == 1 && key[0] >= '0' && key[0] <= '9' {
		// Special case: '0' at start means go to beginning (not a count prefix)
		if key == "0" && tv.PendingCount == "" && !tv.PendingG {
			return false // Let it be handled as regular key
		}
		tv.PendingCount += key
		tv.PendingCountTime = now
		tv.PendingG = false
		return true
	}

	// Handle 'g' key (for 'gg')
	if key == "g" {
		if tv.PendingG {
			// Second 'g' - execute gg (go to first line)
			tv.ExecuteJump(0, "gg")
			return true
		}
		// First 'g' - wait for second
		tv.PendingG = true
		tv.PendingCountTime = now
		return true
	}

	// Handle 'G' key (go to line or last line)
	if key == "G" {
		count := tv.getPendingCount()
		if count > 0 {
			// {n}G - go to line n
			tv.ExecuteJump(count, "G")
		} else {
			// G without count - go to last line
			tv.ExecuteJump(0, "G")
		}
		return true
	}

	// Handle 'j' key (down)
	if key == "j" {
		count := tv.getPendingCount()
		if count == 0 {
			count = 1
		}
		tv.ExecuteJump(count, "j")
		return true
	}

	// Handle 'k' key (up)
	if key == "k" {
		count := tv.getPendingCount()
		if count == 0 {
			count = 1
		}
		tv.ExecuteJump(count, "k")
		return true
	}

	// Any other key clears pending state
	if tv.PendingCount != "" || tv.PendingG {
		tv.ClearVimMotion()
	}

	return false
}

// getPendingCount returns the pending count as int and clears it
func (tv *TableView) getPendingCount() int {
	if tv.PendingCount == "" {
		tv.ClearVimMotion()
		return 0
	}
	count, err := strconv.Atoi(tv.PendingCount)
	if err != nil {
		tv.ClearVimMotion()
		return 0
	}
	tv.ClearVimMotion()
	return count
}

// ClearVimMotion clears vim motion state
func (tv *TableView) ClearVimMotion() {
	tv.PendingCount = ""
	tv.PendingG = false
}

// ExecuteJump executes a vim-style jump
func (tv *TableView) ExecuteJump(count int, motion string) {
	tv.ClearVimMotion()

	maxRow := len(tv.Rows) - 1
	if maxRow < 0 {
		return
	}

	switch motion {
	case "gg":
		// Go to first line (or line N if count provided)
		if count > 0 {
			tv.SelectedRow = count - 1 // Convert to 0-indexed
		} else {
			tv.SelectedRow = 0
		}

	case "G":
		// Go to last line (or line N if count provided)
		if count > 0 {
			tv.SelectedRow = count - 1 // Convert to 0-indexed
		} else {
			tv.SelectedRow = maxRow
		}

	case "j":
		// Move down N lines
		tv.SelectedRow += count

	case "k":
		// Move up N lines
		tv.SelectedRow -= count
	}

	// Clamp to valid range
	if tv.SelectedRow < 0 {
		tv.SelectedRow = 0
	}
	if tv.SelectedRow > maxRow {
		tv.SelectedRow = maxRow
	}

	// Adjust scroll to keep selected row visible
	tv.ensureRowVisible()

	// Update preview pane if visible
	if tv.PreviewPane != nil && tv.PreviewPane.Visible {
		tv.UpdatePreviewPane()
	}
}

// ensureRowVisible adjusts TopRow to keep SelectedRow visible
func (tv *TableView) ensureRowVisible() {
	if tv.SelectedRow < tv.TopRow {
		tv.TopRow = tv.SelectedRow
	} else if tv.SelectedRow >= tv.TopRow+tv.VisibleRows {
		tv.TopRow = tv.SelectedRow - tv.VisibleRows + 1
		if tv.TopRow < 0 {
			tv.TopRow = 0
		}
	}
}

// GetVimMotionStatus returns the current vim motion state for status bar
func (tv *TableView) GetVimMotionStatus() string {
	if tv.PendingG {
		if tv.PendingCount != "" {
			return tv.PendingCount + "g_"
		}
		return "g_"
	}
	if tv.PendingCount != "" {
		return tv.PendingCount + "_"
	}
	return ""
}

// HasPendingVimMotion returns true if there's pending vim motion input
func (tv *TableView) HasPendingVimMotion() bool {
	return tv.PendingCount != "" || tv.PendingG
}

// TogglePin pins or unpins the currently selected row
func (tv *TableView) TogglePin() error {
	rowIndex := tv.SelectedRow

	// Check if already pinned
	for i, pinnedIdx := range tv.PinnedRows {
		if pinnedIdx == rowIndex {
			// Unpin
			tv.PinnedRows = append(tv.PinnedRows[:i], tv.PinnedRows[i+1:]...)
			tv.PinnedData = append(tv.PinnedData[:i], tv.PinnedData[i+1:]...)
			return nil
		}
	}

	// Pin
	if tv.MaxPinnedRows > 0 && len(tv.PinnedRows) >= tv.MaxPinnedRows {
		return fmt.Errorf("maximum pinned rows (%d) reached", tv.MaxPinnedRows)
	}

	if rowIndex < 0 || rowIndex >= len(tv.Rows) {
		return fmt.Errorf("invalid row index: %d", rowIndex)
	}

	// Copy the row data
	rowData := make([]string, len(tv.Rows[rowIndex]))
	copy(rowData, tv.Rows[rowIndex])

	tv.PinnedRows = append(tv.PinnedRows, rowIndex)
	tv.PinnedData = append(tv.PinnedData, rowData)
	return nil
}

// IsPinned returns true if the given row is pinned
func (tv *TableView) IsPinned(rowIndex int) bool {
	for _, pinnedIdx := range tv.PinnedRows {
		if pinnedIdx == rowIndex {
			return true
		}
	}
	return false
}

// ClearPins removes all pinned rows
func (tv *TableView) ClearPins() {
	tv.PinnedRows = nil
	tv.PinnedData = nil
}

// GetPinnedCount returns the number of pinned rows
func (tv *TableView) GetPinnedCount() int {
	return len(tv.PinnedRows)
}

// JumpToNextPinnedRow cycles through pinned rows
// If current row is not pinned, jumps to first pinned row
// If current row is pinned, jumps to next pinned row (wraps around)
func (tv *TableView) JumpToNextPinnedRow() {
	if len(tv.PinnedRows) == 0 {
		return
	}

	// Find current position in pinned rows
	currentIdx := -1
	for i, pinnedRow := range tv.PinnedRows {
		if pinnedRow == tv.SelectedRow {
			currentIdx = i
			break
		}
	}

	// Jump to next pinned row (or first if not on pinned row)
	nextIdx := 0
	if currentIdx >= 0 {
		nextIdx = (currentIdx + 1) % len(tv.PinnedRows)
	}

	// Set selected row and ensure it's visible
	tv.SelectedRow = tv.PinnedRows[nextIdx]

	// Adjust visible window if needed
	if tv.SelectedRow < tv.TopRow {
		tv.TopRow = tv.SelectedRow
	}
	if tv.SelectedRow >= tv.TopRow+tv.VisibleRows {
		tv.TopRow = tv.SelectedRow - tv.VisibleRows + 1
	}
}

// NeedsPrefetch returns true if background prefetch should be triggered
func (tv *TableView) NeedsPrefetch() bool {
	if tv.IsPaginating || tv.IsPrefetching {
		return false
	}
	if tv.PrefetchThreshold <= 0 {
		return false
	}
	remaining := len(tv.Rows) - tv.SelectedRow
	return remaining < tv.PrefetchThreshold && len(tv.Rows) < tv.TotalRows
}
