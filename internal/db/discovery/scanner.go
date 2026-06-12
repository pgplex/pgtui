package discovery

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/pgplex/pgtui/internal/models"
)

// DefaultPorts are the default PostgreSQL ports to scan
var DefaultPorts = []int{5432, 5433, 5434, 5435}

// Scanner discovers PostgreSQL instances
type Scanner struct {
	timeout time.Duration
}

// NewScanner creates a new scanner
func NewScanner() *Scanner {
	return &Scanner{
		timeout: 2 * time.Second,
	}
}

// ScanPorts scans for PostgreSQL on common ports
func (s *Scanner) ScanPorts(ctx context.Context, host string, ports []int) []models.DiscoveredInstance {
	if len(ports) == 0 {
		ports = DefaultPorts
	}

	instances := make([]models.DiscoveredInstance, 0, len(ports))
	resultChan := make(chan models.DiscoveredInstance, len(ports))

	var wg sync.WaitGroup
	for _, port := range ports {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			instance := s.scanPort(ctx, host, p)
			select {
			case resultChan <- instance:
			case <-ctx.Done():
			}
		}(port)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	for instance := range resultChan {
		if instance.Available {
			instances = append(instances, instance)
		}
	}

	return instances
}

// scanPort checks if a port is open
func (s *Scanner) scanPort(ctx context.Context, host string, port int) models.DiscoveredInstance {
	instance := models.DiscoveredInstance{
		Host:   host,
		Port:   port,
		Source: models.SourcePortScan,
	}

	start := time.Now()
	address := fmt.Sprintf("%s:%d", host, port)

	dialer := &net.Dialer{
		Timeout: s.timeout,
	}

	conn, err := dialer.DialContext(ctx, "tcp", address)
	instance.ResponseTime = time.Since(start)

	if err != nil {
		instance.Available = false
		return instance
	}

	_ = conn.Close()
	instance.Available = true

	return instance
}

// ScanLocalhost scans for PostgreSQL on localhost
func (s *Scanner) ScanLocalhost(ctx context.Context) []models.DiscoveredInstance {
	return s.ScanPorts(ctx, "localhost", DefaultPorts)
}
