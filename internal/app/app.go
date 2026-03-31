package app

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone"
	"github.com/rebelice/lazypg/internal/app/delegates"
	"github.com/rebelice/lazypg/internal/app/messages"
	"github.com/rebelice/lazypg/internal/commands"
	"github.com/rebelice/lazypg/internal/config"
	"github.com/rebelice/lazypg/internal/connection_history"
	"github.com/rebelice/lazypg/internal/db/connection"
	"github.com/rebelice/lazypg/internal/db/discovery"
	"github.com/rebelice/lazypg/internal/db/metadata"
	"github.com/rebelice/lazypg/internal/db/query"
	"github.com/rebelice/lazypg/internal/favorites"
	filterBuilder "github.com/rebelice/lazypg/internal/filter"
	"github.com/rebelice/lazypg/internal/history"
	"github.com/rebelice/lazypg/internal/jsonb"
	"github.com/rebelice/lazypg/internal/models"
	"github.com/rebelice/lazypg/internal/ui/components"
	"github.com/rebelice/lazypg/internal/ui/help"
	"github.com/rebelice/lazypg/internal/ui/theme"
)

// pendingPassword holds password info to save after successful connection
type pendingPassword struct {
	Host     string
	Port     int
	Database string
	User     string
	Password string
}

// App is the main application model
type App struct {
	state      models.AppState
	config     *config.Config
	theme      theme.Theme
	leftPanel  components.Panel
	rightPanel components.Panel

	// Phase 2: Connection management
	connectionManager *connection.Manager
	discoverer        *discovery.Discoverer

	// Connection dialog
	showConnectionDialog bool
	connectionDialog     *components.ConnectionDialog
	isConnecting         bool      // True when connection attempt is in progress
	connectingStart      time.Time // When connection attempt started
	connectingConfig     models.ConnectionConfig

	// Error overlay
	showError    bool
	errorOverlay *components.ErrorOverlay

	// Phase 3: Navigation tree
	treeView *components.TreeView

	// Table view
	tableView    *components.TableView
	currentTable string // "schema.table"

	// Phase 4: Command palette
	showCommandPalette bool
	commandPalette     *components.CommandPalette
	commandRegistry    *commands.Registry

	// SQL Editor
	sqlEditor  *components.SQLEditor
	resultTabs *components.ResultTabs

	// History
	historyStore *history.Store

	// Filter builder
	showFilterBuilder bool
	filterBuilder     *components.FilterBuilder
	activeFilter      *models.Filter

	// JSONB viewer
	showJSONBViewer bool
	jsonbViewer     *components.JSONBViewer

	// Structure view
	showStructureView bool
	structureView     *components.StructureView
	currentTab        int // 0=Data, 1=Columns, 2=Constraints, 3=Indexes

	// Code editor for viewing/editing database object definitions
	codeEditor             *components.CodeEditor
	showCodeEditor         bool
	isLoadingObjectDetails bool // Loading indicator for function/sequence/etc details

	// Favorites
	showFavorites    bool
	favoritesManager *favorites.Manager
	favoritesDialog  *components.FavoritesDialog

	// Connection history
	connectionHistory *connection_history.Manager

	// Password dialog for missing passwords
	showPasswordDialog    bool
	passwordDialog        *components.PasswordDialog
	pendingConnectionInfo *models.ConnectionHistoryEntry
	pendingPasswordSave   *pendingPassword // Password to save after successful connection

	// Search input
	showSearch  bool
	searchInput *components.SearchInput

	// Query execution state
	executeCancelFn context.CancelFunc
	executeSpinner  spinner.Model

	// Cached styles for performance (avoid recreating on every render)
	cachedStyles *appStyles

	// Delegates for message handling (Phase 2 refactoring)
	delegates []delegates.Delegate
}

// appStyles holds pre-computed styles for App rendering
type appStyles struct {
	appName        lipgloss.Style
	connGreen      lipgloss.Style
	connGray       lipgloss.Style
	connText       lipgloss.Style
	topBarHelp     lipgloss.Style
	topBarHelpText lipgloss.Style
	keyStyle       lipgloss.Style
	dimStyle       lipgloss.Style
	separatorStyle lipgloss.Style
	filterStyle    lipgloss.Style
	vimStyle       lipgloss.Style
	overlayBg      lipgloss.Color
}

// initAppStyles initializes cached styles for App rendering
func (a *App) initAppStyles() {
	a.cachedStyles = &appStyles{
		appName: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#cba6f7")). // Mauve
			Background(lipgloss.Color("#313244")), // Surface0
		connGreen: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#a6e3a1")), // Green
		connGray: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6c7086")), // Overlay0
		connText: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#cdd6f4")), // Text
		topBarHelp: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#89b4fa")), // Blue
		topBarHelpText: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6c7086")), // Overlay0
		keyStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#89b4fa")). // Blue for keys
			Bold(true),
		dimStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6c7086")), // Overlay0 for descriptions
		separatorStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#45475a")), // Surface1 for separators
		filterStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#f9e2af")), // Yellow for filter
		vimStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#a6e3a1")). // Green for vim input
			Bold(true),
		overlayBg: lipgloss.Color("#555555"),
	}
}

// New creates a new App instance with config
func New(cfg *config.Config) *App {
	state := models.NewAppState()

	// Load theme
	themeName := "default"
	if cfg != nil && cfg.UI.Theme != "" {
		themeName = cfg.UI.Theme
	}
	th := theme.GetTheme(themeName)

	// Apply config to state
	if cfg != nil && cfg.UI.PanelWidthRatio > 0 && cfg.UI.PanelWidthRatio < 100 {
		state.LeftPanelWidth = cfg.UI.PanelWidthRatio
	}

	// Create empty tree root
	emptyRoot := models.NewTreeNode("root", models.TreeNodeTypeRoot, "Databases")
	emptyRoot.Expanded = true

	// Initialize command registry
	registry := commands.NewRegistry()
	for _, cmd := range commands.GetBuiltinCommands() {
		registry.Register(cmd)
	}

	// Initialize history store
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Printf("Warning: Could not get home directory: %v", err)
		homeDir = "."
	}
	configDir := filepath.Join(homeDir, ".config", "lazypg")
	_ = os.MkdirAll(configDir, 0755)

	historyPath := filepath.Join(configDir, "history.db")
	historyStore, err := history.NewStore(historyPath)
	if err != nil {
		log.Printf("Warning: Could not open history: %v", err)
	}

	// Initialize favorites manager
	favoritesManager, err := favorites.NewManager(configDir)
	if err != nil {
		log.Printf("Warning: Could not initialize favorites: %v", err)
	}

	// Initialize connection history manager
	connectionHistory, err := connection_history.NewManager(configDir)
	if err != nil {
		log.Printf("Warning: Could not initialize connection history: %v", err)
	}

	// Initialize filter builder
	filterBuilder := components.NewFilterBuilder(th)

	// Initialize JSONB viewer
	jsonbViewer := components.NewJSONBViewer(th)

	// Initialize table view (needed by structure view)
	tableView := components.NewTableView(th)

	// Initialize structure view with shared table view
	structureView := components.NewStructureView(th, tableView)

	// Initialize favorites dialog
	favoritesDialog := components.NewFavoritesDialog(th)

	// Initialize search input
	searchInput := components.NewSearchInput(th)

	// Initialize spinner for query execution
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(th.Info)

	app := &App{
		state:             state,
		config:            cfg,
		theme:             th,
		connectionManager: connection.NewManager(),
		discoverer:        discovery.NewDiscoverer(),
		connectionDialog:  components.NewConnectionDialog(th),
		errorOverlay:      components.NewErrorOverlay(th),
		treeView:          components.NewTreeView(emptyRoot, th),
		commandRegistry:   registry,
		commandPalette:    components.NewCommandPalette(th),
		sqlEditor:         components.NewSQLEditor(th),
		resultTabs:        components.NewResultTabs(th),
		historyStore:      historyStore,
		tableView:         tableView,
		showFilterBuilder: false,
		filterBuilder:     filterBuilder,
		activeFilter:      nil,
		showJSONBViewer:   false,
		jsonbViewer:       jsonbViewer,
		showStructureView: false,
		structureView:     structureView,
		currentTab:        0,
		showFavorites:     false,
		favoritesManager:  favoritesManager,
		favoritesDialog:   favoritesDialog,
		connectionHistory: connectionHistory,
		passwordDialog:    components.NewPasswordDialog(th),
		showSearch:        false,
		searchInput:       searchInput,
		executeSpinner:    s,
		leftPanel: components.Panel{
			Title:   "Explorer",
			Content: "Databases\n└─ (empty)",
			Style:   lipgloss.NewStyle().BorderForeground(th.BorderFocused),
		},
		rightPanel: components.Panel{
			Title:   "", // No title for right panel
			Content: "Select a database object to view",
			Style:   lipgloss.NewStyle().BorderForeground(th.Border),
		},
	}

	// Share spinner with TreeView
	app.treeView.Spinner = &app.executeSpinner

	// Set initial panel dimensions and styles
	app.updatePanelDimensions()
	app.updatePanelStyles()

	// Initialize cached styles for performance
	app.initAppStyles()

	// Initialize delegates for message handling
	app.delegates = []delegates.Delegate{
		delegates.NewConnectionDelegate(),
		delegates.NewTreeDelegate(),
		delegates.NewDataDelegate(),
		delegates.NewQueryDelegate(),
		delegates.NewDialogDelegate(),
	}

	return app
}

// Init implements tea.Model
func (a *App) Init() tea.Cmd {
	// Load connection history if available
	if a.connectionHistory != nil {
		history := a.connectionHistory.GetRecent(10) // Show up to 10 recent connections
		a.connectionDialog.SetHistoryEntries(history)
	}

	// If no active connection, automatically show connection dialog on startup
	if a.state.ActiveConnection == nil {
		a.showConnectionDialog = true
		return tea.Batch(
			a.triggerDiscovery(),
			a.connectionDialog.Init(), // Start cursor blinking
		)
	}
	return a.connectionDialog.Init() // Always init textinput cursors
}

// dispatchToDelegate sends a message to all delegates until one handles it.
// Returns true and a command if a delegate handled the message.
func (a *App) dispatchToDelegate(msg tea.Msg) (bool, tea.Cmd) {
	for _, d := range a.delegates {
		if handled, cmd := d.Update(msg, a); handled {
			return true, cmd
		}
	}
	return false, nil
}

// Update implements tea.Model
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// First, try to dispatch to delegates
	if handled, cmd := a.dispatchToDelegate(msg); handled {
		return a, cmd
	}

	switch msg := msg.(type) {
	case tea.MouseMsg:
		return a.handleMouseEvent(msg)

	case spinner.TickMsg:
		// Update spinner when there's a pending query, tree or table is loading, or connecting
		needsSpinner := a.resultTabs.HasPendingQuery() ||
			a.treeView.IsLoading ||
			a.treeView.LoadingNodeID != "" ||
			a.tableView.IsPaginating ||
			a.isConnecting ||
			a.isLoadingObjectDetails

		// Also check active tab's table view
		if activeTab := a.resultTabs.GetActiveTab(); activeTab != nil {
			if sv := activeTab.Structure; sv != nil {
				if tv := sv.GetTableView(); tv != nil && tv.IsLoading {
					needsSpinner = true
				}
			}
		}

		if needsSpinner {
			var cmd tea.Cmd
			a.executeSpinner, cmd = a.executeSpinner.Update(msg)
			return a, cmd
		}
		return a, nil

	case commands.ConnectCommandMsg:
		// Handle connect command from palette
		a.showConnectionDialog = true
		return a, a.triggerDiscovery()

	case commands.RefreshCommandMsg:
		// Handle refresh command
		if a.state.ActiveConnection != nil {
			return a, func() tea.Msg {
				return messages.LoadTreeMsg{}
			}
		}
		return a, nil

	case commands.QuickQueryCommandMsg:
		// Open SQL editor (expand if collapsed)
		if !a.sqlEditor.IsExpanded() {
			a.sqlEditor.Expand()
		}
		a.state.FocusArea = models.FocusSQLEditor
		a.updatePanelStyles()
		return a, nil

	case commands.QueryEditorCommandMsg:
		// External query editor - planned feature
		a.ShowError("Not Implemented", "Query editor is a future enhancement")
		return a, nil

	case commands.HistoryCommandMsg:
		// Query history browsing - planned feature
		a.ShowError("Not Implemented", "Query history browsing is planned for a future release")
		return a, nil

	case commands.FavoritesCommandMsg:
		// Open favorites dialog
		if a.favoritesManager != nil {
			a.favoritesDialog.SetFavorites(a.favoritesManager.GetAll())
		}
		a.showFavorites = true
		return a, nil

	case commands.ExportFavoritesCSVMsg:
		// Export favorites to CSV
		if a.favoritesManager == nil {
			a.ShowError("Export Not Available", "Favorites manager is not initialized.\n\nPlease restart the application.")
			return a, nil
		}

		path, err := a.favoritesManager.ExportToCSV()
		if err != nil {
			a.ShowError("Export Failed", fmt.Sprintf("Failed to export favorites to CSV:\n\n%v\n\nPlease check that you have write permissions and try again.", err))
			return a, nil
		}

		// Show success notification
		a.ShowError("Export Complete", fmt.Sprintf("Successfully exported favorites to:\n\n%s\n\nYou can now import this file or share it with others.", path))
		return a, nil

	case commands.ExportFavoritesJSONMsg:
		// Export favorites to JSON
		if a.favoritesManager == nil {
			a.ShowError("Export Not Available", "Favorites manager is not initialized.\n\nPlease restart the application.")
			return a, nil
		}

		path, err := a.favoritesManager.ExportToJSON()
		if err != nil {
			a.ShowError("Export Failed", fmt.Sprintf("Failed to export favorites to JSON:\n\n%v\n\nPlease check that you have write permissions and try again.", err))
			return a, nil
		}

		// Show success notification
		a.ShowError("Export Complete", fmt.Sprintf("Successfully exported favorites to:\n\n%s\n\nYou can now import this file or share it with others.", path))
		return a, nil

	case components.OpenExternalEditorMsg:
		// Open external editor
		return a, a.openExternalEditor(msg.Content)

	case components.ExternalEditorResultMsg:
		if msg.Error != nil {
			a.ShowError("Editor Error", msg.Error.Error())
			return a, nil
		}
		a.sqlEditor.SetContent(msg.Content)
		return a, nil

	case components.ExecuteQueryMsg:
		// Handle query execution from SQL editor
		if a.state.ActiveConnection == nil {
			a.ShowError("No Connection", "Please connect to a database first")
			return a, nil
		}

		// Create pending tab immediately
		a.resultTabs.StartPendingQuery(msg.SQL)

		// Immediately switch focus to data panel and collapse editor
		a.sqlEditor.Collapse()
		a.state.FocusArea = models.FocusDataPanel
		a.updatePanelStyles()

		// Create cancellable context for query execution
		ctx, cancel := context.WithCancel(context.Background())
		a.executeCancelFn = cancel

		// Execute query asynchronously and start spinner
		return a, tea.Batch(
			a.executeSpinner.Tick,
			func() tea.Msg {
				conn, err := a.connectionManager.GetActive()
				if err != nil {
					return messages.QueryResultMsg{
						SQL: msg.SQL,
						Result: models.QueryResult{
							Error: fmt.Errorf("failed to get connection: %w", err),
						},
					}
				}

				result := query.Execute(ctx, conn.Pool.GetPool(), msg.SQL)
				return messages.QueryResultMsg{
					SQL:    msg.SQL,
					Result: result,
				}
			},
		)

	case messages.QueryResultMsg:
		// Clear execution state
		a.executeCancelFn = nil

		// Record query to history
		if a.historyStore != nil {
			connName := ""
			dbName := ""
			if a.state.ActiveConnection != nil {
				connName = a.state.ActiveConnection.Config.Name
				dbName = a.state.ActiveConnection.Config.Database
			}

			entry := history.HistoryEntry{
				ConnectionName: connName,
				DatabaseName:   dbName,
				Query:          msg.SQL,
				Duration:       msg.Result.Duration,
				RowsAffected:   msg.Result.RowsAffected,
				Success:        msg.Result.Error == nil,
			}

			if msg.Result.Error != nil {
				entry.ErrorMessage = msg.Result.Error.Error()
			}

			// Record to history (ignore errors to not interrupt user flow)
			_ = a.historyStore.Add(entry)
		}

		// Handle query result
		if msg.Result.Error != nil {
			// Check if it was cancelled (context cancelled error)
			if msg.Result.Error.Error() == "context canceled" {
				// Already handled by CancelPendingQuery, just return
				return a, nil
			}
			// Show error and remove pending tab
			a.resultTabs.CancelPendingQuery()
			a.ShowError("Query Error", msg.Result.Error.Error())
			return a, nil
		}

		// Complete the pending query with results
		a.resultTabs.CompletePendingQuery(msg.SQL, msg.Result)

		return a, nil

	case components.ApplyFilterMsg:
		// Apply the filter and reload table data
		a.showFilterBuilder = false
		a.activeFilter = &msg.Filter

		// Reload table with filter
		if a.state.TreeSelected != nil && a.state.TreeSelected.Type == models.TreeNodeTypeTable {
			return a, a.loadTableDataWithFilter(*a.activeFilter)
		}
		return a, nil

	case components.CloseFilterBuilderMsg:
		a.showFilterBuilder = false
		return a, nil

	case components.CloseJSONBViewerMsg:
		a.showJSONBViewer = false
		return a, nil

	case components.CloseErrorOverlayMsg:
		a.showError = false
		return a, nil

	case components.PasswordSubmitMsg:
		// User submitted password from password dialog
		a.showPasswordDialog = false
		if a.pendingConnectionInfo != nil {
			// Create connection config with the entered password
			config := a.pendingConnectionInfo.ToConnectionConfig()
			config.Password = msg.Password

			// Store password info to save after successful connection
			// This avoids: 1) saving wrong passwords, 2) double keychain prompts
			a.pendingPasswordSave = &pendingPassword{
				Host:     config.Host,
				Port:     config.Port,
				Database: config.Database,
				User:     config.User,
				Password: msg.Password,
			}

			a.pendingConnectionInfo = nil
			return a.performConnection(config)
		}
		return a, nil

	case components.PasswordCancelMsg:
		// User cancelled password dialog
		a.showPasswordDialog = false
		a.pendingConnectionInfo = nil
		// Re-show connection dialog
		a.showConnectionDialog = true
		return a, nil

	case components.CloseCommandPaletteMsg:
		a.showCommandPalette = false
		return a, nil

	case components.ExecuteFavoriteMsg:
		// Execute favorite query
		if a.state.ActiveConnection == nil {
			a.ShowError("No Database Connection", "Please connect to a database before executing queries.\n\nPress 'c' to open the connection dialog.")
			return a, nil
		}

		// Record usage
		if a.favoritesManager != nil {
			if err := a.favoritesManager.RecordUsage(msg.Favorite.ID); err != nil {
				// Log error but don't block execution
				log.Printf("Warning: Failed to record favorite usage: %v", err)
			}
		}

		// Execute query asynchronously
		a.showFavorites = false
		return a, func() tea.Msg {
			conn, err := a.connectionManager.GetActive()
			if err != nil {
				return messages.QueryResultMsg{
					SQL: msg.Favorite.Query,
					Result: models.QueryResult{
						Error: fmt.Errorf("connection error: %w", err),
					},
				}
			}

			result := query.Execute(context.Background(), conn.Pool.GetPool(), msg.Favorite.Query)
			return messages.QueryResultMsg{
				SQL:    msg.Favorite.Query,
				Result: result,
			}
		}

	case components.CloseFavoritesDialogMsg:
		a.showFavorites = false
		return a, nil

	case components.SearchInputMsg:
		// Handle search request from search input
		a.showSearch = false
		if msg.Query == "" {
			return a, nil
		}

		// Get the active table view (Result Tabs or main TableView)
		activeTable := a.getActiveTableView()

		if msg.Mode == "local" {
			// Local search - search only loaded data
			if activeTable != nil {
				activeTable.SearchLocal(msg.Query)
			}
		} else {
			// For Result Tabs, always use local search (data is already loaded)
			if a.resultTabs.HasTabs() {
				if activeTable != nil {
					activeTable.SearchLocal(msg.Query)
				}
				return a, nil
			}

			// Table search - query the database (only for table browser)
			if a.state.ActiveConnection == nil {
				a.ShowError("No Connection", "Please connect to a database first")
				return a, nil
			}

			if a.currentTable == "" {
				a.ShowError("No Table", "Please select a table first")
				return a, nil
			}

			// Execute table search
			return a, a.searchTable(msg.Query)
		}
		return a, nil

	case components.CloseSearchMsg:
		a.showSearch = false
		a.searchInput.Reset()
		return a, nil

	case SearchTableResultMsg:
		if msg.Err != nil {
			a.ShowError("Search Error", msg.Err.Error())
			return a, nil
		}

		if msg.Data == nil || len(msg.Data.Rows) == 0 {
			a.ShowError("No Results", fmt.Sprintf("No matches found for '%s'", msg.Query))
			return a, nil
		}

		// Replace table data with search results
		a.tableView.SetData(msg.Data.Columns, msg.Data.Rows, int(msg.Data.TotalRows))

		// Build matches from all cells that contain the query
		queryLower := strings.ToLower(msg.Query)
		var matches []components.MatchPos
		for rowIdx, row := range msg.Data.Rows {
			for colIdx, cell := range row {
				if strings.Contains(strings.ToLower(cell), queryLower) {
					matches = append(matches, components.MatchPos{Row: rowIdx, Col: colIdx})
				}
			}
		}

		a.tableView.SetSearchResults(msg.Query, matches)
		return a, nil

	case components.AddFavoriteMsg:
		if a.favoritesManager != nil {
			conn := ""
			if a.state.ActiveConnection != nil {
				conn = a.state.ActiveConnection.Config.Name
			}
			db := a.state.CurrentDatabase
			_, err := a.favoritesManager.Add(msg.Name, msg.Description, msg.Query, conn, db, msg.Tags)
			if err != nil {
				a.ShowError("Cannot Add Favorite", fmt.Sprintf("Failed to add favorite:\n\n%v\n\nPlease check your input and try again.", err))
			} else {
				// Refresh the dialog
				a.favoritesDialog.SetFavorites(a.favoritesManager.GetAll())
			}
		} else {
			a.ShowError("Favorites Not Available", "Favorites manager is not initialized.\n\nPlease restart the application.")
		}
		return a, nil

	case components.EditFavoriteMsg:
		if a.favoritesManager != nil {
			err := a.favoritesManager.Update(msg.FavoriteID, msg.Name, msg.Description, msg.Query, msg.Tags)
			if err != nil {
				a.ShowError("Cannot Update Favorite", fmt.Sprintf("Failed to update favorite:\n\n%v\n\nPlease check your input and try again.", err))
			} else {
				// Refresh the dialog
				a.favoritesDialog.SetFavorites(a.favoritesManager.GetAll())
			}
		} else {
			a.ShowError("Favorites Not Available", "Favorites manager is not initialized.\n\nPlease restart the application.")
		}
		return a, nil

	case components.DeleteFavoriteMsg:
		if a.favoritesManager != nil {
			err := a.favoritesManager.Delete(msg.FavoriteID)
			if err != nil {
				a.ShowError("Cannot Delete Favorite", fmt.Sprintf("Failed to delete favorite:\n\n%v\n\nThe favorite may have already been deleted.", err))
			} else {
				// Refresh the dialog
				a.favoritesDialog.SetFavorites(a.favoritesManager.GetAll())
				// Adjust selection if needed
				if a.favoritesDialog != nil {
					favorites := a.favoritesManager.GetAll()
					if len(favorites) > 0 {
						// Keep selection valid
						a.favoritesDialog.SetFavorites(favorites)
					}
				}
			}
		} else {
			a.ShowError("Favorites Not Available", "Favorites manager is not initialized.\n\nPlease restart the application.")
		}
		return a, nil

	case messages.ErrorMsg:
		// Handle error messages
		a.ShowError(msg.Title, msg.Message)
		return a, nil

	case tea.KeyMsg:
		// Handle error overlay dismissal first if visible
		if a.showError {
			key := msg.String()
			if key == "esc" || key == "enter" {
				a.DismissError()
				return a, nil
			}
			// Allow quit keys to pass through even when error is showing
			if key == "q" || key == "ctrl+c" {
				return a, tea.Quit
			}
			// Consume all other keys when error is showing
			return a, nil
		}

		// Handle connection dialog if visible
		if a.showConnectionDialog {
			return a.handleConnectionDialog(msg)
		}

		// Handle password dialog if visible
		if a.showPasswordDialog {
			var cmd tea.Cmd
			a.passwordDialog, cmd = a.passwordDialog.Update(msg)
			return a, cmd
		}

		// Handle command palette if visible
		if a.showCommandPalette {
			return a.handleCommandPalette(msg)
		}

		// Handle filter builder input
		if a.showFilterBuilder {
			return a.handleFilterBuilder(msg)
		}

		// Handle JSONB viewer input
		if a.showJSONBViewer {
			return a.handleJSONBViewer(msg)
		}

		// Handle favorites dialog if visible
		if a.showFavorites {
			return a.handleFavoritesDialog(msg)
		}

		// Handle search input if visible
		if a.showSearch {
			return a.handleSearchInput(msg)
		}

		// Handle TreeView search mode - route keys to TreeView
		// This must come before global key handlers to capture typing during search
		// and to allow Esc to clear filter in SearchFilterActive mode
		if a.treeView != nil && a.state.FocusArea == models.FocusTreeView {
			// In SearchInputting mode, route ALL keys to TreeView
			if a.treeView.IsSearchInputting() {
				var cmd tea.Cmd
				a.treeView, cmd = a.treeView.Update(msg)
				return a, cmd
			}
			// In SearchFilterActive mode, route Esc and / to TreeView
			if a.treeView.IsSearchActive() && (msg.String() == "esc" || msg.String() == "/") {
				var cmd tea.Cmd
				a.treeView, cmd = a.treeView.Update(msg)
				return a, cmd
			}
		}

		// Handle code editor input if visible and DataPanel is focused
		if a.state.FocusArea == models.FocusDataPanel {
			// Keys that the code editor handles in read-only mode
			codeEditorReadOnlyKeys := map[string]bool{
				"j": true, "k": true, "up": true, "down": true, // Scrolling
				"g": true, "G": true, // Scroll to top/bottom
				"ctrl+d": true, "ctrl+u": true, // Page scroll
				"y":   true, // Copy
				"e":   true, // Enter edit mode
				"esc": true, // Close (q is reserved for quitting app)
			}
			key := msg.String()

			// Check for tab-based code editor first
			if activeTab := a.resultTabs.GetActiveTab(); activeTab != nil && activeTab.Type == components.TabTypeCodeEditor && activeTab.CodeEditor != nil {
				ce := activeTab.CodeEditor
				// In edit mode, route most keys to editor; in read-only mode, only route specific keys
				if !ce.ReadOnly || codeEditorReadOnlyKeys[key] {
					_, cmd := ce.Update(msg)
					return a, cmd
				}
			}
			// Legacy: handle global code editor
			if a.showCodeEditor && a.codeEditor != nil {
				if !a.codeEditor.ReadOnly || codeEditorReadOnlyKeys[key] {
					_, cmd := a.codeEditor.Update(msg)
					return a, cmd
				}
			}
		}

		// If SQL editor is focused, handle input
		if a.isSQLEditorFocused() {
			// Handle escape to unfocus
			if msg.String() == "esc" {
				if a.sqlEditor.IsExpanded() {
					a.sqlEditor.Collapse()
				}
				a.state.FocusArea = models.FocusDataPanel
				a.updatePanelStyles()
				return a, nil
			}

			// Handle ctrl+e to toggle expand/collapse
			if msg.String() == "ctrl+e" {
				if a.sqlEditor.IsExpanded() {
					a.sqlEditor.Collapse()
					a.state.FocusArea = models.FocusDataPanel
					a.updatePanelStyles()
				} else {
					a.sqlEditor.Expand()
				}
				return a, nil
			}

			// Tab is handled in the unified Tab case below for focus cycling
			if msg.String() == "tab" || msg.String() == "shift+tab" || msg.String() == "backtab" {
				// Let Tab fall through to the switch case for focus cycling
			} else if a.sqlEditor.IsExpanded() {
				// Route other keys to SQL editor when expanded
				_, cmd := a.sqlEditor.Update(msg)
				return a, cmd
			} else if isPrintableInput(msg.String()) {
				// Auto-expand and route printable input when collapsed but focused
				a.sqlEditor.Expand()
				_, cmd := a.sqlEditor.Update(msg)
				return a, cmd
			}
		}

		switch msg.String() {
		// Ctrl+E to toggle SQL editor expand/collapse
		case "ctrl+e":
			a.sqlEditor.Toggle()
			if a.sqlEditor.IsExpanded() {
				a.state.FocusArea = models.FocusSQLEditor
				a.updatePanelStyles()
			}
			return a, nil

		// Ctrl+Shift+Up to increase editor height preset
		case "ctrl+shift+up":
			if a.isSQLEditorFocused() && a.sqlEditor.IsExpanded() {
				a.sqlEditor.IncreaseHeight()
			}
			return a, nil

		// Ctrl+Shift+Down to decrease editor height preset
		case "ctrl+shift+down":
			if a.isSQLEditorFocused() && a.sqlEditor.IsExpanded() {
				a.sqlEditor.DecreaseHeight()
			}
			return a, nil

		// Tab switching with [ and ] (like lazygit)
		case "[":
			// Previous result tab (when not in SQL editor)
			if a.resultTabs.HasTabs() && !a.isSQLEditorFocused() {
				a.resultTabs.PrevTab()
				// Sync SQL editor content with the active tab's SQL
				if sql := a.resultTabs.GetActiveSQL(); sql != "" {
					a.sqlEditor.SetContent(sql)
				}
				return a, nil
			}
			// Structure view tab switching (existing behavior)
			if a.currentTab > 0 {
				a.currentTab--
				a.structureView.SwitchTab(a.currentTab)
			}
			return a, nil

		case "]":
			// Next result tab (when not in SQL editor)
			if a.resultTabs.HasTabs() && !a.isSQLEditorFocused() {
				a.resultTabs.NextTab()
				// Sync SQL editor content with the active tab's SQL
				if sql := a.resultTabs.GetActiveSQL(); sql != "" {
					a.sqlEditor.SetContent(sql)
				}
				return a, nil
			}
			// Structure view tab switching (existing behavior)
			if a.currentTab < 3 {
				a.currentTab++
				a.structureView.SwitchTab(a.currentTab)
			}
			return a, nil

		case "1", "2", "3", "4":
			// Switch structure view sub-tabs when active tab is TableData
			if !a.isSQLEditorFocused() {
				activeTab := a.resultTabs.GetActiveTab()
				if activeTab != nil && activeTab.Type == components.TabTypeTableData && activeTab.Structure != nil {
					tabIndex := int(msg.String()[0] - '1') // Convert "1"-"4" to 0-3
					activeTab.Structure.SwitchTab(tabIndex)
					// Trigger metadata loading for Columns/Constraints/Indexes tabs
					if tabIndex >= 1 {
						if cmd := a.triggerStructureMetadataLoad(activeTab.Structure, activeTab.ObjectID); cmd != nil {
							return a, cmd
						}
					}
					return a, nil
				}
				// Legacy: also handle global structure view
				if a.currentTable != "" {
					tabIndex := int(msg.String()[0] - '1')
					a.currentTab = tabIndex
					a.structureView.SwitchTab(tabIndex)
					return a, nil
				}
			}

		case "ctrl+p":
			// Open SQL editor (expand if collapsed)
			if !a.sqlEditor.IsExpanded() {
				a.sqlEditor.Expand()
			}
			a.state.FocusArea = models.FocusSQLEditor
			a.updatePanelStyles()
			return a, nil
		case "ctrl+k":
			// Open unified command palette
			a.commandPalette.Reset()
			a.commandPalette.SetCommands(a.getBuiltinCommands())
			a.commandPalette.SetTables(a.getTableCommands())
			a.commandPalette.SetHistory(a.getHistoryCommands())
			a.showCommandPalette = true
			return a, nil
		case "ctrl+b":
			// Open favorites dialog
			if a.favoritesManager != nil {
				a.favoritesDialog.SetFavorites(a.favoritesManager.GetAll())
			}
			a.showFavorites = true
			return a, nil
		case "q", "ctrl+c":
			// Don't quit if in help mode, exit help instead
			if a.state.ViewMode == models.HelpMode {
				a.state.ViewMode = models.NormalMode
				return a, nil
			}
			return a, tea.Quit
		case "?":
			// Toggle help
			if a.state.ViewMode == models.HelpMode {
				a.state.ViewMode = models.NormalMode
			} else {
				a.state.ViewMode = models.HelpMode
			}
		case "esc":
			// Cancel executing query first
			if a.resultTabs.HasPendingQuery() && a.executeCancelFn != nil {
				a.executeCancelFn()
				a.executeCancelFn = nil
				a.resultTabs.CancelPendingQuery()
				return a, nil
			}
			// Exit help mode
			if a.state.ViewMode == models.HelpMode {
				a.state.ViewMode = models.NormalMode
			}
		case "c":
			// Open connection dialog and trigger discovery
			a.showConnectionDialog = true
			return a, a.triggerDiscovery()
		case "f":
			// Open filter builder if on table view
			if a.state.FocusArea == models.FocusDataPanel && a.state.TreeSelected != nil {
				if a.state.TreeSelected.Type == models.TreeNodeTypeTable {
					// Get column info from current table
					columns := a.getTableColumns()
					// Extract schema and table names
					schemaNode := a.state.TreeSelected.Parent
					if schemaNode != nil {
						a.filterBuilder.SetColumns(columns)
						a.filterBuilder.SetTable(schemaNode.Label, a.state.TreeSelected.Label)
						a.showFilterBuilder = true
					}
				}
			}
			return a, nil
		case "ctrl+f":
			// Quick filter from current cell
			if a.state.FocusArea == models.FocusDataPanel && a.tableView != nil {
				selectedRow, selectedCol := a.tableView.GetSelectedCell()
				if selectedRow >= 0 && selectedCol >= 0 && selectedCol < len(a.tableView.Columns) {
					// Get column name and value
					columnName := a.tableView.Columns[selectedCol]
					cellValue := a.tableView.Rows[selectedRow][selectedCol]

					// Create quick filter
					columns := a.getTableColumns()
					var columnInfo models.ColumnInfo
					for _, col := range columns {
						if col.Name == columnName {
							columnInfo = col
							break
						}
					}

					// Create filter with single condition
					quickFilter := models.Filter{
						Schema:    a.state.TreeSelected.Parent.Label,
						TableName: a.state.TreeSelected.Label,
						RootGroup: models.FilterGroup{
							Conditions: []models.FilterCondition{
								{
									Column:   columnName,
									Operator: models.OpEqual,
									Value:    cellValue,
									Type:     columnInfo.DataType,
								},
							},
							Logic: "AND",
						},
					}

					a.activeFilter = &quickFilter
					return a, a.loadTableDataWithFilter(quickFilter)
				}
			}
			return a, nil
		case "ctrl+r":
			// Refresh current table data (preserve sort and filter)
			if a.currentTable != "" {
				parts := strings.Split(a.currentTable, ".")
				if len(parts) == 2 {
					msg := messages.LoadTableDataMsg{
						Schema:     parts[0],
						Table:      parts[1],
						Limit:      100,
						Offset:     0,
						SortColumn: a.tableView.GetSortColumn(),
						SortDir:    a.tableView.GetSortDirection(),
						NullsFirst: a.tableView.GetNullsFirst(),
					}
					if a.activeFilter != nil {
						return a, a.loadTableDataWithFilter(*a.activeFilter)
					}
					return a, a.loadTableData(msg)
				}
			}
			return a, nil
		case "ctrl+x":
			// Clear filter and reload
			if a.activeFilter != nil && a.state.TreeSelected != nil {
				a.activeFilter = nil
				schemaNode := a.state.TreeSelected.Parent
				if schemaNode != nil {
					return a, a.loadTableData(messages.LoadTableDataMsg{
						Schema:     schemaNode.Label,
						Table:      a.state.TreeSelected.Label,
						Limit:      100,
						Offset:     0,
						SortColumn: a.tableView.GetSortColumn(),
						SortDir:    a.tableView.GetSortDirection(),
						NullsFirst: a.tableView.GetNullsFirst(),
					})
				}
			}
			return a, nil
		case "y":
			// Copy functionality in structure view (copy name)
			if a.currentTab > 0 {
				statusMsg := a.structureView.CopyCurrentName()
				if statusMsg != "" {
					// Show status message (log.Println is acceptable per plan)
					log.Println(statusMsg)
				}
				return a, nil
			}
		case "Y":
			// Copy functionality in structure view (copy definition)
			if a.currentTab > 0 {
				statusMsg := a.structureView.CopyCurrentDefinition()
				if statusMsg != "" {
					log.Println(statusMsg)
				}
				return a, nil
			}
		case "tab":
			// Only handle tab in normal mode
			if a.state.ViewMode == models.NormalMode {
				if a.isEditingText() {
					// In edit mode, Tab inserts indentation
					if a.showCodeEditor && a.codeEditor != nil && !a.codeEditor.ReadOnly {
						a.codeEditor.Update(msg)
					} else if a.isSQLEditorFocused() && a.sqlEditor.IsExpanded() {
						a.sqlEditor.Update(msg)
					}
				} else {
					// Normal mode: cycle focus TreeView -> DataPanel -> SQLEditor -> TreeView
					a.nextFocus()
				}
				return a, nil
			}
		case "shift+tab", "backtab":
			// Reverse focus cycle (only when not editing)
			if a.state.ViewMode == models.NormalMode && !a.isEditingText() {
				a.prevFocus()
				return a, nil
			}
		default:
			// Handle tree navigation when TreeView is focused
			if a.state.FocusArea == models.FocusTreeView && a.state.ViewMode == models.NormalMode {
				var cmd tea.Cmd
				a.treeView, cmd = a.treeView.Update(msg)
				return a, cmd
			}

			// Handle table navigation when DataPanel is focused
			if a.state.FocusArea == models.FocusDataPanel && a.state.ViewMode == models.NormalMode {
				// Get the active table view (Result Tabs, Structure View, or main TableView)
				activeTable := a.getActiveTableView()

				// Handle preview pane scrolling (when visible)
				if activeTable != nil && activeTable.PreviewPane != nil && activeTable.PreviewPane.Visible {
					switch msg.String() {
					case "K":
						activeTable.PreviewPane.ScrollUp()
						return a, nil
					case "J":
						activeTable.PreviewPane.ScrollDown()
						return a, nil
					}
				}

				// Handle preview pane toggle
				if msg.String() == "p" {
					if activeTable != nil {
						activeTable.TogglePreviewPane()
					}
					return a, nil
				}

				// Handle yank: y = copy current cell, Y = copy preview pane content
				if msg.String() == "y" {
					if activeTable != nil {
						row, col := activeTable.GetSelectedCell()
						if row >= 0 && col >= 0 && row < len(activeTable.Rows) && col < len(activeTable.Rows[row]) {
							cellContent := activeTable.Rows[row][col]
							if err := clipboard.WriteAll(cellContent); err == nil {
								log.Println("Copied cell content to clipboard")
							}
						}
					}
					return a, nil
				}
				if msg.String() == "Y" {
					if activeTable != nil && activeTable.PreviewPane != nil && activeTable.PreviewPane.Visible {
						if err := activeTable.PreviewPane.CopyContent(); err == nil {
							log.Println("Copied preview content to clipboard")
						}
					}
					return a, nil
				}

				// Handle pin toggle (Shift+8 = *)
				if msg.String() == "*" {
					if activeTable != nil {
						if err := activeTable.TogglePin(); err != nil {
							log.Printf("Pin error: %v", err)
						}
					}
					return a, nil
				}

				// Handle jump to pinned rows (cycle through)
				if msg.String() == "'" {
					if activeTable != nil {
						activeTable.JumpToNextPinnedRow()
					}
					return a, nil
				}

				// If structure view is active (not on Data tab) and no Result Tabs, route to structure view
				if a.currentTab > 0 && !a.resultTabs.HasTabs() {
					a.structureView.Update(msg)
					return a, nil
				}

				// Skip navigation handling if no active table
				if activeTable == nil {
					return a, nil
				}

				// Toggle relative line numbers
				if msg.String() == "ctrl+n" {
					activeTable.ToggleRelativeNumbers()
					return a, nil
				}

				// Handle Vim motion (number prefixes, g, G, etc.)
				// This must come before individual key handling
				if activeTable.HandleVimMotion(msg.String()) {
					if cmd := a.checkLazyLoad(); cmd != nil {
						return a, cmd
					}
					return a, nil
				}

				switch msg.String() {
				case "up":
					activeTable.MoveSelection(-1)
					return a, nil
				case "down":
					activeTable.MoveSelection(1)
					if cmd := a.checkLazyLoad(); cmd != nil {
						return a, cmd
					}
					return a, nil
				case "left", "h":
					activeTable.MoveSelectionHorizontal(-1)
					return a, nil
				case "right", "l":
					activeTable.MoveSelectionHorizontal(1)
					return a, nil
				case "H":
					// Jump scroll left (half screen)
					activeTable.JumpScrollHorizontal(-1)
					return a, nil
				case "L":
					// Jump scroll right (half screen)
					activeTable.JumpScrollHorizontal(1)
					return a, nil
				case "0":
					// Jump to first column
					activeTable.JumpToFirstColumn()
					return a, nil
				case "$":
					// Jump to last column
					activeTable.JumpToLastColumn()
					return a, nil
				case "ctrl+u":
					activeTable.PageUp()
					return a, nil
				case "ctrl+d":
					activeTable.PageDown()
					if cmd := a.checkLazyLoad(); cmd != nil {
						return a, cmd
					}
					return a, nil
				case "s":
					// Sort by current column (only for main table browsing, not result tabs)
					if !a.resultTabs.HasTabs() {
						activeTable.ToggleSort()
						// Reload data with new sort
						if a.currentTable != "" {
							parts := strings.Split(a.currentTable, ".")
							if len(parts) == 2 {
								return a, func() tea.Msg {
									return messages.LoadTableDataMsg{
										Schema:     parts[0],
										Table:      parts[1],
										Offset:     0,
										Limit:      100,
										SortColumn: activeTable.GetSortColumn(),
										SortDir:    activeTable.GetSortDirection(),
										NullsFirst: activeTable.GetNullsFirst(),
									}
								}
							}
						}
					}
					return a, nil
				case "S":
					// Toggle NULLS FIRST/LAST (only for main table browsing)
					if !a.resultTabs.HasTabs() && activeTable.SortColumn >= 0 {
						activeTable.ToggleNullsFirst()
						// Reload data
						if a.currentTable != "" {
							parts := strings.Split(a.currentTable, ".")
							if len(parts) == 2 {
								return a, func() tea.Msg {
									return messages.LoadTableDataMsg{
										Schema:     parts[0],
										Table:      parts[1],
										Offset:     0,
										Limit:      100,
										SortColumn: activeTable.GetSortColumn(),
										SortDir:    activeTable.GetSortDirection(),
										NullsFirst: activeTable.GetNullsFirst(),
									}
								}
							}
						}
					}
					return a, nil
				case "r":
					// Reverse sort direction (only for main table browsing)
					if !a.resultTabs.HasTabs() && activeTable.ReverseSortDirection() {
						// Reload data with reversed sort
						if a.currentTable != "" {
							parts := strings.Split(a.currentTable, ".")
							if len(parts) == 2 {
								return a, func() tea.Msg {
									return messages.LoadTableDataMsg{
										Schema:     parts[0],
										Table:      parts[1],
										Offset:     0,
										Limit:      100,
										SortColumn: activeTable.GetSortColumn(),
										SortDir:    activeTable.GetSortDirection(),
										NullsFirst: activeTable.GetNullsFirst(),
									}
								}
							}
						}
					}
					return a, nil
				case "v":
					// Open JSONB viewer if cell contains JSONB
					selectedRow, selectedCol := activeTable.GetSelectedCell()
					if selectedRow >= 0 && selectedCol >= 0 && selectedRow < len(activeTable.Rows) && selectedCol < len(activeTable.Columns) {
						cellValue := activeTable.Rows[selectedRow][selectedCol]
						if jsonb.IsJSONB(cellValue) {
							// Set viewer dimensions based on terminal size (with max width limit)
							viewerWidth := a.state.Width * 2 / 3
							if viewerWidth > 100 {
								viewerWidth = 100
							}
							a.jsonbViewer.Width = viewerWidth
							a.jsonbViewer.Height = a.state.Height * 3 / 4
							if err := a.jsonbViewer.SetValue(cellValue); err == nil {
								a.showJSONBViewer = true
							}
						}
					}
					return a, nil
				case "/":
					// Open search input
					a.searchInput.Reset()
					a.searchInput.Width = a.rightPanel.Width - 4
					a.showSearch = true
					return a, nil
				case "n":
					// Next search match
					if activeTable.SearchActive {
						activeTable.NextMatch()
					}
					return a, nil
				case "N":
					// Previous search match
					if activeTable.SearchActive {
						activeTable.PrevMatch()
					}
					return a, nil
				case "enter", " ":
					// Consume enter/space in table view (no action needed for now)
					// This prevents the key from propagating to tree view
					return a, nil
				}
			}
		}
	case messages.DiscoveryCompleteMsg:
		// Update connection dialog with discovered instances
		a.connectionDialog.SetDiscoveredInstances(msg.Instances)
		return a, nil

	case messages.ConnectionStartMsg:
		// Start async connection
		a.isConnecting = true
		a.connectingStart = time.Now()
		a.connectingConfig = msg.Config
		return a, tea.Batch(a.connectAsync(msg.Config), a.executeSpinner.Tick)

	case messages.ConnectionResultMsg:
		// Ignore result if user already cancelled
		if !a.isConnecting {
			return a, nil
		}
		a.isConnecting = false
		if msg.Err != nil {
			// Connection failed - clear pending password (don't save wrong password)
			a.pendingPasswordSave = nil
			a.ShowError("Connection Failed", fmt.Sprintf("Could not connect to %s:%d\n\nError: %v",
				msg.Config.Host, msg.Config.Port, msg.Err))
			return a, nil
		}

		// Connection succeeded - save manually entered password to keyring
		if a.pendingPasswordSave != nil && a.connectionHistory != nil {
			p := a.pendingPasswordSave
			if err := a.connectionHistory.SavePassword(p.Host, p.Port, p.Database, p.User, p.Password); err != nil {
				log.Printf("Warning: Failed to save password: %v", err)
			}
			a.pendingPasswordSave = nil
		}

		// Update active connection in state
		conn, err := a.connectionManager.GetActive()
		if err == nil && conn != nil {
			a.state.ActiveConnection = &models.Connection{
				ID:          msg.ConnID,
				Config:      msg.Config,
				Connected:   conn.Connected,
				ConnectedAt: conn.ConnectedAt,
				LastPing:    conn.LastPing,
				Error:       conn.Error,
			}
		}

		// Save to connection history (ignore errors)
		if a.connectionHistory != nil {
			result, err := a.connectionHistory.Add(msg.Config)
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

		// Trigger tree loading
		a.showConnectionDialog = false
		return a, func() tea.Msg {
			return messages.LoadTreeMsg{}
		}

	case messages.LoadTreeMsg:
		a.treeView.IsLoading = true
		a.treeView.LoadingStart = time.Now()
		a.treeView.Root = nil // Clear root to show loading state
		return a, tea.Batch(a.loadTree, a.executeSpinner.Tick)

	case messages.TreeLoadedMsg:
		a.treeView.IsLoading = false
		a.treeView.LoadingNodeID = ""
		if msg.Err != nil {
			a.ShowError("Database Error", fmt.Sprintf("Failed to load database structure:\n\n%v", msg.Err))
			return a, nil
		}
		// Update tree view with loaded data
		a.treeView.Root = msg.Root

		// Auto-expand: Root -> Database -> only "public" schema (skip extensions)
		if msg.Root != nil {
			msg.Root.Expanded = true
			for _, dbNode := range msg.Root.Children {
				dbNode.Expanded = true
				// Only expand "public" schema, skip extensions group
				for _, child := range dbNode.Children {
					if child.Type == models.TreeNodeTypeSchema && child.Label == "public" {
						child.Expanded = true
					}
					// Extensions group and other schemas remain collapsed
				}
			}
		}
		return a, nil

	case messages.LoadNodeChildrenMsg:
		a.treeView.LoadingNodeID = msg.NodeID
		return a, tea.Batch(a.loadNodeChildren(msg.NodeID), a.executeSpinner.Tick)

	case messages.NodeChildrenLoadedMsg:
		a.treeView.LoadingNodeID = ""
		if msg.Err != nil {
			a.ShowError("Load Error", fmt.Sprintf("Failed to load children:\n\n%v", msg.Err))
			return a, nil
		}
		// Find the node and add children
		node := a.treeView.Root.FindByID(msg.NodeID)
		if node != nil {
			for _, child := range msg.Children {
				child.Parent = node
				node.AddChild(child)
			}
			node.Loaded = true
			node.Expanded = true
		}
		return a, nil

	case components.TreeNodeExpandedMsg:
		// Check if this node needs lazy loading
		if msg.Expanded && msg.Node != nil && !msg.Node.Loaded && len(msg.Node.Children) == 0 {
			// Trigger lazy load
			return a, func() tea.Msg {
				return messages.LoadNodeChildrenMsg{NodeID: msg.Node.ID}
			}
		}
		return a, nil

	case components.TreeNodeSelectedMsg:
		// Handle selection based on node type
		if msg.Node == nil {
			return a, nil
		}

		switch msg.Node.Type {
		case models.TreeNodeTypeTable, models.TreeNodeTypeView, models.TreeNodeTypeMaterializedView:
			// Get schema name by traversing up the tree
			var schemaName string
			current := msg.Node.Parent
			for current != nil {
				if current.Type == models.TreeNodeTypeSchema {
					schemaName = strings.Split(current.Label, " ")[0]
					break
				}
				current = current.Parent
			}

			if schemaName == "" {
				return a, nil
			}

			// Store selected node
			a.state.TreeSelected = msg.Node

			// Create object ID for tab deduplication
			objectID := schemaName + "." + msg.Node.Label

			// Check if tab for this table already exists
			existingFound := false
			for i, tab := range a.resultTabs.GetAllTabs() {
				if tab.ObjectID == objectID && tab.Type == components.TabTypeTableData {
					a.resultTabs.SetActiveTab(i)
					existingFound = true
					a.state.FocusArea = models.FocusDataPanel
					a.updatePanelStyles()
					break
				}
			}

			if !existingFound {
				// Create new StructureView for this table
				tableView := components.NewTableView(a.theme)
				tableView.Spinner = &a.executeSpinner
				structureView := components.NewStructureView(a.theme, tableView)

				// Set loading state
				tableView.IsLoading = true
				tableView.LoadingStart = time.Now()

				// Add as a new tab
				a.resultTabs.AddTableData(objectID, msg.Node.Label, structureView)

				// Switch focus immediately to show loading state
				a.state.FocusArea = models.FocusDataPanel
				a.updatePanelStyles()

				// Load table data asynchronously and start spinner tick
				return a, tea.Batch(
					a.loadTableDataForTab(schemaName, msg.Node.Label, objectID),
					a.executeSpinner.Tick,
				)
			}
			return a, nil

		case models.TreeNodeTypeFunction, models.TreeNodeTypeProcedure:
			// Display function/procedure source code
			a.state.TreeSelected = msg.Node
			a.currentTable = "" // Clear current table
			a.isLoadingObjectDetails = true
			return a, tea.Batch(a.loadFunctionSource(msg.Node), a.executeSpinner.Tick)

		case models.TreeNodeTypeTriggerFunction:
			// Display trigger function source code
			a.state.TreeSelected = msg.Node
			a.currentTable = "" // Clear current table
			a.isLoadingObjectDetails = true
			return a, tea.Batch(a.loadTriggerFunctionSource(msg.Node), a.executeSpinner.Tick)

		case models.TreeNodeTypeSequence:
			// Display sequence properties
			a.state.TreeSelected = msg.Node
			a.currentTable = "" // Clear current table
			a.isLoadingObjectDetails = true
			return a, tea.Batch(a.loadSequenceDetails(msg.Node), a.executeSpinner.Tick)

		case models.TreeNodeTypeIndex:
			// Display index DDL definition
			a.state.TreeSelected = msg.Node
			a.currentTable = "" // Clear current table
			a.isLoadingObjectDetails = true
			return a, tea.Batch(a.loadIndexDetails(msg.Node), a.executeSpinner.Tick)

		case models.TreeNodeTypeTrigger:
			// Display trigger DDL definition
			a.state.TreeSelected = msg.Node
			a.currentTable = "" // Clear current table
			a.isLoadingObjectDetails = true
			return a, tea.Batch(a.loadTriggerDetails(msg.Node), a.executeSpinner.Tick)

		case models.TreeNodeTypeExtension:
			// Display extension info
			a.state.TreeSelected = msg.Node
			a.currentTable = "" // Clear current table
			a.isLoadingObjectDetails = true
			return a, tea.Batch(a.loadExtensionDetails(msg.Node), a.executeSpinner.Tick)

		case models.TreeNodeTypeCompositeType:
			// Display composite type definition
			a.state.TreeSelected = msg.Node
			a.currentTable = "" // Clear current table
			a.isLoadingObjectDetails = true
			return a, tea.Batch(a.loadCompositeTypeDetails(msg.Node), a.executeSpinner.Tick)

		case models.TreeNodeTypeEnumType:
			// Display enum type definition
			a.state.TreeSelected = msg.Node
			a.currentTable = "" // Clear current table
			a.isLoadingObjectDetails = true
			return a, tea.Batch(a.loadEnumTypeDetails(msg.Node), a.executeSpinner.Tick)

		case models.TreeNodeTypeDomainType:
			// Display domain type definition
			a.state.TreeSelected = msg.Node
			a.currentTable = "" // Clear current table
			a.isLoadingObjectDetails = true
			return a, tea.Batch(a.loadDomainTypeDetails(msg.Node), a.executeSpinner.Tick)

		case models.TreeNodeTypeRangeType:
			// Display range type definition
			a.state.TreeSelected = msg.Node
			a.currentTable = "" // Clear current table
			a.isLoadingObjectDetails = true
			return a, tea.Batch(a.loadRangeTypeDetails(msg.Node), a.executeSpinner.Tick)

		default:
			return a, nil
		}

	case messages.LoadTableDataMsg:
		return a, a.loadTableData(msg)

	case messages.ObjectDetailsLoadedMsg:
		a.isLoadingObjectDetails = false // Clear loading state
		if msg.Err != nil {
			a.ShowError("Error", fmt.Sprintf("Failed to load %s details:\n\n%v", msg.ObjectType, msg.Err))
			return a, nil
		}

		// Check if tab for this object already exists
		for i, tab := range a.resultTabs.GetAllTabs() {
			if tab.ObjectID == msg.ObjectID && tab.Type == components.TabTypeCodeEditor {
				a.resultTabs.SetActiveTab(i)
				a.state.FocusArea = models.FocusDataPanel
				a.updatePanelStyles()
				return a, nil
			}
		}

		// Create code editor to display the object details
		codeEditor := components.NewCodeEditor(a.theme)
		codeEditor.SetContent(msg.Content, msg.ObjectType, msg.Title)
		codeEditor.ObjectName = msg.ObjectName

		// Add as a new tab
		a.resultTabs.AddCodeEditor(msg.ObjectID, msg.Title, codeEditor)
		a.state.FocusArea = models.FocusDataPanel
		a.updatePanelStyles()
		return a, nil

	case components.CodeEditorCloseMsg:
		// Close the active tab if it's a code editor tab
		activeTab := a.resultTabs.GetActiveTab()
		if activeTab != nil && activeTab.Type == components.TabTypeCodeEditor {
			a.resultTabs.CloseActiveTab()
		}
		// Legacy: also clear the global code editor state
		a.showCodeEditor = false
		a.codeEditor = nil
		// If no more tabs, return to tree
		if !a.resultTabs.HasTabs() {
			a.state.FocusArea = models.FocusTreeView
		}
		a.updatePanelStyles()
		return a, nil

	case components.SaveObjectMsg:
		// Execute the save SQL
		return a, a.saveObjectDefinition(msg)

	case components.ObjectSavedMsg:
		if msg.Error != nil {
			a.ShowError("Save Error", fmt.Sprintf("Failed to save object:\n\n%v", msg.Error))
			return a, nil
		}
		// Success - update the code editor's original content and exit edit mode
		activeTab := a.resultTabs.GetActiveTab()
		if activeTab != nil && activeTab.Type == components.TabTypeCodeEditor && activeTab.CodeEditor != nil {
			activeTab.CodeEditor.Original = activeTab.CodeEditor.GetContent()
			activeTab.CodeEditor.Modified = false
			activeTab.CodeEditor.ExitEditMode(false) // Keep changes
		}
		// Legacy: also update global code editor
		if a.codeEditor != nil {
			a.codeEditor.Original = a.codeEditor.GetContent()
			a.codeEditor.Modified = false
			a.codeEditor.ExitEditMode(false) // Keep changes
		}
		return a, nil

	case messages.TableDataLoadedMsg:
		if msg.Err != nil {
			a.ShowError("Database Error", fmt.Sprintf("Failed to load table data:\n\n%v", msg.Err))
			return a, nil
		}

		// Check if this is initial load or pagination
		// Initial load if:
		// 1. No existing rows (first load ever)
		// 2. Offset is 0 (fresh load request, even for same table)
		// 3. Columns changed (different table selected)
		isInitialLoad := len(a.tableView.Rows) == 0 ||
			msg.Offset == 0 ||
			(len(msg.Columns) > 0 && len(a.tableView.Columns) > 0 && msg.Columns[0] != a.tableView.Columns[0])

		if isInitialLoad {
			// Initial load - replace all data
			a.tableView.SetData(msg.Columns, msg.Rows, msg.TotalRows)
			a.tableView.SelectedRow = 0
			a.tableView.TopRow = 0
			a.state.FocusArea = models.FocusDataPanel
			a.updatePanelStyles()
		} else {
			// Append paginated data (same table, loading more rows)
			a.tableView.Rows = append(a.tableView.Rows, msg.Rows...)
			a.tableView.TotalRows = msg.TotalRows
		}
		a.tableView.IsPaginating = false
		return a, nil

	case messages.TabTableDataLoadedMsg:
		// Clear loading state for the tab's table view
		if tab := a.resultTabs.GetTabByObjectID(msg.ObjectID); tab != nil {
			if sv := tab.Structure; sv != nil {
				if tv := sv.GetTableView(); tv != nil {
					tv.IsLoading = false
				}
			}
		}

		if msg.Err != nil {
			a.ShowError("Database Error", fmt.Sprintf("Failed to load table data:\n\n%v", msg.Err))
			return a, nil
		}

		// Find the tab with this objectID and update its data
		for _, tab := range a.resultTabs.GetAllTabs() {
			if tab.ObjectID == msg.ObjectID && tab.Type == components.TabTypeTableData {
				if tab.Structure != nil {
					// Set table data in the structure view
					tab.Structure.GetTableView().SetData(msg.Columns, msg.Rows, msg.TotalRows)
					// Also load structure metadata (columns, constraints, indexes)
					conn, err := a.connectionManager.GetActive()
					if err == nil && conn != nil && conn.Pool != nil {
						ctx := context.Background()
						_ = tab.Structure.SetTable(ctx, conn.Pool, msg.Schema, msg.Table)
					}
				}
				break
			}
		}
		a.state.FocusArea = models.FocusDataPanel
		a.updatePanelStyles()
		return a, nil

	case tea.WindowSizeMsg:
		a.state.Width = msg.Width
		a.state.Height = msg.Height
		a.updatePanelDimensions()

	default:
		// Forward other messages (like textinput's internal pasteMsg) to active input components
		// This enables clipboard paste functionality (Ctrl+V) in text inputs
		// Only components that use charmbracelet/bubbles/textinput need these messages
		var cmd tea.Cmd
		if a.showConnectionDialog {
			a.connectionDialog, cmd = a.connectionDialog.Update(msg)
			return a, cmd
		}
		if a.showSearch {
			a.searchInput, cmd = a.searchInput.Update(msg)
			return a, cmd
		}
	}
	return a, nil
}

// getActiveSchemaTable returns the schema and table name for the active context.
// For tab-based views, it extracts from the active tab's ObjectID.
// Otherwise, it parses a.currentTable.
func (a *App) getActiveSchemaTable() (string, string) {
	if a.resultTabs.HasTabs() {
		tab := a.resultTabs.GetActiveTab()
		if tab != nil && tab.Type == components.TabTypeTableData && tab.ObjectID != "" {
			parts := strings.Split(tab.ObjectID, ".")
			if len(parts) == 2 {
				return parts[0], parts[1]
			}
		}
	}
	if a.currentTable != "" {
		parts := strings.Split(a.currentTable, ".")
		if len(parts) == 2 {
			return parts[0], parts[1]
		}
	}
	return "", ""
}

// checkLazyLoad checks if we need to load more data and returns a command if so
func (a *App) checkLazyLoad() tea.Cmd {
	activeTable := a.getActiveTableView()
	if activeTable == nil {
		return nil
	}

	var cmds []tea.Cmd

	// Check if we need to load more data (lazy loading)
	if activeTable.SelectedRow >= len(activeTable.Rows)-10 &&
		len(activeTable.Rows) < activeTable.TotalRows &&
		!activeTable.IsPaginating {

		schema, table := a.getActiveSchemaTable()
		if schema != "" && table != "" {
			activeTable.IsPaginating = true
			offset := len(activeTable.Rows)

			if a.resultTabs.HasTabs() {
				// For tab-based views, use prefetch path (PrefetchCompleteMsg
				// appends to the active table view without routing issues)
				cmds = append(cmds, func() tea.Msg {
					return messages.PrefetchDataMsg{
						Schema:     schema,
						Table:      table,
						Offset:     offset,
						Limit:      100,
						SortColumn: activeTable.GetSortColumn(),
						SortDir:    activeTable.GetSortDirection(),
						NullsFirst: activeTable.GetNullsFirst(),
					}
				})
			} else {
				// For main table view, use the existing LoadTableData path
				cmds = append(cmds, func() tea.Msg {
					return messages.LoadTableDataMsg{
						Schema:     schema,
						Table:      table,
						Offset:     offset,
						Limit:      100,
						SortColumn: activeTable.GetSortColumn(),
						SortDir:    activeTable.GetSortDirection(),
						NullsFirst: activeTable.GetNullsFirst(),
					}
				})
			}
		}
	}

	// Check if prefetch is needed
	if cmd := a.checkAndTriggerPrefetch(); cmd != nil {
		cmds = append(cmds, cmd)
	}

	if len(cmds) > 0 {
		return tea.Batch(cmds...)
	}
	return nil
}

// PrefetchData prefetches table data in background
func (a *App) PrefetchData(schema, table string, offset, limit int, sortCol, sortDir string, nullsFirst bool) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		conn, err := a.connectionManager.GetActive()
		if err != nil {
			return messages.PrefetchCompleteMsg{Err: err}
		}

		var sort *metadata.SortOptions
		if sortCol != "" {
			sort = &metadata.SortOptions{
				Column:     sortCol,
				Direction:  sortDir,
				NullsFirst: nullsFirst,
			}
		}

		data, err := metadata.QueryTableData(ctx, conn.Pool, schema, table, offset, limit, sort)
		if err != nil {
			return messages.PrefetchCompleteMsg{Err: err}
		}

		return messages.PrefetchCompleteMsg{
			Rows:   data.Rows,
			Offset: offset,
		}
	}
}

// checkAndTriggerPrefetch checks if prefetch is needed and returns command
func (a *App) checkAndTriggerPrefetch() tea.Cmd {
	activeTable := a.getActiveTableView()
	if activeTable == nil || !activeTable.NeedsPrefetch() {
		return nil
	}

	// Parse current table (schema.table format)
	parts := strings.Split(a.currentTable, ".")
	if len(parts) != 2 {
		return nil
	}

	offset := len(activeTable.Rows)
	limit := 100 // default prefetch batch size
	if offset+limit > activeTable.TotalRows {
		limit = activeTable.TotalRows - offset
	}
	if limit <= 0 {
		return nil
	}

	activeTable.IsPrefetching = true

	return func() tea.Msg {
		return messages.PrefetchDataMsg{
			Schema:     parts[0],
			Table:      parts[1],
			Offset:     offset,
			Limit:      limit,
			SortColumn: activeTable.GetSortColumn(),
			SortDir:    activeTable.GetSortDirection(),
			NullsFirst: activeTable.GetNullsFirst(),
		}
	}
}

// getActiveTableView returns the appropriate TableView based on current context:
// - If Result Tabs has tabs, use the active result tab's TableView
// - If on structure tabs (columns, constraints, indexes), use structure view's TableView
// - Otherwise use the main tableView (for Data tab browsing)
func (a *App) getActiveTableView() *components.TableView {
	if a.resultTabs.HasTabs() {
		return a.resultTabs.GetActiveTableView()
	}
	if a.currentTab > 0 {
		return a.structureView.GetActiveTableView()
	}
	return a.tableView
}

// View implements tea.Model
func (a *App) View() string {
	// If error overlay is showing, render it centered on top of everything
	if a.showError {
		return lipgloss.Place(
			a.state.Width, a.state.Height,
			lipgloss.Center, lipgloss.Center,
			a.errorOverlay.View(),
		)
	}

	// If connection dialog is showing, render it with zone.Scan for mouse support
	if a.showConnectionDialog {
		return zone.Scan(a.renderConnectionDialog())
	}

	// If password dialog is showing, render it
	if a.showPasswordDialog {
		return zone.Scan(a.renderPasswordDialog())
	}

	// If in help mode, show help overlay
	if a.state.ViewMode == models.HelpMode {
		return help.Render(a.state.Width, a.state.Height, lipgloss.NewStyle())
	}

	// Wrap normal view with zone.Scan for mouse support
	return zone.Scan(a.renderNormalView())
}

// renderNormalView renders the normal application view
func (a *App) renderNormalView() string {
	// Use cached styles for performance
	styles := a.cachedStyles

	// Top bar with modern Catppuccin styling
	connStatus := ""
	if a.state.ActiveConnection != nil {
		// Build connection string with elegant formatting
		conn := a.state.ActiveConnection
		connStr := fmt.Sprintf("%s@%s:%d/%s",
			conn.Config.User,
			conn.Config.Host,
			conn.Config.Port,
			conn.Config.Database)

		connStatus = "  " + styles.connGreen.Render("") + " " + styles.connText.Render(connStr)
	} else {
		connStatus = "  " + styles.connGray.Render("") + " " + styles.connGray.Render("Not connected")
	}

	topBarLeft := styles.appName.Render("  LazyPG ") + connStatus
	topBarRight := styles.topBarHelp.Render("? ") + styles.topBarHelpText.Render("help")
	topBarContent := a.formatStatusBar(topBarLeft, topBarRight)

	// Create modern top bar with subtle border
	// Width must account for border: lipgloss Width() sets content area,
	// border chars are added outside, so subtract border width (2) to avoid overflow
	topBar := lipgloss.NewStyle().
		Width(a.state.Width-2).
		Background(lipgloss.Color("#313244")).
		Foreground(lipgloss.Color("#cdd6f4")).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#45475a")).
		Padding(0, 1).
		Render(topBarContent)

	// Context-sensitive bottom bar with cached styles
	var bottomBarLeft string
	// Focus area label style
	focusLabelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#1e1e2e")). // Dark text
		Background(lipgloss.Color("#89b4fa")). // Blue background
		Padding(0, 1).
		Bold(true)

	if a.isSQLEditorFocused() {
		// SQL editor mode
		focusLabel := focusLabelStyle.Render("SQL")
		bottomBarLeft = focusLabel + styles.separatorStyle.Render(" │ ") +
			styles.keyStyle.Render("Ctrl+S") + styles.dimStyle.Render(" execute") +
			styles.separatorStyle.Render(" │ ") +
			styles.keyStyle.Render("Ctrl+O") + styles.dimStyle.Render(" editor") +
			styles.separatorStyle.Render(" │ ") +
			styles.keyStyle.Render("Esc") + styles.dimStyle.Render(" close")
	} else if a.state.FocusArea == models.FocusTreeView {
		// Tree navigation keys with icons
		focusLabel := focusLabelStyle.Render("Tree")
		bottomBarLeft = focusLabel + styles.separatorStyle.Render(" │ ") +
			styles.keyStyle.Render("↑↓") + styles.dimStyle.Render(" navigate") +
			styles.separatorStyle.Render(" │ ") +
			styles.keyStyle.Render("→←") + styles.dimStyle.Render(" expand") +
			styles.separatorStyle.Render(" │ ") +
			styles.keyStyle.Render("Enter") + styles.dimStyle.Render(" select") +
			styles.separatorStyle.Render(" │ ") +
			styles.keyStyle.Render("/") + styles.dimStyle.Render(" search")
	} else {
		// Data panel - include SQL editor shortcut
		focusLabel := focusLabelStyle.Render("Data")
		bottomBarLeft = focusLabel + styles.separatorStyle.Render(" │ ") +
			styles.keyStyle.Render("↑↓") + styles.dimStyle.Render(" navigate") +
			styles.separatorStyle.Render(" │ ") +
			styles.keyStyle.Render("Ctrl+D/U") + styles.dimStyle.Render(" page") +
			styles.separatorStyle.Render(" │ ") +
			styles.keyStyle.Render("Ctrl+E") + styles.dimStyle.Render(" sql") +
			styles.separatorStyle.Render(" │ ") +
			styles.keyStyle.Render("p") + styles.dimStyle.Render(" preview") +
			styles.separatorStyle.Render(" │ ") +
			styles.keyStyle.Render("*") + styles.dimStyle.Render(" pin") +
			styles.separatorStyle.Render(" │ ") +
			styles.keyStyle.Render("'") + styles.dimStyle.Render(" goto pin")
	}

	// Add filter indicator if active
	if a.activeFilter != nil && len(a.activeFilter.RootGroup.Conditions) > 0 {
		filterCount := len(a.activeFilter.RootGroup.Conditions)
		filterSuffix := ""
		if filterCount > 1 {
			filterSuffix = "s"
		}
		filterIndicator := styles.separatorStyle.Render(" │ ") +
			styles.filterStyle.Render("") + styles.dimStyle.Render(fmt.Sprintf(" %d filter%s", filterCount, filterSuffix))
		bottomBarLeft = bottomBarLeft + filterIndicator
	}

	// Add Vim motion status if pending
	if a.state.FocusArea == models.FocusDataPanel && a.currentTab == 0 {
		vimStatus := a.tableView.GetVimMotionStatus()
		if vimStatus != "" {
			bottomBarLeft = bottomBarLeft + styles.separatorStyle.Render(" │ ") + styles.vimStyle.Render(vimStatus)
		}
	}

	// Common keys on the right with icons
	bottomBarRight := styles.keyStyle.Render("Tab") + styles.dimStyle.Render(" switch") +
		styles.separatorStyle.Render(" │ ") +
		styles.keyStyle.Render("[]") + styles.dimStyle.Render(" tabs")

	// Show 1-4 hint when active tab is TableData
	activeTab := a.resultTabs.GetActiveTab()
	if activeTab != nil && activeTab.Type == components.TabTypeTableData {
		bottomBarRight += styles.separatorStyle.Render(" │ ") +
			styles.keyStyle.Render("1-4") + styles.dimStyle.Render(" structure")
	}

	bottomBarRight += styles.separatorStyle.Render(" │ ") +
		styles.keyStyle.Render("c") + styles.dimStyle.Render(" connect") +
		styles.separatorStyle.Render(" │ ") +
		styles.keyStyle.Render("q") + styles.dimStyle.Render(" quit")

	bottomBarContent := a.formatStatusBar(bottomBarLeft, bottomBarRight)

	// Create modern bottom bar
	// Width must account for border: subtract border width (2) to avoid overflow
	bottomBar := lipgloss.NewStyle().
		Width(a.state.Width-2).
		Background(lipgloss.Color("#313244")).
		Foreground(lipgloss.Color("#cdd6f4")).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#45475a")).
		Padding(0, 1).
		Render(bottomBarContent)

	// Update tree view dimensions and render
	// Calculate available content height: panel height - borders (2) - title line (1) - padding (0)
	treeContentHeight := a.leftPanel.Height - 3 // -2 for top/bottom borders, -1 for title
	if treeContentHeight < 1 {
		treeContentHeight = 1
	}
	a.treeView.Width = a.leftPanel.Width - 2 // -2 for horizontal padding inside panel
	a.treeView.Height = treeContentHeight
	a.leftPanel.Content = a.treeView.View()
	a.leftPanel.Title = "Explorer"

	// Update right panel content
	// Calculate available content height: panel height - borders (2) - padding (0)
	rightContentHeight := a.rightPanel.Height - 2 // -2 for top/bottom borders (no title)
	if rightContentHeight < 1 {
		rightContentHeight = 1
	}
	rightContentWidth := a.rightPanel.Width - 2 // -2 for horizontal padding inside panel

	a.rightPanel.Content = a.renderRightPanel(rightContentWidth, rightContentHeight)

	// Panels side by side
	panels := lipgloss.JoinHorizontal(
		lipgloss.Top,
		a.leftPanel.View(),
		a.rightPanel.View(),
	)

	// Combine all
	mainView := lipgloss.JoinVertical(
		lipgloss.Left,
		topBar,
		panels,
		bottomBar,
	)

	// Render filter builder if visible
	if a.showFilterBuilder {
		mainView = lipgloss.Place(
			a.state.Width,
			a.state.Height,
			lipgloss.Center,
			lipgloss.Center,
			a.filterBuilder.View(),
			lipgloss.WithWhitespaceChars(" "),
			lipgloss.WithWhitespaceForeground(lipgloss.Color("#555555")),
		)
	}

	// Render JSONB viewer if visible
	if a.showJSONBViewer {
		jsonbView := a.jsonbViewer.View()

		// Check if preview panel should be shown below
		if a.jsonbViewer.PreviewVisible() {
			// Preview panel below JSONB viewer
			// Use same width as JSONB viewer, height is 1/4 of screen
			previewHeight := a.state.Height / 4
			if previewHeight < 6 {
				previewHeight = 6
			}
			if previewHeight > 12 {
				previewHeight = 12
			}

			previewPanel := a.jsonbViewer.RenderPreviewPanel(a.jsonbViewer.Width, previewHeight)

			// Join panels vertically
			combined := lipgloss.JoinVertical(
				lipgloss.Left,
				jsonbView,
				previewPanel,
			)

			mainView = lipgloss.Place(
				a.state.Width,
				a.state.Height,
				lipgloss.Center,
				lipgloss.Center,
				combined,
				lipgloss.WithWhitespaceChars(" "),
				lipgloss.WithWhitespaceForeground(lipgloss.Color("#555555")),
			)
		} else {
			mainView = lipgloss.Place(
				a.state.Width,
				a.state.Height,
				lipgloss.Center,
				lipgloss.Center,
				jsonbView,
				lipgloss.WithWhitespaceChars(" "),
				lipgloss.WithWhitespaceForeground(lipgloss.Color("#555555")),
			)
		}
	}

	// Render favorites dialog if visible
	if a.showFavorites {
		mainView = lipgloss.Place(
			a.state.Width,
			a.state.Height,
			lipgloss.Center,
			lipgloss.Center,
			a.favoritesDialog.View(),
			lipgloss.WithWhitespaceChars(" "),
			lipgloss.WithWhitespaceForeground(lipgloss.Color("#555555")),
		)
	}

	// Render command palette if visible (as overlay on top of mainView)
	if a.showCommandPalette {
		a.commandPalette.Width = 80
		if a.commandPalette.Width > a.state.Width-4 {
			a.commandPalette.Width = a.state.Width - 4
		}
		a.commandPalette.Height = 20

		mainView = a.overlayCommandPalette(mainView)
	}

	// Render search input if visible (as overlay on top of mainView)
	if a.showSearch {
		a.searchInput.Width = 60
		if a.searchInput.Width > a.state.Width-4 {
			a.searchInput.Width = a.state.Width - 4
		}
		mainView = a.overlaySearchInput(mainView)
	}

	return mainView
}

// renderRightPanel renders the right panel content based on current state
func (a *App) renderRightPanel(width, height int) string {
	// Calculate SQL editor height
	editorHeight := a.sqlEditor.GetCollapsedHeight()
	if a.sqlEditor.IsExpanded() {
		editorHeight = int(float64(height) * a.sqlEditor.GetHeightRatio())
		if editorHeight < 5 {
			editorHeight = 5
		}
	}

	// Calculate data panel height (reserve space for tab bar if tabs exist)
	tabBarHeight := 0
	if a.resultTabs.HasTabs() {
		tabBarHeight = 1
	}
	dataPanelHeight := height - editorHeight - tabBarHeight
	if dataPanelHeight < 5 {
		dataPanelHeight = 5
	}

	// Render tab bar
	tabBar := ""
	if a.resultTabs.HasTabs() {
		tabBar = a.resultTabs.RenderTabBar(width)
	}

	// Render data panel
	dataPanel := a.renderDataPanel(width, dataPanelHeight)

	// Render SQL editor
	a.sqlEditor.Width = width
	a.sqlEditor.Height = editorHeight
	sqlEditorView := a.sqlEditor.View()

	// Combine vertically: tab bar + data + editor
	if tabBar != "" {
		return lipgloss.JoinVertical(lipgloss.Left, tabBar, dataPanel, sqlEditorView)
	}
	return lipgloss.JoinVertical(lipgloss.Left, dataPanel, sqlEditorView)
}

// renderDataPanel renders the data panel (table view or structure view)
func (a *App) renderDataPanel(width, height int) string {
	// Show loading spinner when loading object details (function, sequence, etc.)
	if a.isLoadingObjectDetails {
		spinnerView := a.executeSpinner.View()
		statusText := lipgloss.NewStyle().
			Foreground(a.theme.Foreground).
			Render("Loading...")

		content := lipgloss.JoinVertical(lipgloss.Center,
			"",
			spinnerView+" "+statusText,
		)

		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
	}

	// If we have result tabs, show the active tab's content
	if a.resultTabs.HasTabs() {
		activeTab := a.resultTabs.GetActiveTab()
		if activeTab != nil {
			// If active tab is pending, show spinner
			if activeTab.IsPending {
				elapsed := a.resultTabs.GetPendingElapsed()
				elapsedStr := fmt.Sprintf("%.1fs", elapsed.Seconds())

				spinnerView := a.executeSpinner.View()
				statusText := lipgloss.NewStyle().
					Foreground(a.theme.Foreground).
					Render(fmt.Sprintf("Executing query... (%s)", elapsedStr))

				cancelHint := lipgloss.NewStyle().
					Foreground(a.theme.Border).
					Render("Press Esc to cancel")

				content := lipgloss.JoinVertical(lipgloss.Center,
					"",
					spinnerView+" "+statusText,
					"",
					cancelHint,
				)

				return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
			}

			// If active tab was cancelled, show cancelled message
			if activeTab.IsCancelled {
				cancelledText := lipgloss.NewStyle().
					Foreground(a.theme.Warning).
					Bold(true).
					Render("Query Cancelled")

				hintText := lipgloss.NewStyle().
					Foreground(a.theme.Border).
					Render("Press Ctrl+E to edit and re-execute")

				content := lipgloss.JoinVertical(lipgloss.Center,
					"",
					cancelledText,
					"",
					hintText,
				)

				return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
			}

			// Render based on tab type
			switch activeTab.Type {
			case components.TabTypeQueryResult:
				// Show query result table view
				activeTable := a.resultTabs.GetActiveTableView()
				if activeTable != nil {
					activeTable.Width = width
					activeTable.Height = height - 1
					// Add empty line placeholder to align with TableData mode
					return "\n" + activeTable.View()
				}

			case components.TabTypeTableData:
				// Show table data with structure view
				if activeTab.Structure != nil {
					// Calculate preview pane height (only if visible)
					structureView := activeTab.Structure
					activeTable := structureView.GetActiveTableView()
					previewHeight := 0
					if activeTable != nil && activeTable.PreviewPane != nil && activeTable.PreviewPane.Visible {
						maxPreviewHeight := height / 3
						if maxPreviewHeight < 5 {
							maxPreviewHeight = 5
						}
						activeTable.SetPreviewPaneDimensions(width, maxPreviewHeight)
						previewHeight = activeTable.GetPreviewPaneHeight()
					}

					// Calculate main content height
					mainHeight := height - previewHeight
					if mainHeight < 5 {
						mainHeight = 5
					}

					// Update structure view dimensions
					structureView.Width = width
					structureView.Height = mainHeight

					// Render main content
					mainContent := structureView.View()

					// If preview pane is visible, append it
					if activeTable != nil && previewHeight > 0 {
						previewContent := activeTable.PreviewPane.View()
						return lipgloss.JoinVertical(lipgloss.Left, mainContent, previewContent)
					}

					return mainContent
				}

			case components.TabTypeCodeEditor:
				// Show code editor
				if activeTab.CodeEditor != nil {
					activeTab.CodeEditor.Width = width
					activeTab.CodeEditor.Height = height - 1
					// Add empty line placeholder to align with TableData mode
					return "\n" + activeTab.CodeEditor.View()
				}
			}
		}
	}

	// Legacy: If table is selected in tree (without tabs), show structure view
	if a.currentTable != "" {
		// Calculate preview pane height (only if visible)
		// Get active table view for preview pane handling
		activeTable := a.structureView.GetActiveTableView()
		previewHeight := 0
		if activeTable != nil && activeTable.PreviewPane != nil && activeTable.PreviewPane.Visible {
			// Set preview pane dimensions (max 1/3 of available height)
			maxPreviewHeight := height / 3
			if maxPreviewHeight < 5 {
				maxPreviewHeight = 5
			}
			activeTable.SetPreviewPaneDimensions(width, maxPreviewHeight)
			previewHeight = activeTable.GetPreviewPaneHeight()
		}

		// Calculate main content height (subtract preview pane height)
		mainHeight := height - previewHeight
		if mainHeight < 5 {
			mainHeight = 5
		}

		// Update structure view dimensions
		a.structureView.Width = width
		a.structureView.Height = mainHeight

		// Load table structure if needed (when table changes)
		conn, err := a.connectionManager.GetActive()
		if err == nil && conn != nil && conn.Pool != nil {
			parts := strings.Split(a.currentTable, ".")
			if len(parts) == 2 {
				// Only load if we haven't loaded this table yet
				if !a.structureView.HasTableLoaded(parts[0], parts[1]) {
					ctx := context.Background()
					err := a.structureView.SetTable(ctx, conn.Pool, parts[0], parts[1])
					if err != nil {
						log.Printf("Failed to load structure: %v", err)
					}
				}
			}
		}

		// Render main content
		mainContent := a.structureView.View()

		// If preview pane is visible, append it
		if activeTable != nil && previewHeight > 0 {
			previewContent := activeTable.PreviewPane.View()
			return lipgloss.JoinVertical(lipgloss.Left, mainContent, previewContent)
		}

		return mainContent
	}

	// Legacy: If we have code editor to display (function source, sequence info, etc.)
	if a.showCodeEditor && a.codeEditor != nil {
		a.codeEditor.Width = width
		a.codeEditor.Height = height
		return a.codeEditor.View()
	}

	// No data - show placeholder
	placeholderStyle := lipgloss.NewStyle().
		Foreground(a.theme.Comment).
		Width(width).
		Height(height).
		Align(lipgloss.Center, lipgloss.Center)

	return placeholderStyle.Render("No data to display\n\nPress Ctrl+E to open SQL editor")
}

// updatePanelDimensions calculates panel sizes based on window size
func (a *App) updatePanelDimensions() {
	if a.state.Width <= 0 || a.state.Height <= 0 {
		return
	}

	// Reserve space for top bar (3 lines) and bottom bar (3 lines)
	// Total: 6 lines, leaving Height - 6 for panels
	// Note: Panel.Height includes borders, so the actual content area is Height - 6
	contentHeight := a.state.Height - 6
	if contentHeight < 5 {
		contentHeight = 5
	}

	// Calculate panel widths
	// Each panel has a border (2 chars wide: left + right borders)
	// Total border width: 4 chars (2 per panel)
	leftWidth := (a.state.Width * a.state.LeftPanelWidth) / 100
	if leftWidth < 20 {
		leftWidth = 20
	}

	// Right panel gets remaining width after accounting for left panel and both borders
	// Subtract 4 to account for borders on both panels (2 chars each)
	rightWidth := a.state.Width - leftWidth - 4
	if rightWidth < 20 {
		rightWidth = 20
		// If right panel is too small, reduce left panel width
		leftWidth = a.state.Width - rightWidth - 4
	}

	a.leftPanel.Width = leftWidth
	a.leftPanel.Height = contentHeight
	a.rightPanel.Width = rightWidth
	a.rightPanel.Height = contentHeight
}

// updatePanelStyles updates panel styling based on focus with Catppuccin colors
func (a *App) updatePanelStyles() {
	// Update legacy FocusedPanel for compatibility
	if a.state.FocusArea == models.FocusTreeView {
		a.state.FocusedPanel = models.LeftPanel
	} else {
		a.state.FocusedPanel = models.RightPanel
	}

	// Left panel style
	if a.state.FocusArea == models.FocusTreeView {
		a.leftPanel.Style = lipgloss.NewStyle().
			BorderForeground(lipgloss.Color("#89b4fa")). // Blue - focused
			Foreground(lipgloss.Color("#cdd6f4"))        // Text
	} else {
		a.leftPanel.Style = lipgloss.NewStyle().
			BorderForeground(lipgloss.Color("#45475a")). // Surface1 - unfocused
			Foreground(lipgloss.Color("#cdd6f4"))        // Text
	}

	// Right panel style (focused when DataPanel or SQLEditor)
	if a.state.FocusArea == models.FocusDataPanel || a.state.FocusArea == models.FocusSQLEditor {
		a.rightPanel.Style = lipgloss.NewStyle().
			BorderForeground(lipgloss.Color("#89b4fa")). // Blue - focused
			Foreground(lipgloss.Color("#cdd6f4"))        // Text
	} else {
		a.rightPanel.Style = lipgloss.NewStyle().
			BorderForeground(lipgloss.Color("#45475a")). // Surface1 - unfocused
			Foreground(lipgloss.Color("#cdd6f4"))        // Text
	}

	// Update SQL Editor focused state
	a.sqlEditor.Focused = (a.state.FocusArea == models.FocusSQLEditor)

	// Update Code Editor focused state
	if a.codeEditor != nil {
		a.codeEditor.Focused = (a.state.FocusArea == models.FocusDataPanel)
	}

	// Update TableView focused state
	// DataPanel is focused and CodeEditor is not shown
	isTableFocused := a.state.FocusArea == models.FocusDataPanel && !a.showCodeEditor
	a.tableView.Focused = isTableFocused
}

// nextFocus moves focus to the next region in cycle: TreeView -> DataPanel -> SQLEditor -> TreeView
func (a *App) nextFocus() {
	switch a.state.FocusArea {
	case models.FocusTreeView:
		a.state.FocusArea = models.FocusDataPanel
	case models.FocusDataPanel:
		a.state.FocusArea = models.FocusSQLEditor
	case models.FocusSQLEditor:
		a.state.FocusArea = models.FocusTreeView
	}
	a.updatePanelStyles()
}

// prevFocus moves focus to the previous region in cycle
func (a *App) prevFocus() {
	switch a.state.FocusArea {
	case models.FocusTreeView:
		a.state.FocusArea = models.FocusSQLEditor
	case models.FocusDataPanel:
		a.state.FocusArea = models.FocusTreeView
	case models.FocusSQLEditor:
		a.state.FocusArea = models.FocusDataPanel
	}
	a.updatePanelStyles()
}

// isEditingText returns true if user is actively editing text (Tab should insert indent)
func (a *App) isEditingText() bool {
	// CodeEditor in edit mode
	if a.state.FocusArea == models.FocusDataPanel && a.showCodeEditor && a.codeEditor != nil && !a.codeEditor.ReadOnly {
		return true
	}
	// SQLEditor expanded and focused
	if a.state.FocusArea == models.FocusSQLEditor && a.sqlEditor.IsExpanded() {
		return true
	}
	return false
}

// isSQLEditorFocused returns true if SQL editor has focus (compatibility helper)
func (a *App) isSQLEditorFocused() bool {
	return a.state.FocusArea == models.FocusSQLEditor
}

// isPrintableInput checks if a key message is a printable character input
// that should trigger auto-expand in SQL editor (not a control/shortcut key)
func isPrintableInput(key string) bool {
	// Blacklist: keys that should always be shortcuts, not input
	blacklist := map[string]bool{
		"tab": true, "shift+tab": true, "backtab": true,
		"esc": true, "escape": true,
		"[": true, "]": true,
		"up": true, "down": true, "left": true, "right": true,
		"home": true, "end": true, "pgup": true, "pgdown": true,
		"f1": true, "f2": true, "f3": true, "f4": true, "f5": true,
		"f6": true, "f7": true, "f8": true, "f9": true, "f10": true,
		"f11": true, "f12": true,
	}

	if blacklist[key] {
		return false
	}

	// Any key with Ctrl or Alt modifier is a shortcut
	if strings.HasPrefix(key, "ctrl+") || strings.HasPrefix(key, "alt+") {
		return false
	}

	// Single printable characters, space, enter, backspace are input
	if len(key) == 1 {
		return true
	}

	// Special input keys
	inputKeys := map[string]bool{
		"enter": true, "space": true, "backspace": true, "delete": true,
	}
	return inputKeys[key]
}

// handleMouseEvent processes mouse events for scrolling and clicking using bubblezone
func (a *App) handleMouseEvent(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Handle connection dialog mouse events
	if a.showConnectionDialog {
		return a.handleConnectionDialogMouse(msg)
	}

	// Route mouse events to overlays first
	if a.showError {
		handled, cmd := a.errorOverlay.HandleMouseClick(msg)
		if handled {
			if cmd != nil {
				return a, cmd
			}
		}
		// Block other mouse events when error overlay is showing
		return a, nil
	}

	if a.showCommandPalette {
		// Handle scroll wheel
		if a.commandPalette.HandleMouseWheel(msg) {
			return a, nil
		}
		// Handle click
		handled, cmd := a.commandPalette.HandleMouseClick(msg)
		if handled {
			if cmd != nil {
				return a, cmd
			}
			return a, nil
		}
		// Block other mouse events when command palette is showing
		return a, nil
	}

	if a.showFilterBuilder {
		// Handle scroll wheel
		if a.filterBuilder.HandleMouseWheel(msg) {
			return a, nil
		}
		// Handle click
		handled, cmd := a.filterBuilder.HandleMouseClick(msg)
		if handled {
			if cmd != nil {
				return a, cmd
			}
			return a, nil
		}
		// Block other mouse events when filter builder is showing
		return a, nil
	}

	if a.showJSONBViewer {
		// Handle scroll wheel
		if a.jsonbViewer.HandleMouseWheel(msg) {
			return a, nil
		}
		// Handle click
		handled, cmd := a.jsonbViewer.HandleMouseClick(msg)
		if handled {
			if cmd != nil {
				return a, cmd
			}
			return a, nil
		}
		// Block other mouse events when JSONB viewer is showing
		return a, nil
	}

	if a.showFavorites {
		// Handle scroll wheel
		if a.favoritesDialog.HandleMouseWheel(msg) {
			return a, nil
		}
		// Handle click
		handled, cmd := a.favoritesDialog.HandleMouseClick(msg)
		if handled {
			if cmd != nil {
				return a, cmd
			}
			return a, nil
		}
		// Block other mouse events when favorites dialog is showing
		return a, nil
	}

	// Handle structure view tabs (for result tabs or legacy structure view)
	if activeTab := a.resultTabs.GetActiveTab(); activeTab != nil && activeTab.Type == components.TabTypeTableData && activeTab.Structure != nil {
		handled, tabIndex := activeTab.Structure.HandleMouseClick(msg)
		if handled {
			if tabIndex >= 1 {
				if cmd := a.triggerStructureMetadataLoad(activeTab.Structure, activeTab.ObjectID); cmd != nil {
					return a, cmd
				}
			}
			return a, nil
		}
	} else if a.currentTable != "" {
		handled, tabIndex := a.structureView.HandleMouseClick(msg)
		if handled {
			if tabIndex >= 0 {
				a.currentTab = tabIndex
			}
			return a, nil
		}
		// Fall through to normal table handling for the structure view content
	}

	// Search input doesn't need mouse handling currently
	if a.showSearch {
		// Block other mouse events when search is showing
		return a, nil
	}

	// Handle scroll events (these don't need zone detection)
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		// Check which zone we're scrolling in
		for i := 0; i < 100; i++ {
			zoneID := fmt.Sprintf("%s%d", components.ZoneTreeRowPrefix, i)
			if zone.Get(zoneID).InBounds(msg) {
				a.treeView.ScrollUp(3)
				return a, nil
			}
		}
		// Check table cells or rows for scroll (scroll viewport, not selection)
		tableScrolled := false
		for row := 0; row < 100 && !tableScrolled; row++ {
			for col := 0; col < 50; col++ {
				zoneID := fmt.Sprintf("%s%d-%d", components.ZoneTableCellPrefix, row, col)
				if zone.Get(zoneID).InBounds(msg) {
					if activeTable := a.getActiveTableView(); activeTable != nil {
						activeTable.ScrollViewport(-3) // Scroll up
					}
					tableScrolled = true
					break
				}
			}
		}
		if tableScrolled {
			return a, nil
		}
		for i := 0; i < 100; i++ {
			zoneID := fmt.Sprintf("%s%d", components.ZoneTableRowPrefix, i)
			if zone.Get(zoneID).InBounds(msg) {
				if activeTable := a.getActiveTableView(); activeTable != nil {
					activeTable.ScrollViewport(-3) // Scroll up
				}
				return a, nil
			}
		}
		if zone.Get(components.ZoneSQLEditor).InBounds(msg) {
			// Could add scroll in editor if needed
			return a, nil
		}
		return a, nil

	case tea.MouseButtonWheelDown:
		// Check which zone we're scrolling in
		for i := 0; i < 100; i++ {
			zoneID := fmt.Sprintf("%s%d", components.ZoneTreeRowPrefix, i)
			if zone.Get(zoneID).InBounds(msg) {
				a.treeView.ScrollDown(3)
				return a, nil
			}
		}
		// Check table cells or rows for scroll (scroll viewport, not selection)
		tableScrolledDown := false
		for row := 0; row < 100 && !tableScrolledDown; row++ {
			for col := 0; col < 50; col++ {
				zoneID := fmt.Sprintf("%s%d-%d", components.ZoneTableCellPrefix, row, col)
				if zone.Get(zoneID).InBounds(msg) {
					if activeTable := a.getActiveTableView(); activeTable != nil {
						needsLazyLoad := activeTable.ScrollViewport(3) // Scroll down
						if needsLazyLoad {
							if cmd := a.checkLazyLoad(); cmd != nil {
								return a, cmd
							}
						}
					}
					tableScrolledDown = true
					break
				}
			}
		}
		if tableScrolledDown {
			return a, nil
		}
		for i := 0; i < 100; i++ {
			zoneID := fmt.Sprintf("%s%d", components.ZoneTableRowPrefix, i)
			if zone.Get(zoneID).InBounds(msg) {
				if activeTable := a.getActiveTableView(); activeTable != nil {
					needsLazyLoad := activeTable.ScrollViewport(3) // Scroll down
					if needsLazyLoad {
						if cmd := a.checkLazyLoad(); cmd != nil {
							return a, cmd
						}
					}
				}
				return a, nil
			}
		}
		if zone.Get(components.ZoneSQLEditor).InBounds(msg) {
			// Could add scroll in editor if needed
			return a, nil
		}
		return a, nil

	case tea.MouseButtonLeft:
		if msg.Action != tea.MouseActionPress {
			return a, nil
		}

		// Check result tabs first
		for i := 0; i < components.MaxResultTabs; i++ {
			zoneID := fmt.Sprintf("%s%d", components.ZoneResultTabPrefix, i)
			if zone.Get(zoneID).InBounds(msg) {
				a.resultTabs.SetActiveTab(i)
				// Sync SQL editor content with new active tab
				if activeSQL := a.resultTabs.GetActiveSQL(); activeSQL != "" {
					a.sqlEditor.SetContent(activeSQL)
				}
				return a, nil
			}
		}

		// Check tree view rows
		for i := 0; i < 100; i++ {
			zoneID := fmt.Sprintf("%s%d", components.ZoneTreeRowPrefix, i)
			if zone.Get(zoneID).InBounds(msg) {
				a.state.FocusArea = models.FocusTreeView
				a.updatePanelStyles()
				_, cmd := a.treeView.HandleClick(i)
				return a, cmd
			}
		}

		// Check table view cells first (more specific than row)
		for row := 0; row < 100; row++ {
			for col := 0; col < 50; col++ {
				zoneID := fmt.Sprintf("%s%d-%d", components.ZoneTableCellPrefix, row, col)
				if zone.Get(zoneID).InBounds(msg) {
					a.state.FocusArea = models.FocusDataPanel
					a.updatePanelStyles()
					if activeTable := a.getActiveTableView(); activeTable != nil {
						// Convert visible col index to actual col index
						actualCol := activeTable.LeftColOffset + col
						actualRow := activeTable.TopRow + row

						// Check if clicking on already selected cell
						if activeTable.SelectedRow == actualRow && activeTable.SelectedCol == actualCol {
							// Double-click behavior: open JSONB viewer or preview pane
							if actualRow >= 0 && actualRow < len(activeTable.Rows) && actualCol >= 0 && actualCol < len(activeTable.Columns) {
								cellValue := activeTable.Rows[actualRow][actualCol]
								// Check if it's JSON/JSONB data
								if jsonb.IsJSONB(cellValue) {
									// Set viewer dimensions based on terminal size (with max width limit)
									viewerWidth := a.state.Width * 2 / 3
									if viewerWidth > 100 {
										viewerWidth = 100
									}
									a.jsonbViewer.Width = viewerWidth
									a.jsonbViewer.Height = a.state.Height * 3 / 4
									if err := a.jsonbViewer.SetValue(cellValue); err == nil {
										a.showJSONBViewer = true
									}
								} else {
									// For non-JSONB, toggle preview pane
									activeTable.TogglePreviewPane()
								}
							}
						} else {
							// First click: just select the cell
							activeTable.SetSelectedRow(actualRow)
							activeTable.SelectedCol = actualCol
						}
					}
					return a, nil
				}
			}
		}

		// Check table view rows (fallback for areas without cell zones)
		for i := 0; i < 100; i++ {
			zoneID := fmt.Sprintf("%s%d", components.ZoneTableRowPrefix, i)
			if zone.Get(zoneID).InBounds(msg) {
				a.state.FocusArea = models.FocusDataPanel
				a.updatePanelStyles()
				if activeTable := a.getActiveTableView(); activeTable != nil {
					activeTable.SetSelectedRow(activeTable.TopRow + i)
				}
				return a, nil
			}
		}

		// Check SQL editor
		if zone.Get(components.ZoneSQLEditor).InBounds(msg) {
			a.state.FocusArea = models.FocusSQLEditor
			a.updatePanelStyles()

			// Expand editor if collapsed
			if !a.sqlEditor.IsExpanded() {
				a.sqlEditor.Expand()
			}
			return a, nil
		}

		return a, nil
	}

	return a, nil
}

// handleConnectionDialogMouse handles mouse events for connection dialog using bubblezone
func (a *App) handleConnectionDialogMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Handle scroll
	if msg.Button == tea.MouseButtonWheelUp {
		a.connectionDialog.MoveSelection(-1)
		return a, nil
	}
	if msg.Button == tea.MouseButtonWheelDown {
		a.connectionDialog.MoveSelection(1)
		return a, nil
	}

	// Only handle left click press
	if msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress {
		return a, nil
	}

	// Check if click is on search box
	if zone.Get(components.ZoneSearchBox).InBounds(msg) {
		a.connectionDialog.EnterSearchMode()
		return a, nil
	}

	// Check history items (up to 5)
	filteredHistory := a.connectionDialog.GetFilteredHistory()
	for i := 0; i < 5 && i < len(filteredHistory); i++ {
		zoneID := fmt.Sprintf("%s%d", components.ZoneHistoryPrefix, i)
		if zone.Get(zoneID).InBounds(msg) {
			wasAlreadySelected := a.connectionDialog.InHistorySection && a.connectionDialog.SelectedIndex == i

			a.connectionDialog.InHistorySection = true
			a.connectionDialog.SelectedIndex = i

			// If clicking already selected item, trigger connect (lazygit-style)
			if wasAlreadySelected {
				return a.connectToHistoryEntry(filteredHistory[i])
			}
			return a, nil
		}
	}

	// Check discovered items (up to 3)
	filteredDiscovered := a.connectionDialog.GetFilteredDiscovered()
	for i := 0; i < 3 && i < len(filteredDiscovered); i++ {
		zoneID := fmt.Sprintf("%s%d", components.ZoneDiscoveredPrefix, i)
		if zone.Get(zoneID).InBounds(msg) {
			wasAlreadySelected := !a.connectionDialog.InHistorySection && a.connectionDialog.SelectedIndex == i

			a.connectionDialog.InHistorySection = false
			a.connectionDialog.SelectedIndex = i

			// If clicking already selected item, trigger connect (lazygit-style)
			if wasAlreadySelected {
				return a.connectToDiscoveredInstance(filteredDiscovered[i])
			}
			return a, nil
		}
	}

	return a, nil
}

// connectToHistoryEntry connects using a history entry
func (a *App) connectToHistoryEntry(entry models.ConnectionHistoryEntry) (tea.Model, tea.Cmd) {
	var config models.ConnectionConfig

	// Convert history entry to connection config WITH password from keyring
	if a.connectionHistory != nil {
		result := a.connectionHistory.GetConnectionConfigWithPassword(&entry)
		config = result.Config

		// If password is missing, show password dialog
		if result.PasswordMissing {
			entryCopy := entry
			a.pendingConnectionInfo = &entryCopy
			a.passwordDialog.SetConnectionInfo(entry.Host, entry.Port, entry.Database, entry.User)
			a.showPasswordDialog = true
			a.showConnectionDialog = false
			return a, a.passwordDialog.Init()
		}
	} else {
		config = entry.ToConnectionConfig()
	}

	return a.performConnection(config)
}

// connectToDiscoveredInstance connects using a discovered instance
func (a *App) connectToDiscoveredInstance(instance models.DiscoveredInstance) (tea.Model, tea.Cmd) {
	return a.performConnection(discovery.BuildConnectionConfig(instance))
}

// performConnection starts an async connection attempt
func (a *App) performConnection(config models.ConnectionConfig) (tea.Model, tea.Cmd) {
	return a, func() tea.Msg {
		return messages.ConnectionStartMsg{Config: config}
	}
}

// connectAsync performs the actual connection in a goroutine
func (a *App) connectAsync(config models.ConnectionConfig) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		connID, err := a.connectionManager.Connect(ctx, config)
		return messages.ConnectionResultMsg{
			Config: config,
			ConnID: connID,
			Err:    err,
		}
	}
}

// handleTabClick handles clicking on result tabs
// handleTabClick is no longer needed - using bubblezone for tab clicks

// formatStatusBar formats a status bar with left and right aligned content
func (a *App) formatStatusBar(left, right string) string {
	// Calculate available width accounting for:
	// - Borders: 2 chars (left + right)
	// - Padding: 2 chars (left + right, from Padding(0, 1))
	// Total: 4 chars
	availableWidth := a.state.Width - 4
	if availableWidth < 0 {
		availableWidth = 0
	}

	// Use lipgloss.Width to get actual display width (ignoring ANSI codes)
	leftLen := lipgloss.Width(left)
	rightLen := lipgloss.Width(right)

	// If content is too wide, truncate
	if leftLen+rightLen > availableWidth {
		if availableWidth > rightLen {
			// Simple truncation - in a real app you'd want smarter truncation
			return left + right
		}
		return left
	}

	// Calculate spacing between left and right content
	spacing := availableWidth - leftLen - rightLen
	if spacing < 0 {
		spacing = 0
	}

	return left + lipgloss.NewStyle().Width(spacing).Render("") + right
}

// handleConnectionDialog handles key events when connection dialog is visible
func (a *App) handleConnectionDialog(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle search mode
	if a.connectionDialog.SearchMode {
		switch msg.String() {
		case "esc":
			// Exit search mode and clear search
			a.connectionDialog.ExitSearchMode(true)
			return a, nil
		case "enter":
			// Exit search mode but keep search results
			a.connectionDialog.ExitSearchMode(false)
			return a, nil
		default:
			// Pass keys to search input
			var cmd tea.Cmd
			a.connectionDialog, cmd = a.connectionDialog.Update(msg)
			return a, cmd
		}
	}

	switch msg.String() {
	case "esc":
		// Cancel connection attempt if in progress
		if a.isConnecting {
			a.isConnecting = false
			// Connection will complete in background but result will be ignored
			// since isConnecting is false
			return a, nil
		}
		a.showConnectionDialog = false
		return a, nil

	case "/", "ctrl+f":
		// Enter search mode (only in discovery mode)
		if !a.connectionDialog.ManualMode {
			a.connectionDialog.EnterSearchMode()
		}
		return a, nil

	case "up", "k":
		if !a.connectionDialog.ManualMode {
			a.connectionDialog.MoveSelection(-1)
		}
		return a, nil

	case "down", "j":
		if !a.connectionDialog.ManualMode {
			a.connectionDialog.MoveSelection(1)
		}
		return a, nil

	case "tab":
		if a.connectionDialog.ManualMode {
			a.connectionDialog.NextInput()
		} else {
			// In discovery mode, switch between history and discovered sections
			a.connectionDialog.SwitchSection()
		}
		return a, nil

	case "shift+tab":
		if a.connectionDialog.ManualMode {
			a.connectionDialog.PrevInput()
		}
		return a, nil

	case "m":
		// Only handle 'm' key in discovery mode, not in manual mode (to allow typing 'm')
		if !a.connectionDialog.ManualMode {
			a.connectionDialog.ToggleMode()
			return a, nil
		}
		// In manual mode, pass 'm' to textinput
		var cmd tea.Cmd
		a.connectionDialog, cmd = a.connectionDialog.Update(msg)
		return a, cmd

	case "ctrl+d":
		// Use Ctrl+D to switch back to discovery mode to avoid conflict with typing 'd'
		if a.connectionDialog.ManualMode {
			a.connectionDialog.ToggleMode()
		}
		return a, nil

	case "enter":
		if a.connectionDialog.ManualMode {
			config, err := a.connectionDialog.GetManualConfig()
			if err != nil {
				// Invalid input - show error and don't close dialog
				a.ShowError("Invalid Configuration", fmt.Sprintf("Could not parse connection configuration\n\nError: %v", err))
				return a, nil
			}
			return a.performConnection(config)
		} else {
			var config models.ConnectionConfig

			// Check if browsing history or discovered instances
			if a.connectionDialog.InHistorySection {
				// Get selected history entry
				historyEntry := a.connectionDialog.GetSelectedHistory()
				if historyEntry == nil {
					// No history entry selected
					return a, nil
				}

				// Convert history entry to connection config WITH password from keyring
				if a.connectionHistory != nil {
					result := a.connectionHistory.GetConnectionConfigWithPassword(historyEntry)
					config = result.Config

					// If password is missing, show password dialog
					if result.PasswordMissing {
						entryCopy := *historyEntry
						a.pendingConnectionInfo = &entryCopy
						a.passwordDialog.SetConnectionInfo(historyEntry.Host, historyEntry.Port, historyEntry.Database, historyEntry.User)
						a.showPasswordDialog = true
						a.showConnectionDialog = false
						return a, a.passwordDialog.Init()
					}
				} else {
					config = historyEntry.ToConnectionConfig()
				}
			} else {
				// Get selected discovered instance
				instance := a.connectionDialog.GetSelectedInstance()
				if instance == nil {
					// No instance selected
					return a, nil
				}

				config = discovery.BuildConnectionConfig(*instance)
			}

			return a.performConnection(config)
		}

	default:
		// In manual mode, delegate to textinput for cursor and text handling
		if a.connectionDialog.ManualMode {
			var cmd tea.Cmd
			a.connectionDialog, cmd = a.connectionDialog.Update(msg)
			return a, cmd
		}
		return a, nil
	}
}

// getBuiltinCommands returns built-in commands with icons
func (a *App) getBuiltinCommands() []models.Command {
	cmds := a.commandRegistry.GetAll()
	// Ensure commands have icons
	for i := range cmds {
		if cmds[i].Icon == "" {
			cmds[i].Icon = "▸"
		}
	}
	return cmds
}

// getTableCommands returns tables and views as commands
func (a *App) getTableCommands() []models.Command {
	var cmds []models.Command

	if a.treeView.Root == nil {
		return cmds
	}

	// Traverse tree to find all tables and views
	var traverse func(node *models.TreeNode)
	traverse = func(node *models.TreeNode) {
		if node == nil {
			return
		}

		if node.Type == models.TreeNodeTypeTable || node.Type == models.TreeNodeTypeView {
			// Get schema name from parent chain
			var schemaName string
			parent := node.Parent
			for parent != nil {
				if parent.Type == models.TreeNodeTypeSchema {
					schemaName = strings.Split(parent.Label, " ")[0]
					break
				}
				parent = parent.Parent
			}
			if schemaName != "" {
				icon := "▦"
				prefix := "table:"
				if node.Type == models.TreeNodeTypeView {
					icon = "◎"
					prefix = "view:"
				}
				cmds = append(cmds, models.Command{
					ID:          fmt.Sprintf("%s%s.%s", prefix, schemaName, node.Label),
					Label:       fmt.Sprintf("%s.%s", schemaName, node.Label),
					Description: "",
					Icon:        icon,
					Tags:        []string{schemaName, node.Label},
				})
			}
		}

		for _, child := range node.Children {
			traverse(child)
		}
	}

	traverse(a.treeView.Root)
	return cmds
}

// getHistoryCommands returns query history as commands
func (a *App) getHistoryCommands() []models.Command {
	var cmds []models.Command

	if a.historyStore == nil {
		return cmds
	}

	entries, err := a.historyStore.GetRecent(20)
	if err != nil {
		return cmds
	}

	for _, entry := range entries {
		// Truncate long queries for display
		displayQuery := entry.Query
		if len(displayQuery) > 60 {
			displayQuery = displayQuery[:57] + "..."
		}

		cmds = append(cmds, models.Command{
			ID:          fmt.Sprintf("history:%d", entry.ID),
			Type:        models.CommandTypeHistory,
			Label:       displayQuery,
			Description: fmt.Sprintf("From %s • %s", entry.DatabaseName, entry.ExecutedAt.Format("Jan 2 15:04")),
			Icon:        "📜",
			Tags:        []string{"history", entry.DatabaseName},
			Action: func(sql string) tea.Cmd {
				return func() tea.Msg {
					return components.ExecuteQueryMsg{SQL: sql}
				}
			}(entry.Query),
		})
	}

	return cmds
}

// handleCommandPalette handles key events when command palette is visible
func (a *App) handleCommandPalette(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle close message
	var cmd tea.Cmd
	a.commandPalette, cmd = a.commandPalette.Update(msg)

	// Check if we got a close message
	if msg.String() == "esc" || msg.String() == "ctrl+c" {
		a.showCommandPalette = false
		return a, nil
	}

	// Execute the command if Enter was pressed
	if msg.String() == "enter" {
		a.showCommandPalette = false

		selected := a.commandPalette.GetSelectedCommand()
		if selected == nil {
			return a, nil
		}

		// Handle table/view selection (ID starts with "table:" or "view:")
		if strings.HasPrefix(selected.ID, "table:") || strings.HasPrefix(selected.ID, "view:") {
			// Parse schema.table from ID (format: "table:schema.name" or "view:schema.name")
			var prefix string
			if strings.HasPrefix(selected.ID, "table:") {
				prefix = "table:"
			} else {
				prefix = "view:"
			}
			parts := strings.SplitN(strings.TrimPrefix(selected.ID, prefix), ".", 2)
			if len(parts) == 2 {
				schema := parts[0]
				table := parts[1]
				a.currentTable = schema + "." + table

				// Sync tree view position - find the node and expand ancestors
				if a.state.ActiveConnection != nil {
					dbName := a.state.ActiveConnection.Config.Database
					nodeID := fmt.Sprintf("%s%s.%s.%s", prefix, dbName, schema, table)
					a.treeView.ExpandAndNavigateToNode(nodeID)
				}

				return a, func() tea.Msg {
					return messages.LoadTableDataMsg{
						Schema: schema,
						Table:  table,
						Offset: 0,
						Limit:  100,
					}
				}
			}
		}

		// Handle regular command with action
		if selected.Action != nil {
			return a, selected.Action
		}
	}

	return a, cmd
}

// handleFilterBuilder handles key events when filter builder is visible
func (a *App) handleFilterBuilder(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	a.filterBuilder, cmd = a.filterBuilder.Update(msg)
	return a, cmd
}

// handleJSONBViewer handles key events when JSONB viewer is visible
func (a *App) handleJSONBViewer(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	a.jsonbViewer, cmd = a.jsonbViewer.Update(msg)
	return a, cmd
}

// handleFavoritesDialog handles key events when favorites dialog is visible
func (a *App) handleFavoritesDialog(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	a.favoritesDialog, cmd = a.favoritesDialog.Update(msg)
	return a, cmd
}

// handleSearchInput handles key events when search input is visible
func (a *App) handleSearchInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	a.searchInput, cmd = a.searchInput.Update(msg)
	return a, cmd
}

// getTableColumns returns column info for the current table
func (a *App) getTableColumns() []models.ColumnInfo {
	if a.state.TreeSelected == nil || a.state.TreeSelected.Type != models.TreeNodeTypeTable {
		return nil
	}

	conn, err := a.connectionManager.GetActive()
	if err != nil {
		return nil
	}

	// Get schema from parent node
	schemaNode := a.state.TreeSelected.Parent
	if schemaNode == nil {
		return nil
	}

	columns, err := metadata.GetTableColumns(
		context.Background(),
		conn.Pool,
		schemaNode.Label,
		a.state.TreeSelected.Label,
	)
	if err != nil {
		return nil
	}

	return columns
}

// renderConnectionDialog renders the connection dialog centered on screen
func (a *App) renderConnectionDialog() string {
	// If connecting, show loading overlay instead
	if a.isConnecting {
		return a.renderConnectingOverlay()
	}

	// Center the dialog
	dialogWidth := 60
	dialogHeight := 20

	a.connectionDialog.Width = dialogWidth
	a.connectionDialog.Height = dialogHeight

	dialog := a.connectionDialog.View()

	// Center it
	verticalPadding := (a.state.Height - dialogHeight) / 2
	horizontalPadding := (a.state.Width - dialogWidth) / 2

	if verticalPadding < 0 {
		verticalPadding = 0
	}
	if horizontalPadding < 0 {
		horizontalPadding = 0
	}

	style := lipgloss.NewStyle().
		Padding(verticalPadding, 0, 0, horizontalPadding)

	return style.Render(dialog)
}

// renderConnectingOverlay renders the connecting loading state
func (a *App) renderConnectingOverlay() string {
	elapsed := time.Since(a.connectingStart)
	elapsedStr := fmt.Sprintf("(%.1fs)", elapsed.Seconds())

	loadingStyle := lipgloss.NewStyle().Foreground(a.theme.BorderFocused)
	elapsedStyle := lipgloss.NewStyle().Foreground(a.theme.Metadata)
	hostStyle := lipgloss.NewStyle().Foreground(a.theme.Foreground)
	hintStyle := lipgloss.NewStyle().Foreground(a.theme.Metadata)

	hostInfo := fmt.Sprintf("%s@%s:%d/%s",
		a.connectingConfig.User,
		a.connectingConfig.Host,
		a.connectingConfig.Port,
		a.connectingConfig.Database,
	)

	// Build each line separately
	line1 := a.executeSpinner.View() + " " + loadingStyle.Render("Connecting...") + " " + elapsedStyle.Render(elapsedStr)
	line2 := hostStyle.Render(hostInfo)
	line3 := hintStyle.Render("Press Esc to cancel")

	// Calculate dialog width based on content
	maxLen := len(hostInfo)
	if maxLen < 40 {
		maxLen = 40
	}
	dialogWidth := maxLen + 8 // padding

	// Create content with proper centering
	contentStyle := lipgloss.NewStyle().Width(dialogWidth - 6).Align(lipgloss.Center)

	content := lipgloss.JoinVertical(lipgloss.Center,
		"",
		contentStyle.Render(line1),
		"",
		contentStyle.Render(line2),
		"",
		contentStyle.Render(line3),
	)

	dialogStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(a.theme.BorderFocused).
		Padding(1, 2)

	dialog := dialogStyle.Render(content)

	// Center on screen
	dialogHeight := 10
	verticalPadding := (a.state.Height - dialogHeight) / 2
	horizontalPadding := (a.state.Width - dialogWidth - 4) / 2

	if verticalPadding < 0 {
		verticalPadding = 0
	}
	if horizontalPadding < 0 {
		horizontalPadding = 0
	}

	style := lipgloss.NewStyle().
		Padding(verticalPadding, 0, 0, horizontalPadding)

	return style.Render(dialog)
}

func (a *App) renderPasswordDialog() string {
	// Dialog dimensions (must match password_dialog.go)
	dialogWidth := 56 + 2 // content width + border
	dialogHeight := 14

	a.passwordDialog.Width = dialogWidth
	a.passwordDialog.Height = dialogHeight

	dialog := a.passwordDialog.View()

	// Center on screen
	verticalPadding := (a.state.Height - dialogHeight) / 2
	horizontalPadding := (a.state.Width - dialogWidth) / 2

	if verticalPadding < 0 {
		verticalPadding = 0
	}
	if horizontalPadding < 0 {
		horizontalPadding = 0
	}

	style := lipgloss.NewStyle().
		Padding(verticalPadding, 0, 0, horizontalPadding)

	return style.Render(dialog)
}

// triggerDiscovery runs discovery in the background and returns a command
func (a *App) triggerDiscovery() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		instances := a.discoverer.DiscoverAll(ctx)
		return messages.DiscoveryCompleteMsg{Instances: instances}
	}
}

// loadTree loads the database structure skeleton with object counts (fast)
func (a *App) loadTree() tea.Msg {
	ctx := context.Background()

	conn, err := a.connectionManager.GetActive()
	if err != nil {
		return messages.TreeLoadedMsg{Err: fmt.Errorf("no active connection: %w", err)}
	}

	currentDB := conn.Config.Database

	// Build root with database node
	root := models.BuildDatabaseTree([]string{currentDB}, currentDB)

	// Load extensions (usually fast, small number)
	extensions, _ := metadata.ListExtensions(ctx, conn.Pool)

	// Get all schema objects in ONE query
	schemaObjects, err := metadata.GetAllSchemaObjects(ctx, conn.Pool)
	if err != nil {
		return messages.TreeLoadedMsg{Err: fmt.Errorf("failed to load schema objects: %w", err)}
	}

	dbNode := root.FindByID(fmt.Sprintf("db:%s", currentDB))
	if dbNode == nil {
		return messages.TreeLoadedMsg{Root: root}
	}

	// Organize objects by schema -> type -> names
	type funcInfo struct {
		name string
		args string
	}
	type schemaData struct {
		tables           []string
		views            []string
		matViews         []string
		sequences        []string
		functions        []funcInfo
		procedures       []funcInfo
		triggerFunctions []string
		compositeTypes   []string
		enumTypes        []string
		domainTypes      []string
		rangeTypes       []string
	}
	schemaMap := make(map[string]*schemaData)

	for _, obj := range schemaObjects {
		sd, ok := schemaMap[obj.SchemaName]
		if !ok {
			sd = &schemaData{}
			schemaMap[obj.SchemaName] = sd
		}
		switch obj.ObjectType {
		case "table":
			sd.tables = append(sd.tables, obj.ObjectName)
		case "view":
			sd.views = append(sd.views, obj.ObjectName)
		case "matview":
			sd.matViews = append(sd.matViews, obj.ObjectName)
		case "sequence":
			sd.sequences = append(sd.sequences, obj.ObjectName)
		case "function":
			sd.functions = append(sd.functions, funcInfo{name: obj.ObjectName, args: obj.Arguments})
		case "procedure":
			sd.procedures = append(sd.procedures, funcInfo{name: obj.ObjectName, args: obj.Arguments})
		case "trigger_function":
			sd.triggerFunctions = append(sd.triggerFunctions, obj.ObjectName)
		case "composite_type":
			sd.compositeTypes = append(sd.compositeTypes, obj.ObjectName)
		case "enum_type":
			sd.enumTypes = append(sd.enumTypes, obj.ObjectName)
		case "domain_type":
			sd.domainTypes = append(sd.domainTypes, obj.ObjectName)
		case "range_type":
			sd.rangeTypes = append(sd.rangeTypes, obj.ObjectName)
		}
	}

	// Add extensions group
	if len(extensions) > 0 {
		extGroup := models.NewTreeNode(
			fmt.Sprintf("extensions:%s", currentDB),
			models.TreeNodeTypeExtensionGroup,
			fmt.Sprintf("Extensions (%d)", len(extensions)),
		)
		extGroup.Selectable = false
		for _, ext := range extensions {
			extNode := models.NewTreeNode(
				fmt.Sprintf("extension:%s.%s", currentDB, ext.Name),
				models.TreeNodeTypeExtension,
				fmt.Sprintf("%s v%s", ext.Name, ext.Version),
			)
			extNode.Selectable = true
			extNode.Metadata = ext
			extNode.Loaded = true
			extGroup.AddChild(extNode)
		}
		extGroup.Loaded = true
		dbNode.AddChild(extGroup)
	}

	// Build tree with pre-populated object nodes
	// Sort schema names for consistent ordering
	schemaNames := make([]string, 0, len(schemaMap))
	for name := range schemaMap {
		schemaNames = append(schemaNames, name)
	}
	sort.Strings(schemaNames)

	for _, schemaName := range schemaNames {
		sd := schemaMap[schemaName]
		schemaNode := models.NewTreeNode(
			fmt.Sprintf("schema:%s.%s", currentDB, schemaName),
			models.TreeNodeTypeSchema,
			schemaName,
		)
		schemaNode.Selectable = true

		// Tables group with actual table nodes
		if len(sd.tables) > 0 {
			tablesGroup := models.NewTreeNode(
				fmt.Sprintf("tables:%s.%s", currentDB, schemaName),
				models.TreeNodeTypeTableGroup,
				fmt.Sprintf("Tables (%d)", len(sd.tables)),
			)
			tablesGroup.Selectable = false
			for _, tableName := range sd.tables {
				tableNode := models.NewTreeNode(
					fmt.Sprintf("table:%s.%s.%s", currentDB, schemaName, tableName),
					models.TreeNodeTypeTable,
					tableName,
				)
				tableNode.Selectable = true
				tableNode.Loaded = false // Columns/indexes still lazy load
				tablesGroup.AddChild(tableNode)
			}
			tablesGroup.Loaded = true // Group has all children
			schemaNode.AddChild(tablesGroup)
		}

		// Views group with actual view nodes
		if len(sd.views) > 0 {
			viewsGroup := models.NewTreeNode(
				fmt.Sprintf("views:%s.%s", currentDB, schemaName),
				models.TreeNodeTypeViewGroup,
				fmt.Sprintf("Views (%d)", len(sd.views)),
			)
			viewsGroup.Selectable = false
			for _, viewName := range sd.views {
				viewNode := models.NewTreeNode(
					fmt.Sprintf("view:%s.%s.%s", currentDB, schemaName, viewName),
					models.TreeNodeTypeView,
					viewName,
				)
				viewNode.Selectable = true
				viewNode.Loaded = true // Views don't have children
				viewsGroup.AddChild(viewNode)
			}
			viewsGroup.Loaded = true
			schemaNode.AddChild(viewsGroup)
		}

		// Materialized Views group with actual matview nodes
		if len(sd.matViews) > 0 {
			matViewsGroup := models.NewTreeNode(
				fmt.Sprintf("matviews:%s.%s", currentDB, schemaName),
				models.TreeNodeTypeMaterializedViewGroup,
				fmt.Sprintf("Materialized Views (%d)", len(sd.matViews)),
			)
			matViewsGroup.Selectable = false
			for _, matViewName := range sd.matViews {
				matViewNode := models.NewTreeNode(
					fmt.Sprintf("matview:%s.%s.%s", currentDB, schemaName, matViewName),
					models.TreeNodeTypeMaterializedView,
					matViewName,
				)
				matViewNode.Selectable = true
				matViewNode.Loaded = true // MatViews don't have children
				matViewsGroup.AddChild(matViewNode)
			}
			matViewsGroup.Loaded = true
			schemaNode.AddChild(matViewsGroup)
		}

		// Functions group with actual function nodes
		if len(sd.functions) > 0 {
			funcsGroup := models.NewTreeNode(
				fmt.Sprintf("functions:%s.%s", currentDB, schemaName),
				models.TreeNodeTypeFunctionGroup,
				fmt.Sprintf("Functions (%d)", len(sd.functions)),
			)
			funcsGroup.Selectable = false
			for _, f := range sd.functions {
				// Label includes arguments for unique identification (e.g., "my_func(integer, text)")
				funcLabel := fmt.Sprintf("%s(%s)", f.name, f.args)
				funcNode := models.NewTreeNode(
					fmt.Sprintf("function:%s.%s.%s", currentDB, schemaName, f.name),
					models.TreeNodeTypeFunction,
					funcLabel,
				)
				funcNode.Selectable = true
				funcNode.Loaded = true // Functions don't have children
				funcsGroup.AddChild(funcNode)
			}
			funcsGroup.Loaded = true
			schemaNode.AddChild(funcsGroup)
		}

		// Procedures group with actual procedure nodes
		if len(sd.procedures) > 0 {
			procsGroup := models.NewTreeNode(
				fmt.Sprintf("procedures:%s.%s", currentDB, schemaName),
				models.TreeNodeTypeProcedureGroup,
				fmt.Sprintf("Procedures (%d)", len(sd.procedures)),
			)
			procsGroup.Selectable = false
			for _, p := range sd.procedures {
				// Label includes arguments for unique identification (e.g., "my_proc(integer, text)")
				procLabel := fmt.Sprintf("%s(%s)", p.name, p.args)
				procNode := models.NewTreeNode(
					fmt.Sprintf("procedure:%s.%s.%s", currentDB, schemaName, p.name),
					models.TreeNodeTypeProcedure,
					procLabel,
				)
				procNode.Selectable = true
				procNode.Loaded = true // Procedures don't have children
				procsGroup.AddChild(procNode)
			}
			procsGroup.Loaded = true
			schemaNode.AddChild(procsGroup)
		}

		// Trigger Functions group with actual trigger function nodes
		if len(sd.triggerFunctions) > 0 {
			trigFuncsGroup := models.NewTreeNode(
				fmt.Sprintf("triggerfuncs:%s.%s", currentDB, schemaName),
				models.TreeNodeTypeTriggerFunctionGroup,
				fmt.Sprintf("Trigger Functions (%d)", len(sd.triggerFunctions)),
			)
			trigFuncsGroup.Selectable = false
			for _, trigFuncName := range sd.triggerFunctions {
				trigFuncNode := models.NewTreeNode(
					fmt.Sprintf("triggerfunction:%s.%s.%s", currentDB, schemaName, trigFuncName),
					models.TreeNodeTypeTriggerFunction,
					trigFuncName,
				)
				trigFuncNode.Selectable = true
				trigFuncNode.Loaded = true // Trigger functions don't have children
				trigFuncsGroup.AddChild(trigFuncNode)
			}
			trigFuncsGroup.Loaded = true
			schemaNode.AddChild(trigFuncsGroup)
		}

		// Sequences group with actual sequence nodes
		if len(sd.sequences) > 0 {
			seqsGroup := models.NewTreeNode(
				fmt.Sprintf("sequences:%s.%s", currentDB, schemaName),
				models.TreeNodeTypeSequenceGroup,
				fmt.Sprintf("Sequences (%d)", len(sd.sequences)),
			)
			seqsGroup.Selectable = false
			for _, seqName := range sd.sequences {
				seqNode := models.NewTreeNode(
					fmt.Sprintf("sequence:%s.%s.%s", currentDB, schemaName, seqName),
					models.TreeNodeTypeSequence,
					seqName,
				)
				seqNode.Selectable = true
				seqNode.Loaded = true // Sequences don't have children
				seqsGroup.AddChild(seqNode)
			}
			seqsGroup.Loaded = true
			schemaNode.AddChild(seqsGroup)
		}

		// Composite Types group with actual composite type nodes
		if len(sd.compositeTypes) > 0 {
			compTypesGroup := models.NewTreeNode(
				fmt.Sprintf("compositetypes:%s.%s", currentDB, schemaName),
				models.TreeNodeTypeCompositeTypeGroup,
				fmt.Sprintf("Composite Types (%d)", len(sd.compositeTypes)),
			)
			compTypesGroup.Selectable = false
			for _, compTypeName := range sd.compositeTypes {
				compTypeNode := models.NewTreeNode(
					fmt.Sprintf("compositetype:%s.%s.%s", currentDB, schemaName, compTypeName),
					models.TreeNodeTypeCompositeType,
					compTypeName,
				)
				compTypeNode.Selectable = true
				compTypeNode.Loaded = true // Composite types don't have children
				compTypesGroup.AddChild(compTypeNode)
			}
			compTypesGroup.Loaded = true
			schemaNode.AddChild(compTypesGroup)
		}

		// Enum Types group with actual enum type nodes
		if len(sd.enumTypes) > 0 {
			enumTypesGroup := models.NewTreeNode(
				fmt.Sprintf("enumtypes:%s.%s", currentDB, schemaName),
				models.TreeNodeTypeEnumTypeGroup,
				fmt.Sprintf("Enum Types (%d)", len(sd.enumTypes)),
			)
			enumTypesGroup.Selectable = false
			for _, enumTypeName := range sd.enumTypes {
				enumTypeNode := models.NewTreeNode(
					fmt.Sprintf("enumtype:%s.%s.%s", currentDB, schemaName, enumTypeName),
					models.TreeNodeTypeEnumType,
					enumTypeName,
				)
				enumTypeNode.Selectable = true
				enumTypeNode.Loaded = true // Enum types don't have children
				enumTypesGroup.AddChild(enumTypeNode)
			}
			enumTypesGroup.Loaded = true
			schemaNode.AddChild(enumTypesGroup)
		}

		// Domain Types group with actual domain type nodes
		if len(sd.domainTypes) > 0 {
			domainTypesGroup := models.NewTreeNode(
				fmt.Sprintf("domaintypes:%s.%s", currentDB, schemaName),
				models.TreeNodeTypeDomainTypeGroup,
				fmt.Sprintf("Domain Types (%d)", len(sd.domainTypes)),
			)
			domainTypesGroup.Selectable = false
			for _, domainTypeName := range sd.domainTypes {
				domainTypeNode := models.NewTreeNode(
					fmt.Sprintf("domaintype:%s.%s.%s", currentDB, schemaName, domainTypeName),
					models.TreeNodeTypeDomainType,
					domainTypeName,
				)
				domainTypeNode.Selectable = true
				domainTypeNode.Loaded = true // Domain types don't have children
				domainTypesGroup.AddChild(domainTypeNode)
			}
			domainTypesGroup.Loaded = true
			schemaNode.AddChild(domainTypesGroup)
		}

		// Range Types group with actual range type nodes
		if len(sd.rangeTypes) > 0 {
			rangeTypesGroup := models.NewTreeNode(
				fmt.Sprintf("rangetypes:%s.%s", currentDB, schemaName),
				models.TreeNodeTypeRangeTypeGroup,
				fmt.Sprintf("Range Types (%d)", len(sd.rangeTypes)),
			)
			rangeTypesGroup.Selectable = false
			for _, rangeTypeName := range sd.rangeTypes {
				rangeTypeNode := models.NewTreeNode(
					fmt.Sprintf("rangetype:%s.%s.%s", currentDB, schemaName, rangeTypeName),
					models.TreeNodeTypeRangeType,
					rangeTypeName,
				)
				rangeTypeNode.Selectable = true
				rangeTypeNode.Loaded = true // Range types don't have children
				rangeTypesGroup.AddChild(rangeTypeNode)
			}
			rangeTypesGroup.Loaded = true
			schemaNode.AddChild(rangeTypesGroup)
		}

		schemaNode.Loaded = true
		dbNode.AddChild(schemaNode)
	}

	return messages.TreeLoadedMsg{Root: root}
}

// loadNodeChildren loads children for table nodes (indexes and triggers).
// All other object types are pre-populated during initial tree load.
func (a *App) loadNodeChildren(nodeID string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		conn, err := a.connectionManager.GetActive()
		if err != nil {
			return messages.NodeChildrenLoadedMsg{NodeID: nodeID, Err: err}
		}

		// Parse node ID to determine what to load
		node := a.treeView.Root.FindByID(nodeID)
		if node == nil {
			return messages.NodeChildrenLoadedMsg{NodeID: nodeID, Err: fmt.Errorf("node not found: %s", nodeID)}
		}

		var children []*models.TreeNode
		currentDB := conn.Config.Database

		switch node.Type {
		case models.TreeNodeTypeTable:
			// Load indexes and triggers for a table
			schema, table := extractSchemaAndTableFromNodeID(nodeID)
			indexes, _ := metadata.ListTableIndexes(ctx, conn.Pool, schema, table)
			triggers, _ := metadata.ListTableTriggers(ctx, conn.Pool, schema, table)

			if len(indexes) > 0 {
				indexGroup := models.NewTreeNode(
					fmt.Sprintf("indexes:%s.%s.%s", currentDB, schema, table),
					models.TreeNodeTypeIndexGroup,
					fmt.Sprintf("Indexes (%d)", len(indexes)),
				)
				indexGroup.Selectable = false
				for _, idx := range indexes {
					idxNode := models.NewTreeNode(
						fmt.Sprintf("index:%s.%s.%s.%s", currentDB, schema, table, idx.Name),
						models.TreeNodeTypeIndex,
						idx.Name,
					)
					idxNode.Selectable = true
					idxNode.Metadata = idx
					idxNode.Loaded = true
					indexGroup.AddChild(idxNode)
				}
				indexGroup.Loaded = true
				children = append(children, indexGroup)
			}

			if len(triggers) > 0 {
				triggerGroup := models.NewTreeNode(
					fmt.Sprintf("triggers:%s.%s.%s", currentDB, schema, table),
					models.TreeNodeTypeTriggerGroup,
					fmt.Sprintf("Triggers (%d)", len(triggers)),
				)
				triggerGroup.Selectable = false
				for _, trg := range triggers {
					trgNode := models.NewTreeNode(
						fmt.Sprintf("trigger:%s.%s.%s.%s", currentDB, schema, table, trg.Name),
						models.TreeNodeTypeTrigger,
						trg.Name,
					)
					trgNode.Selectable = true
					trgNode.Metadata = trg
					trgNode.Loaded = true
					triggerGroup.AddChild(trgNode)
				}
				triggerGroup.Loaded = true
				children = append(children, triggerGroup)
			}
		}

		return messages.NodeChildrenLoadedMsg{NodeID: nodeID, Children: children}
	}
}

// extractSchemaAndTableFromNodeID extracts schema and table from node ID like "table:db.schema.table"
func extractSchemaAndTableFromNodeID(nodeID string) (string, string) {
	parts := strings.Split(nodeID, ":")
	if len(parts) < 2 {
		return "", ""
	}
	dbSchemaTable := strings.Split(parts[1], ".")
	if len(dbSchemaTable) < 3 {
		return "", ""
	}
	return dbSchemaTable[1], dbSchemaTable[2]
}

// loadTableData loads table data with pagination
func (a *App) loadTableData(msg messages.LoadTableDataMsg) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		conn, err := a.connectionManager.GetActive()
		if err != nil {
			return messages.TableDataLoadedMsg{Err: fmt.Errorf("no active connection: %w", err)}
		}

		var sort *metadata.SortOptions
		if msg.SortColumn != "" {
			sort = &metadata.SortOptions{
				Column:     msg.SortColumn,
				Direction:  msg.SortDir,
				NullsFirst: msg.NullsFirst,
			}
		}

		data, err := metadata.QueryTableData(ctx, conn.Pool, msg.Schema, msg.Table, msg.Offset, msg.Limit, sort)
		if err != nil {
			return messages.TableDataLoadedMsg{Err: err}
		}

		return messages.TableDataLoadedMsg{
			Columns:   data.Columns,
			Rows:      data.Rows,
			TotalRows: int(data.TotalRows),
			Offset:    msg.Offset,
		}
	}
}

// loadTableDataForTab loads table data for a specific tab
func (a *App) loadTableDataForTab(schema, table, objectID string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		conn, err := a.connectionManager.GetActive()
		if err != nil {
			return messages.TabTableDataLoadedMsg{ObjectID: objectID, Err: fmt.Errorf("no active connection: %w", err)}
		}

		data, err := metadata.QueryTableData(ctx, conn.Pool, schema, table, 0, 100, nil)
		if err != nil {
			return messages.TabTableDataLoadedMsg{ObjectID: objectID, Err: err}
		}

		return messages.TabTableDataLoadedMsg{
			ObjectID:  objectID,
			Schema:    schema,
			Table:     table,
			Columns:   data.Columns,
			Rows:      data.Rows,
			TotalRows: int(data.TotalRows),
		}
	}
}

// loadStructureMetadata loads columns, constraints, and indexes for a table asynchronously
func (a *App) loadStructureMetadata(schema, table, objectID string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		conn, err := a.connectionManager.GetActive()
		if err != nil {
			return messages.StructureMetadataLoadedMsg{ObjectID: objectID, Err: fmt.Errorf("no active connection: %w", err)}
		}

		columns, err := metadata.GetColumnDetails(ctx, conn.Pool, schema, table)
		if err != nil {
			return messages.StructureMetadataLoadedMsg{ObjectID: objectID, Err: err}
		}

		constraints, err := metadata.GetConstraints(ctx, conn.Pool, schema, table)
		if err != nil {
			return messages.StructureMetadataLoadedMsg{ObjectID: objectID, Err: err}
		}

		indexes, err := metadata.GetIndexes(ctx, conn.Pool, schema, table)
		if err != nil {
			return messages.StructureMetadataLoadedMsg{ObjectID: objectID, Err: err}
		}

		return messages.StructureMetadataLoadedMsg{
			ObjectID:    objectID,
			Columns:     columns,
			Constraints: constraints,
			Indexes:     indexes,
		}
	}
}

// triggerStructureMetadataLoad checks if the active tab needs metadata and triggers loading.
// Returns a command if loading was triggered, nil otherwise.
func (a *App) triggerStructureMetadataLoad(structure *components.StructureView, objectID string) tea.Cmd {
	if structure == nil || !structure.NeedsMetadata() {
		return nil
	}

	parts := strings.Split(objectID, ".")
	if len(parts) != 2 {
		return nil
	}

	structure.SetMetadataLoading()
	return a.loadStructureMetadata(parts[0], parts[1], objectID)
}

// loadTableDataWithFilter loads table data with an applied filter
func (a *App) loadTableDataWithFilter(filter models.Filter) tea.Cmd {
	return func() tea.Msg {
		conn, err := a.connectionManager.GetActive()
		if err != nil {
			return messages.ErrorMsg{Title: "Connection Error", Message: err.Error()}
		}

		node := a.state.TreeSelected
		if node == nil || node.Type != models.TreeNodeTypeTable {
			return messages.ErrorMsg{Title: "Error", Message: "No table selected"}
		}

		// Get schema from parent node
		schemaNode := node.Parent
		if schemaNode == nil {
			return messages.ErrorMsg{Title: "Error", Message: "Cannot determine schema"}
		}

		// Build filtered query
		builder := filterBuilder.NewBuilder()
		whereClause, args, err := builder.BuildWhere(filter)
		if err != nil {
			return messages.ErrorMsg{Title: "Filter Error", Message: err.Error()}
		}

		// Construct query
		query := fmt.Sprintf(
			`SELECT * FROM "%s"."%s" %s LIMIT 100`,
			schemaNode.Label,
			node.Label,
			whereClause,
		)

		// Execute query
		result, err := conn.Pool.QueryWithColumns(context.Background(), query, args...)
		if err != nil {
			return messages.ErrorMsg{Title: "Query Error", Message: err.Error()}
		}

		// Convert to string rows for display
		var rows [][]string
		for _, row := range result.Rows {
			var strRow []string
			for _, col := range result.Columns {
				val := row[col]
				if val == nil {
					strRow = append(strRow, "NULL")
				} else {
					strRow = append(strRow, fmt.Sprintf("%v", val))
				}
			}
			rows = append(rows, strRow)
		}

		return messages.TableDataLoadedMsg{
			Columns:   result.Columns,
			Rows:      rows,
			TotalRows: len(rows),
			Offset:    0,
		}
	}
}

// ShowError displays an error overlay with the given title and message
func (a *App) ShowError(title, message string) {
	a.errorOverlay.SetError(title, message)
	a.showError = true
}

// DismissError hides the error overlay
func (a *App) DismissError() {
	a.showError = false
}

// overlayCommandPalette renders the command palette as an overlay on top of background
func (a *App) overlayCommandPalette(background string) string {
	paletteView := a.commandPalette.View()
	paletteLines := strings.Split(paletteView, "\n")
	bgLines := strings.Split(background, "\n")

	// Calculate center position
	paletteHeight := len(paletteLines)
	paletteWidth := lipgloss.Width(paletteLines[0]) // Use first line width

	startY := (a.state.Height - paletteHeight) / 2
	startX := (a.state.Width - paletteWidth) / 2

	if startY < 0 {
		startY = 0
	}
	if startX < 0 {
		startX = 0
	}

	// Overlay palette on background
	result := make([]string, len(bgLines))
	for i, bgLine := range bgLines {
		if i >= startY && i < startY+paletteHeight {
			paletteLineIdx := i - startY
			if paletteLineIdx < len(paletteLines) {
				// Overlay this palette line onto background
				result[i] = a.overlayLine(bgLine, paletteLines[paletteLineIdx], startX)
			} else {
				result[i] = bgLine
			}
		} else {
			result[i] = bgLine
		}
	}

	return strings.Join(result, "\n")
}

// overlaySearchInput renders the search input as an overlay on top of background
func (a *App) overlaySearchInput(background string) string {
	searchView := a.searchInput.View()
	searchLines := strings.Split(searchView, "\n")
	bgLines := strings.Split(background, "\n")

	// Calculate center position
	searchHeight := len(searchLines)
	searchWidth := lipgloss.Width(searchLines[0]) // Use first line width

	startY := (a.state.Height - searchHeight) / 2
	startX := (a.state.Width - searchWidth) / 2

	if startY < 0 {
		startY = 0
	}
	if startX < 0 {
		startX = 0
	}

	// Overlay search box on background
	result := make([]string, len(bgLines))
	for i, bgLine := range bgLines {
		if i >= startY && i < startY+searchHeight {
			searchLineIdx := i - startY
			if searchLineIdx < len(searchLines) {
				// Overlay this search line onto background
				result[i] = a.overlayLine(bgLine, searchLines[searchLineIdx], startX)
			} else {
				result[i] = bgLine
			}
		} else {
			result[i] = bgLine
		}
	}

	return strings.Join(result, "\n")
}

// overlayLine overlays foreground onto background at given x position
// Handles ANSI escape sequences correctly
func (a *App) overlayLine(background, foreground string, startX int) string {
	fgWidth := ansi.StringWidth(foreground)

	// Truncate background to startX (visual width)
	leftPart := ansi.Truncate(background, startX, "")

	// Get visible width of left part
	leftWidth := ansi.StringWidth(leftPart)

	// Pad if needed
	if leftWidth < startX {
		leftPart += strings.Repeat(" ", startX-leftWidth)
	}

	// Cut the right part of background after the overlay
	rightStart := startX + fgWidth
	bgWidth := ansi.StringWidth(background)

	var rightPart string
	if rightStart < bgWidth {
		// Cut from background starting at rightStart position
		rightPart = ansi.Cut(background, rightStart, bgWidth)
	}

	return leftPart + foreground + rightPart
}

// SearchTableResultMsg is sent when table search completes
type SearchTableResultMsg struct {
	Query string
	Data  *metadata.TableData
	Err   error
}

// searchTable executes a table-wide search
func (a *App) searchTable(query string) tea.Cmd {
	return func() tea.Msg {
		conn, err := a.connectionManager.GetActive()
		if err != nil {
			return SearchTableResultMsg{Query: query, Err: fmt.Errorf("no active connection: %w", err)}
		}

		parts := strings.Split(a.currentTable, ".")
		if len(parts) != 2 {
			return SearchTableResultMsg{Query: query, Err: fmt.Errorf("invalid table: %s", a.currentTable)}
		}

		schema, table := parts[0], parts[1]

		data, err := metadata.SearchTableData(
			context.Background(),
			conn.Pool,
			schema,
			table,
			a.tableView.Columns,
			query,
			500, // Max results
		)
		if err != nil {
			return SearchTableResultMsg{Query: query, Err: err}
		}

		return SearchTableResultMsg{Query: query, Data: data}
	}
}

// openExternalEditor opens the content in an external editor
func (a *App) openExternalEditor(content string) tea.Cmd {
	return func() tea.Msg {
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "vim"
		}

		// Create temp file
		tmpFile, err := os.CreateTemp("", "lazypg-*.sql")
		if err != nil {
			return components.ExternalEditorResultMsg{Error: err}
		}
		defer func() { _ = os.Remove(tmpFile.Name()) }()

		// Write content
		if _, err := tmpFile.WriteString(content); err != nil {
			return components.ExternalEditorResultMsg{Error: err}
		}
		_ = tmpFile.Close()

		// Open editor
		cmd := exec.Command(editor, tmpFile.Name())
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			return components.ExternalEditorResultMsg{Error: err}
		}

		// Read result
		result, err := os.ReadFile(tmpFile.Name())
		if err != nil {
			return components.ExternalEditorResultMsg{Error: err}
		}

		return components.ExternalEditorResultMsg{Content: string(result)}
	}
}

// getSchemaFromNode traverses up the tree to find the schema name
func (a *App) getSchemaFromNode(node *models.TreeNode) string {
	current := node.Parent
	for current != nil {
		if current.Type == models.TreeNodeTypeSchema {
			// Schema label might have count info, split by space
			return strings.Split(current.Label, " ")[0]
		}
		current = current.Parent
	}
	return ""
}

// getTableFromNode traverses up the tree to find the table name (for indexes/triggers)
func (a *App) getTableFromNode(node *models.TreeNode) string {
	current := node.Parent
	for current != nil {
		if current.Type == models.TreeNodeTypeTable {
			return current.Label
		}
		// Skip group nodes (IndexGroup, TriggerGroup)
		current = current.Parent
	}
	return ""
}

// loadFunctionSource loads the source code of a function or procedure
func (a *App) loadFunctionSource(node *models.TreeNode) tea.Cmd {
	return func() tea.Msg {
		conn, err := a.connectionManager.GetActive()
		if err != nil {
			return messages.ObjectDetailsLoadedMsg{ObjectType: "function", Err: err}
		}

		schema := a.getSchemaFromNode(node)
		if schema == "" {
			return messages.ObjectDetailsLoadedMsg{ObjectType: "function", Err: fmt.Errorf("could not determine schema")}
		}

		// Parse function name and arguments from node label
		// Label format: "function_name(args)" or just "function_name"
		name := node.Label
		args := ""
		if idx := strings.Index(name, "("); idx != -1 {
			args = name[idx+1 : len(name)-1] // Remove parentheses
			name = name[:idx]
		}

		ctx := context.Background()
		source, err := metadata.GetFunctionSource(ctx, conn.Pool, schema, name, args)
		if err != nil {
			return messages.ObjectDetailsLoadedMsg{ObjectType: "function", Err: err}
		}

		title := fmt.Sprintf("%s.%s(%s)", schema, source.Name, source.Arguments)
		content := source.Definition
		objectID := fmt.Sprintf("func:%s.%s(%s)", schema, source.Name, source.Arguments)

		return messages.ObjectDetailsLoadedMsg{
			ObjectType: "function",
			ObjectName: fmt.Sprintf("%s.%s(%s)", schema, source.Name, source.Arguments),
			ObjectID:   objectID,
			Title:      title,
			Content:    content,
		}
	}
}

// loadTriggerFunctionSource loads the source code of a trigger function
func (a *App) loadTriggerFunctionSource(node *models.TreeNode) tea.Cmd {
	return func() tea.Msg {
		conn, err := a.connectionManager.GetActive()
		if err != nil {
			return messages.ObjectDetailsLoadedMsg{ObjectType: "trigger_function", Err: err}
		}

		schema := a.getSchemaFromNode(node)
		if schema == "" {
			return messages.ObjectDetailsLoadedMsg{ObjectType: "trigger_function", Err: fmt.Errorf("could not determine schema")}
		}

		ctx := context.Background()
		source, err := metadata.GetTriggerFunctionSource(ctx, conn.Pool, schema, node.Label)
		if err != nil {
			return messages.ObjectDetailsLoadedMsg{ObjectType: "trigger_function", Err: err}
		}

		title := fmt.Sprintf("%s.%s() → trigger", schema, source.Name)
		content := source.Definition
		objectID := fmt.Sprintf("trigfunc:%s.%s", schema, source.Name)

		return messages.ObjectDetailsLoadedMsg{
			ObjectType: "trigger_function",
			ObjectName: fmt.Sprintf("%s.%s", schema, source.Name),
			ObjectID:   objectID,
			Title:      title,
			Content:    content,
		}
	}
}

// loadSequenceDetails loads sequence properties
func (a *App) loadSequenceDetails(node *models.TreeNode) tea.Cmd {
	return func() tea.Msg {
		conn, err := a.connectionManager.GetActive()
		if err != nil {
			return messages.ObjectDetailsLoadedMsg{ObjectType: "sequence", Err: err}
		}

		schema := a.getSchemaFromNode(node)
		if schema == "" {
			return messages.ObjectDetailsLoadedMsg{ObjectType: "sequence", Err: fmt.Errorf("could not determine schema")}
		}

		ctx := context.Background()
		details, err := metadata.GetSequenceDetails(ctx, conn.Pool, schema, node.Label)
		if err != nil {
			return messages.ObjectDetailsLoadedMsg{ObjectType: "sequence", Err: err}
		}

		// Format as CREATE SEQUENCE statement
		var b strings.Builder
		b.WriteString(fmt.Sprintf("-- Current Value: %d\n", details.CurrentValue))
		if details.Owner != "" {
			b.WriteString(fmt.Sprintf("-- Owner: %s\n", details.Owner))
		}
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("CREATE SEQUENCE %s.%s\n", schema, details.Name))
		b.WriteString(fmt.Sprintf("    INCREMENT BY %d\n", details.Increment))
		b.WriteString(fmt.Sprintf("    MINVALUE %d\n", details.MinValue))
		b.WriteString(fmt.Sprintf("    MAXVALUE %d\n", details.MaxValue))
		b.WriteString(fmt.Sprintf("    START WITH %d\n", details.StartValue))
		if details.Cycle {
			b.WriteString("    CYCLE")
		} else {
			b.WriteString("    NO CYCLE")
		}
		b.WriteString(";")

		objectID := fmt.Sprintf("seq:%s.%s", schema, details.Name)
		return messages.ObjectDetailsLoadedMsg{
			ObjectType: "sequence",
			ObjectID:   objectID,
			Title:      fmt.Sprintf("%s.%s", schema, details.Name),
			Content:    b.String(),
		}
	}
}

// loadIndexDetails loads index DDL definition
func (a *App) loadIndexDetails(node *models.TreeNode) tea.Cmd {
	return func() tea.Msg {
		conn, err := a.connectionManager.GetActive()
		if err != nil {
			return messages.ObjectDetailsLoadedMsg{ObjectType: "index", Err: err}
		}

		schema := a.getSchemaFromNode(node)
		table := a.getTableFromNode(node)
		if schema == "" || table == "" {
			return messages.ObjectDetailsLoadedMsg{ObjectType: "index", Err: fmt.Errorf("could not determine schema/table")}
		}

		ctx := context.Background()
		indexes, err := metadata.ListTableIndexes(ctx, conn.Pool, schema, table)
		if err != nil {
			return messages.ObjectDetailsLoadedMsg{ObjectType: "index", Err: err}
		}

		// Find the specific index
		var content string
		for _, idx := range indexes {
			if idx.Name == node.Label {
				content = idx.Definition
				break
			}
		}

		if content == "" {
			return messages.ObjectDetailsLoadedMsg{ObjectType: "index", Err: fmt.Errorf("index %s not found", node.Label)}
		}

		objectID := fmt.Sprintf("idx:%s.%s.%s", schema, table, node.Label)
		return messages.ObjectDetailsLoadedMsg{
			ObjectType: "index",
			ObjectID:   objectID,
			Title:      fmt.Sprintf("%s.%s (on %s)", schema, node.Label, table),
			Content:    content,
		}
	}
}

// loadTriggerDetails loads trigger DDL definition
func (a *App) loadTriggerDetails(node *models.TreeNode) tea.Cmd {
	return func() tea.Msg {
		conn, err := a.connectionManager.GetActive()
		if err != nil {
			return messages.ObjectDetailsLoadedMsg{ObjectType: "trigger", Err: err}
		}

		schema := a.getSchemaFromNode(node)
		table := a.getTableFromNode(node)
		if schema == "" || table == "" {
			return messages.ObjectDetailsLoadedMsg{ObjectType: "trigger", Err: fmt.Errorf("could not determine schema/table")}
		}

		ctx := context.Background()
		triggers, err := metadata.ListTableTriggers(ctx, conn.Pool, schema, table)
		if err != nil {
			return messages.ObjectDetailsLoadedMsg{ObjectType: "trigger", Err: err}
		}

		// Find the specific trigger
		var content string
		for _, trg := range triggers {
			if trg.Name == node.Label {
				content = trg.Definition
				break
			}
		}

		if content == "" {
			return messages.ObjectDetailsLoadedMsg{ObjectType: "trigger", Err: fmt.Errorf("trigger %s not found", node.Label)}
		}

		objectID := fmt.Sprintf("trig:%s.%s.%s", schema, table, node.Label)
		return messages.ObjectDetailsLoadedMsg{
			ObjectType: "trigger",
			ObjectID:   objectID,
			Title:      fmt.Sprintf("%s.%s (on %s)", schema, node.Label, table),
			Content:    content,
		}
	}
}

// loadExtensionDetails loads extension information
func (a *App) loadExtensionDetails(node *models.TreeNode) tea.Cmd {
	return func() tea.Msg {
		conn, err := a.connectionManager.GetActive()
		if err != nil {
			return messages.ObjectDetailsLoadedMsg{ObjectType: "extension", Err: err}
		}

		// Parse extension name from label (format: "name vX.Y")
		name := node.Label
		if idx := strings.Index(name, " v"); idx != -1 {
			name = name[:idx]
		}

		ctx := context.Background()
		details, err := metadata.GetExtensionDetails(ctx, conn.Pool, name)
		if err != nil {
			return messages.ObjectDetailsLoadedMsg{ObjectType: "extension", Err: err}
		}

		// Format as CREATE EXTENSION statement
		var b strings.Builder
		b.WriteString(fmt.Sprintf("-- Extension: %s\n", details.Name))
		b.WriteString(fmt.Sprintf("-- Version: %s\n", details.Version))
		if details.Description != "" {
			b.WriteString(fmt.Sprintf("-- %s\n", details.Description))
		}
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("CREATE EXTENSION IF NOT EXISTS %s\n", details.Name))
		b.WriteString(fmt.Sprintf("    SCHEMA %s\n", details.Schema))
		b.WriteString(fmt.Sprintf("    VERSION '%s';", details.Version))

		objectID := fmt.Sprintf("ext:%s", details.Name)
		return messages.ObjectDetailsLoadedMsg{
			ObjectType: "extension",
			ObjectID:   objectID,
			Title:      details.Name,
			Content:    b.String(),
		}
	}
}

// loadCompositeTypeDetails loads composite type definition
func (a *App) loadCompositeTypeDetails(node *models.TreeNode) tea.Cmd {
	return func() tea.Msg {
		conn, err := a.connectionManager.GetActive()
		if err != nil {
			return messages.ObjectDetailsLoadedMsg{ObjectType: "type", Err: err}
		}

		schema := a.getSchemaFromNode(node)
		if schema == "" {
			return messages.ObjectDetailsLoadedMsg{ObjectType: "type", Err: fmt.Errorf("could not determine schema")}
		}

		ctx := context.Background()
		details, err := metadata.GetCompositeTypeDetails(ctx, conn.Pool, schema, node.Label)
		if err != nil {
			return messages.ObjectDetailsLoadedMsg{ObjectType: "type", Err: err}
		}

		// Format as CREATE TYPE statement
		var b strings.Builder
		b.WriteString(fmt.Sprintf("CREATE TYPE %s.%s AS (\n", schema, details.Name))
		for i, attr := range details.Attributes {
			comma := ","
			if i == len(details.Attributes)-1 {
				comma = ""
			}
			b.WriteString(fmt.Sprintf("    %s %s%s\n", attr.Name, attr.Type, comma))
		}
		b.WriteString(");")

		objectID := fmt.Sprintf("type:composite:%s.%s", schema, details.Name)
		return messages.ObjectDetailsLoadedMsg{
			ObjectType: "type",
			ObjectID:   objectID,
			Title:      fmt.Sprintf("%s.%s (composite)", schema, details.Name),
			Content:    b.String(),
		}
	}
}

// loadEnumTypeDetails loads enum type definition
func (a *App) loadEnumTypeDetails(node *models.TreeNode) tea.Cmd {
	return func() tea.Msg {
		conn, err := a.connectionManager.GetActive()
		if err != nil {
			return messages.ObjectDetailsLoadedMsg{ObjectType: "type", Err: err}
		}

		schema := a.getSchemaFromNode(node)
		if schema == "" {
			return messages.ObjectDetailsLoadedMsg{ObjectType: "type", Err: fmt.Errorf("could not determine schema")}
		}

		ctx := context.Background()
		enums, err := metadata.ListEnumTypes(ctx, conn.Pool, schema)
		if err != nil {
			return messages.ObjectDetailsLoadedMsg{ObjectType: "type", Err: err}
		}

		// Find the specific enum
		var enumType *metadata.EnumType
		for _, e := range enums {
			if e.Name == node.Label {
				enumType = &e
				break
			}
		}

		if enumType == nil {
			return messages.ObjectDetailsLoadedMsg{ObjectType: "type", Err: fmt.Errorf("enum type %s not found", node.Label)}
		}

		// Format as CREATE TYPE statement
		var b strings.Builder
		b.WriteString(fmt.Sprintf("CREATE TYPE %s.%s AS ENUM (\n", schema, enumType.Name))
		for i, label := range enumType.Labels {
			comma := ","
			if i == len(enumType.Labels)-1 {
				comma = ""
			}
			b.WriteString(fmt.Sprintf("    '%s'%s\n", label, comma))
		}
		b.WriteString(");")

		objectID := fmt.Sprintf("type:enum:%s.%s", schema, enumType.Name)
		return messages.ObjectDetailsLoadedMsg{
			ObjectType: "type",
			ObjectID:   objectID,
			Title:      fmt.Sprintf("%s.%s (enum)", schema, enumType.Name),
			Content:    b.String(),
		}
	}
}

// loadDomainTypeDetails loads domain type definition
func (a *App) loadDomainTypeDetails(node *models.TreeNode) tea.Cmd {
	return func() tea.Msg {
		conn, err := a.connectionManager.GetActive()
		if err != nil {
			return messages.ObjectDetailsLoadedMsg{ObjectType: "type", Err: err}
		}

		schema := a.getSchemaFromNode(node)
		if schema == "" {
			return messages.ObjectDetailsLoadedMsg{ObjectType: "type", Err: fmt.Errorf("could not determine schema")}
		}

		ctx := context.Background()
		details, err := metadata.GetDomainTypeDetails(ctx, conn.Pool, schema, node.Label)
		if err != nil {
			return messages.ObjectDetailsLoadedMsg{ObjectType: "type", Err: err}
		}

		// Format as CREATE DOMAIN statement
		var b strings.Builder
		b.WriteString(fmt.Sprintf("CREATE DOMAIN %s.%s AS %s", schema, details.Name, details.BaseType))
		if details.NotNull {
			b.WriteString(" NOT NULL")
		}
		if details.Default != "" {
			b.WriteString(fmt.Sprintf(" DEFAULT %s", details.Default))
		}
		for _, constraint := range details.Constraints {
			b.WriteString(fmt.Sprintf("\n    %s", constraint))
		}
		b.WriteString(";")

		objectID := fmt.Sprintf("type:domain:%s.%s", schema, details.Name)
		return messages.ObjectDetailsLoadedMsg{
			ObjectType: "type",
			ObjectID:   objectID,
			Title:      fmt.Sprintf("%s.%s (domain)", schema, details.Name),
			Content:    b.String(),
		}
	}
}

// loadRangeTypeDetails loads range type definition
func (a *App) loadRangeTypeDetails(node *models.TreeNode) tea.Cmd {
	return func() tea.Msg {
		conn, err := a.connectionManager.GetActive()
		if err != nil {
			return messages.ObjectDetailsLoadedMsg{ObjectType: "type", Err: err}
		}

		schema := a.getSchemaFromNode(node)
		if schema == "" {
			return messages.ObjectDetailsLoadedMsg{ObjectType: "type", Err: fmt.Errorf("could not determine schema")}
		}

		ctx := context.Background()
		ranges, err := metadata.ListRangeTypes(ctx, conn.Pool, schema)
		if err != nil {
			return messages.ObjectDetailsLoadedMsg{ObjectType: "type", Err: err}
		}

		// Find the specific range type
		var rangeType *metadata.RangeType
		for _, r := range ranges {
			if r.Name == node.Label {
				rangeType = &r
				break
			}
		}

		if rangeType == nil {
			return messages.ObjectDetailsLoadedMsg{ObjectType: "type", Err: fmt.Errorf("range type %s not found", node.Label)}
		}

		// Format as CREATE TYPE statement
		content := fmt.Sprintf("CREATE TYPE %s.%s AS RANGE (\n    SUBTYPE = %s\n);",
			schema, rangeType.Name, rangeType.Subtype)

		objectID := fmt.Sprintf("type:range:%s.%s", schema, rangeType.Name)
		return messages.ObjectDetailsLoadedMsg{
			ObjectType: "type",
			ObjectID:   objectID,
			Title:      fmt.Sprintf("%s.%s (range)", schema, rangeType.Name),
			Content:    content,
		}
	}
}

// saveObjectDefinition executes the SQL to save an object definition
func (a *App) saveObjectDefinition(msg components.SaveObjectMsg) tea.Cmd {
	return func() tea.Msg {
		conn, err := a.connectionManager.GetActive()
		if err != nil {
			return components.ObjectSavedMsg{Success: false, Error: err}
		}

		ctx := context.Background()

		// The content should be the full CREATE OR REPLACE statement for functions/procedures
		// For other object types, we may need to generate appropriate SQL
		sql := msg.Content

		_, err = conn.Pool.Execute(ctx, sql)
		if err != nil {
			return components.ObjectSavedMsg{Success: false, Error: err}
		}

		return components.ObjectSavedMsg{Success: true}
	}
}
