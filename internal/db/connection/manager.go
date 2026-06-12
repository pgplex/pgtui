package connection

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/pgplex/pgtui/internal/models"
)

// Manager manages multiple database connections
type Manager struct {
	connections map[string]*Connection
	active      string
	mu          sync.RWMutex
}

// Connection wraps a pool with metadata
type Connection struct {
	ID          string
	Config      models.ConnectionConfig
	Pool        *Pool
	Connected   bool
	ConnectedAt time.Time
	LastPing    time.Time
	Error       error
}

// NewManager creates a new connection manager
func NewManager() *Manager {
	return &Manager{
		connections: make(map[string]*Connection),
	}
}

// Connect establishes a new connection
func (m *Manager) Connect(ctx context.Context, config models.ConnectionConfig) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := generateConnectionID(config)

	// Close existing connection if present
	if existing, ok := m.connections[id]; ok {
		if existing.Pool != nil {
			existing.Pool.Close()
		}
	}

	pool, err := NewPool(ctx, config)
	if err != nil {
		conn := &Connection{
			ID:        id,
			Config:    config,
			Connected: false,
			Error:     err,
		}
		m.connections[id] = conn
		return id, err
	}

	conn := &Connection{
		ID:          id,
		Config:      config,
		Pool:        pool,
		Connected:   true,
		ConnectedAt: time.Now(),
		LastPing:    time.Now(),
	}

	m.connections[id] = conn
	m.active = id

	return id, nil
}

// Disconnect closes a connection
func (m *Manager) Disconnect(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	conn, ok := m.connections[id]
	if !ok {
		return fmt.Errorf("connection %s not found", id)
	}

	if conn.Pool != nil {
		conn.Pool.Close()
	}

	delete(m.connections, id)

	if m.active == id {
		m.active = ""
	}

	return nil
}

// GetActive returns the active connection
func (m *Manager) GetActive() (*Connection, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.active == "" {
		return nil, fmt.Errorf("no active connection")
	}

	conn, ok := m.connections[m.active]
	if !ok {
		return nil, fmt.Errorf("active connection not found")
	}

	return conn, nil
}

// SetActive sets the active connection
func (m *Manager) SetActive(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.connections[id]; !ok {
		return fmt.Errorf("connection %s not found", id)
	}

	m.active = id
	return nil
}

// GetAll returns all connections
func (m *Manager) GetAll() []*Connection {
	m.mu.RLock()
	defer m.mu.RUnlock()

	conns := make([]*Connection, 0, len(m.connections))
	for _, conn := range m.connections {
		conns = append(conns, conn)
	}
	return conns
}

// Ping tests the active connection
func (m *Manager) Ping(ctx context.Context) error {
	m.mu.RLock()
	activeID := m.active
	conn, ok := m.connections[activeID]
	m.mu.RUnlock()

	if !ok || activeID == "" {
		return fmt.Errorf("no active connection")
	}

	if conn.Pool == nil {
		return fmt.Errorf("connection pool not initialized")
	}

	err := conn.Pool.Ping(ctx)

	m.mu.Lock()
	// Verify connection still exists and is still active
	if c, exists := m.connections[activeID]; exists && c == conn {
		if err != nil {
			c.Error = err
			c.Connected = false
		} else {
			c.LastPing = time.Now()
			c.Connected = true
			c.Error = nil
		}
	}
	m.mu.Unlock()

	return err
}

// generateConnectionID creates a unique connection ID
func generateConnectionID(config models.ConnectionConfig) string {
	if config.Name != "" {
		return config.Name
	}
	return fmt.Sprintf("%s@%s:%d/%s", config.User, config.Host, config.Port, config.Database)
}
