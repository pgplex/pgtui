package favorites

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pgplex/pgtui/internal/export"
	"github.com/pgplex/pgtui/internal/models"
	"gopkg.in/yaml.v3"
)

// Manager manages query favorites
type Manager struct {
	path      string
	favorites []models.Favorite
}

// NewManager creates a new favorites manager
func NewManager(configDir string) (*Manager, error) {
	path := filepath.Join(configDir, "favorites.yaml")

	m := &Manager{
		path:      path,
		favorites: []models.Favorite{},
	}

	// Load existing favorites if file exists
	if _, err := os.Stat(path); err == nil {
		if err := m.Load(); err != nil {
			return nil, fmt.Errorf("failed to load favorites: %w", err)
		}
	}

	return m, nil
}

// Load loads favorites from YAML file
func (m *Manager) Load() error {
	data, err := os.ReadFile(m.path)
	if err != nil {
		return fmt.Errorf("failed to read favorites file: %w", err)
	}

	if err := yaml.Unmarshal(data, &m.favorites); err != nil {
		return fmt.Errorf("failed to parse favorites: %w", err)
	}

	return nil
}

// Save saves favorites to YAML file
func (m *Manager) Save() error {
	data, err := yaml.Marshal(m.favorites)
	if err != nil {
		return fmt.Errorf("failed to marshal favorites: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(m.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	if err := os.WriteFile(m.path, data, 0644); err != nil {
		return fmt.Errorf("failed to write favorites file: %w", err)
	}

	return nil
}

// Add adds a new favorite
func (m *Manager) Add(name, description, query, connection, database string, tags []string) (*models.Favorite, error) {
	// Validate inputs
	name = strings.TrimSpace(name)
	query = strings.TrimSpace(query)

	if name == "" {
		return nil, fmt.Errorf("favorite name cannot be empty")
	}
	if query == "" {
		return nil, fmt.Errorf("favorite query cannot be empty")
	}

	// Check for duplicate names (case-insensitive)
	for _, fav := range m.favorites {
		if strings.EqualFold(fav.Name, name) {
			return nil, fmt.Errorf("a favorite with the name '%s' already exists (names are case-insensitive)", name)
		}
	}

	favorite := models.Favorite{
		ID:          uuid.New().String(),
		Name:        name,
		Description: strings.TrimSpace(description),
		Query:       query,
		Tags:        tags,
		Connection:  connection,
		Database:    database,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		UsageCount:  0,
		LastUsed:    time.Time{},
	}

	m.favorites = append(m.favorites, favorite)

	if err := m.Save(); err != nil {
		return nil, fmt.Errorf("failed to save favorite: %w", err)
	}

	return &favorite, nil
}

// Update updates an existing favorite
func (m *Manager) Update(id string, name, description, query string, tags []string) error {
	// Validate inputs
	name = strings.TrimSpace(name)
	query = strings.TrimSpace(query)

	if name == "" {
		return fmt.Errorf("favorite name cannot be empty")
	}
	if query == "" {
		return fmt.Errorf("favorite query cannot be empty")
	}

	// Check for duplicate names (case-insensitive, excluding the current favorite)
	for _, fav := range m.favorites {
		if fav.ID != id && strings.EqualFold(fav.Name, name) {
			return fmt.Errorf("a favorite with the name '%s' already exists (names are case-insensitive)", name)
		}
	}

	for i, fav := range m.favorites {
		if fav.ID == id {
			m.favorites[i].Name = name
			m.favorites[i].Description = strings.TrimSpace(description)
			m.favorites[i].Query = query
			m.favorites[i].Tags = tags
			m.favorites[i].UpdatedAt = time.Now()
			if err := m.Save(); err != nil {
				return fmt.Errorf("failed to save favorite: %w", err)
			}
			return nil
		}
	}
	return fmt.Errorf("favorite with ID '%s' was not found", id)
}

// Delete deletes a favorite by ID
func (m *Manager) Delete(id string) error {
	for i, fav := range m.favorites {
		if fav.ID == id {
			m.favorites = append(m.favorites[:i], m.favorites[i+1:]...)
			if err := m.Save(); err != nil {
				return fmt.Errorf("failed to save favorites after deletion: %w", err)
			}
			return nil
		}
	}
	return fmt.Errorf("favorite with ID '%s' was not found", id)
}

// Get returns a favorite by ID
func (m *Manager) Get(id string) (*models.Favorite, error) {
	for _, fav := range m.favorites {
		if fav.ID == id {
			return &fav, nil
		}
	}
	return nil, fmt.Errorf("favorite with ID '%s' was not found", id)
}

// GetAll returns all favorites
func (m *Manager) GetAll() []models.Favorite {
	return m.favorites
}

// Search searches favorites by name, description, or tags
func (m *Manager) Search(query string) []models.Favorite {
	if query == "" {
		return m.favorites
	}

	query = strings.ToLower(query)
	var results []models.Favorite

	for _, fav := range m.favorites {
		// Search in name
		if strings.Contains(strings.ToLower(fav.Name), query) {
			results = append(results, fav)
			continue
		}

		// Search in description
		if strings.Contains(strings.ToLower(fav.Description), query) {
			results = append(results, fav)
			continue
		}

		// Search in tags
		for _, tag := range fav.Tags {
			if strings.Contains(strings.ToLower(tag), query) {
				results = append(results, fav)
				break
			}
		}
	}

	return results
}

// RecordUsage updates usage statistics for a favorite
func (m *Manager) RecordUsage(id string) error {
	for i, fav := range m.favorites {
		if fav.ID == id {
			m.favorites[i].UsageCount++
			m.favorites[i].LastUsed = time.Now()
			if err := m.Save(); err != nil {
				return fmt.Errorf("failed to save usage statistics: %w", err)
			}
			return nil
		}
	}
	return fmt.Errorf("favorite with ID '%s' was not found", id)
}

// GetMostUsed returns the most frequently used favorites
func (m *Manager) GetMostUsed(limit int) []models.Favorite {
	sorted := make([]models.Favorite, len(m.favorites))
	copy(sorted, m.favorites)

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].UsageCount > sorted[j].UsageCount
	})

	if limit > 0 && limit < len(sorted) {
		sorted = sorted[:limit]
	}

	return sorted
}

// GetRecent returns the most recently used favorites
func (m *Manager) GetRecent(limit int) []models.Favorite {
	sorted := make([]models.Favorite, len(m.favorites))
	copy(sorted, m.favorites)

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].LastUsed.After(sorted[j].LastUsed)
	})

	if limit > 0 && limit < len(sorted) {
		sorted = sorted[:limit]
	}

	return sorted
}

// ExportToCSV exports all favorites to a CSV file
func (m *Manager) ExportToCSV(customPath ...string) (string, error) {
	if len(m.favorites) == 0 {
		return "", fmt.Errorf("no favorites to export")
	}

	// Determine export path
	path := filepath.Join(filepath.Dir(m.path), "favorites.csv")
	if len(customPath) > 0 && customPath[0] != "" {
		path = customPath[0]
	}

	// Export favorites
	if err := export.ExportToCSV(m.favorites, path); err != nil {
		return "", fmt.Errorf("failed to export favorites to CSV: %w", err)
	}

	return path, nil
}

// ExportToJSON exports all favorites to a JSON file
func (m *Manager) ExportToJSON(customPath ...string) (string, error) {
	if len(m.favorites) == 0 {
		return "", fmt.Errorf("no favorites to export")
	}

	// Determine export path
	path := filepath.Join(filepath.Dir(m.path), "favorites.json")
	if len(customPath) > 0 && customPath[0] != "" {
		path = customPath[0]
	}

	// Export favorites
	if err := export.ExportToJSON(m.favorites, path); err != nil {
		return "", fmt.Errorf("failed to export favorites to JSON: %w", err)
	}

	return path, nil
}
