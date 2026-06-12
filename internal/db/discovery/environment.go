package discovery

import (
	"os"
	"strconv"

	"github.com/pgplex/pgtui/internal/models"
)

// ParseEnvironment reads PostgreSQL environment variables
func ParseEnvironment() *models.DiscoveredInstance {
	host := os.Getenv("PGHOST")
	portStr := os.Getenv("PGPORT")

	if host == "" {
		return nil
	}

	port := 5432
	if portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil && p > 0 && p <= 65535 {
			port = p
		}
	}

	return &models.DiscoveredInstance{
		Host:      host,
		Port:      port,
		Source:    models.SourceEnvironment,
		Available: true, // Assume available, will be verified on connect
	}
}

// GetEnvironmentConfig gets connection config from environment
func GetEnvironmentConfig() *models.ConnectionConfig {
	host := os.Getenv("PGHOST")
	portStr := os.Getenv("PGPORT")
	database := os.Getenv("PGDATABASE")
	user := os.Getenv("PGUSER")
	password := os.Getenv("PGPASSWORD")
	sslMode := os.Getenv("PGSSLMODE")

	if host == "" && database == "" && user == "" {
		return nil
	}

	// Set defaults
	if host == "" {
		host = "localhost"
	}
	if user == "" {
		user = os.Getenv("USER")
	}
	if database == "" {
		database = user
	}

	port := 5432
	if portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil && p > 0 && p <= 65535 {
			port = p
		}
	}

	if sslMode == "" {
		sslMode = "prefer"
	}

	return &models.ConnectionConfig{
		Name:     "Environment",
		Host:     host,
		Port:     port,
		Database: database,
		User:     user,
		Password: password,
		SSLMode:  sslMode,
	}
}
