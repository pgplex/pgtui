// Package messages defines all message types used by the App.
// These are extracted into a separate package to avoid import cycles
// between the app and delegates packages.
package messages

import (
	"github.com/pgplex/pgtui/internal/db/metadata"
	"github.com/pgplex/pgtui/internal/models"
)

// DiscoveryCompleteMsg is sent when instance discovery completes
type DiscoveryCompleteMsg struct {
	Instances []models.DiscoveredInstance
}

// ErrorMsg is sent when an error occurs
type ErrorMsg struct {
	Title   string
	Message string
}

// ConnectionStartMsg starts an async connection
type ConnectionStartMsg struct {
	Config models.ConnectionConfig
}

// ConnectionResultMsg is sent when connection attempt completes
type ConnectionResultMsg struct {
	Config models.ConnectionConfig
	ConnID string
	Err    error
}

// LoadTreeMsg requests loading the navigation tree
type LoadTreeMsg struct{}

// TreeLoadedMsg is sent when tree data is loaded
type TreeLoadedMsg struct {
	Root       *models.TreeNode
	AllObjects []metadata.SchemaObject // Pre-loaded objects for search
	Err        error
}

// LoadNodeChildrenMsg requests loading children for a tree node
type LoadNodeChildrenMsg struct {
	NodeID string
}

// NodeChildrenLoadedMsg is sent when node children are loaded
type NodeChildrenLoadedMsg struct {
	NodeID   string
	Children []*models.TreeNode
	Err      error
}

// LoadTableDataMsg requests loading table data
type LoadTableDataMsg struct {
	Schema     string
	Table      string
	Offset     int
	Limit      int
	SortColumn string
	SortDir    string
	NullsFirst bool
}

// TableDataLoadedMsg is sent when table data is loaded
type TableDataLoadedMsg struct {
	Columns   []string
	Rows      [][]string
	TotalRows int
	Offset    int // Offset used in the query (0 for initial load)
	Err       error
}

// PrefetchDataMsg requests prefetching data in background
type PrefetchDataMsg struct {
	Schema     string
	Table      string
	Offset     int
	Limit      int
	SortColumn string
	SortDir    string
	NullsFirst bool
}

// PrefetchCompleteMsg is sent when prefetch completes
type PrefetchCompleteMsg struct {
	Rows   [][]string
	Offset int
	Err    error
}

// QueryResultMsg is sent when a query has been executed
type QueryResultMsg struct {
	SQL    string
	Result models.QueryResult
}

// ObjectDetailsLoadedMsg is sent when object details are loaded
type ObjectDetailsLoadedMsg struct {
	ObjectType string // "function", "sequence", "extension", "type", "index", "trigger"
	ObjectName string // "schema.name" for save operations
	ObjectID   string // Unique ID for tab deduplication (e.g., "schema.function_name")
	Title      string
	Content    string // Formatted content to display
	Err        error
}

// TabTableDataLoadedMsg is sent when table data for a tab is loaded
type TabTableDataLoadedMsg struct {
	ObjectID  string // schema.table identifier
	Schema    string
	Table     string
	Columns   []string
	Rows      [][]string
	TotalRows int
	Err       error
}

// StructureMetadataLoadedMsg is sent when table structure metadata is loaded
type StructureMetadataLoadedMsg struct {
	ObjectID    string // schema.table identifier for routing to correct tab
	Columns     []models.ColumnDetail
	Constraints []models.Constraint
	Indexes     []models.IndexInfo
	Err         error
}

// SearchTableMsg requests searching within a table
type SearchTableMsg struct {
	Query string
}

// SearchResultsMsg is sent when search results are ready
type SearchResultsMsg struct {
	Query   string
	Matches []int // Row indices that match
}

// SearchTableResultMsg is sent when a database table search completes
type SearchTableResultMsg struct {
	Query string
	Data  *TableSearchData
	Err   error
}

// TableSearchData holds the search result data
type TableSearchData struct {
	Columns   []string
	Rows      [][]string
	TotalRows int
}
