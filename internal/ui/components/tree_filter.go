// internal/ui/components/tree_filter.go
package components

import (
	"strings"

	"github.com/pgplex/pgtui/internal/models"
)

// SearchQuery represents a parsed search query
type SearchQuery struct {
	Pattern    string // The search pattern (after removing prefix/type)
	Negate     bool   // True if query starts with !
	TypeFilter string // Normalized type filter (e.g., "table", "function")
}

// Type prefix mappings
var typePrefixes = map[string]string{
	// Short prefixes
	"t:":   "table",
	"v:":   "view",
	"f:":   "function",
	"s:":   "schema",
	"seq:": "sequence",
	"ext:": "extension",
	"col:": "column",
	"idx:": "index",
	// Long prefixes
	"table:":     "table",
	"view:":      "view",
	"func:":      "function",
	"function:":  "function",
	"schema:":    "schema",
	"sequence:":  "sequence",
	"extension:": "extension",
	"column:":    "column",
	"index:":     "index",
}

// ParseSearchQuery parses a search query string into structured form
// Examples:
//   - "plan" → {Pattern: "plan", Negate: false, TypeFilter: ""}
//   - "!test" → {Pattern: "test", Negate: true, TypeFilter: ""}
//   - "t:plan" → {Pattern: "plan", Negate: false, TypeFilter: "table"}
//   - "!f:get" → {Pattern: "get", Negate: true, TypeFilter: "function"}
func ParseSearchQuery(query string) SearchQuery {
	q := SearchQuery{}

	// Check for negation prefix
	if strings.HasPrefix(query, "!") {
		q.Negate = true
		query = query[1:]
	}

	// Check for type prefix
	queryLower := strings.ToLower(query)
	for prefix, typeName := range typePrefixes {
		if strings.HasPrefix(queryLower, prefix) {
			q.TypeFilter = typeName
			query = query[len(prefix):]
			break
		}
	}

	q.Pattern = query
	return q
}

// FuzzyMatch performs fuzzy subsequence matching
// Returns whether the pattern matches and the positions of matched characters
// Matching is case-insensitive
func FuzzyMatch(pattern, target string) (bool, []int) {
	if pattern == "" {
		return true, []int{}
	}

	patternLower := strings.ToLower(pattern)
	targetLower := strings.ToLower(target)

	positions := make([]int, 0, len(pattern))
	patternIdx := 0

	for i := 0; i < len(targetLower) && patternIdx < len(patternLower); i++ {
		if targetLower[i] == patternLower[patternIdx] {
			positions = append(positions, i)
			patternIdx++
		}
	}

	if patternIdx == len(patternLower) {
		return true, positions
	}
	return false, nil
}

// nodeTypeMapping maps type filter strings to TreeNodeTypes
var nodeTypeMapping = map[string][]models.TreeNodeType{
	"table":     {models.TreeNodeTypeTable},
	"view":      {models.TreeNodeTypeView, models.TreeNodeTypeMaterializedView},
	"function":  {models.TreeNodeTypeFunction, models.TreeNodeTypeTriggerFunction},
	"schema":    {models.TreeNodeTypeSchema},
	"sequence":  {models.TreeNodeTypeSequence},
	"extension": {models.TreeNodeTypeExtension},
	"column":    {models.TreeNodeTypeColumn},
	"index":     {models.TreeNodeTypeIndex},
}

// NodeMatchesType checks if a node matches the given type filter
// Empty filter matches all nodes
func NodeMatchesType(node *models.TreeNode, typeFilter string) bool {
	if typeFilter == "" {
		return true
	}

	nodeTypes, ok := nodeTypeMapping[typeFilter]
	if !ok {
		return false
	}

	for _, nt := range nodeTypes {
		if node.Type == nt {
			return true
		}
	}
	return false
}

// isSearchableNode returns true if this node type should be included in search results
func isSearchableNode(node *models.TreeNode) bool {
	switch node.Type {
	case models.TreeNodeTypeTable,
		models.TreeNodeTypeView,
		models.TreeNodeTypeMaterializedView,
		models.TreeNodeTypeFunction,
		models.TreeNodeTypeProcedure,
		models.TreeNodeTypeTriggerFunction,
		models.TreeNodeTypeSequence,
		models.TreeNodeTypeIndex,
		models.TreeNodeTypeTrigger,
		models.TreeNodeTypeExtension,
		models.TreeNodeTypeCompositeType,
		models.TreeNodeTypeEnumType,
		models.TreeNodeTypeDomainType,
		models.TreeNodeTypeRangeType,
		models.TreeNodeTypeSchema,
		models.TreeNodeTypeColumn:
		return true
	default:
		return false
	}
}

// FilterTree filters the tree based on search query
// Returns a flat list of matching nodes
func FilterTree(root *models.TreeNode, query SearchQuery) []*models.TreeNode {
	var matches []*models.TreeNode

	var traverse func(node *models.TreeNode)
	traverse = func(node *models.TreeNode) {
		if node == nil {
			return
		}

		// Check if this node should be considered for matching
		if isSearchableNode(node) {
			// Check type filter first
			typeMatches := NodeMatchesType(node, query.TypeFilter)

			// Check pattern match
			patternMatches := true
			if query.Pattern != "" {
				patternMatches, _ = FuzzyMatch(query.Pattern, node.Label)
			}

			// Apply negation logic
			shouldInclude := false
			if query.Negate {
				// Include if it does NOT match (type or pattern)
				if query.TypeFilter != "" && !typeMatches {
					// Type doesn't match the filter, include it
					shouldInclude = true
				} else if typeMatches && !patternMatches {
					// Type matches but pattern doesn't, include it
					shouldInclude = true
				} else if query.TypeFilter == "" && !patternMatches {
					// No type filter, pattern doesn't match, include it
					shouldInclude = true
				}
			} else {
				// Normal match: include if type and pattern both match
				shouldInclude = typeMatches && patternMatches
			}

			if shouldInclude {
				matches = append(matches, node)
			}
		}

		// Always traverse children
		for _, child := range node.Children {
			traverse(child)
		}
	}

	traverse(root)
	return matches
}
