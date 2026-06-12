package metadata

import (
	"context"
	"fmt"

	"github.com/pgplex/pgtui/internal/db/connection"
	"github.com/pgplex/pgtui/internal/models"
)

// GetTableColumns retrieves column metadata for a table
func GetTableColumns(ctx context.Context, pool *connection.Pool, schema, table string) ([]models.ColumnInfo, error) {
	query := `
		SELECT
			column_name,
			data_type,
			udt_name,
			CASE WHEN data_type = 'ARRAY' THEN true ELSE false END as is_array
		FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = $2
		ORDER BY ordinal_position
	`

	rows, err := pool.Query(ctx, query, schema, table)
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	var columns []models.ColumnInfo
	for _, row := range rows {
		var col models.ColumnInfo
		col.Name = toString(row["column_name"])
		col.DataType = toString(row["data_type"])
		udtName := toString(row["udt_name"])

		if isArray, ok := row["is_array"].(bool); ok {
			col.IsArray = isArray
		}

		// Check if it's JSONB
		col.IsJsonb = udtName == "jsonb"

		columns = append(columns, col)
	}

	return columns, nil
}
