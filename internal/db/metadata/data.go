package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pgplex/pgtui/internal/db/connection"
)

// TableData represents paginated table data
type TableData struct {
	Columns   []string
	Rows      [][]string
	TotalRows int64
}

// SortOptions holds sorting configuration
type SortOptions struct {
	Column     string
	Direction  string // "ASC" or "DESC"
	NullsFirst bool
}

// QueryTableData fetches paginated table data with optional sorting
func QueryTableData(ctx context.Context, pool *connection.Pool, schema, table string, offset, limit int, sort *SortOptions) (*TableData, error) {
	// Get exact row count - uses Index-Only Scan for tables with PK/index
	countQuery := fmt.Sprintf("SELECT COUNT(*) as count FROM %s.%s", schema, table)
	countRow, err := pool.QueryRow(ctx, countQuery)
	if err != nil {
		countRow = map[string]interface{}{"count": int64(0)}
	}

	totalRows := int64(0)
	if count, ok := countRow["count"].(int64); ok {
		totalRows = count
	}

	// Build query with optional ORDER BY
	query := fmt.Sprintf("SELECT * FROM %s.%s", schema, table)

	if sort != nil && sort.Column != "" {
		nullsClause := "NULLS LAST"
		if sort.NullsFirst {
			nullsClause = "NULLS FIRST"
		}
		query += fmt.Sprintf(" ORDER BY \"%s\" %s %s", sort.Column, sort.Direction, nullsClause)
	}

	query += fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)

	result, err := pool.QueryWithColumns(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query table data: %w", err)
	}

	if len(result.Rows) == 0 {
		return &TableData{
			Columns:   result.Columns,
			Rows:      [][]string{},
			TotalRows: totalRows,
		}, nil
	}

	columns := result.Columns

	// Convert rows to string slices
	data := make([][]string, len(result.Rows))
	for i, row := range result.Rows {
		rowData := make([]string, len(columns))
		for j, col := range columns {
			val := row[col]
			if val == nil {
				rowData[j] = "NULL"
			} else {
				rowData[j] = convertValueToString(val)
			}
		}
		data[i] = rowData
	}

	return &TableData{
		Columns:   columns,
		Rows:      data,
		TotalRows: totalRows,
	}, nil
}

// convertValueToString converts a database value to string, handling JSONB properly
func convertValueToString(val interface{}) string {
	// Check if it's a map or slice (JSONB types)
	switch v := val.(type) {
	case map[string]interface{}, []interface{}:
		// Convert to JSON string
		jsonBytes, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", val)
		}
		return string(jsonBytes)
	case [16]byte:
		// UUID from PostgreSQL
		return fmt.Sprintf("%x-%x-%x-%x-%x", v[:4], v[4:6], v[6:8], v[8:10], v[10:])
	case []byte:
		// Might be raw JSON bytes
		return string(v)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// SearchTableData searches entire table using ILIKE on all columns
func SearchTableData(ctx context.Context, pool *connection.Pool, schema, table string, columns []string, keyword string, limit int) (*TableData, error) {
	if keyword == "" || len(columns) == 0 {
		return &TableData{
			Columns:   columns,
			Rows:      [][]string{},
			TotalRows: 0,
		}, nil
	}

	// Build WHERE clause with ILIKE for all columns
	// Using ::text to cast all columns to text for comparison
	var conditions []string
	escapedKeyword := strings.ReplaceAll(keyword, "'", "''")
	escapedKeyword = strings.ReplaceAll(escapedKeyword, "%", "\\%")
	escapedKeyword = strings.ReplaceAll(escapedKeyword, "_", "\\_")

	for _, col := range columns {
		conditions = append(conditions, fmt.Sprintf("\"%s\"::text ILIKE '%%%s%%'", col, escapedKeyword))
	}

	whereClause := strings.Join(conditions, " OR ")
	query := fmt.Sprintf("SELECT * FROM %s.%s WHERE %s LIMIT %d", schema, table, whereClause, limit)

	result, err := pool.QueryWithColumns(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("search query failed: %w", err)
	}

	if len(result.Rows) == 0 {
		return &TableData{
			Columns:   result.Columns,
			Rows:      [][]string{},
			TotalRows: 0,
		}, nil
	}

	cols := result.Columns

	// Convert rows to string slices
	data := make([][]string, len(result.Rows))
	for i, row := range result.Rows {
		rowData := make([]string, len(cols))
		for j, col := range cols {
			val := row[col]
			if val == nil {
				rowData[j] = "NULL"
			} else {
				rowData[j] = convertValueToString(val)
			}
		}
		data[i] = rowData
	}

	return &TableData{
		Columns:   cols,
		Rows:      data,
		TotalRows: int64(len(data)),
	}, nil
}
