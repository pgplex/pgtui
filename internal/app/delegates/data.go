package delegates

import (
	"fmt"
	"log"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/pgplex/pgtui/internal/app/messages"
	"github.com/pgplex/pgtui/internal/models"
	"github.com/pgplex/pgtui/internal/ui/components"
)

// DataDelegate handles table data loading and display messages.
type DataDelegate struct{}

// NewDataDelegate creates a new DataDelegate.
func NewDataDelegate() *DataDelegate {
	return &DataDelegate{}
}

// Name returns the delegate name.
func (d *DataDelegate) Name() string {
	return "data"
}

// Update processes data-related messages.
func (d *DataDelegate) Update(msg tea.Msg, app AppAccess) (bool, tea.Cmd) {
	switch msg := msg.(type) {

	case messages.LoadTableDataMsg:
		return true, app.LoadTableData(msg.Schema, msg.Table, msg.Offset, msg.Limit, msg.SortColumn, msg.SortDir, msg.NullsFirst)

	case messages.TableDataLoadedMsg:
		return d.handleTableDataLoaded(msg, app)

	case messages.TabTableDataLoadedMsg:
		return d.handleTabTableDataLoaded(msg, app)

	case messages.PrefetchDataMsg:
		return true, app.PrefetchData(msg.Schema, msg.Table, msg.Offset, msg.Limit, msg.SortColumn, msg.SortDir, msg.NullsFirst)

	case messages.PrefetchCompleteMsg:
		return d.handlePrefetchComplete(msg, app)

	case messages.StructureMetadataLoadedMsg:
		return d.handleStructureMetadataLoaded(msg, app)
	}

	return false, nil
}

// handleTableDataLoaded handles table data loading completion.
func (d *DataDelegate) handleTableDataLoaded(msg messages.TableDataLoadedMsg, app AppAccess) (bool, tea.Cmd) {
	if msg.Err != nil {
		app.ShowError("Database Error", fmt.Sprintf("Failed to load table data:\n\n%v", msg.Err))
		return true, nil
	}

	tableView := app.GetTableView()

	// Check if this is initial load or pagination
	// Initial load if:
	// 1. No existing rows (first load ever)
	// 2. Offset is 0 (fresh load request, even for same table)
	// 3. Columns changed (different table selected)
	isInitialLoad := len(tableView.Rows) == 0 ||
		msg.Offset == 0 ||
		(len(msg.Columns) > 0 && len(tableView.Columns) > 0 && msg.Columns[0] != tableView.Columns[0])

	if isInitialLoad {
		// Initial load - replace all data
		tableView.SetData(msg.Columns, msg.Rows, msg.TotalRows)
		tableView.SelectedRow = 0
		tableView.TopRow = 0
		app.SetFocusArea(models.FocusDataPanel)
		app.UpdatePanelStyles()
	} else {
		// Append paginated data (same table, loading more rows)
		tableView.Rows = append(tableView.Rows, msg.Rows...)
		tableView.TotalRows = msg.TotalRows
	}
	tableView.IsPaginating = false
	return true, nil
}

// handleTabTableDataLoaded handles table data loading for a specific tab.
func (d *DataDelegate) handleTabTableDataLoaded(msg messages.TabTableDataLoadedMsg, app AppAccess) (bool, tea.Cmd) {
	resultTabs := app.GetResultTabs()

	// Clear loading state for the tab's table view
	if tab := resultTabs.GetTabByObjectID(msg.ObjectID); tab != nil {
		if sv := tab.Structure; sv != nil {
			if tv := sv.GetTableView(); tv != nil {
				tv.IsLoading = false
			}
		}
	}

	if msg.Err != nil {
		app.ShowError("Database Error", fmt.Sprintf("Failed to load table data:\n\n%v", msg.Err))
		return true, nil
	}

	// Find the tab with this objectID and update its data
	for _, tab := range resultTabs.GetAllTabs() {
		if tab.ObjectID == msg.ObjectID && tab.Type == components.TabTypeTableData {
			if tab.Structure != nil {
				// Set table data in the structure view
				tab.Structure.GetTableView().SetData(msg.Columns, msg.Rows, msg.TotalRows)
				// Note: Structure metadata (columns, constraints, indexes) is loaded
				// lazily when user switches to those tabs to avoid blocking the UI
			}
			break
		}
	}
	app.SetFocusArea(models.FocusDataPanel)
	app.UpdatePanelStyles()
	return true, nil
}

// handleStructureMetadataLoaded handles structure metadata loading completion.
func (d *DataDelegate) handleStructureMetadataLoaded(msg messages.StructureMetadataLoadedMsg, app AppAccess) (bool, tea.Cmd) {
	resultTabs := app.GetResultTabs()

	tab := resultTabs.GetTabByObjectID(msg.ObjectID)
	if tab == nil || tab.Structure == nil {
		return true, nil
	}

	if msg.Err != nil {
		log.Printf("Warning: failed to load structure metadata for %s: %v", msg.ObjectID, msg.Err)
		return true, nil
	}

	tab.Structure.SetMetadata(msg.Columns, msg.Constraints, msg.Indexes)
	return true, nil
}

// handlePrefetchComplete handles prefetch data completion.
func (d *DataDelegate) handlePrefetchComplete(msg messages.PrefetchCompleteMsg, app AppAccess) (bool, tea.Cmd) {
	tableView := app.GetActiveTableView()
	if tableView == nil {
		return true, nil
	}

	tableView.IsPrefetching = false
	tableView.IsPaginating = false

	if msg.Err != nil {
		// Silent fail for prefetch - don't show error to user
		log.Printf("Warning: prefetch failed at offset %d: %v", msg.Offset, msg.Err)
		return true, nil
	}

	// Append prefetched rows
	tableView.Rows = append(tableView.Rows, msg.Rows...)

	return true, nil
}
