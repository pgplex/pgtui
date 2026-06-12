// internal/ui/components/tree_filter_test.go
package components

import (
	"strings"
	"testing"

	"github.com/pgplex/pgtui/internal/models"
)

func TestParseSearchQuery_Simple(t *testing.T) {
	q := ParseSearchQuery("plan")

	if q.Pattern != "plan" {
		t.Errorf("expected pattern 'plan', got '%s'", q.Pattern)
	}
	if q.Negate {
		t.Error("expected Negate=false")
	}
	if q.TypeFilter != "" {
		t.Errorf("expected empty TypeFilter, got '%s'", q.TypeFilter)
	}
}

func TestParseSearchQuery_Negate(t *testing.T) {
	q := ParseSearchQuery("!test")

	if q.Pattern != "test" {
		t.Errorf("expected pattern 'test', got '%s'", q.Pattern)
	}
	if !q.Negate {
		t.Error("expected Negate=true")
	}
}

func TestParseSearchQuery_TypeShort(t *testing.T) {
	q := ParseSearchQuery("t:plan")

	if q.Pattern != "plan" {
		t.Errorf("expected pattern 'plan', got '%s'", q.Pattern)
	}
	if q.TypeFilter != "table" {
		t.Errorf("expected TypeFilter 'table', got '%s'", q.TypeFilter)
	}
}

func TestParseSearchQuery_TypeLong(t *testing.T) {
	q := ParseSearchQuery("table:plan")

	if q.Pattern != "plan" {
		t.Errorf("expected pattern 'plan', got '%s'", q.Pattern)
	}
	if q.TypeFilter != "table" {
		t.Errorf("expected TypeFilter 'table', got '%s'", q.TypeFilter)
	}
}

func TestParseSearchQuery_NegateWithType(t *testing.T) {
	q := ParseSearchQuery("!f:get")

	if q.Pattern != "get" {
		t.Errorf("expected pattern 'get', got '%s'", q.Pattern)
	}
	if !q.Negate {
		t.Error("expected Negate=true")
	}
	if q.TypeFilter != "function" {
		t.Errorf("expected TypeFilter 'function', got '%s'", q.TypeFilter)
	}
}

func TestParseSearchQuery_TypeOnlyNoPattern(t *testing.T) {
	q := ParseSearchQuery("!t:")

	if q.Pattern != "" {
		t.Errorf("expected empty pattern, got '%s'", q.Pattern)
	}
	if !q.Negate {
		t.Error("expected Negate=true")
	}
	if q.TypeFilter != "table" {
		t.Errorf("expected TypeFilter 'table', got '%s'", q.TypeFilter)
	}
}

func TestFuzzyMatch_ExactPrefix(t *testing.T) {
	match, positions := FuzzyMatch("plan", "plan_check_run")

	if !match {
		t.Error("expected match")
	}
	if len(positions) != 4 || positions[0] != 0 || positions[1] != 1 || positions[2] != 2 || positions[3] != 3 {
		t.Errorf("expected positions [0,1,2,3], got %v", positions)
	}
}

func TestFuzzyMatch_Subsequence(t *testing.T) {
	match, positions := FuzzyMatch("pcr", "plan_check_run")

	if !match {
		t.Error("expected match")
	}
	// p=0, c=5, r=11
	if len(positions) != 3 {
		t.Errorf("expected 3 positions, got %d", len(positions))
	}
}

func TestFuzzyMatch_NoMatch(t *testing.T) {
	match, _ := FuzzyMatch("xyz", "plan_check_run")

	if match {
		t.Error("expected no match")
	}
}

func TestFuzzyMatch_CaseInsensitive(t *testing.T) {
	match, _ := FuzzyMatch("PLAN", "plan_check_run")

	if !match {
		t.Error("expected case-insensitive match")
	}
}

func TestFuzzyMatch_EmptyPattern(t *testing.T) {
	match, positions := FuzzyMatch("", "anything")

	if !match {
		t.Error("empty pattern should match everything")
	}
	if len(positions) != 0 {
		t.Error("empty pattern should have no positions")
	}
}

func TestNodeMatchesType_Table(t *testing.T) {
	node := &models.TreeNode{Type: models.TreeNodeTypeTable}

	if !NodeMatchesType(node, "table") {
		t.Error("table node should match 'table' type filter")
	}
	if NodeMatchesType(node, "view") {
		t.Error("table node should not match 'view' type filter")
	}
}

func TestNodeMatchesType_EmptyFilter(t *testing.T) {
	node := &models.TreeNode{Type: models.TreeNodeTypeTable}

	if !NodeMatchesType(node, "") {
		t.Error("empty filter should match any node")
	}
}

func TestNodeMatchesType_Function(t *testing.T) {
	node := &models.TreeNode{Type: models.TreeNodeTypeFunction}

	if !NodeMatchesType(node, "function") {
		t.Error("function node should match 'function' type filter")
	}
}

func TestNodeMatchesType_Schema(t *testing.T) {
	node := &models.TreeNode{Type: models.TreeNodeTypeSchema}

	if !NodeMatchesType(node, "schema") {
		t.Error("schema node should match 'schema' type filter")
	}
}

func createTestTree() *models.TreeNode {
	root := models.NewTreeNode("root", models.TreeNodeTypeRoot, "Root")
	root.Expanded = true

	db := models.NewTreeNode("db:test", models.TreeNodeTypeDatabase, "test")
	db.Expanded = true
	root.AddChild(db)

	schema := models.NewTreeNode("schema:test.public", models.TreeNodeTypeSchema, "public")
	schema.Expanded = true
	db.AddChild(schema)

	tables := models.NewTreeNode("tables", models.TreeNodeTypeTableGroup, "Tables")
	tables.Expanded = true
	schema.AddChild(tables)

	// Add test tables
	plan := models.NewTreeNode("table:test.public.plan", models.TreeNodeTypeTable, "plan")
	planCheck := models.NewTreeNode("table:test.public.plan_check_run", models.TreeNodeTypeTable, "plan_check_run")
	users := models.NewTreeNode("table:test.public.users", models.TreeNodeTypeTable, "users")
	tables.AddChild(plan)
	tables.AddChild(planCheck)
	tables.AddChild(users)

	funcs := models.NewTreeNode("funcs", models.TreeNodeTypeFunctionGroup, "Functions")
	funcs.Expanded = true
	schema.AddChild(funcs)

	getUser := models.NewTreeNode("func:test.public.get_user", models.TreeNodeTypeFunction, "get_user")
	funcs.AddChild(getUser)

	return root
}

func TestFilterTree_SimpleMatch(t *testing.T) {
	root := createTestTree()
	query := ParseSearchQuery("plan")

	matches := FilterTree(root, query)

	if len(matches) != 2 {
		t.Errorf("expected 2 matches (plan, plan_check_run), got %d", len(matches))
	}
}

func TestFilterTree_TypeFilter(t *testing.T) {
	root := createTestTree()
	query := ParseSearchQuery("t:plan")

	matches := FilterTree(root, query)

	// Should only match tables containing "plan"
	if len(matches) != 2 {
		t.Errorf("expected 2 table matches, got %d", len(matches))
	}
	for _, m := range matches {
		if m.Type != models.TreeNodeTypeTable {
			t.Errorf("expected only table nodes, got %s", m.Type)
		}
	}
}

func TestFilterTree_Negate(t *testing.T) {
	root := createTestTree()
	query := ParseSearchQuery("!plan")

	matches := FilterTree(root, query)

	// Should match users, get_user, and public schema (not plan or plan_check_run)
	for _, m := range matches {
		if strings.Contains(strings.ToLower(m.Label), "plan") {
			t.Errorf("negated query should not match '%s'", m.Label)
		}
	}
}

func TestFilterTree_EmptyQuery(t *testing.T) {
	root := createTestTree()
	query := ParseSearchQuery("")

	matches := FilterTree(root, query)

	// Empty query should return all searchable nodes
	if len(matches) == 0 {
		t.Error("empty query should return all searchable nodes")
	}
}
