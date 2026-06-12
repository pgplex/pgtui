package metadata

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pgplex/pgtui/internal/db/connection"
	"github.com/pgplex/pgtui/internal/models"
)

// GetConstraints retrieves all constraints for a table
func GetConstraints(ctx context.Context, pool *connection.Pool, schema, table string) ([]models.Constraint, error) {
	query := `
		SELECT
			con.conname AS constraint_name,
			con.contype AS constraint_type,
			pg_get_constraintdef(con.oid) AS definition,
			ARRAY(
				SELECT att.attname
				FROM unnest(con.conkey) WITH ORDINALITY AS u(attnum, attposition)
				JOIN pg_catalog.pg_attribute att ON att.attrelid = con.conrelid
					AND att.attnum = u.attnum
				ORDER BY u.attposition
			) AS columns,
			COALESCE(nf.nspname || '.' || clf.relname, '') AS foreign_table,
			ARRAY(
				SELECT att.attname
				FROM unnest(con.confkey) WITH ORDINALITY AS u(attnum, attposition)
				JOIN pg_catalog.pg_attribute att ON att.attrelid = con.confrelid
					AND att.attnum = u.attnum
				ORDER BY u.attposition
			) AS foreign_columns
		FROM pg_catalog.pg_constraint con
		JOIN pg_catalog.pg_class cl ON con.conrelid = cl.oid
		JOIN pg_catalog.pg_namespace ns ON cl.relnamespace = ns.oid
		LEFT JOIN pg_catalog.pg_class clf ON con.confrelid = clf.oid
		LEFT JOIN pg_catalog.pg_namespace nf ON clf.relnamespace = nf.oid
		WHERE ns.nspname = $1 AND cl.relname = $2
		ORDER BY
			CASE con.contype
				WHEN 'p' THEN 1
				WHEN 'u' THEN 2
				WHEN 'f' THEN 3
				WHEN 'c' THEN 4
			END,
			con.conname
	`

	rows, err := pool.Query(ctx, query, schema, table)
	if err != nil {
		return nil, fmt.Errorf("failed to get constraints: %w", err)
	}

	var constraints []models.Constraint
	for _, row := range rows {
		constraint := models.Constraint{
			Name:         toString(row["constraint_name"]),
			Type:         toString(row["constraint_type"]),
			Definition:   toString(row["definition"]),
			ForeignTable: toString(row["foreign_table"]),
		}

		// Parse columns array
		if colsArray, ok := row["columns"].(pgtype.Array[string]); ok {
			constraint.Columns = colsArray.Elements
		}

		// Parse foreign columns array
		if fkArray, ok := row["foreign_columns"].(pgtype.Array[string]); ok {
			constraint.ForeignCols = fkArray.Elements
		}

		constraints = append(constraints, constraint)
	}

	return constraints, nil
}

// FormatConstraintType returns a short type label
func FormatConstraintType(conType string) string {
	switch conType {
	case "p":
		return "PK"
	case "f":
		return "FK"
	case "u":
		return "UQ"
	case "c":
		return "CK"
	default:
		return strings.ToUpper(conType)
	}
}
