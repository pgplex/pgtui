package delegates

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/pgplex/pgtui/internal/app/messages"
	"github.com/pgplex/pgtui/internal/models"
	"github.com/pgplex/pgtui/internal/ui/components"
)

// QueryDelegate handles query execution and result messages.
type QueryDelegate struct{}

// NewQueryDelegate creates a new QueryDelegate.
func NewQueryDelegate() *QueryDelegate {
	return &QueryDelegate{}
}

// Name returns the delegate name.
func (d *QueryDelegate) Name() string {
	return "query"
}

// Update processes query-related messages.
func (d *QueryDelegate) Update(msg tea.Msg, app AppAccess) (bool, tea.Cmd) {
	switch msg := msg.(type) {

	case components.ExecuteQueryMsg:
		return d.handleExecuteQuery(msg, app)

	case messages.QueryResultMsg:
		return d.handleQueryResult(msg, app)

	case components.SaveObjectMsg:
		return d.handleSaveObject(msg, app)

	case components.ObjectSavedMsg:
		return d.handleObjectSaved(msg, app)
	}

	return false, nil
}

// handleExecuteQuery handles query execution from SQL editor.
func (d *QueryDelegate) handleExecuteQuery(msg components.ExecuteQueryMsg, app AppAccess) (bool, tea.Cmd) {
	if app.GetState().ActiveConnection == nil {
		app.ShowError("No Connection", "Please connect to a database first")
		return true, nil
	}

	// Create pending tab immediately
	app.StartPendingQuery(msg.SQL)

	// Immediately switch focus to data panel and collapse editor
	app.GetSQLEditor().Collapse()
	app.SetFocusArea(models.FocusDataPanel)
	app.UpdatePanelStyles()

	// Execute query asynchronously and start spinner
	return true, tea.Batch(
		app.GetSpinnerTickCmd(),
		app.ExecuteQuery(msg.SQL),
	)
}

// handleQueryResult handles query execution result.
func (d *QueryDelegate) handleQueryResult(msg messages.QueryResultMsg, app AppAccess) (bool, tea.Cmd) {
	// Clear execution cancel function
	app.SetExecuteCancelFn(nil)

	// Handle query result
	if msg.Result.Error != nil {
		// Check if it was cancelled (context cancelled error)
		if msg.Result.Error.Error() == "context canceled" {
			// Already handled by CancelPendingQuery, just return
			return true, nil
		}
		// Show error and remove pending tab
		app.CancelPendingQuery()
		app.ShowError("Query Error", msg.Result.Error.Error())
		return true, nil
	}

	// Complete the pending query with results
	app.CompletePendingQuery(msg.SQL, msg.Result)

	return true, nil
}

// handleSaveObject handles object definition save request.
func (d *QueryDelegate) handleSaveObject(msg components.SaveObjectMsg, app AppAccess) (bool, tea.Cmd) {
	return true, app.SaveObjectDefinition(msg)
}

// handleObjectSaved handles object save completion.
func (d *QueryDelegate) handleObjectSaved(msg components.ObjectSavedMsg, app AppAccess) (bool, tea.Cmd) {
	if msg.Error != nil {
		app.ShowError("Save Error", fmt.Sprintf("Failed to save object:\n\n%v", msg.Error))
		return true, nil
	}

	// Success - update the code editor's original content and exit edit mode
	resultTabs := app.GetResultTabs()
	activeTab := resultTabs.GetActiveTab()
	if activeTab != nil && activeTab.Type == components.TabTypeCodeEditor && activeTab.CodeEditor != nil {
		activeTab.CodeEditor.Original = activeTab.CodeEditor.GetContent()
		activeTab.CodeEditor.Modified = false
		activeTab.CodeEditor.ExitEditMode(false) // Keep changes
	}

	// Legacy: also update global code editor
	codeEditor := app.GetCodeEditor()
	if codeEditor != nil {
		codeEditor.Original = codeEditor.GetContent()
		codeEditor.Modified = false
		codeEditor.ExitEditMode(false) // Keep changes
	}

	return true, nil
}
