package query

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgplex/pgtui/internal/models"
)

// Execute executes a SQL query and returns the results
func Execute(ctx context.Context, pool *pgxpool.Pool, sql string) models.QueryResult {
	start := time.Now()

	rows, err := pool.Query(ctx, sql)
	if err != nil {
		return models.QueryResult{
			Error:    err,
			Duration: time.Since(start),
		}
	}
	defer rows.Close()

	// Get column names
	fieldDescs := rows.FieldDescriptions()
	columns := make([]string, len(fieldDescs))
	for i, fd := range fieldDescs {
		columns[i] = string(fd.Name)
	}

	// Get rows
	var result [][]string
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return models.QueryResult{
				Error:    err,
				Duration: time.Since(start),
			}
		}

		row := make([]string, len(values))
		for i, v := range values {
			if v == nil {
				row[i] = "NULL"
			} else {
				row[i] = convertValueToString(v)
			}
		}
		result = append(result, row)
	}

	// Check for errors from iteration
	if err := rows.Err(); err != nil {
		return models.QueryResult{
			Error:    err,
			Duration: time.Since(start),
		}
	}

	return models.QueryResult{
		Columns:      columns,
		Rows:         result,
		RowsAffected: int64(len(result)),
		Duration:     time.Since(start),
	}
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
