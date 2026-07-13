package discovery

import (
	"os"
	"strings"

	"github.com/pgplex/pgtui/internal/models"
)

// BuildConnectionConfig turns a discovered instance into a connection config.
// The returned config intentionally omits the password for environment and
// .pgpass sources: libpq (pgx) reads PGPASSWORD and ~/.pgpass itself, and
// leaving the password empty prevents it from being persisted to the keyring
// (those secrets already have their own source).
func BuildConnectionConfig(instance models.DiscoveredInstance) models.ConnectionConfig {
	switch instance.Source {
	case models.SourceEnvironment:
		if envConfig := GetEnvironmentConfig(); envConfig != nil {
			config := *envConfig
			config.Name = "" // avoid leaking the generic "Environment" label into connection ID/history
			config.Password = ""
			return config
		}
	case models.SourcePgPass:
		if pgpassConfig := buildPgPassConfig(instance.Host, instance.Port); pgpassConfig != nil {
			config := *pgpassConfig
			config.Password = ""
			return config
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
	return models.ConnectionConfig{
		Host:     instance.Host,
		Port:     instance.Port,
		Database: "postgres",
		User:     defaultUser(),
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
