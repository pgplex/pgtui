package export

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/pgplex/pgtui/internal/models"
)

// ExportToCSV exports favorites to a CSV file
func ExportToCSV(favorites []models.Favorite, path string) error {
	// Create the file
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create CSV file: %w", err)
	}
	defer func() { _ = file.Close() }()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	header := []string{"Name", "Description", "Query", "Tags", "Connection", "Database", "Created", "Updated", "Last Used", "Usage Count"}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write each favorite
	for _, fav := range favorites {
		// Join tags with commas
		tags := strings.Join(fav.Tags, ", ")

		// Format timestamps
		created := fav.CreatedAt.Format("2006-01-02 15:04:05")
		updated := fav.UpdatedAt.Format("2006-01-02 15:04:05")
		lastUsed := ""
		if !fav.LastUsed.IsZero() {
			lastUsed = fav.LastUsed.Format("2006-01-02 15:04:05")
		}

		// Build row
		row := []string{
			fav.Name,
			fav.Description,
			fav.Query,
			tags,
			fav.Connection,
			fav.Database,
			created,
			updated,
			lastUsed,
			fmt.Sprintf("%d", fav.UsageCount),
		}

		if err := writer.Write(row); err != nil {
			return fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	return nil
}

// ExportToJSON exports favorites to a JSON file
func ExportToJSON(favorites []models.Favorite, path string) error {
	// Marshal to JSON with pretty printing
	data, err := json.MarshalIndent(favorites, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal favorites to JSON: %w", err)
	}

	// Write to file
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write JSON file: %w", err)
	}

	return nil
}
