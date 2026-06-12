package discovery

import (
	"context"
	"sort"
	"strconv"

	"github.com/pgplex/pgtui/internal/models"
)

// Discoverer coordinates all discovery methods
type Discoverer struct {
	scanner *Scanner
}

// NewDiscoverer creates a new discoverer
func NewDiscoverer() *Discoverer {
	return &Discoverer{
		scanner: NewScanner(),
	}
}

// DiscoverAll runs all discovery methods
func (d *Discoverer) DiscoverAll(ctx context.Context) []models.DiscoveredInstance {
	instances := make([]models.DiscoveredInstance, 0)

	// 1. Check environment variables
	if envInstance := ParseEnvironment(); envInstance != nil {
		instances = append(instances, *envInstance)
	}

	// 2. Scan localhost ports
	localInstances := d.scanner.ScanLocalhost(ctx)
	instances = append(instances, localInstances...)

	// 3. Parse .pgpass
	pgpassInstances := GetDiscoveredInstances()
	instances = append(instances, pgpassInstances...)

	// Deduplicate
	instances = deduplicateInstances(instances)

	// Sort by source priority
	sort.Slice(instances, func(i, j int) bool {
		return instances[i].Source < instances[j].Source
	})

	return instances
}

// deduplicateInstances removes duplicate host:port combinations
func deduplicateInstances(instances []models.DiscoveredInstance) []models.DiscoveredInstance {
	seen := make(map[string]models.DiscoveredInstance)

	for _, instance := range instances {
		key := instance.Host + ":" + strconv.Itoa(instance.Port)

		// Keep the one with higher priority source
		if existing, exists := seen[key]; !exists || instance.Source < existing.Source {
			seen[key] = instance
		}
	}

	result := make([]models.DiscoveredInstance, 0, len(seen))
	for _, instance := range seen {
		result = append(result, instance)
	}

	return result
}
