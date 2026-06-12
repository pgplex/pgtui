package discovery

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/pgplex/pgtui/internal/models"
)

// PgPassEntry represents a line in .pgpass file
type PgPassEntry struct {
	Host     string
	Port     int
	Database string
	User     string
	Password string
}

// ParsePgPass reads and parses .pgpass file
func ParsePgPass() ([]PgPassEntry, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	pgpassPath := filepath.Join(home, ".pgpass")

	// Check file permissions on non-Windows systems
	if runtime.GOOS != "windows" {
		fileInfo, err := os.Stat(pgpassPath)
		if err != nil {
			if os.IsNotExist(err) {
				return []PgPassEntry{}, nil
			}
			return nil, err
		}

		mode := fileInfo.Mode()
		if mode.Perm()&0077 != 0 {
			return nil, fmt.Errorf(".pgpass file has insecure permissions %v, must be 0600", mode.Perm())
		}
	}

	file, err := os.Open(pgpassPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []PgPassEntry{}, nil
		}
		return nil, err
	}
	defer func() { _ = file.Close() }()

	var entries []PgPassEntry
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		entry, err := parsePgPassLine(line)
		if err != nil {
			continue // Skip invalid lines
		}

		entries = append(entries, entry)
	}

	return entries, scanner.Err()
}

// parsePgPassLine parses a single .pgpass line
// Format: hostname:port:database:username:password
// Handles escape sequences: \: and \\
func parsePgPassLine(line string) (PgPassEntry, error) {
	parts := make([]string, 0, 5)
	var current strings.Builder
	escaped := false

	for i := 0; i < len(line); i++ {
		ch := line[i]

		if escaped {
			current.WriteByte(ch)
			escaped = false
		} else if ch == '\\' {
			escaped = true
		} else if ch == ':' {
			parts = append(parts, current.String())
			current.Reset()
		} else {
			current.WriteByte(ch)
		}
	}

	// Add the last field
	parts = append(parts, current.String())

	if len(parts) != 5 {
		return PgPassEntry{}, os.ErrInvalid
	}

	port := 5432
	if parts[1] != "*" {
		p, err := strconv.Atoi(parts[1])
		if err != nil {
			return PgPassEntry{}, fmt.Errorf("invalid port: %s", parts[1])
		}
		if p < 1 || p > 65535 {
			return PgPassEntry{}, fmt.Errorf("port out of range: %d", p)
		}
		port = p
	}

	return PgPassEntry{
		Host:     parts[0],
		Port:     port,
		Database: parts[2],
		User:     parts[3],
		Password: parts[4],
	}, nil
}

// GetDiscoveredInstances converts pgpass entries to discovered instances
func GetDiscoveredInstances() []models.DiscoveredInstance {
	entries, err := ParsePgPass()
	if err != nil {
		return []models.DiscoveredInstance{}
	}

	instances := make([]models.DiscoveredInstance, 0, len(entries))
	seen := make(map[string]bool)

	for _, entry := range entries {
		// Skip wildcards for discovery
		if entry.Host == "*" {
			continue
		}

		key := entry.Host + ":" + strconv.Itoa(entry.Port)
		if seen[key] {
			continue
		}
		seen[key] = true

		instances = append(instances, models.DiscoveredInstance{
			Host:      entry.Host,
			Port:      entry.Port,
			Source:    models.SourcePgPass,
			Available: true,
		})
	}

	return instances
}

// FindPassword looks up password for a connection
func FindPassword(host string, port int, database, user string) string {
	entries, err := ParsePgPass()
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		if matches(entry.Host, host) &&
			matches(strconv.Itoa(entry.Port), strconv.Itoa(port)) &&
			matches(entry.Database, database) &&
			matches(entry.User, user) {
			return entry.Password
		}
	}

	return ""
}

// matches checks if pattern matches value (* is wildcard)
func matches(pattern, value string) bool {
	return pattern == "*" || pattern == value
}
