package delegates

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/pgplex/pgtui/internal/app/messages"
	"github.com/pgplex/pgtui/internal/models"
	"github.com/pgplex/pgtui/internal/ui/components"
)

// TreeDelegate handles tree navigation and node selection messages.
type TreeDelegate struct{}

// NewTreeDelegate creates a new TreeDelegate.
func NewTreeDelegate() *TreeDelegate {
	return &TreeDelegate{}
}

// Name returns the delegate name.
func (d *TreeDelegate) Name() string {
	return "tree"
}

// Update processes tree-related messages.
func (d *TreeDelegate) Update(msg tea.Msg, app AppAccess) (bool, tea.Cmd) {
	switch msg := msg.(type) {

	case messages.LoadTreeMsg:
		treeView := app.GetTreeView()
		treeView.IsLoading = true
		treeView.LoadingStart = time.Now()
		treeView.Root = nil // Clear root to show loading state
		return true, tea.Batch(app.LoadTree(), app.GetSpinnerTickCmd())

	case messages.TreeLoadedMsg:
		treeView := app.GetTreeView()
		treeView.IsLoading = false
		treeView.LoadingNodeID = ""
		if msg.Err != nil {
			app.ShowError("Database Error", fmt.Sprintf("Failed to load database structure:\n\n%v", msg.Err))
			return true, nil
		}
		// Update tree view with loaded data
		treeView.Root = msg.Root

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
		return true, nil

	case messages.LoadNodeChildrenMsg:
		app.GetTreeView().LoadingNodeID = msg.NodeID
		return true, tea.Batch(app.LoadNodeChildren(msg.NodeID), app.GetSpinnerTickCmd())

	case messages.NodeChildrenLoadedMsg:
		treeView := app.GetTreeView()
		treeView.LoadingNodeID = ""
		if msg.Err != nil {
			app.ShowError("Load Error", fmt.Sprintf("Failed to load children:\n\n%v", msg.Err))
			return true, nil
		}
		// Find the node and add children
		node := treeView.Root.FindByID(msg.NodeID)
		if node != nil {
			for _, child := range msg.Children {
				child.Parent = node
				node.AddChild(child)
			}
			node.Loaded = true
			node.Expanded = true
		}
		return true, nil

	case components.TreeNodeExpandedMsg:
		// Check if this node needs lazy loading
		if msg.Expanded && msg.Node != nil && !msg.Node.Loaded && len(msg.Node.Children) == 0 {
			// Trigger lazy load
			return true, func() tea.Msg {
				return messages.LoadNodeChildrenMsg{NodeID: msg.Node.ID}
			}
		}
		return true, nil

	case components.TreeNodeSelectedMsg:
		return d.handleNodeSelected(msg, app)

	case messages.ObjectDetailsLoadedMsg:
		return d.handleObjectDetailsLoaded(msg, app)

	case components.CodeEditorCloseMsg:
		return d.handleCodeEditorClose(app)
	}

	return false, nil
}

// handleNodeSelected handles tree node selection based on node type.
func (d *TreeDelegate) handleNodeSelected(msg components.TreeNodeSelectedMsg, app AppAccess) (bool, tea.Cmd) {
	if msg.Node == nil {
		return true, nil
	}

	switch msg.Node.Type {
	case models.TreeNodeTypeTable, models.TreeNodeTypeView, models.TreeNodeTypeMaterializedView:
		return d.handleTableNodeSelected(msg.Node, app)

	case models.TreeNodeTypeFunction, models.TreeNodeTypeProcedure:
		return d.handleFunctionNodeSelected(msg.Node, app)

	case models.TreeNodeTypeTriggerFunction:
		return d.handleTriggerFunctionNodeSelected(msg.Node, app)

	case models.TreeNodeTypeSequence:
		return d.handleSequenceNodeSelected(msg.Node, app)

	case models.TreeNodeTypeIndex:
		return d.handleIndexNodeSelected(msg.Node, app)

	case models.TreeNodeTypeTrigger:
		return d.handleTriggerNodeSelected(msg.Node, app)

	case models.TreeNodeTypeExtension:
		return d.handleExtensionNodeSelected(msg.Node, app)

	case models.TreeNodeTypeCompositeType:
		return d.handleCompositeTypeNodeSelected(msg.Node, app)

	case models.TreeNodeTypeEnumType:
		return d.handleEnumTypeNodeSelected(msg.Node, app)

	case models.TreeNodeTypeDomainType:
		return d.handleDomainTypeNodeSelected(msg.Node, app)

	case models.TreeNodeTypeRangeType:
		return d.handleRangeTypeNodeSelected(msg.Node, app)

	default:
		return true, nil
	}
}

// handleTableNodeSelected handles table/view/materialized view node selection.
func (d *TreeDelegate) handleTableNodeSelected(node *models.TreeNode, app AppAccess) (bool, tea.Cmd) {
	// Get schema name by traversing up the tree
	schemaName := d.findSchemaName(node)
	if schemaName == "" {
		return true, nil
	}

	// Store selected node
	app.SetTreeSelected(node)

	// Create object ID for tab deduplication
	objectID := schemaName + "." + node.Label

	// Check if tab for this table already exists
	resultTabs := app.GetResultTabs()
	for i, tab := range resultTabs.GetAllTabs() {
		if tab.ObjectID == objectID && tab.Type == components.TabTypeTableData {
			resultTabs.SetActiveTab(i)
			app.SetFocusArea(models.FocusDataPanel)
			app.UpdatePanelStyles()
			return true, nil
		}
	}

	// Create new tab and load data
	return true, app.CreateTableDataTab(objectID, node.Label, schemaName, node.Label)
}

// handleFunctionNodeSelected handles function/procedure node selection.
func (d *TreeDelegate) handleFunctionNodeSelected(node *models.TreeNode, app AppAccess) (bool, tea.Cmd) {
	app.SetTreeSelected(node)
	app.SetCurrentTable("") // Clear current table
	app.SetLoadingObjectDetails(true)
	return true, tea.Batch(app.LoadObjectDetails(node), app.GetSpinnerTickCmd())
}

// handleTriggerFunctionNodeSelected handles trigger function node selection.
func (d *TreeDelegate) handleTriggerFunctionNodeSelected(node *models.TreeNode, app AppAccess) (bool, tea.Cmd) {
	app.SetTreeSelected(node)
	app.SetCurrentTable("")
	app.SetLoadingObjectDetails(true)
	return true, tea.Batch(app.LoadObjectDetails(node), app.GetSpinnerTickCmd())
}

// handleSequenceNodeSelected handles sequence node selection.
func (d *TreeDelegate) handleSequenceNodeSelected(node *models.TreeNode, app AppAccess) (bool, tea.Cmd) {
	app.SetTreeSelected(node)
	app.SetCurrentTable("")
	app.SetLoadingObjectDetails(true)
	return true, tea.Batch(app.LoadObjectDetails(node), app.GetSpinnerTickCmd())
}

// handleIndexNodeSelected handles index node selection.
func (d *TreeDelegate) handleIndexNodeSelected(node *models.TreeNode, app AppAccess) (bool, tea.Cmd) {
	app.SetTreeSelected(node)
	app.SetCurrentTable("")
	app.SetLoadingObjectDetails(true)
	return true, tea.Batch(app.LoadObjectDetails(node), app.GetSpinnerTickCmd())
}

// handleTriggerNodeSelected handles trigger node selection.
func (d *TreeDelegate) handleTriggerNodeSelected(node *models.TreeNode, app AppAccess) (bool, tea.Cmd) {
	app.SetTreeSelected(node)
	app.SetCurrentTable("")
	app.SetLoadingObjectDetails(true)
	return true, tea.Batch(app.LoadObjectDetails(node), app.GetSpinnerTickCmd())
}

// handleExtensionNodeSelected handles extension node selection.
func (d *TreeDelegate) handleExtensionNodeSelected(node *models.TreeNode, app AppAccess) (bool, tea.Cmd) {
	app.SetTreeSelected(node)
	app.SetCurrentTable("")
	app.SetLoadingObjectDetails(true)
	return true, tea.Batch(app.LoadObjectDetails(node), app.GetSpinnerTickCmd())
}

// handleCompositeTypeNodeSelected handles composite type node selection.
func (d *TreeDelegate) handleCompositeTypeNodeSelected(node *models.TreeNode, app AppAccess) (bool, tea.Cmd) {
	app.SetTreeSelected(node)
	app.SetCurrentTable("")
	app.SetLoadingObjectDetails(true)
	return true, tea.Batch(app.LoadObjectDetails(node), app.GetSpinnerTickCmd())
}

// handleEnumTypeNodeSelected handles enum type node selection.
func (d *TreeDelegate) handleEnumTypeNodeSelected(node *models.TreeNode, app AppAccess) (bool, tea.Cmd) {
	app.SetTreeSelected(node)
	app.SetCurrentTable("")
	app.SetLoadingObjectDetails(true)
	return true, tea.Batch(app.LoadObjectDetails(node), app.GetSpinnerTickCmd())
}

// handleDomainTypeNodeSelected handles domain type node selection.
func (d *TreeDelegate) handleDomainTypeNodeSelected(node *models.TreeNode, app AppAccess) (bool, tea.Cmd) {
	app.SetTreeSelected(node)
	app.SetCurrentTable("")
	app.SetLoadingObjectDetails(true)
	return true, tea.Batch(app.LoadObjectDetails(node), app.GetSpinnerTickCmd())
}

// handleRangeTypeNodeSelected handles range type node selection.
func (d *TreeDelegate) handleRangeTypeNodeSelected(node *models.TreeNode, app AppAccess) (bool, tea.Cmd) {
	app.SetTreeSelected(node)
	app.SetCurrentTable("")
	app.SetLoadingObjectDetails(true)
	return true, tea.Batch(app.LoadObjectDetails(node), app.GetSpinnerTickCmd())
}

// handleObjectDetailsLoaded handles the loaded object details.
func (d *TreeDelegate) handleObjectDetailsLoaded(msg messages.ObjectDetailsLoadedMsg, app AppAccess) (bool, tea.Cmd) {
	app.SetLoadingObjectDetails(false) // Clear loading state
	if msg.Err != nil {
		app.ShowError("Error", fmt.Sprintf("Failed to load %s details:\n\n%v", msg.ObjectType, msg.Err))
		return true, nil
	}

	// Check if tab for this object already exists
	resultTabs := app.GetResultTabs()
	for i, tab := range resultTabs.GetAllTabs() {
		if tab.ObjectID == msg.ObjectID && tab.Type == components.TabTypeCodeEditor {
			resultTabs.SetActiveTab(i)
			app.SetFocusArea(models.FocusDataPanel)
			app.UpdatePanelStyles()
			return true, nil
		}
	}

	// Create code editor tab
	app.CreateCodeEditorTab(msg.ObjectID, msg.Title, msg.Content, msg.ObjectType, msg.ObjectName)
	app.SetFocusArea(models.FocusDataPanel)
	app.UpdatePanelStyles()
	return true, nil
}

// handleCodeEditorClose handles closing a code editor tab.
func (d *TreeDelegate) handleCodeEditorClose(app AppAccess) (bool, tea.Cmd) {
	// Close the active tab if it's a code editor tab
	resultTabs := app.GetResultTabs()
	activeTab := resultTabs.GetActiveTab()
	if activeTab != nil && activeTab.Type == components.TabTypeCodeEditor {
		resultTabs.CloseActiveTab()
	}

	// Legacy: also clear the global code editor state
	app.SetShowCodeEditor(false)
	app.ClearCodeEditor()

	// If no more tabs, return to tree
	if !resultTabs.HasTabs() {
		app.SetFocusArea(models.FocusTreeView)
	}
	app.UpdatePanelStyles()
	return true, nil
}

// findSchemaName finds the schema name by traversing up the tree.
func (d *TreeDelegate) findSchemaName(node *models.TreeNode) string {
	current := node.Parent
	for current != nil {
		if current.Type == models.TreeNodeTypeSchema {
			return strings.Split(current.Label, " ")[0]
		}
		current = current.Parent
	}
	return ""
}
