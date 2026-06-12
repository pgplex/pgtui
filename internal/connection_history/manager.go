package connection_history

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/pgplex/pgtui/internal/models"
	"gopkg.in/yaml.v3"
)

// ConnectionConfigResult contains the result of getting a connection config with password
type ConnectionConfigResult struct {
	Config          models.ConnectionConfig
	PasswordMissing bool
	Error           error
}

// Manager manages connection history
type Manager struct {
	path          string
	configDir     string
	history       []models.ConnectionHistoryEntry
	passwordStore *PasswordStore
}

// NewManager creates a new connection history manager
func NewManager(configDir string) (*Manager, error) {
	path := filepath.Join(configDir, "connection_history.yaml")

	m := &Manager{
		path:      path,
		configDir: configDir,
		history:   []models.ConnectionHistoryEntry{},
	}

	// Initialize password store
	passwordStore, err := NewPasswordStore(configDir)
	if err != nil {
		// Log warning but continue without password storage
		log.Printf("Warning: Failed to initialize password store: %v", err)
	} else {
		m.passwordStore = passwordStore
		if passwordStore.IsUsingFallback() {
			log.Printf("Info: Using encrypted file for password storage (system keyring unavailable)")
		}
	}

	// Load existing history if file exists
	if _, err := os.Stat(path); err == nil {
		if err := m.Load(); err != nil {
			return nil, fmt.Errorf("failed to load connection history: %w", err)
		}
	}

	return m, nil
}

// IsUsingFallbackStorage returns true if passwords are stored in encrypted files
// instead of the native OS keyring
func (m *Manager) IsUsingFallbackStorage() bool {
	if m.passwordStore == nil {
		return false
	}
	return m.passwordStore.IsUsingFallback()
}

// Load loads connection history from YAML file
func (m *Manager) Load() error {
	data, err := os.ReadFile(m.path)
	if err != nil {
		return fmt.Errorf("failed to read connection history file: %w", err)
	}

	if err := yaml.Unmarshal(data, &m.history); err != nil {
		return fmt.Errorf("failed to parse connection history: %w", err)
	}

	return nil
}

// Save saves connection history to YAML file
func (m *Manager) Save() error {
	data, err := yaml.Marshal(m.history)
	if err != nil {
		return fmt.Errorf("failed to marshal connection history: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(m.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	if err := os.WriteFile(m.path, data, 0600); err != nil { // 0600 for security (connection info)
		return fmt.Errorf("failed to write connection history file: %w", err)
	}

	return nil
}

// AddResult contains the result of adding a connection
type AddResult struct {
	PasswordSaveError error
}

// Add adds or updates a connection in history
func (m *Manager) Add(config models.ConnectionConfig) (*AddResult, error) {
	result := &AddResult{}

	// Check if this connection already exists (match by host, port, database, user)
	for i, entry := range m.history {
		if entry.Host == config.Host &&
			entry.Port == config.Port &&
			entry.Database == config.Database &&
			entry.User == config.User {
			// Update existing entry
			m.history[i].LastUsed = time.Now()
			m.history[i].UsageCount++
			m.history[i].SSLMode = config.SSLMode
			// Update name if config has one
			if config.Name != "" {
				m.history[i].Name = config.Name
			}
			// Note: Don't save password here for existing connections
			// Password from keyring is already saved; manually entered password
			// will be saved separately after successful connection
			return result, m.Save()
		}
	}

	// For NEW connections, save password to secure keyring (if provided)
	if config.Password != "" && m.passwordStore != nil {
		if err := m.passwordStore.Save(config.Host, config.Port, config.Database, config.User, config.Password); err != nil {
			// Store the error but don't fail - caller can decide how to handle
			result.PasswordSaveError = err
		}
	}

	// Create new entry
	name := config.Name
	if name == "" {
		name = fmt.Sprintf("%s@%s:%d/%s", config.User, config.Host, config.Port, config.Database)
	}

	entry := models.ConnectionHistoryEntry{
		ID:         uuid.New().String(),
		Name:       name,
		Host:       config.Host,
		Port:       config.Port,
		Database:   config.Database,
		User:       config.User,
		SSLMode:    config.SSLMode,
		LastUsed:   time.Now(),
		UsageCount: 1,
		CreatedAt:  time.Now(),
	}

	m.history = append(m.history, entry)

	return result, m.Save()
}

// GetAll returns all connection history entries
func (m *Manager) GetAll() []models.ConnectionHistoryEntry {
	return m.history
}

// GetRecent returns the most recently used connections
func (m *Manager) GetRecent(limit int) []models.ConnectionHistoryEntry {
	sorted := make([]models.ConnectionHistoryEntry, len(m.history))
	copy(sorted, m.history)

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].LastUsed.After(sorted[j].LastUsed)
	})

	if limit > 0 && limit < len(sorted) {
		sorted = sorted[:limit]
	}

	return sorted
}

// GetMostUsed returns the most frequently used connections
func (m *Manager) GetMostUsed(limit int) []models.ConnectionHistoryEntry {
	sorted := make([]models.ConnectionHistoryEntry, len(m.history))
	copy(sorted, m.history)

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].UsageCount > sorted[j].UsageCount
	})

	if limit > 0 && limit < len(sorted) {
		sorted = sorted[:limit]
	}

	return sorted
}

// Delete removes a connection from history by ID
func (m *Manager) Delete(id string) error {
	for i, entry := range m.history {
		if entry.ID == id {
			// Also delete password from keyring
			if m.passwordStore != nil {
				_ = m.passwordStore.Delete(entry.Host, entry.Port, entry.Database, entry.User)
			}
			m.history = append(m.history[:i], m.history[i+1:]...)
			return m.Save()
		}
	}
	return fmt.Errorf("connection history entry with ID '%s' not found", id)
}

// GetConnectionConfigWithPassword returns a ConnectionConfig with password retrieved from keyring.
// If password retrieval fails, PasswordMissing will be true and the caller should prompt for password.
func (m *Manager) GetConnectionConfigWithPassword(entry *models.ConnectionHistoryEntry) ConnectionConfigResult {
	config := entry.ToConnectionConfig()

	if m.passwordStore == nil {
		return ConnectionConfigResult{
			Config:          config,
			PasswordMissing: true,
			Error:           fmt.Errorf("password store not initialized"),
		}
	}

	password, err := m.passwordStore.Get(entry.Host, entry.Port, entry.Database, entry.User)
	if err != nil {
		if errors.Is(err, ErrPasswordNotFound) {
			return ConnectionConfigResult{
				Config:          config,
				PasswordMissing: true,
			}
		}
		return ConnectionConfigResult{
			Config:          config,
			PasswordMissing: true,
			Error:           err,
		}
	}

	// Empty password also means missing (user might need to enter it)
	if password == "" {
		return ConnectionConfigResult{
			Config:          config,
			PasswordMissing: true,
		}
	}

	config.Password = password
	return ConnectionConfigResult{Config: config}
}

// SavePassword saves a password for an existing connection
func (m *Manager) SavePassword(host string, port int, database, user, password string) error {
	if m.passwordStore == nil {
		return fmt.Errorf("password store not initialized")
	}
	return m.passwordStore.Save(host, port, database, user, password)
}
