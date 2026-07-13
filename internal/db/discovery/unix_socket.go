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

var broadUnixSocketDirs = map[string]struct{}{
	"/tmp":         {},
	"/private/tmp": {},
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
	if isBroadUnixSocketDir(dir) {
		return s.scanKnownUnixSocketPorts(ctx, dir)
	}

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

func (s *Scanner) scanKnownUnixSocketPorts(ctx context.Context, dir string) []models.DiscoveredInstance {
	instances := make([]models.DiscoveredInstance, 0, len(DefaultPorts))

	for _, port := range candidateUnixSocketPorts() {
		if ctx.Err() != nil {
			break
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
	dirs := make([]string, 0, len(defaultUnixSocketDirs)+len(strings.Split(os.Getenv("PGHOST"), ",")))

	for _, host := range strings.Split(os.Getenv("PGHOST"), ",") {
		host = strings.TrimSpace(host)
		if strings.HasPrefix(host, "/") {
			dirs = append(dirs, host)
		}
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
		dir = filepath.Clean(dir)

		key := socketDirKey(dir)
		if _, exists := seen[key]; exists {
			continue
		}

		seen[key] = struct{}{}
		unique = append(unique, dir)
	}

	return unique
}

func socketDirKey(dir string) string {
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return dir
	}

	return filepath.Clean(resolved)
}

func isBroadUnixSocketDir(dir string) bool {
	_, ok := broadUnixSocketDirs[socketDirKey(filepath.Clean(dir))]
	return ok
}

func candidateUnixSocketPorts() []int {
	ports := append([]int(nil), DefaultPorts...)

	if portStr := strings.TrimSpace(os.Getenv("PGPORT")); portStr != "" {
		port, err := strconv.Atoi(portStr)
		if err == nil && port >= 1 && port <= 65535 {
			ports = append([]int{port}, ports...)
		}
	}

	unique := ports[:0]
	seen := make(map[int]struct{}, len(ports))
	for _, port := range ports {
		if _, exists := seen[port]; exists {
			continue
		}

		seen[port] = struct{}{}
		unique = append(unique, port)
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
