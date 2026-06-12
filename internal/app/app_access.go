package app

import (
	"context"
	"fmt"
	"log"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/pgplex/pgtui/internal/app/delegates"
	"github.com/pgplex/pgtui/internal/app/messages"
	"github.com/pgplex/pgtui/internal/db/connection"
	"github.com/pgplex/pgtui/internal/db/query"
	"github.com/pgplex/pgtui/internal/models"
	"github.com/pgplex/pgtui/internal/ui/components"
)

// Ensure App implements AppAccess interface
var _ delegates.AppAccess = (*App)(nil)

// =============================================================================
// StateAccess implementation
// =============================================================================

// GetState returns the app state
func (a *App) GetState() *models.AppState {
	return &a.state
}

// SetFocusArea updates the current focus area
func (a *App) SetFocusArea(area models.FocusArea) {
	a.state.FocusArea = area
}

// SetTreeSelected updates the selected tree node
func (a *App) SetTreeSelected(node *models.TreeNode) {
	a.state.TreeSelected = node
}

// SetActiveConnection updates the active connection
func (a *App) SetActiveConnection(conn *models.Connection) {
	a.state.ActiveConnection = conn
}

// =============================================================================
// ComponentAccess implementation
// =============================================================================

// GetTreeView returns the tree view component
func (a *App) GetTreeView() *components.TreeView {
	return a.treeView
}

// GetTableView returns the table view component
func (a *App) GetTableView() *components.TableView {
	return a.tableView
}

// GetSQLEditor returns the SQL editor component
func (a *App) GetSQLEditor() *components.SQLEditor {
	return a.sqlEditor
}

// GetResultTabs returns the result tabs component
func (a *App) GetResultTabs() *components.ResultTabs {
	return a.resultTabs
}

// GetConnectionDialog returns the connection dialog component
func (a *App) GetConnectionDialog() *components.ConnectionDialog {
	return a.connectionDialog
}

// GetConnectionManager returns the connection manager
func (a *App) GetConnectionManager() *connection.Manager {
	return a.connectionManager
}

// =============================================================================
// ConnectionAccess implementation
// =============================================================================

// IsConnecting returns whether a connection attempt is in progress
func (a *App) IsConnecting() bool {
	return a.isConnecting
}

// SetConnecting updates the connecting state
func (a *App) SetConnecting(v bool) {
	a.isConnecting = v
}

// GetConnectingConfig returns the config being connected to
func (a *App) GetConnectingConfig() models.ConnectionConfig {
	return a.connectingConfig
}

// SetConnectingConfig updates the connecting config
func (a *App) SetConnectingConfig(config models.ConnectionConfig) {
	a.connectingConfig = config
}

// SetConnectingStart sets when connection attempt started
func (a *App) SetConnectingStart(t time.Time) {
	a.connectingStart = t
}

// SetShowConnectionDialog shows/hides the connection dialog
func (a *App) SetShowConnectionDialog(show bool) {
	a.showConnectionDialog = show
}

// SetShowPasswordDialog shows/hides the password dialog
func (a *App) SetShowPasswordDialog(show bool) {
	a.showPasswordDialog = show
}

// GetPendingConnectionInfo returns pending connection entry
func (a *App) GetPendingConnectionInfo() *models.ConnectionHistoryEntry {
	return a.pendingConnectionInfo
}

// SetPendingConnectionInfo sets pending connection entry
func (a *App) SetPendingConnectionInfo(entry *models.ConnectionHistoryEntry) {
	a.pendingConnectionInfo = entry
}

// ConnectAsync initiates an async connection
func (a *App) ConnectAsync(config models.ConnectionConfig) tea.Cmd {
	return func() tea.Msg {
		connID, err := a.connectionManager.Connect(context.Background(), config)
		return messages.ConnectionResultMsg{
			Config: config,
			ConnID: connID,
			Err:    err,
		}
	}
}

// TriggerDiscovery starts instance discovery
func (a *App) TriggerDiscovery() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		instances := a.discoverer.DiscoverAll(ctx)
		return messages.DiscoveryCompleteMsg{Instances: instances}
	}
}

// SavePassword saves password after successful connection
func (a *App) SavePassword(host string, port int, database, user, password string) error {
	if a.connectionHistory != nil && password != "" {
		return a.connectionHistory.SavePassword(host, port, database, user, password)
	}
	return nil
}

// AddToConnectionHistory saves connection to history and reloads dialog
func (a *App) AddToConnectionHistory(config models.ConnectionConfig) {
	if a.connectionHistory != nil {
		result, err := a.connectionHistory.Add(config)
		if err != nil {
			log.Printf("Warning: Failed to save connection to history: %v", err)
		} else {
			if result != nil && result.PasswordSaveError != nil {
				log.Printf("Warning: Failed to save password: %v", result.PasswordSaveError)
			}
			// Reload history in dialog
			history := a.connectionHistory.GetRecent(10)
			a.connectionDialog.SetHistoryEntries(history)
		}
	}
}

// ClearPendingPasswordSave clears the pending password save
func (a *App) ClearPendingPasswordSave() {
	a.pendingPasswordSave = nil
}

// =============================================================================
// DataAccess implementation
// =============================================================================

// LoadTree loads the navigation tree
func (a *App) LoadTree() tea.Cmd {
	return a.loadTree
}

// LoadNodeChildren loads children for a tree node
func (a *App) LoadNodeChildren(nodeID string) tea.Cmd {
	return a.loadNodeChildren(nodeID)
}

// LoadTableData loads table data with options
func (a *App) LoadTableData(schema, table string, offset, limit int, sortColumn, sortDir string, nullsFirst bool) tea.Cmd {
	return a.loadTableData(messages.LoadTableDataMsg{
		Schema:     schema,
		Table:      table,
		Offset:     offset,
		Limit:      limit,
		SortColumn: sortColumn,
		SortDir:    sortDir,
		NullsFirst: nullsFirst,
	})
}

// LoadTableDataForTab loads table data for a specific tab
func (a *App) LoadTableDataForTab(schema, table, objectID string) tea.Cmd {
	return a.loadTableDataForTab(schema, table, objectID)
}

// LoadTableDataWithFilter loads table data with a filter
func (a *App) LoadTableDataWithFilter(filter models.Filter) tea.Cmd {
	return a.loadTableDataWithFilter(filter)
}

// LoadObjectDetails loads details for a database object
func (a *App) LoadObjectDetails(node *models.TreeNode) tea.Cmd {
	switch node.Type {
	case models.TreeNodeTypeFunction, models.TreeNodeTypeProcedure:
		return a.loadFunctionSource(node)
	case models.TreeNodeTypeTriggerFunction:
		return a.loadTriggerFunctionSource(node)
	case models.TreeNodeTypeSequence:
		return a.loadSequenceDetails(node)
	case models.TreeNodeTypeIndex:
		return a.loadIndexDetails(node)
	case models.TreeNodeTypeTrigger:
		return a.loadTriggerDetails(node)
	case models.TreeNodeTypeExtension:
		return a.loadExtensionDetails(node)
	case models.TreeNodeTypeCompositeType:
		return a.loadCompositeTypeDetails(node)
	case models.TreeNodeTypeEnumType:
		return a.loadEnumTypeDetails(node)
	case models.TreeNodeTypeDomainType:
		return a.loadDomainTypeDetails(node)
	case models.TreeNodeTypeRangeType:
		return a.loadRangeTypeDetails(node)
	default:
		return nil
	}
}

// GetActiveFilter returns the currently active filter
func (a *App) GetActiveFilter() *models.Filter {
	return a.activeFilter
}

// SetActiveFilter sets the active filter
func (a *App) SetActiveFilter(filter *models.Filter) {
	a.activeFilter = filter
}

// =============================================================================
// QueryAccess implementation
// =============================================================================

// ExecuteQuery executes a SQL query asynchronously
func (a *App) ExecuteQuery(sql string) tea.Cmd {
	// Create cancellable context for query execution
	ctx, cancel := context.WithCancel(context.Background())
	a.executeCancelFn = cancel

	return func() tea.Msg {
		conn, err := a.connectionManager.GetActive()
		if err != nil {
			return messages.QueryResultMsg{
				SQL: sql,
				Result: models.QueryResult{
					Error: fmt.Errorf("failed to get connection: %w", err),
				},
			}
		}

		result := query.Execute(ctx, conn.Pool.GetPool(), sql)
		return messages.QueryResultMsg{
			SQL:    sql,
			Result: result,
		}
	}
}

// SaveObjectDefinition saves an object definition
func (a *App) SaveObjectDefinition(msg components.SaveObjectMsg) tea.Cmd {
	return a.saveObjectDefinition(msg)
}

// StartPendingQuery creates a pending query tab
func (a *App) StartPendingQuery(sql string) {
	a.resultTabs.StartPendingQuery(sql)
}

// CompletePendingQuery completes a pending query with results
func (a *App) CompletePendingQuery(sql string, result models.QueryResult) {
	a.resultTabs.CompletePendingQuery(sql, result)
}

// CancelPendingQuery cancels and removes a pending query
func (a *App) CancelPendingQuery() {
	a.resultTabs.CancelPendingQuery()
}

// SetExecuteCancelFn sets the cancel function for query execution
func (a *App) SetExecuteCancelFn(cancel func()) {
	if cancel == nil {
		a.executeCancelFn = nil
	} else {
		a.executeCancelFn = cancel
	}
}

// GetExecuteCancelFn returns the cancel function for query execution
func (a *App) GetExecuteCancelFn() func() {
	if a.executeCancelFn == nil {
		return nil
	}
	return func() {
		a.executeCancelFn()
	}
}

// =============================================================================
// UIAccess implementation
// =============================================================================

// Note: ShowError is defined in app.go

// UpdatePanelStyles refreshes panel styling based on focus
func (a *App) UpdatePanelStyles() {
	a.updatePanelStyles()
}

// SetCurrentTable sets the current table identifier
func (a *App) SetCurrentTable(table string) {
	a.currentTable = table
}

// GetCurrentTable returns the current table identifier
func (a *App) GetCurrentTable() string {
	return a.currentTable
}

// SetShowFilterBuilder shows/hides the filter builder
func (a *App) SetShowFilterBuilder(show bool) {
	a.showFilterBuilder = show
}

// SetShowJSONBViewer shows/hides the JSONB viewer
func (a *App) SetShowJSONBViewer(show bool) {
	a.showJSONBViewer = show
}

// SetShowStructureView shows/hides the structure view
func (a *App) SetShowStructureView(show bool) {
	a.showStructureView = show
}

// SetShowCodeEditor shows/hides the code editor
func (a *App) SetShowCodeEditor(show bool) {
	a.showCodeEditor = show
}

// GetCodeEditor returns the code editor
func (a *App) GetCodeEditor() *components.CodeEditor {
	return a.codeEditor
}

// ClearCodeEditor clears the global code editor state
func (a *App) ClearCodeEditor() {
	a.codeEditor = nil
}

// SetLoadingObjectDetails sets whether object details are loading
func (a *App) SetLoadingObjectDetails(loading bool) {
	a.isLoadingObjectDetails = loading
}

// IsLoadingObjectDetails returns whether object details are loading
func (a *App) IsLoadingObjectDetails() bool {
	return a.isLoadingObjectDetails
}

// CreateTableDataTab creates a new table data tab and returns the cmd to load data
func (a *App) CreateTableDataTab(objectID, label, schema, table string) tea.Cmd {
	// Create new StructureView for this table
	tableView := components.NewTableView(a.theme)
	tableView.Spinner = &a.executeSpinner
	structureView := components.NewStructureView(a.theme, tableView)

	// Set loading state
	tableView.IsLoading = true
	tableView.LoadingStart = time.Now()

	// Add as a new tab
	a.resultTabs.AddTableData(objectID, label, structureView)

	// Switch focus immediately to show loading state
	a.state.FocusArea = models.FocusDataPanel
	a.updatePanelStyles()

	// Load table data asynchronously and start spinner tick
	return tea.Batch(
		a.loadTableDataForTab(schema, table, objectID),
		a.executeSpinner.Tick,
	)
}

// CreateCodeEditorTab creates a new code editor tab for object details
func (a *App) CreateCodeEditorTab(objectID, title, content, objectType, objectName string) {
	codeEditor := components.NewCodeEditor(a.theme)
	codeEditor.SetContent(content, objectType, title)
	codeEditor.ObjectName = objectName

	// Add as a new tab
	a.resultTabs.AddCodeEditor(objectID, title, codeEditor)
}

// GetSpinnerTickCmd returns a command to tick the spinner
func (a *App) GetSpinnerTickCmd() tea.Cmd {
	return a.executeSpinner.Tick
}

// SetShowError shows/hides the error overlay
func (a *App) SetShowError(show bool) {
	a.showError = show
}

// SetShowCommandPalette shows/hides the command palette
func (a *App) SetShowCommandPalette(show bool) {
	a.showCommandPalette = show
}

// SetShowFavorites shows/hides the favorites dialog
func (a *App) SetShowFavorites(show bool) {
	a.showFavorites = show
}

// SetShowSearch shows/hides the search dialog
func (a *App) SetShowSearch(show bool) {
	a.showSearch = show
}

// GetSearchInput returns the search input component
func (a *App) GetSearchInput() delegates.SearchInput {
	return a.searchInput
}

// GetActiveTableView returns the active table view (Result Tabs or main)
func (a *App) GetActiveTableView() *components.TableView {
	return a.getActiveTableView()
}

// SearchTable searches the database table
func (a *App) SearchTable(queryText string) tea.Cmd {
	return a.searchTable(queryText)
}
