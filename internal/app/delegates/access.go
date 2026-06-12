package delegates

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/pgplex/pgtui/internal/db/connection"
	"github.com/pgplex/pgtui/internal/models"
	"github.com/pgplex/pgtui/internal/ui/components"
)

// AppAccess defines what delegates can access from the App.
// This interface controls the boundary between delegates and the App,
// enabling testability and clear separation of concerns.
type AppAccess interface {
	// State access
	StateAccess

	// Component access
	ComponentAccess

	// Connection operations
	ConnectionAccess

	// Data loading operations
	DataAccess

	// Query operations
	QueryAccess

	// UI operations
	UIAccess
}

// StateAccess provides read/write access to app state
type StateAccess interface {
	// GetState returns the app state (read-only access preferred)
	GetState() *models.AppState

	// SetFocusArea updates the current focus area
	SetFocusArea(area models.FocusArea)

	// SetTreeSelected updates the selected tree node
	SetTreeSelected(node *models.TreeNode)

	// SetActiveConnection updates the active connection
	SetActiveConnection(conn *models.Connection)
}

// ComponentAccess provides access to UI components
type ComponentAccess interface {
	// Tree view
	GetTreeView() *components.TreeView

	// Table view
	GetTableView() *components.TableView

	// SQL editor
	GetSQLEditor() *components.SQLEditor

	// Result tabs
	GetResultTabs() *components.ResultTabs

	// Connection dialog
	GetConnectionDialog() *components.ConnectionDialog

	// Connection manager
	GetConnectionManager() *connection.Manager
}

// ConnectionAccess provides connection-related operations
type ConnectionAccess interface {
	// IsConnecting returns whether a connection attempt is in progress
	IsConnecting() bool

	// SetConnecting updates the connecting state
	SetConnecting(v bool)

	// GetConnectingConfig returns the config being connected to
	GetConnectingConfig() models.ConnectionConfig

	// SetConnectingConfig updates the connecting config
	SetConnectingConfig(config models.ConnectionConfig)

	// SetConnectingStart sets when connection attempt started
	SetConnectingStart(t time.Time)

	// ShowConnectionDialog shows/hides the connection dialog
	SetShowConnectionDialog(show bool)

	// ShowPasswordDialog shows/hides the password dialog
	SetShowPasswordDialog(show bool)

	// GetPendingConnectionInfo returns pending connection entry
	GetPendingConnectionInfo() *models.ConnectionHistoryEntry

	// SetPendingConnectionInfo sets pending connection entry
	SetPendingConnectionInfo(entry *models.ConnectionHistoryEntry)

	// ConnectAsync initiates an async connection
	ConnectAsync(config models.ConnectionConfig) tea.Cmd

	// TriggerDiscovery starts instance discovery
	TriggerDiscovery() tea.Cmd

	// SavePassword saves password after successful connection
	SavePassword(host string, port int, database, user, password string) error

	// AddToConnectionHistory saves connection to history and reloads dialog
	AddToConnectionHistory(config models.ConnectionConfig)

	// ClearPendingPasswordSave clears the pending password save
	ClearPendingPasswordSave()
}

// DataAccess provides data loading operations
type DataAccess interface {
	// LoadTree loads the navigation tree
	LoadTree() tea.Cmd

	// LoadNodeChildren loads children for a tree node
	LoadNodeChildren(nodeID string) tea.Cmd

	// LoadTableData loads table data with options
	LoadTableData(schema, table string, offset, limit int, sortColumn, sortDir string, nullsFirst bool) tea.Cmd

	// LoadTableDataForTab loads table data for a specific tab
	LoadTableDataForTab(schema, table, objectID string) tea.Cmd

	// LoadTableDataWithFilter loads table data with a filter
	LoadTableDataWithFilter(filter models.Filter) tea.Cmd

	// LoadObjectDetails loads details for a database object (function, sequence, etc.)
	LoadObjectDetails(node *models.TreeNode) tea.Cmd

	// GetActiveFilter returns the currently active filter
	GetActiveFilter() *models.Filter

	// SetActiveFilter sets the active filter
	SetActiveFilter(filter *models.Filter)

	// PrefetchData prefetches table data in background
	PrefetchData(schema, table string, offset, limit int, sortCol, sortDir string, nullsFirst bool) tea.Cmd
}

// QueryAccess provides query execution operations
type QueryAccess interface {
	// ExecuteQuery executes a SQL query asynchronously
	ExecuteQuery(sql string) tea.Cmd

	// SaveObjectDefinition saves an object definition (function, view, etc.)
	SaveObjectDefinition(msg components.SaveObjectMsg) tea.Cmd

	// StartPendingQuery creates a pending query tab
	StartPendingQuery(sql string)

	// CompletePendingQuery completes a pending query with results
	CompletePendingQuery(sql string, result models.QueryResult)

	// CancelPendingQuery cancels and removes a pending query
	CancelPendingQuery()

	// SetExecuteCancelFn sets the cancel function for query execution
	SetExecuteCancelFn(cancel func())

	// GetExecuteCancelFn returns the cancel function for query execution
	GetExecuteCancelFn() func()
}

// UIAccess provides UI-related operations
type UIAccess interface {
	// ShowError displays an error overlay
	ShowError(title, message string)

	// UpdatePanelStyles refreshes panel styling based on focus
	UpdatePanelStyles()

	// SetCurrentTable sets the current table identifier
	SetCurrentTable(table string)

	// GetCurrentTable returns the current table identifier
	GetCurrentTable() string

	// SetShowFilterBuilder shows/hides the filter builder
	SetShowFilterBuilder(show bool)

	// SetShowJSONBViewer shows/hides the JSONB viewer
	SetShowJSONBViewer(show bool)

	// SetShowStructureView shows/hides the structure view
	SetShowStructureView(show bool)

	// SetShowCodeEditor shows/hides the code editor
	SetShowCodeEditor(show bool)

	// GetCodeEditor returns the code editor
	GetCodeEditor() *components.CodeEditor

	// ClearCodeEditor clears the global code editor state
	ClearCodeEditor()

	// SetLoadingObjectDetails sets whether object details are loading
	SetLoadingObjectDetails(loading bool)

	// IsLoadingObjectDetails returns whether object details are loading
	IsLoadingObjectDetails() bool

	// CreateTableDataTab creates a new table data tab and returns the cmd to load data
	CreateTableDataTab(objectID, label, schema, table string) tea.Cmd

	// CreateCodeEditorTab creates a new code editor tab for object details
	CreateCodeEditorTab(objectID, title, content, objectType, objectName string)

	// GetSpinnerTickCmd returns a command to tick the spinner
	GetSpinnerTickCmd() tea.Cmd

	// SetShowError shows/hides the error overlay
	SetShowError(show bool)

	// SetShowCommandPalette shows/hides the command palette
	SetShowCommandPalette(show bool)

	// SetShowFavorites shows/hides the favorites dialog
	SetShowFavorites(show bool)

	// SetShowSearch shows/hides the search dialog
	SetShowSearch(show bool)

	// GetSearchInput returns the search input component
	GetSearchInput() SearchInput

	// GetActiveTableView returns the active table view (Result Tabs or main)
	GetActiveTableView() *components.TableView

	// SearchTable searches the database table
	SearchTable(query string) tea.Cmd
}

// SearchInput represents the search input interface
type SearchInput interface {
	Reset()
}
