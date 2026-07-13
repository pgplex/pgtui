package discovery

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/pgplex/pgtui/internal/models"
)

func TestScanUnixSocketDirs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix sockets are not supported on windows")
	}

	tempDir := t.TempDir()
	acceptDone := listenPostgresSocket(t, tempDir, 6543)

	scanner := NewScanner()
	instances := scanner.ScanUnixSocketDirs(context.Background(), []string{tempDir})

	if len(instances) != 1 {
		t.Fatalf("expected 1 discovered socket, got %d", len(instances))
	}

	instance := instances[0]
	if instance.Host != tempDir {
		t.Fatalf("expected host %q, got %q", tempDir, instance.Host)
	}
	if instance.Port != 6543 {
		t.Fatalf("expected port 6543, got %d", instance.Port)
	}
	if instance.Source != models.SourceUnixSocket {
		t.Fatalf("expected source %v, got %v", models.SourceUnixSocket, instance.Source)
	}
	if !instance.Available {
		t.Fatal("expected discovered socket to be available")
	}

	<-acceptDone
}

func TestBuildConnectionConfigForSocketDefaults(t *testing.T) {
	t.Setenv("PGUSER", "")
	t.Setenv("USER", "socket-user")
	t.Setenv("USERNAME", "")

	config := BuildConnectionConfig(models.DiscoveredInstance{
		Host:   "/tmp",
		Port:   5432,
		Source: models.SourceUnixSocket,
	})

	if config.Host != "/tmp" {
		t.Fatalf("expected host /tmp, got %q", config.Host)
	}
	if config.Port != 5432 {
		t.Fatalf("expected port 5432, got %d", config.Port)
	}
	if config.User != "socket-user" {
		t.Fatalf("expected user socket-user, got %q", config.User)
	}
	if config.Database != "postgres" {
		t.Fatalf("expected database postgres, got %q", config.Database)
	}
	if config.SSLMode != "prefer" {
		t.Fatalf("expected sslmode prefer, got %q", config.SSLMode)
	}
	if !config.UsesUnixSocket() {
		t.Fatal("expected config to use a unix socket")
	}
	if got := config.DisplayTarget(); got != filepath.Join("/tmp", ".s.PGSQL.5432") {
		t.Fatalf("unexpected display target %q", got)
	}
}

func TestGetEnvironmentConfigUsesUsernameFallback(t *testing.T) {
	t.Setenv("PGHOST", "localhost")
	t.Setenv("PGPORT", "")
	t.Setenv("PGDATABASE", "")
	t.Setenv("PGUSER", "")
	t.Setenv("PGPASSWORD", "")
	t.Setenv("PGSSLMODE", "")
	t.Setenv("USER", "")
	t.Setenv("USERNAME", "windows-user")

	config := GetEnvironmentConfig()
	if config == nil {
		t.Fatal("expected environment config")
	}
	if config.User != "windows-user" {
		t.Fatalf("expected user windows-user, got %q", config.User)
	}
	if config.Database != "windows-user" {
		t.Fatalf("expected database windows-user, got %q", config.Database)
	}
	if config.SSLMode != "prefer" {
		t.Fatalf("expected sslmode prefer, got %q", config.SSLMode)
	}
}

func TestBuildConnectionConfigForEnvironmentUsesEnvironmentConfig(t *testing.T) {
	t.Setenv("PGHOST", " localhost ")
	t.Setenv("PGPORT", " 5432 ")
	t.Setenv("PGDATABASE", " envdb ")
	t.Setenv("PGUSER", " envuser ")
	t.Setenv("PGPASSWORD", "secret")
	t.Setenv("PGSSLMODE", " require ")

	config := BuildConnectionConfig(models.DiscoveredInstance{
		Host:   "localhost",
		Port:   5432,
		Source: models.SourceEnvironment,
	})

	if config.Host != "localhost" {
		t.Fatalf("expected trimmed environment host, got %q", config.Host)
	}
	if config.Port != 5432 {
		t.Fatalf("expected environment port 5432, got %d", config.Port)
	}
	if config.Database != "envdb" {
		t.Fatalf("expected trimmed environment database, got %q", config.Database)
	}
	if config.User != "envuser" {
		t.Fatalf("expected trimmed environment user, got %q", config.User)
	}
	// Password is intentionally left empty: libpq reads PGPASSWORD from the
	// environment, and keeping it out of the config prevents persisting it to
	// the keyring.
	if config.Password != "" {
		t.Fatalf("expected empty password (sourced via PGPASSWORD), got %q", config.Password)
	}
	if config.Name != "" {
		t.Fatalf("expected empty name to avoid leaking into connection ID/history, got %q", config.Name)
	}
	if config.SSLMode != "require" {
		t.Fatalf("expected trimmed environment sslmode, got %q", config.SSLMode)
	}
}

func TestParseEnvironmentTrimsHost(t *testing.T) {
	t.Setenv("PGHOST", " /var/run/postgresql ")
	t.Setenv("PGPORT", " 5433 ")

	instance := ParseEnvironment()
	if instance == nil {
		t.Fatal("expected environment instance")
	}
	if instance.Host != "/var/run/postgresql" {
		t.Fatalf("expected trimmed host, got %q", instance.Host)
	}
	if instance.Port != 5433 {
		t.Fatalf("expected port 5433, got %d", instance.Port)
	}
}

func TestCandidateUnixSocketDirsSplitsPGHost(t *testing.T) {
	t.Setenv("PGHOST", "/custom/socket,localhost, /tmp ")

	dirs := candidateUnixSocketDirs()
	if len(dirs) < 2 {
		t.Fatalf("expected PGHOST dirs plus defaults, got %#v", dirs)
	}
	if dirs[0] != "/custom/socket" {
		t.Fatalf("expected first custom socket dir, got %#v", dirs)
	}
	if dirs[1] != "/tmp" {
		t.Fatalf("expected second custom socket dir, got %#v", dirs)
	}
}

func TestUniqueSocketDirsNormalizesAndResolvesSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevated permissions on some windows setups")
	}

	tempDir := t.TempDir()
	target := filepath.Join(tempDir, "target")
	link := filepath.Join(tempDir, "link")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink target: %v", err)
	}

	dirs := uniqueSocketDirs([]string{
		filepath.Join(target, "."),
		target + string(os.PathSeparator),
		link,
	})

	if len(dirs) != 1 {
		t.Fatalf("expected symlinked dirs to deduplicate, got %#v", dirs)
	}
	if dirs[0] != target {
		t.Fatalf("expected cleaned target dir, got %q", dirs[0])
	}
}

func TestInstanceKeyNormalizesSocketSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevated permissions on some windows setups")
	}

	tempDir := t.TempDir()
	target := filepath.Join(tempDir, "target")
	link := filepath.Join(tempDir, "link")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink target: %v", err)
	}

	targetInstance := models.DiscoveredInstance{Host: target, Port: 5432}
	linkInstance := models.DiscoveredInstance{Host: link, Port: 5432}
	if instanceKey(targetInstance) != instanceKey(linkInstance) {
		t.Fatalf("expected symlinked socket dirs to share an instance key")
	}
	if targetInstance.DisplayTarget() == linkInstance.DisplayTarget() {
		t.Fatalf("expected display targets to preserve original paths")
	}
}

func TestDeduplicateInstancesPrefersCredentialSources(t *testing.T) {
	instances := deduplicateInstances([]models.DiscoveredInstance{
		{Host: "localhost", Port: 5432, Source: models.SourcePortScan},
		{Host: "localhost", Port: 5432, Source: models.SourcePgPass},
		{Host: "localhost", Port: 5432, Source: models.SourceEnvironment},
	})

	if len(instances) != 1 {
		t.Fatalf("expected 1 deduplicated instance, got %d", len(instances))
	}
	if instances[0].Source != models.SourceEnvironment {
		t.Fatalf("expected environment source to win, got %v", instances[0].Source)
	}
}

func TestDiscoverAllSortsByDiscoveryPriority(t *testing.T) {
	instances := []models.DiscoveredInstance{
		{Host: "localhost", Port: 5432, Source: models.SourcePortScan},
		{Host: "localhost", Port: 5433, Source: models.SourceEnvironment},
	}

	sortDiscoveredInstances(instances)

	if instances[0].Source != models.SourceEnvironment {
		t.Fatalf("expected environment source first, got %v", instances[0].Source)
	}
}

func TestScanBroadUnixSocketDirChecksKnownPortsOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix sockets are not supported on windows")
	}
	t.Setenv("PGPORT", "")

	oldBroadDirs := broadUnixSocketDirs
	tempDir, err := os.MkdirTemp("/tmp", "pgtui-socket-test-")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(tempDir)
	})

	broadUnixSocketDirs = map[string]struct{}{socketDirKey(tempDir): {}}
	t.Cleanup(func() {
		broadUnixSocketDirs = oldBroadDirs
	})

	listenPostgresSocket(t, tempDir, 6543)

	scanner := NewScanner()
	instances := scanner.scanUnixSocketDir(context.Background(), tempDir)
	if len(instances) != 0 {
		t.Fatalf("expected broad scan to ignore non-candidate port, got %#v", instances)
	}
}

func TestCandidateUnixSocketPortsIncludesPGPort(t *testing.T) {
	t.Setenv("PGPORT", "6543")

	ports := candidateUnixSocketPorts()
	if len(ports) == 0 || ports[0] != 6543 {
		t.Fatalf("expected PGPORT first, got %#v", ports)
	}
}

func listenPostgresSocket(t *testing.T, dir string, port int) <-chan struct{} {
	t.Helper()

	socketPath := filepath.Join(dir, ".s.PGSQL."+strconv.Itoa(port))
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen on unix socket: %v", err)
	}
	t.Cleanup(func() {
		_ = listener.Close()
	})

	acceptDone := make(chan struct{})
	go func() {
		defer close(acceptDone)
		conn, err := listener.Accept()
		if err == nil {
			_ = conn.Close()
		}
	}()

	return acceptDone
}
