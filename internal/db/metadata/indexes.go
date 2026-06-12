package metadata

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pgplex/pgtui/internal/db/connection"
	"github.com/pgplex/pgtui/internal/models"
)

// GetIndexes retrieves all indexes for a table
func GetIndexes(ctx context.Context, pool *connection.Pool, schema, table string) ([]models.IndexInfo, error) {
	query := `
		SELECT
			i.indexrelid::regclass::text AS index_name,
			am.amname AS index_type,
			pg_get_indexdef(i.indexrelid) AS definition,
			i.indisunique AS is_unique,
			i.indisprimary AS is_primary,
			pg_relation_size(i.indexrelid) AS size,
			ARRAY(
				SELECT a.attname
				FROM unnest(i.indkey) WITH ORDINALITY AS u(attnum, attposition)
				LEFT JOIN pg_catalog.pg_attribute a ON a.attrelid = i.indrelid
					AND a.attnum = u.attnum
				WHERE u.attnum > 0
				ORDER BY u.attposition
			) AS columns,
			pg_get_expr(i.indpred, i.indrelid) AS predicate
		FROM pg_catalog.pg_index i
		JOIN pg_catalog.pg_class c ON c.oid = i.indrelid
		JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_catalog.pg_class ic ON ic.oid = i.indexrelid
		JOIN pg_catalog.pg_am am ON am.oid = ic.relam
		WHERE n.nspname = $1 AND c.relname = $2
		ORDER BY i.indisprimary DESC, i.indisunique DESC, index_name
	`

	rows, err := pool.Query(ctx, query, schema, table)
	if err != nil {
		return nil, fmt.Errorf("failed to get indexes: %w", err)
	}

	var indexes []models.IndexInfo
	for _, row := range rows {
		index := models.IndexInfo{
			Name:       toString(row["index_name"]),
			Type:       toString(row["index_type"]),
			Definition: toString(row["definition"]),
			IsUnique:   toBool(row["is_unique"]),
			IsPrimary:  toBool(row["is_primary"]),
			Predicate:  toString(row["predicate"]),
		}

		if size, ok := row["size"].(int64); ok {
			index.Size = size
		}

		index.IsPartial = index.Predicate != ""

		// Parse columns array
		if colsArray, ok := row["columns"].(pgtype.Array[string]); ok {
			index.Columns = colsArray.Elements
		}

		indexes = append(indexes, index)
	}

	return indexes, nil
}

// FormatSize converts bytes to human-readable format
func FormatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
