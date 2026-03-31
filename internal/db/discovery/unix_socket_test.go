package discovery

import (
	"context"
	"net"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/rebelice/lazypg/internal/models"
)

func TestScanUnixSocketDirs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix sockets are not supported on windows")
	}

	tempDir := t.TempDir()
	socketPath := filepath.Join(tempDir, ".s.PGSQL.6543")

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen on unix socket: %v", err)
	}
	defer func() {
		_ = listener.Close()
	}()

	acceptDone := make(chan struct{})
	go func() {
		defer close(acceptDone)
		conn, err := listener.Accept()
		if err == nil {
			_ = conn.Close()
		}
	}()

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
	if config.Database != "socket-user" {
		t.Fatalf("expected database socket-user, got %q", config.Database)
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
