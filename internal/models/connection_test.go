package models

import "testing"

func TestConnectionEndpointFormatsIPv6(t *testing.T) {
	config := ConnectionConfig{
		Host:     "::1",
		Port:     5432,
		Database: "postgres",
		User:     "alice",
	}

	if got := config.Endpoint(); got != "[::1]:5432" {
		t.Fatalf("expected bracketed IPv6 endpoint, got %q", got)
	}
	if got := config.DisplayTarget(); got != "[::1]:5432" {
		t.Fatalf("expected bracketed IPv6 display target, got %q", got)
	}
	if got := config.ConnectionLabel(); got != "alice@[::1]:5432/postgres" {
		t.Fatalf("unexpected connection label %q", got)
	}
}

func TestConnectionEndpointFormatsUnixSocket(t *testing.T) {
	config := ConnectionConfig{
		Host: "/tmp",
		Port: 5432,
	}

	if got := config.Endpoint(); got != "/tmp/.s.PGSQL.5432" {
		t.Fatalf("unexpected unix socket endpoint %q", got)
	}
}
