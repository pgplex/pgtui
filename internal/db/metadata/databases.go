package metadata

import (
	"context"

	"github.com/pgplex/pgtui/internal/db/connection"
)

// Database represents a PostgreSQL database
type Database struct {
	Name  string
	Owner string
	Size  string
}

// ListDatabases returns all databases
func ListDatabases(ctx context.Context, pool *connection.Pool) ([]Database, error) {
	query := `
		SELECT
			datname as name,
			pg_catalog.pg_get_userbyid(datdba) as owner,
			pg_catalog.pg_size_pretty(pg_catalog.pg_database_size(datname)) as size
		FROM pg_catalog.pg_database
		WHERE datistemplate = false
		ORDER BY datname;
	`

	rows, err := pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}

	databases := make([]Database, 0, len(rows))
	for _, row := range rows {
		databases = append(databases, Database{
			Name:  toString(row["name"]),
			Owner: toString(row["owner"]),
			Size:  toString(row["size"]),
		})
	}

	return databases, nil
}
