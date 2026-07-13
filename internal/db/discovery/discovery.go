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

	// 2. Scan common Unix socket directories
	instances = append(instances, d.scanner.ScanUnixSockets(ctx)...)

	// 3. Scan localhost ports
	instances = append(instances, d.scanner.ScanLocalhost(ctx)...)

	// 4. Parse .pgpass
	instances = append(instances, GetDiscoveredInstances()...)

	// Deduplicate
	instances = deduplicateInstances(instances)

	// Sort by source priority
	sortDiscoveredInstances(instances)

	return instances
}

func sortDiscoveredInstances(instances []models.DiscoveredInstance) {
	sort.Slice(instances, func(i, j int) bool {
		if instances[i].Source != instances[j].Source {
			return discoverySourcePriority(instances[i].Source) < discoverySourcePriority(instances[j].Source)
		}

		if instances[i].Host != instances[j].Host {
			return instances[i].Host < instances[j].Host
		}

		return instances[i].Port < instances[j].Port
	})
}

// deduplicateInstances removes duplicate connection targets.
func deduplicateInstances(instances []models.DiscoveredInstance) []models.DiscoveredInstance {
	seen := make(map[string]models.DiscoveredInstance)

	for _, instance := range instances {
		key := instanceKey(instance)

		// Keep the one with higher priority source
		if existing, exists := seen[key]; !exists || discoverySourcePriority(instance.Source) < discoverySourcePriority(existing.Source) {
			seen[key] = instance
		}
	}

	result := make([]models.DiscoveredInstance, 0, len(seen))
	for _, instance := range seen {
		result = append(result, instance)
	}

	return result
}

func instanceKey(instance models.DiscoveredInstance) string {
	host := instance.Host
	if instance.UsesUnixSocket() {
		host = socketDirKey(host)
	}

	return host + ":" + strconv.Itoa(instance.Port)
}

func discoverySourcePriority(source models.DiscoverySource) int {
	switch source {
	case models.SourceEnvironment:
		return 0
	case models.SourcePgPass:
		return 1
	case models.SourcePgService:
		return 2
	case models.SourceConfig:
		return 3
	case models.SourceUnixSocket:
		return 4
	case models.SourcePortScan:
		return 5
	default:
		return 100
	}
}
