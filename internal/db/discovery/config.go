package discovery

import (
	"os"
	"strings"

	"github.com/pgplex/pgtui/internal/models"
)

// BuildConnectionConfig turns a discovered instance into a connection config.
func BuildConnectionConfig(instance models.DiscoveredInstance) models.ConnectionConfig {
	switch instance.Source {
	case models.SourceEnvironment:
		if envConfig := GetEnvironmentConfig(); envConfig != nil {
			return *envConfig
		}
	case models.SourcePgPass:
		if pgpassConfig := buildPgPassConfig(instance.Host, instance.Port); pgpassConfig != nil {
			return *pgpassConfig
		}
	}

	return buildDefaultConfig(instance)
}

func buildPgPassConfig(host string, port int) *models.ConnectionConfig {
	entries, err := ParsePgPass()
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		if entry.Host != host || entry.Port != port {
			continue
		}

		user := entry.User
		if user == "" || user == "*" {
			user = defaultUser()
		}

		database := entry.Database
		if database == "" || database == "*" {
			database = user
		}

		return &models.ConnectionConfig{
			Host:     host,
			Port:     port,
			Database: database,
			User:     user,
			Password: entry.Password,
			SSLMode:  "prefer",
		}
	}

	return nil
}

func buildDefaultConfig(instance models.DiscoveredInstance) models.ConnectionConfig {
	user := defaultUser()

	return models.ConnectionConfig{
		Host:     instance.Host,
		Port:     instance.Port,
		Database: "postgres",
		User:     user,
		SSLMode:  "prefer",
	}
}

func defaultUser() string {
	for _, key := range []string{"PGUSER", "USER", "USERNAME"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}

	return "postgres"
}
