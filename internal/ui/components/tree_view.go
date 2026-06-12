package components

// TreeView component provides a visual representation of hierarchical tree data
// with keyboard navigation, expand/collapse functionality, and viewport scrolling.
//
// Features:
//   - Visual tree rendering with Unicode icons (▾ expanded, ▸ collapsed, • leaf)
//   - Keyboard navigation (↑↓/jk, →←/hl, g/G, space, enter)
//   - Automatic viewport scrolling for large trees
//   - Cursor highlighting with theme colors
//   - Active database highlighting
//   - Row count display for tables
//   - Primary key indicators for columns
//   - Empty state handling
//
// Usage:
//
//	root := models.BuildDatabaseTree(databases, activeDB)
//	treeView := components.NewTreeView(root, theme)
//	treeView.Width = 40
//	treeView.Height = 20
//
//	// In your Update method:
//	treeView, cmd := treeView.Update(msg)
//
//	// In your View method:
//	content := treeView.View()

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
	"github.com/pgplex/pgtui/internal/models"
	"github.com/pgplex/pgtui/internal/ui/theme"
)

// Zone ID prefixes for mouse click handling
const (
	ZoneTreeRowPrefix = "tree-row-"
)

// SearchModeState represents the current search state
type SearchModeState int

const (
	SearchOff          SearchModeState = iota // No search active
	SearchInputting                           // User is typing search query (Enter to confirm, Esc to cancel)
	SearchFilterActive                        // Filter confirmed (Enter pressed), Esc to clear filter
)

// TreeView represents a visual tree component for displaying hierarchical data
type TreeView struct {
	Root         *models.TreeNode // Root node of the tree
	CursorIndex  int              // Current cursor position in the flattened list
	Width        int              // Display width
	Height       int              // Display height
	Theme        theme.Theme      // Color theme
	ScrollOffset int              // Vertical scroll offset for viewport

	// Search/filter state
	SearchState    SearchModeState            // Current search state
	SearchQuery    string                     // Current search query text
	FilteredNodes  []*models.TreeNode         // Flat list of nodes matching filter
	MatchPositions map[*models.TreeNode][]int // Match positions for highlighting

	// Loading state
	IsLoading      bool           // True when initial tree is loading
	LoadingNodeID  string         // ID of node currently loading children (for inline spinner)
	LoadingStart   time.Time      // When loading started (for elapsed time)
	Spinner        *spinner.Model // Shared spinner instance
}

// TreeNodeSelectedMsg is sent when a node is selected (Enter key)
type TreeNodeSelectedMsg struct {
	Node *models.TreeNode
}

// TreeNodeExpandedMsg is sent when a node is expanded/collapsed
type TreeNodeExpandedMsg struct {
	Node     *models.TreeNode
	Expanded bool // true if expanded, false if collapsed
}

// NewTreeView creates a new tree view component
func NewTreeView(root *models.TreeNode, theme theme.Theme) *TreeView {
	return &TreeView{
		Root:         root,
		CursorIndex:  0,
		Width:        40,
		Height:       20,
		Theme:        theme,
		ScrollOffset: 0,
	}
}

// getVisibleNodes returns the appropriate node list based on search state
func (tv *TreeView) getVisibleNodes() []*models.TreeNode {
	// Show filtered nodes when in filter active mode or inputting with filter
	if (tv.SearchState == SearchFilterActive || tv.SearchState == SearchInputting) && tv.FilteredNodes != nil {
		return tv.FilteredNodes
	}
	if tv.Root == nil {
		return nil
	}
	return tv.Root.Flatten()
}

// handleSearchInput handles key input during search mode
func (tv *TreeView) handleSearchInput(msg tea.KeyMsg) (*TreeView, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		// Esc during input: clear all search content and effects
		tv.SearchState = SearchOff
		tv.SearchQuery = ""
		tv.FilteredNodes = nil
		tv.MatchPositions = nil
		tv.CursorIndex = 0
		tv.ScrollOffset = 0
		return tv, nil

	case tea.KeyEnter:
		// Enter: confirm search, move to filter active mode
		if tv.SearchQuery != "" {
			tv.SearchState = SearchFilterActive
			tv.CursorIndex = 0
			tv.ScrollOffset = 0
		} else {
			tv.SearchState = SearchOff
		}
		return tv, nil

	case tea.KeyBackspace:
		if len(tv.SearchQuery) > 0 {
			tv.SearchQuery = tv.SearchQuery[:len(tv.SearchQuery)-1]
			tv.applyFilter()
		}
		return tv, nil

	case tea.KeyRunes:
		tv.SearchQuery += string(msg.Runes)
		tv.applyFilter()
		return tv, nil
	}

	return tv, nil
}

// applyFilter applies the current search query to filter the tree
func (tv *TreeView) applyFilter() {
	if tv.SearchQuery == "" {
		tv.FilteredNodes = nil
		tv.MatchPositions = nil
		return
	}

	query := ParseSearchQuery(tv.SearchQuery)
	tv.FilteredNodes = FilterTree(tv.Root, query)

	// Build match positions for highlighting
	tv.MatchPositions = make(map[*models.TreeNode][]int)
	for _, node := range tv.FilteredNodes {
		if query.Pattern != "" && !query.Negate {
			_, positions := FuzzyMatch(query.Pattern, node.Label)
			tv.MatchPositions[node] = positions
		}
	}

	// Reset cursor if out of bounds
	if len(tv.FilteredNodes) > 0 && tv.CursorIndex >= len(tv.FilteredNodes) {
		tv.CursorIndex = 0
	}
}

// View renders the tree as a string
func (tv *TreeView) View() string {
	// Show loading state for initial tree load
	if tv.IsLoading && tv.Root == nil {
		return tv.loadingState()
	}

	if tv.Root == nil {
		return tv.emptyState()
	}

	// Get flattened list of visible nodes
	visibleNodes := tv.getVisibleNodes()

	// Calculate viewport dimensions
	viewHeight := tv.Height
	if viewHeight < 1 {
		viewHeight = 1
	}

	// Calculate search bar height (only in search mode)
	searchBarHeight := tv.getSearchBarHeight()

	// Available height for tree content
	treeViewHeight := viewHeight - searchBarHeight

	if len(visibleNodes) == 0 {
		// Show "No matches found" if we're in search/filter mode
		if tv.SearchState == SearchInputting || tv.SearchState == SearchFilterActive {
			var lines []string

			// Add no matches message
			lines = append(lines, tv.noMatchesState())

			// Fill space before search bar
			for len(lines) < treeViewHeight {
				lines = append(lines, "")
			}

			// Add search bar at bottom
			lines = append(lines, tv.renderSearchBar()...)

			return strings.Join(lines, "\n")
		}
		return tv.emptyState()
	}

	// Ensure cursor is within bounds
	if tv.CursorIndex < 0 {
		tv.CursorIndex = 0
	}
	if tv.CursorIndex >= len(visibleNodes) {
		tv.CursorIndex = len(visibleNodes) - 1
	}

	// Check if we need scroll indicators (content exceeds treeViewHeight)
	nodeViewHeight := treeViewHeight
	needsScrollIndicator := len(visibleNodes) > nodeViewHeight

	// Reserve one line for scroll indicator if needed
	if needsScrollIndicator && nodeViewHeight > 1 {
		nodeViewHeight = nodeViewHeight - 1
	}

	// Auto-scroll to keep cursor visible (use nodeViewHeight for proper scrolling)
	tv.adjustScrollOffset(len(visibleNodes), nodeViewHeight)

	// Build the tree view
	var lines []string

	// Calculate visible range (based on nodeViewHeight, not full viewHeight)
	startIdx := tv.ScrollOffset
	endIdx := tv.ScrollOffset + nodeViewHeight
	if endIdx > len(visibleNodes) {
		endIdx = len(visibleNodes)
	}

	// Render visible nodes with zone marks for mouse support
	for i := startIdx; i < endIdx; i++ {
		node := visibleNodes[i]
		line := tv.renderNode(node, i == tv.CursorIndex)
		// Wrap each row with zone mark for click detection
		// Use visible row index (i - startIdx) for zone ID
		zoneID := fmt.Sprintf("%s%d", ZoneTreeRowPrefix, i-startIdx)
		lines = append(lines, zone.Mark(zoneID, line))

		// If this node is loading, show inline loading indicator after it
		if tv.LoadingNodeID != "" && node.ID == tv.LoadingNodeID {
			lines = append(lines, tv.inlineLoadingNode(node))
		}
	}

	// Add scroll indicator line if needed
	if needsScrollIndicator {
		indicatorLine := tv.renderScrollIndicator(startIdx, endIdx, len(visibleNodes))
		lines = append(lines, indicatorLine)
	}

	// Fill remaining space before search bar
	for len(lines) < treeViewHeight {
		lines = append(lines, "")
	}

	// Add search bar at bottom (in search mode)
	if searchBarHeight > 0 {
		lines = append(lines, tv.renderSearchBar()...)
	}

	return strings.Join(lines, "\n")
}

// Update handles keyboard input for tree navigation
func (tv *TreeView) Update(msg tea.KeyMsg) (*TreeView, tea.Cmd) {
	// Handle search input mode first
	if tv.SearchState == SearchInputting {
		return tv.handleSearchInput(msg)
	}

	// Handle filter active mode (after Enter confirmed search)
	if tv.SearchState == SearchFilterActive {
		switch msg.String() {
		case "esc":
			// Esc clears filter and returns to normal
			tv.SearchState = SearchOff
			tv.SearchQuery = ""
			tv.FilteredNodes = nil
			tv.MatchPositions = nil
			tv.CursorIndex = 0
			tv.ScrollOffset = 0
			return tv, nil
		case "/":
			// "/" starts new search
			tv.SearchState = SearchInputting
			tv.SearchQuery = ""
			tv.FilteredNodes = nil
			tv.MatchPositions = nil
			return tv, nil
		}
		// Fall through to normal navigation on filtered list
	}

	// Normal mode: "/" activates search
	if msg.String() == "/" {
		tv.SearchState = SearchInputting
		tv.SearchQuery = ""
		tv.FilteredNodes = nil
		tv.MatchPositions = nil
		return tv, nil
	}

	if tv.Root == nil {
		return tv, nil
	}

	visibleNodes := tv.getVisibleNodes()
	if len(visibleNodes) == 0 {
		return tv, nil
	}

	var cmd tea.Cmd

	switch msg.String() {
	case "up", "k":
		// Move cursor up
		if tv.CursorIndex > 0 {
			tv.CursorIndex--
		}

	case "down", "j":
		// Move cursor down
		if tv.CursorIndex < len(visibleNodes)-1 {
			tv.CursorIndex++
		}

	case "g":
		// Jump to top
		tv.CursorIndex = 0
		tv.ScrollOffset = 0

	case "G":
		// Jump to bottom
		tv.CursorIndex = len(visibleNodes) - 1

	case "right", "l", " ":
		// Expand node or move into expanded node
		currentNode := visibleNodes[tv.CursorIndex]
		if currentNode != nil {
			wasExpanded := currentNode.Expanded
			currentNode.Toggle()

			// Send expand/collapse message
			if currentNode.Expanded != wasExpanded {
				cmd = func() tea.Msg {
					return TreeNodeExpandedMsg{
						Node:     currentNode,
						Expanded: currentNode.Expanded,
					}
				}
			}
		}

	case "left", "h":
		// Collapse node or move to parent
		currentNode := visibleNodes[tv.CursorIndex]
		if currentNode != nil {
			if currentNode.Expanded {
				// Collapse if expanded
				currentNode.Toggle()
				cmd = func() tea.Msg {
					return TreeNodeExpandedMsg{
						Node:     currentNode,
						Expanded: false,
					}
				}
			} else if currentNode.Parent != nil && currentNode.Parent.Type != models.TreeNodeTypeRoot {
				// Move to parent if collapsed
				parentIndex := tv.findNodeIndex(visibleNodes, currentNode.Parent)
				if parentIndex >= 0 {
					tv.CursorIndex = parentIndex
				}
			}
		}

	case "enter":
		// Select node
		currentNode := visibleNodes[tv.CursorIndex]
		if currentNode != nil && currentNode.Selectable {
			cmd = func() tea.Msg {
				return TreeNodeSelectedMsg{Node: currentNode}
			}
		}
	}

	return tv, cmd
}

// renderNode renders a single tree node with appropriate styling
func (tv *TreeView) renderNode(node *models.TreeNode, selected bool) string {
	if node == nil {
		return ""
	}

	// Calculate indentation based on depth
	// In filter mode, show flat list without indentation
	var indent string
	if tv.SearchState == SearchOff || tv.FilteredNodes == nil {
		depth := node.GetDepth() - 1
		if depth < 0 {
			depth = 0
		}
		indent = strings.Repeat("  ", depth)
	}

	// Choose icon based on node state
	icon := tv.getNodeIcon(node)

	// Build label with match highlighting if in search mode
	label := tv.buildNodeLabelWithHighlight(node, selected)

	// Combine parts
	content := fmt.Sprintf("%s%s %s", indent, icon, label)

	// Truncate if too long
	maxWidth := tv.Width - 2 // Account for padding
	if lipgloss.Width(content) > maxWidth {
		// Simple truncation - may cut mid-escape sequence, but lipgloss handles it
		runes := []rune(content)
		if len(runes) > maxWidth-1 {
			content = string(runes[:maxWidth-1]) + "…"
		}
	}

	// Apply styling
	var style lipgloss.Style
	if selected {
		style = lipgloss.NewStyle().
			Background(tv.Theme.Selection).
			Foreground(tv.Theme.Foreground).
			Bold(true).
			Width(maxWidth)
	} else {
		style = lipgloss.NewStyle().
			Foreground(tv.Theme.Foreground).
			Width(maxWidth)
	}

	return style.Render(content)
}

// getNodeIcon returns the appropriate icon for a node with color
func (tv *TreeView) getNodeIcon(node *models.TreeNode) string {
	var icon string
	var iconColor lipgloss.Color

	switch node.Type {
	case models.TreeNodeTypeDatabase:
		// Check if database is active
		isActive := false
		if meta, ok := node.Metadata.(map[string]interface{}); ok {
			if active, ok := meta["active"].(bool); ok && active {
				isActive = true
			}
		}
		if isActive {
			icon = "●"
			iconColor = tv.Theme.DatabaseActive
		} else {
			icon = "○"
			iconColor = tv.Theme.DatabaseInactive
		}

	case models.TreeNodeTypeSchema:
		if node.Expanded {
			icon = "▾"
			iconColor = tv.Theme.SchemaExpanded
		} else {
			icon = "▸"
			iconColor = tv.Theme.SchemaCollapsed
		}

	case models.TreeNodeTypeTableGroup,
		models.TreeNodeTypeViewGroup,
		models.TreeNodeTypeMaterializedViewGroup,
		models.TreeNodeTypeFunctionGroup,
		models.TreeNodeTypeProcedureGroup,
		models.TreeNodeTypeTriggerFunctionGroup,
		models.TreeNodeTypeSequenceGroup,
		models.TreeNodeTypeTypeGroup,
		models.TreeNodeTypeExtensionGroup,
		models.TreeNodeTypeIndexGroup,
		models.TreeNodeTypeTriggerGroup,
		models.TreeNodeTypeCompositeTypeGroup,
		models.TreeNodeTypeEnumTypeGroup,
		models.TreeNodeTypeDomainTypeGroup,
		models.TreeNodeTypeRangeTypeGroup:
		if node.Expanded {
			icon = "▾"
		} else {
			icon = "▸"
		}
		// Color based on group type
		switch node.Type {
		case models.TreeNodeTypeTableGroup:
			iconColor = tv.Theme.TableIcon
		case models.TreeNodeTypeViewGroup:
			iconColor = tv.Theme.ViewIcon
		case models.TreeNodeTypeMaterializedViewGroup:
			iconColor = tv.Theme.MaterializedViewIcon
		case models.TreeNodeTypeFunctionGroup:
			iconColor = tv.Theme.FunctionIcon
		case models.TreeNodeTypeProcedureGroup:
			iconColor = tv.Theme.ProcedureIcon
		case models.TreeNodeTypeTriggerFunctionGroup:
			iconColor = tv.Theme.TriggerFunctionIcon
		case models.TreeNodeTypeSequenceGroup:
			iconColor = tv.Theme.SequenceIcon
		case models.TreeNodeTypeTypeGroup,
			models.TreeNodeTypeCompositeTypeGroup,
			models.TreeNodeTypeEnumTypeGroup,
			models.TreeNodeTypeDomainTypeGroup,
			models.TreeNodeTypeRangeTypeGroup:
			iconColor = tv.Theme.TypeIcon
		case models.TreeNodeTypeExtensionGroup:
			iconColor = tv.Theme.ExtensionIcon
		case models.TreeNodeTypeIndexGroup:
			iconColor = tv.Theme.IndexIcon
		case models.TreeNodeTypeTriggerGroup:
			iconColor = tv.Theme.TriggerIcon
		default:
			iconColor = tv.Theme.Foreground
		}

	case models.TreeNodeTypeTable:
		icon = "▦"
		iconColor = tv.Theme.TableIcon

	case models.TreeNodeTypeView:
		icon = "◎"
		iconColor = tv.Theme.ViewIcon

	case models.TreeNodeTypeMaterializedView:
		icon = "◉"
		iconColor = tv.Theme.MaterializedViewIcon

	case models.TreeNodeTypeFunction:
		icon = "ƒ"
		iconColor = tv.Theme.FunctionIcon

	case models.TreeNodeTypeProcedure:
		icon = "⚙"
		iconColor = tv.Theme.ProcedureIcon

	case models.TreeNodeTypeTriggerFunction:
		icon = "⚡"
		iconColor = tv.Theme.TriggerFunctionIcon

	case models.TreeNodeTypeSequence:
		icon = "#"
		iconColor = tv.Theme.SequenceIcon

	case models.TreeNodeTypeIndex:
		icon = "⊕"
		iconColor = tv.Theme.IndexIcon

	case models.TreeNodeTypeTrigger:
		icon = "↯"
		iconColor = tv.Theme.TriggerIcon

	case models.TreeNodeTypeExtension:
		icon = "◈"
		iconColor = tv.Theme.ExtensionIcon

	case models.TreeNodeTypeCompositeType:
		icon = "◫"
		iconColor = tv.Theme.TypeIcon

	case models.TreeNodeTypeEnumType:
		icon = "◧"
		iconColor = tv.Theme.TypeIcon

	case models.TreeNodeTypeDomainType:
		icon = "◨"
		iconColor = tv.Theme.TypeIcon

	case models.TreeNodeTypeRangeType:
		icon = "◩"
		iconColor = tv.Theme.TypeIcon

	case models.TreeNodeTypeColumn:
		icon = "•"
		iconColor = tv.Theme.ColumnIcon

	default:
		// Generic expandable/collapsible
		if node.Expanded {
			icon = "▾"
			iconColor = tv.Theme.Foreground
		} else {
			icon = "▸"
			iconColor = tv.Theme.Foreground
		}
	}

	// Apply color and return
	return lipgloss.NewStyle().Foreground(iconColor).Render(icon)
}

// buildNodeLabelWithHighlight builds the display label with match highlighting and schema path
func (tv *TreeView) buildNodeLabelWithHighlight(node *models.TreeNode, selected bool) string {
	metaStyle := lipgloss.NewStyle().Foreground(tv.Theme.Metadata)
	highlightStyle := lipgloss.NewStyle().Foreground(tv.Theme.Warning).Bold(true)

	// Get base label with highlighting
	var labelPart string

	// Check if we have match positions for this node (search mode)
	positions, hasPositions := tv.MatchPositions[node]
	if hasPositions && len(positions) > 0 && !selected {
		// Render label with highlighted match positions
		labelPart = tv.renderHighlightedText(node.Label, positions, highlightStyle)
	} else {
		labelPart = node.Label
	}

	// Build suffix parts (schema path in filter mode, or metadata in normal mode)
	var suffix string

	if len(tv.FilteredNodes) > 0 {
		// In filter mode: add schema path for context
		schemaName := tv.getNodeSchemaName(node)
		if schemaName != "" {
			suffix = " " + metaStyle.Render("("+schemaName+")")
		}
	} else {
		// Normal mode: add metadata based on node type
		switch node.Type {
		case models.TreeNodeTypeSchema:
			if node.Loaded && len(node.Children) == 0 {
				suffix = " " + metaStyle.Render("∅")
			}
		case models.TreeNodeTypeTable:
			if meta, ok := node.Metadata.(map[string]interface{}); ok {
				if rowCount, ok := meta["row_count"].(int64); ok {
					suffix = " " + metaStyle.Render(formatNumber(rowCount))
				}
			}
		case models.TreeNodeTypeColumn:
			if meta, ok := node.Metadata.(models.ColumnInfo); ok {
				if meta.PrimaryKey {
					pkStyle := lipgloss.NewStyle().Foreground(tv.Theme.PrimaryKey)
					suffix = " " + pkStyle.Render("⚿")
				}
			}
		}
	}

	return labelPart + suffix
}

// renderHighlightedText renders text with specific positions highlighted
func (tv *TreeView) renderHighlightedText(text string, positions []int, highlightStyle lipgloss.Style) string {
	if len(positions) == 0 {
		return text
	}

	// Create a set of positions to highlight
	posSet := make(map[int]bool)
	for _, p := range positions {
		posSet[p] = true
	}

	// Build the highlighted string
	var result strings.Builder
	runes := []rune(text)

	for i, r := range runes {
		if posSet[i] {
			result.WriteString(highlightStyle.Render(string(r)))
		} else {
			result.WriteRune(r)
		}
	}

	return result.String()
}

// getNodeSchemaName returns the schema name for a node (for display in filter results)
func (tv *TreeView) getNodeSchemaName(node *models.TreeNode) string {
	// Walk up the tree to find the schema node
	current := node.Parent
	for current != nil {
		if current.Type == models.TreeNodeTypeSchema {
			return current.Label
		}
		current = current.Parent
	}
	return ""
}

// adjustScrollOffset adjusts the scroll offset to keep the cursor visible
func (tv *TreeView) adjustScrollOffset(totalNodes, viewHeight int) {
	// Ensure cursor is visible in viewport
	if tv.CursorIndex < tv.ScrollOffset {
		tv.ScrollOffset = tv.CursorIndex
	}
	if tv.CursorIndex >= tv.ScrollOffset+viewHeight {
		tv.ScrollOffset = tv.CursorIndex - viewHeight + 1
	}

	// Ensure scroll offset is within bounds
	if tv.ScrollOffset < 0 {
		tv.ScrollOffset = 0
	}
	maxScroll := totalNodes - viewHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	if tv.ScrollOffset > maxScroll {
		tv.ScrollOffset = maxScroll
	}
}

// renderScrollIndicator renders a scroll indicator line showing items above/below
func (tv *TreeView) renderScrollIndicator(startIdx, endIdx, total int) string {
	// Build scroll status indicator (e.g., "↑3 ↓5" meaning 3 above, 5 below)
	var indicators []string
	if startIdx > 0 {
		indicators = append(indicators, fmt.Sprintf("↑%d", startIdx))
	}
	if endIdx < total {
		remaining := total - endIdx
		indicators = append(indicators, fmt.Sprintf("↓%d", remaining))
	}

	indicatorText := strings.Join(indicators, " ")

	// Style and right-align the indicator
	maxWidth := tv.Width - 2 // Same as used in renderNode
	return lipgloss.NewStyle().
		Foreground(tv.Theme.Info).
		Width(maxWidth).
		Align(lipgloss.Right).
		Render(indicatorText)
}

// emptyState returns the empty state view
func (tv *TreeView) emptyState() string {
	style := lipgloss.NewStyle().
		Foreground(tv.Theme.Comment).
		Italic(true).
		Width(tv.Width - 2).
		Align(lipgloss.Center)

	return style.Render("No databases connected")
}

// loadingState returns the loading state view for initial tree loading
func (tv *TreeView) loadingState() string {
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

	content := lipgloss.JoinVertical(lipgloss.Center,
		"",
		spinnerView+loadingStyle.Render("Loading databases...")+elapsedStyle.Render(" "+elapsedStr),
		"",
	)

	return lipgloss.Place(tv.Width, tv.Height, lipgloss.Center, lipgloss.Center, content)
}

// inlineLoadingNode returns a loading indicator for a specific node
func (tv *TreeView) inlineLoadingNode(node *models.TreeNode) string {
	// Calculate indentation
	depth := node.GetDepth() - 1
	if depth < 0 {
		depth = 0
	}
	indent := strings.Repeat("  ", depth+1) // Extra indent for child position

	spinnerView := ""
	if tv.Spinner != nil {
		spinnerView = tv.Spinner.View()
	}

	loadingStyle := lipgloss.NewStyle().
		Foreground(tv.Theme.Comment).
		Italic(true)

	content := fmt.Sprintf("%s%s %s", indent, spinnerView, loadingStyle.Render("Loading..."))

	maxWidth := tv.Width - 2
	style := lipgloss.NewStyle().Width(maxWidth)
	return style.Render(content)
}

// noMatchesState returns the no matches view
func (tv *TreeView) noMatchesState() string {
	style := lipgloss.NewStyle().
		Foreground(tv.Theme.Comment).
		Italic(true).
		Width(tv.Width - 2).
		Align(lipgloss.Center)

	return style.Render("No matches found")
}

// findNodeIndex finds the index of a node in the flattened list
func (tv *TreeView) findNodeIndex(nodes []*models.TreeNode, target *models.TreeNode) int {
	for i, node := range nodes {
		if node == target {
			return i
		}
	}
	return -1
}

// formatNumber formats a number with commas for readability
func formatNumber(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 10000 {
		// For 1k-10k, show one decimal place unless it's a round number
		k := float64(n) / 1000.0
		if k == float64(int(k)) {
			return fmt.Sprintf("%.0fk", k)
		}
		return fmt.Sprintf("%.1fk", k)
	}
	if n < 1000000 {
		return fmt.Sprintf("%.0fk", float64(n)/1000.0)
	}
	return fmt.Sprintf("%.1fM", float64(n)/1000000.0)
}

// GetCurrentNode returns the currently selected node
func (tv *TreeView) GetCurrentNode() *models.TreeNode {
	if tv.Root == nil {
		return nil
	}

	visibleNodes := tv.getVisibleNodes()
	if tv.CursorIndex < 0 || tv.CursorIndex >= len(visibleNodes) {
		return nil
	}

	return visibleNodes[tv.CursorIndex]
}

// SetCursorToNode sets the cursor to a specific node (by ID)
func (tv *TreeView) SetCursorToNode(nodeID string) bool {
	if tv.Root == nil {
		return false
	}

	visibleNodes := tv.Root.Flatten()
	for i, node := range visibleNodes {
		if node.ID == nodeID {
			tv.CursorIndex = i
			return true
		}
	}

	return false
}

// ExpandAndNavigateToNode expands all ancestors of a node and moves cursor to it
// This is useful for programmatic navigation (e.g., from table jump dialog)
func (tv *TreeView) ExpandAndNavigateToNode(nodeID string) bool {
	if tv.Root == nil {
		return false
	}

	// Find the node by ID
	targetNode := tv.Root.FindByID(nodeID)
	if targetNode == nil {
		return false
	}

	// Expand all ancestors from root to parent
	current := targetNode.Parent
	for current != nil && current.Type != models.TreeNodeTypeRoot {
		current.Expanded = true
		current = current.Parent
	}

	// Now the node should be visible, set cursor to it
	visibleNodes := tv.Root.Flatten()
	for i, node := range visibleNodes {
		if node.ID == nodeID {
			tv.CursorIndex = i
			// Adjust scroll offset to make the node visible
			tv.adjustScrollOffset(len(visibleNodes), tv.Height)
			return true
		}
	}

	return false
}

// ScrollUp scrolls the tree view up by n lines (for mouse wheel)
func (tv *TreeView) ScrollUp(n int) {
	if tv.Root == nil {
		return
	}

	visibleNodes := tv.getVisibleNodes()
	if len(visibleNodes) == 0 {
		return
	}

	// Scroll viewport up (like lazygit)
	tv.ScrollOffset -= n
	if tv.ScrollOffset < 0 {
		tv.ScrollOffset = 0
	}

	// Keep cursor within visible range
	if tv.CursorIndex >= tv.ScrollOffset+tv.Height {
		tv.CursorIndex = tv.ScrollOffset + tv.Height - 1
	}
	if tv.CursorIndex < tv.ScrollOffset {
		tv.CursorIndex = tv.ScrollOffset
	}
	// Bounds check
	if tv.CursorIndex >= len(visibleNodes) {
		tv.CursorIndex = len(visibleNodes) - 1
	}
	if tv.CursorIndex < 0 {
		tv.CursorIndex = 0
	}
}

// ScrollDown scrolls the tree view down by n lines (for mouse wheel)
func (tv *TreeView) ScrollDown(n int) {
	if tv.Root == nil {
		return
	}

	visibleNodes := tv.getVisibleNodes()
	if len(visibleNodes) == 0 {
		return
	}

	// Scroll viewport down (like lazygit)
	maxScrollOffset := len(visibleNodes) - tv.Height
	if maxScrollOffset < 0 {
		maxScrollOffset = 0
	}
	tv.ScrollOffset += n
	if tv.ScrollOffset > maxScrollOffset {
		tv.ScrollOffset = maxScrollOffset
	}

	// Keep cursor within visible range
	if tv.CursorIndex < tv.ScrollOffset {
		tv.CursorIndex = tv.ScrollOffset
	}
	if tv.CursorIndex >= tv.ScrollOffset+tv.Height {
		tv.CursorIndex = tv.ScrollOffset + tv.Height - 1
	}
	// Bounds check
	if tv.CursorIndex >= len(visibleNodes) {
		tv.CursorIndex = len(visibleNodes) - 1
	}
	if tv.CursorIndex < 0 {
		tv.CursorIndex = 0
	}
}

// HandleClick handles mouse click at a specific row offset from the top of the visible area
// Lazygit-style: clicking already selected item triggers action (select for tables, toggle for expandable)
func (tv *TreeView) HandleClick(clickedRow int) (*TreeView, tea.Cmd) {
	if tv.Root == nil {
		return tv, nil
	}

	visibleNodes := tv.getVisibleNodes()
	if len(visibleNodes) == 0 {
		return tv, nil
	}

	// Calculate which node was clicked
	targetIndex := tv.ScrollOffset + clickedRow
	if targetIndex < 0 || targetIndex >= len(visibleNodes) {
		return tv, nil
	}

	clickedNode := visibleNodes[targetIndex]
	wasAlreadySelected := tv.CursorIndex == targetIndex

	// Update cursor to clicked node
	tv.CursorIndex = targetIndex

	// If clicking already selected node, trigger action
	if wasAlreadySelected {
		// For expandable nodes, toggle expansion
		if len(clickedNode.Children) > 0 || !clickedNode.Loaded {
			clickedNode.Toggle()
			return tv, func() tea.Msg {
				return TreeNodeExpandedMsg{
					Node:     clickedNode,
					Expanded: clickedNode.Expanded,
				}
			}
		}
		// For selectable leaf nodes (tables), select/activate them
		if clickedNode.Selectable {
			return tv, func() tea.Msg {
				return TreeNodeSelectedMsg{Node: clickedNode}
			}
		}
	}

	// First click just selects the node (no action)
	return tv, nil
}

// IsSearchInputting returns true if the TreeView is in search input mode
// Used by app to route all keys to TreeView during search
func (tv *TreeView) IsSearchInputting() bool {
	return tv.SearchState == SearchInputting
}

// IsSearchActive returns true if search/filter is active (inputting or filter applied)
func (tv *TreeView) IsSearchActive() bool {
	return tv.SearchState == SearchInputting || tv.SearchState == SearchFilterActive
}

// GetSearchStatus returns the search status string for display in panel title
// Returns empty string if search is not active
// Format (lazygit style):
//   - SearchInputting: "Search: query" (with block cursor shown separately)
//   - SearchFilterActive: "/query (N)" where N is match count
func (tv *TreeView) GetSearchStatus() string {
	switch tv.SearchState {
	case SearchInputting:
		return fmt.Sprintf("Search: %s", tv.SearchQuery)
	case SearchFilterActive:
		count := len(tv.FilteredNodes)
		return fmt.Sprintf("/%s (%d)", tv.SearchQuery, count)
	}
	return ""
}

// getSearchBarHeight returns the height of the search bar area
// Returns 0 when search is not active
func (tv *TreeView) getSearchBarHeight() int {
	switch tv.SearchState {
	case SearchInputting, SearchFilterActive:
		// Render the search bar and measure its actual height
		searchBarLines := tv.renderSearchBar()
		if len(searchBarLines) == 0 {
			return 0
		}
		// Use lipgloss.Height to measure the rendered content
		return lipgloss.Height(searchBarLines[0])
	default:
		return 0
	}
}

// renderSearchBar renders the bottom search bar with type tag and hints
func (tv *TreeView) renderSearchBar() []string {
	if tv.SearchState == SearchOff {
		return nil
	}

	maxWidth := tv.Width - 2
	var lines []string

	// Build search box content
	searchLine := tv.renderSearchInputLine(maxWidth - 4) // -4 for border padding

	// Hints line
	var hintsLine string
	if tv.SearchState == SearchInputting {
		hintsLine = tv.renderSyntaxHints(maxWidth - 4)
	} else {
		// In filter active mode, show navigation hints
		hintsLine = tv.renderFilterActiveHints(maxWidth - 4)
	}

	// Create bordered box with both lines
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tv.Theme.BorderFocused).
		Width(maxWidth)

	boxContent := searchLine + "\n" + hintsLine
	lines = append(lines, boxStyle.Render(boxContent))

	return lines
}

// renderSearchInputLine renders the search input with type tag
func (tv *TreeView) renderSearchInputLine(maxWidth int) string {
	// Parse the current query to extract type and pattern
	query := ParseSearchQuery(tv.SearchQuery)

	// Build the search line components
	var parts []string

	// Search icon and prompt
	iconStyle := lipgloss.NewStyle().Foreground(tv.Theme.Info)
	promptStyle := lipgloss.NewStyle().Foreground(tv.Theme.Comment)
	parts = append(parts, iconStyle.Render("🔍")+promptStyle.Render(" >"))

	// Type tag (if present)
	if query.TypeFilter != "" {
		tagContent := tv.buildTypeTag(query.TypeFilter, query.Negate)
		parts = append(parts, tagContent)
	} else if query.Negate && query.Pattern != "" {
		// Negation without type filter
		negateStyle := lipgloss.NewStyle().
			Foreground(tv.Theme.Error).
			Bold(true)
		parts = append(parts, negateStyle.Render("!"))
	}

	// Search pattern with cursor
	patternStyle := lipgloss.NewStyle().Foreground(tv.Theme.Foreground)
	pattern := query.Pattern
	if tv.SearchState == SearchInputting {
		// Show cursor
		cursorStyle := lipgloss.NewStyle().
			Foreground(tv.Theme.Cursor).
			Bold(true)
		pattern += cursorStyle.Render("█")
	}
	parts = append(parts, patternStyle.Render(pattern))

	// Match count / position
	countStyle := lipgloss.NewStyle().Foreground(tv.Theme.Metadata)
	if len(tv.FilteredNodes) > 0 {
		// Show position (1-indexed) / total
		pos := tv.CursorIndex + 1
		total := len(tv.FilteredNodes)
		parts = append(parts, countStyle.Render(fmt.Sprintf("(%d/%d)", pos, total)))
	} else if tv.SearchQuery != "" {
		parts = append(parts, countStyle.Render("(0)"))
	}

	// Join parts with space
	content := strings.Join(parts, " ")

	// Ensure fixed width
	style := lipgloss.NewStyle().Width(maxWidth)
	return style.Render(content)
}

// buildTypeTag builds a styled type tag like [▦ Table] or [! ▦ Table]
func (tv *TreeView) buildTypeTag(typeFilter string, negate bool) string {
	// Get icon and label for the type
	icon, label := tv.getTypeIconAndLabel(typeFilter)

	// Build tag content
	var tagContent string
	if negate {
		negateStyle := lipgloss.NewStyle().Foreground(tv.Theme.Error).Bold(true)
		tagContent = negateStyle.Render("!") + " " + icon + " " + label
	} else {
		tagContent = icon + " " + label
	}

	// Tag style with background
	tagStyle := lipgloss.NewStyle().
		Background(tv.Theme.Selection).
		Foreground(tv.Theme.Foreground).
		Padding(0, 1)

	return tagStyle.Render(tagContent)
}

// getTypeIconAndLabel returns the icon and label for a type filter
func (tv *TreeView) getTypeIconAndLabel(typeFilter string) (icon string, label string) {
	iconStyle := func(color lipgloss.Color) lipgloss.Style {
		return lipgloss.NewStyle().Foreground(color)
	}

	switch typeFilter {
	case "table":
		return iconStyle(tv.Theme.TableIcon).Render("▦"), "Table"
	case "view":
		return iconStyle(tv.Theme.ViewIcon).Render("◎"), "View"
	case "function":
		return iconStyle(tv.Theme.FunctionIcon).Render("ƒ"), "Func"
	case "schema":
		return iconStyle(tv.Theme.SchemaExpanded).Render("▾"), "Schema"
	case "sequence":
		return iconStyle(tv.Theme.SequenceIcon).Render("#"), "Seq"
	case "extension":
		return iconStyle(tv.Theme.ExtensionIcon).Render("◈"), "Ext"
	case "column":
		return iconStyle(tv.Theme.ColumnIcon).Render("•"), "Col"
	case "index":
		return iconStyle(tv.Theme.IndexIcon).Render("⊕"), "Idx"
	default:
		return "", typeFilter
	}
}

// renderSyntaxHints renders the syntax hints line (during input mode)
func (tv *TreeView) renderSyntaxHints(maxWidth int) string {
	// Use explicit gray color for hints (Overlay0 from catppuccin)
	hintColor := lipgloss.Color("#6c7086")
	dimStyle := lipgloss.NewStyle().Foreground(hintColor).Italic(true)
	sepStyle := lipgloss.NewStyle().Foreground(hintColor)

	// Format: "t: table │ v: view │ f: func │ !: not │ Esc: exit"
	// Apply style to each item individually to avoid rendering issues
	hints := []string{
		dimStyle.Render("t: table"),
		dimStyle.Render("v: view"),
		dimStyle.Render("f: func"),
		dimStyle.Render("!: not"),
		dimStyle.Render("Esc: exit"),
	}

	content := strings.Join(hints, sepStyle.Render(" │ "))

	// Ensure fixed width
	style := lipgloss.NewStyle().Width(maxWidth)
	return style.Render(content)
}

// renderFilterActiveHints renders hints when filter is active (after Enter)
func (tv *TreeView) renderFilterActiveHints(maxWidth int) string {
	// Use explicit gray color for hints (Overlay0 from catppuccin)
	hintColor := lipgloss.Color("#6c7086")
	dimStyle := lipgloss.NewStyle().Foreground(hintColor).Italic(true)
	sepStyle := lipgloss.NewStyle().Foreground(hintColor)

	// Format: "j/k: navigate │ Enter: select │ /: search │ Esc: clear"
	// Apply style to each item individually to avoid rendering issues
	hints := []string{
		dimStyle.Render("j/k: navigate"),
		dimStyle.Render("Enter: select"),
		dimStyle.Render("/: search"),
		dimStyle.Render("Esc: clear"),
	}

	content := strings.Join(hints, sepStyle.Render(" │ "))

	style := lipgloss.NewStyle().Width(maxWidth)
	return style.Render(content)
}
