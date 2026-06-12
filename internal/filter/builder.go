package filter

import (
	"fmt"
	"strings"

	"github.com/pgplex/pgtui/internal/models"
)

// Builder generates SQL WHERE clauses from Filter models
type Builder struct{}

// NewBuilder creates a new filter builder
func NewBuilder() *Builder {
	return &Builder{}
}

// BuildWhere generates a WHERE clause from a Filter
func (b *Builder) BuildWhere(filter models.Filter) (string, []interface{}, error) {
	if len(filter.RootGroup.Conditions) == 0 && len(filter.RootGroup.Groups) == 0 {
		return "", nil, nil
	}

	clause, args, err := b.buildGroup(filter.RootGroup, 1)
	if err != nil {
		return "", nil, err
	}

	return "WHERE " + clause, args, nil
}

// buildGroup recursively builds a filter group
func (b *Builder) buildGroup(group models.FilterGroup, paramIndex int) (string, []interface{}, error) {
	var clauses []string
	var args []interface{}
	currentParam := paramIndex

	// Build conditions
	for _, cond := range group.Conditions {
		clause, condArgs, err := b.buildCondition(cond, currentParam)
		if err != nil {
			return "", nil, err
		}
		clauses = append(clauses, clause)
		args = append(args, condArgs...)
		currentParam += len(condArgs)
	}

	// Build sub-groups
	for _, subGroup := range group.Groups {
		clause, groupArgs, err := b.buildGroup(subGroup, currentParam)
		if err != nil {
			return "", nil, err
		}
		clauses = append(clauses, "("+clause+")")
		args = append(args, groupArgs...)
		currentParam += len(groupArgs)
	}

	logic := group.Logic
	if logic == "" {
		logic = "AND"
	}

	return strings.Join(clauses, " "+logic+" "), args, nil
}

// buildCondition builds a single filter condition
func (b *Builder) buildCondition(cond models.FilterCondition, paramIndex int) (string, []interface{}, error) {
	// Escape column name to prevent SQL injection and handle reserved keywords
	column := fmt.Sprintf(`"%s"`, cond.Column)

	switch cond.Operator {
	case models.OpIsNull:
		return fmt.Sprintf("%s IS NULL", column), nil, nil
	case models.OpIsNotNull:
		return fmt.Sprintf("%s IS NOT NULL", column), nil, nil
	case models.OpEqual, models.OpNotEqual, models.OpGreaterThan, models.OpGreaterOrEqual,
		models.OpLessThan, models.OpLessOrEqual:
		return fmt.Sprintf("%s %s $%d", column, cond.Operator, paramIndex), []interface{}{cond.Value}, nil
	case models.OpLike, models.OpILike:
		return fmt.Sprintf("%s %s $%d", column, cond.Operator, paramIndex), []interface{}{cond.Value}, nil
	case models.OpIn, models.OpNotIn:
		// IN/NOT IN requires array expansion - not yet implemented
		return "", nil, fmt.Errorf("IN/NOT IN operators not yet implemented")
	case models.OpContains:
		// JSONB @> operator
		// If value is a string, treat it as JSON literal
		return fmt.Sprintf(`%s @> $%d::jsonb`, column, paramIndex), []interface{}{cond.Value}, nil
	case models.OpContainedBy:
		// JSONB <@ operator
		return fmt.Sprintf(`%s <@ $%d::jsonb`, column, paramIndex), []interface{}{cond.Value}, nil
	case models.OpHasKey:
		// JSONB ? operator (has key)
		return fmt.Sprintf(`%s ? $%d`, column, paramIndex), []interface{}{cond.Value}, nil
	case models.OpArrayOverlap:
		return fmt.Sprintf("%s && $%d", column, paramIndex), []interface{}{cond.Value}, nil
	default:
		return "", nil, fmt.Errorf("unsupported operator: %s", cond.Operator)
	}
}

// Validate checks if a filter is valid
func (b *Builder) Validate(filter models.Filter) error {
	if filter.TableName == "" {
		return fmt.Errorf("table name is required")
	}

	return b.validateGroup(filter.RootGroup)
}

// validateGroup validates a filter group
func (b *Builder) validateGroup(group models.FilterGroup) error {
	for _, cond := range group.Conditions {
		if err := b.validateCondition(cond); err != nil {
			return err
		}
	}

	for _, subGroup := range group.Groups {
		if err := b.validateGroup(subGroup); err != nil {
			return err
		}
	}

	return nil
}

// validateCondition validates a single condition
func (b *Builder) validateCondition(cond models.FilterCondition) error {
	if cond.Column == "" {
		return fmt.Errorf("column name is required")
	}

	// Check if value is required for operator
	requiresValue := cond.Operator != models.OpIsNull && cond.Operator != models.OpIsNotNull
	if requiresValue && cond.Value == nil {
		return fmt.Errorf("value is required for operator %s", cond.Operator)
	}

	return nil
}

// GetOperatorsForType returns available operators for a given PostgreSQL type
func GetOperatorsForType(dataType string) []models.FilterOperator {
	// Normalize to lowercase for case-insensitive matching
	dataType = strings.ToLower(dataType)

	switch {
	case strings.Contains(dataType, "int") || strings.Contains(dataType, "numeric") ||
		strings.Contains(dataType, "real") || strings.Contains(dataType, "double"):
		return []models.FilterOperator{
			models.OpEqual, models.OpNotEqual,
			models.OpGreaterThan, models.OpGreaterOrEqual,
			models.OpLessThan, models.OpLessOrEqual,
			models.OpIsNull, models.OpIsNotNull,
		}
	case strings.Contains(dataType, "char") || strings.Contains(dataType, "text"):
		return []models.FilterOperator{
			models.OpEqual, models.OpNotEqual,
			models.OpLike, models.OpILike,
			models.OpIsNull, models.OpIsNotNull,
		}
	case strings.Contains(dataType, "jsonb"):
		return []models.FilterOperator{
			models.OpEqual, models.OpNotEqual,
			models.OpContains, models.OpContainedBy, models.OpHasKey,
			models.OpIsNull, models.OpIsNotNull,
		}
	case strings.Contains(dataType, "array"):
		return []models.FilterOperator{
			models.OpEqual, models.OpNotEqual,
			models.OpArrayOverlap, models.OpContains, models.OpContainedBy,
			models.OpIsNull, models.OpIsNotNull,
		}
	case strings.Contains(dataType, "bool"):
		return []models.FilterOperator{
			models.OpEqual, models.OpNotEqual,
			models.OpIsNull, models.OpIsNotNull,
		}
	case strings.Contains(dataType, "date") || strings.Contains(dataType, "time"):
		return []models.FilterOperator{
			models.OpEqual, models.OpNotEqual,
			models.OpGreaterThan, models.OpGreaterOrEqual,
			models.OpLessThan, models.OpLessOrEqual,
			models.OpIsNull, models.OpIsNotNull,
		}
	default:
		return []models.FilterOperator{
			models.OpEqual, models.OpNotEqual,
			models.OpIsNull, models.OpIsNotNull,
		}
	}
}
