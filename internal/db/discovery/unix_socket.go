package discovery

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/rebelice/lazypg/internal/models"
)

var defaultUnixSocketDirs = []string{
	"/var/run/postgresql",
	"/run/postgresql",
	"/tmp",
	"/private/tmp",
	"/var/pgsql_socket",
	"/private/var/run/postgresql",
	"/opt/homebrew/var/run/postgresql",
	"/usr/local/var/run/postgresql",
}

// ScanUnixSockets scans common PostgreSQL socket directories.
func (s *Scanner) ScanUnixSockets(ctx context.Context) []models.DiscoveredInstance {
	if runtime.GOOS == "windows" {
		return nil
	}

	return s.ScanUnixSocketDirs(ctx, candidateUnixSocketDirs())
}

// ScanUnixSocketDirs scans the provided directories for PostgreSQL socket files.
func (s *Scanner) ScanUnixSocketDirs(ctx context.Context, dirs []string) []models.DiscoveredInstance {
	if runtime.GOOS == "windows" {
		return nil
	}

	instances := make([]models.DiscoveredInstance, 0)
	seen := make(map[string]struct{})

	for _, dir := range uniqueSocketDirs(dirs) {
		if ctx.Err() != nil {
			break
		}

		for _, instance := range s.scanUnixSocketDir(ctx, dir) {
			key := instance.DisplayTarget()
			if _, exists := seen[key]; exists {
				continue
			}

			seen[key] = struct{}{}
			instances = append(instances, instance)
		}
	}

	return instances
}

func (s *Scanner) scanUnixSocketDir(ctx context.Context, dir string) []models.DiscoveredInstance {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	instances := make([]models.DiscoveredInstance, 0)
	for _, entry := range entries {
		if ctx.Err() != nil {
			break
		}

		port, ok := postgresSocketPort(entry.Name())
		if !ok {
			continue
		}

		instance := s.scanUnixSocket(ctx, dir, port)
		if instance.Available {
			instances = append(instances, instance)
		}
	}

	return instances
}

func (s *Scanner) scanUnixSocket(ctx context.Context, dir string, port int) models.DiscoveredInstance {
	instance := models.DiscoveredInstance{
		Host:   dir,
		Port:   port,
		Source: models.SourceUnixSocket,
	}

	start := time.Now()
	socketPath := filepath.Join(dir, fmt.Sprintf(".s.PGSQL.%d", port))

	dialer := &net.Dialer{Timeout: s.timeout}
	conn, err := dialer.DialContext(ctx, "unix", socketPath)
	instance.ResponseTime = time.Since(start)
	if err != nil {
		return instance
	}

	_ = conn.Close()
	instance.Available = true
	return instance
}

func candidateUnixSocketDirs() []string {
	dirs := make([]string, 0, len(defaultUnixSocketDirs)+1)

	if host := strings.TrimSpace(os.Getenv("PGHOST")); strings.HasPrefix(host, "/") {
		dirs = append(dirs, host)
	}

	dirs = append(dirs, defaultUnixSocketDirs...)
	return dirs
}

func uniqueSocketDirs(dirs []string) []string {
	unique := make([]string, 0, len(dirs))
	seen := make(map[string]struct{})

	for _, dir := range dirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}

		if _, exists := seen[dir]; exists {
			continue
		}

		seen[dir] = struct{}{}
		unique = append(unique, dir)
	}

	return unique
}

func postgresSocketPort(name string) (int, bool) {
	if !strings.HasPrefix(name, ".s.PGSQL.") {
		return 0, false
	}

	portStr := strings.TrimPrefix(name, ".s.PGSQL.")
	if portStr == "" || strings.Contains(portStr, ".") {
		return 0, false
	}

	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return 0, false
	}

	return port, true
}
