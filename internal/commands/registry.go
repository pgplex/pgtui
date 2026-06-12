package commands

import (
	"strings"
	"sync"

	"github.com/pgplex/pgtui/internal/models"
)

// Registry manages available commands
type Registry struct {
	commands map[string]models.Command
	mu       sync.RWMutex
}

// NewRegistry creates a new command registry
func NewRegistry() *Registry {
	return &Registry{
		commands: make(map[string]models.Command),
	}
}

// Register adds a command to the registry
func (r *Registry) Register(cmd models.Command) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commands[cmd.ID] = cmd
}

// Get retrieves a command by ID
func (r *Registry) Get(id string) (models.Command, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cmd, ok := r.commands[id]
	return cmd, ok
}

// GetAll returns all registered commands
func (r *Registry) GetAll() []models.Command {
	r.mu.RLock()
	defer r.mu.RUnlock()

	commands := make([]models.Command, 0, len(r.commands))
	for _, cmd := range r.commands {
		commands = append(commands, cmd)
	}
	return commands
}

// Search searches commands by query string
func (r *Registry) Search(query string) []models.Command {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query = strings.ToLower(query)
	var results []models.Command

	for _, cmd := range r.commands {
		// Simple substring matching
		if strings.Contains(strings.ToLower(cmd.Label), query) ||
			strings.Contains(strings.ToLower(cmd.Description), query) {
			results = append(results, cmd)
		} else {
			// Check tags
			for _, tag := range cmd.Tags {
				if strings.Contains(strings.ToLower(tag), query) {
					results = append(results, cmd)
					break
				}
			}
		}
	}

	return results
}

// Unregister removes a command from the registry
func (r *Registry) Unregister(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.commands, id)
}
