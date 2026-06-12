package delegates

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/pgplex/pgtui/internal/app/messages"
	"github.com/pgplex/pgtui/internal/models"
	"github.com/pgplex/pgtui/internal/ui/components"
)

// DialogDelegate handles dialog and overlay messages.
type DialogDelegate struct{}

// NewDialogDelegate creates a new DialogDelegate.
func NewDialogDelegate() *DialogDelegate {
	return &DialogDelegate{}
}

// Name returns the delegate name.
func (d *DialogDelegate) Name() string {
	return "dialog"
}

// Update processes dialog-related messages.
func (d *DialogDelegate) Update(msg tea.Msg, app AppAccess) (bool, tea.Cmd) {
	switch msg := msg.(type) {

	case components.ApplyFilterMsg:
		return d.handleApplyFilter(msg, app)

	case components.CloseFilterBuilderMsg:
		app.SetShowFilterBuilder(false)
		return true, nil

	case components.CloseJSONBViewerMsg:
		app.SetShowJSONBViewer(false)
		return true, nil

	case components.CloseErrorOverlayMsg:
		app.SetShowError(false)
		return true, nil

	case components.CloseCommandPaletteMsg:
		app.SetShowCommandPalette(false)
		return true, nil

	case components.CloseFavoritesDialogMsg:
		app.SetShowFavorites(false)
		return true, nil

	case components.SearchInputMsg:
		return d.handleSearchInput(msg, app)

	case components.CloseSearchMsg:
		app.SetShowSearch(false)
		app.GetSearchInput().Reset()
		return true, nil

	case messages.SearchTableResultMsg:
		return d.handleSearchTableResult(msg, app)
	}

	return false, nil
}

// handleApplyFilter handles applying a filter to the table data.
func (d *DialogDelegate) handleApplyFilter(msg components.ApplyFilterMsg, app AppAccess) (bool, tea.Cmd) {
	app.SetShowFilterBuilder(false)
	app.SetActiveFilter(&msg.Filter)

	// Reload table with filter
	state := app.GetState()
	if state.TreeSelected != nil && state.TreeSelected.Type == models.TreeNodeTypeTable {
		return true, app.LoadTableDataWithFilter(msg.Filter)
	}
	return true, nil
}

// handleSearchInput handles search input from the search dialog.
func (d *DialogDelegate) handleSearchInput(msg components.SearchInputMsg, app AppAccess) (bool, tea.Cmd) {
	app.SetShowSearch(false)
	if msg.Query == "" {
		return true, nil
	}

	// Get the active table view (Result Tabs or main TableView)
	activeTable := app.GetActiveTableView()

	if msg.Mode == "local" {
		// Local search - search only loaded data
		if activeTable != nil {
			activeTable.SearchLocal(msg.Query)
		}
		return true, nil
	}

	// For Result Tabs, always use local search (data is already loaded)
	resultTabs := app.GetResultTabs()
	if resultTabs.HasTabs() {
		if activeTable != nil {
			activeTable.SearchLocal(msg.Query)
		}
		return true, nil
	}

	// Table search - query the database (only for table browser)
	state := app.GetState()
	if state.ActiveConnection == nil {
		app.ShowError("No Connection", "Please connect to a database first")
		return true, nil
	}

	currentTable := app.GetCurrentTable()
	if currentTable == "" {
		app.ShowError("No Table", "Please select a table first")
		return true, nil
	}

	// Execute table search
	return true, app.SearchTable(msg.Query)
}

// handleSearchTableResult handles search result from the database.
func (d *DialogDelegate) handleSearchTableResult(msg messages.SearchTableResultMsg, app AppAccess) (bool, tea.Cmd) {
	if msg.Err != nil {
		app.ShowError("Search Error", msg.Err.Error())
		return true, nil
	}

	if msg.Data == nil || len(msg.Data.Rows) == 0 {
		app.ShowError("No Results", fmt.Sprintf("No matches found for '%s'", msg.Query))
		return true, nil
	}

	// Update table view with search results
	tableView := app.GetTableView()
	tableView.SetData(msg.Data.Columns, msg.Data.Rows, msg.Data.TotalRows)
	tableView.SelectedRow = 0
	tableView.TopRow = 0
	app.SetFocusArea(models.FocusDataPanel)
	app.UpdatePanelStyles()

	return true, nil
}
