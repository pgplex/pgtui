package discovery

import (
	"os"
	"strings"

	"github.com/pgplex/pgtui/internal/models"
)

// ParseEnvironment reads PostgreSQL environment variables
func ParseEnvironment() *models.DiscoveredInstance {
	host := envString("PGHOST")
	portStr := envString("PGPORT")

	if host == "" {
		return nil
	}

	port := 5432
	if p, ok := validPort(portStr); ok {
		port = p
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
	host := envString("PGHOST")
	portStr := envString("PGPORT")
	database := envString("PGDATABASE")
	user := envString("PGUSER")
	// Password is not trimmed: leading/trailing spaces can be intentional.
	password := os.Getenv("PGPASSWORD")
	sslMode := envString("PGSSLMODE")

	if host == "" && database == "" && user == "" {
		return nil
	}

	// Set defaults
	if host == "" {
		host = "localhost"
	}
	if user == "" {
		user = defaultUser()
	}
	if database == "" {
		database = user
	}

	port := 5432
	if p, ok := validPort(portStr); ok {
		port = p
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

func envString(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}
