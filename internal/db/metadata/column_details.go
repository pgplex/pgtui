package metadata

import (
	"context"
	"fmt"

	"github.com/pgplex/pgtui/internal/db/connection"
	"github.com/pgplex/pgtui/internal/models"
)

// GetColumnDetails retrieves detailed column information including constraints
func GetColumnDetails(ctx context.Context, pool *connection.Pool, schema, table string) ([]models.ColumnDetail, error) {
	query := `
		WITH column_constraints AS (
			SELECT
				a.attname AS column_name,
				bool_or(con.contype = 'p') AS is_pk,
				bool_or(con.contype = 'f') AS is_fk,
				bool_or(con.contype = 'u') AS is_unique,
				bool_or(con.contype = 'c') AS has_check
			FROM pg_catalog.pg_attribute a
			LEFT JOIN pg_catalog.pg_constraint con ON con.conrelid = a.attrelid
				AND a.attnum = ANY(con.conkey)
			WHERE a.attrelid = ($1 || '.' || $2)::regclass
				AND a.attnum > 0
				AND NOT a.attisdropped
			GROUP BY a.attname
		)
		SELECT
			c.column_name,
			c.data_type,
			CASE
				WHEN c.character_maximum_length IS NOT NULL
				THEN c.data_type || '(' || c.character_maximum_length || ')'
				WHEN c.numeric_precision IS NOT NULL
				THEN c.data_type || '(' || c.numeric_precision || ',' || c.numeric_scale || ')'
				ELSE c.data_type
			END AS formatted_type,
			c.is_nullable = 'YES' AS is_nullable,
			COALESCE(c.column_default, '-') AS default_value,
			COALESCE(cc.is_pk, false) AS is_primary_key,
			COALESCE(cc.is_fk, false) AS is_foreign_key,
			COALESCE(cc.is_unique, false) AS is_unique,
			COALESCE(cc.has_check, false) AS has_check,
			COALESCE(d.description, '-') AS comment
		FROM information_schema.columns c
		LEFT JOIN column_constraints cc ON cc.column_name = c.column_name
		LEFT JOIN pg_catalog.pg_attribute a ON a.attname = c.column_name
			AND a.attrelid = ($1 || '.' || $2)::regclass
		LEFT JOIN pg_catalog.pg_description d ON d.objoid = a.attrelid
			AND d.objsubid = a.attnum
		WHERE c.table_schema = $1 AND c.table_name = $2
		ORDER BY c.ordinal_position
	`

	rows, err := pool.Query(ctx, query, schema, table)
	if err != nil {
		return nil, fmt.Errorf("failed to get column details: %w", err)
	}

	var columns []models.ColumnDetail
	for _, row := range rows {
		col := models.ColumnDetail{
			Name:          toString(row["column_name"]),
			DataType:      toString(row["formatted_type"]),
			IsNullable:    toBool(row["is_nullable"]),
			DefaultValue:  toString(row["default_value"]),
			IsPrimaryKey:  toBool(row["is_primary_key"]),
			IsForeignKey:  toBool(row["is_foreign_key"]),
			IsUnique:      toBool(row["is_unique"]),
			HasCheck:      toBool(row["has_check"]),
			Comment:       toString(row["comment"]),
		}
		columns = append(columns, col)
	}

	return columns, nil
}

func toBool(v interface{}) bool {
	if v == nil {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}
