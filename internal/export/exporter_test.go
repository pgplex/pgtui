package export

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pgplex/pgtui/internal/models"
)

func TestExportToCSV(t *testing.T) {
	// Create test favorites
	favorites := []models.Favorite{
		{
			ID:          "test-1",
			Name:        "Test Query 1",
			Description: "A test query with commas, quotes \"and\" special chars",
			Query:       "SELECT * FROM users WHERE name = 'test'",
			Tags:        []string{"test", "users"},
			Connection:  "test-conn",
			Database:    "testdb",
			CreatedAt:   time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC),
			LastUsed:    time.Date(2024, 1, 3, 12, 0, 0, 0, time.UTC),
			UsageCount:  5,
		},
		{
			ID:          "test-2",
			Name:        "Test Query 2",
			Description: "Another test",
			Query:       "SELECT COUNT(*) FROM orders",
			Tags:        []string{"test"},
			Connection:  "test-conn",
			Database:    "testdb",
			CreatedAt:   time.Date(2024, 1, 1, 13, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Date(2024, 1, 2, 13, 0, 0, 0, time.UTC),
			UsageCount:  2,
		},
	}

	// Create temp file
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "test.csv")

	// Export
	err := ExportToCSV(favorites, csvPath)
	if err != nil {
		t.Fatalf("ExportToCSV failed: %v", err)
	}

	// Verify file exists and has correct permissions
	info, err := os.Stat(csvPath)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}

	if info.Mode().Perm() != 0644 {
		t.Errorf("Expected file permissions 0644, got %o", info.Mode().Perm())
	}

	// Read and verify CSV content
	file, err := os.Open(csvPath)
	if err != nil {
		t.Fatalf("Failed to open CSV: %v", err)
	}
	defer func() { _ = file.Close() }()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("Failed to read CSV: %v", err)
	}

	// Verify header
	if len(records) != 3 { // header + 2 rows
		t.Fatalf("Expected 3 records, got %d", len(records))
	}

	expectedHeader := []string{"Name", "Description", "Query", "Tags", "Connection", "Database", "Created", "Updated", "Last Used", "Usage Count"}
	if !slicesEqual(records[0], expectedHeader) {
		t.Errorf("Header mismatch.\nExpected: %v\nGot: %v", expectedHeader, records[0])
	}

	// Verify first row
	row1 := records[1]
	if row1[0] != "Test Query 1" {
		t.Errorf("Expected name 'Test Query 1', got '%s'", row1[0])
	}
	if row1[3] != "test, users" {
		t.Errorf("Expected tags 'test, users', got '%s'", row1[3])
	}
	if row1[9] != "5" {
		t.Errorf("Expected usage count '5', got '%s'", row1[9])
	}
}

func TestExportToJSON(t *testing.T) {
	// Create test favorites
	favorites := []models.Favorite{
		{
			ID:          "test-1",
			Name:        "Test Query 1",
			Description: "A test query",
			Query:       "SELECT * FROM users",
			Tags:        []string{"test", "users"},
			Connection:  "test-conn",
			Database:    "testdb",
			CreatedAt:   time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC),
			LastUsed:    time.Date(2024, 1, 3, 12, 0, 0, 0, time.UTC),
			UsageCount:  5,
		},
	}

	// Create temp file
	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "test.json")

	// Export
	err := ExportToJSON(favorites, jsonPath)
	if err != nil {
		t.Fatalf("ExportToJSON failed: %v", err)
	}

	// Verify file exists and has correct permissions
	info, err := os.Stat(jsonPath)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}

	if info.Mode().Perm() != 0644 {
		t.Errorf("Expected file permissions 0644, got %o", info.Mode().Perm())
	}

	// Read and verify JSON content
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("Failed to read JSON: %v", err)
	}

	// Verify it's valid JSON
	var parsed []models.Favorite
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if len(parsed) != 1 {
		t.Fatalf("Expected 1 favorite, got %d", len(parsed))
	}

	if parsed[0].Name != "Test Query 1" {
		t.Errorf("Expected name 'Test Query 1', got '%s'", parsed[0].Name)
	}

	// Verify JSON is pretty-printed (contains newlines and indentation)
	jsonStr := string(data)
	if !strings.Contains(jsonStr, "\n") {
		t.Error("JSON should be pretty-printed with newlines")
	}
	if !strings.Contains(jsonStr, "  ") {
		t.Error("JSON should be indented")
	}
}

func TestExportEmptyFavorites(t *testing.T) {
	tmpDir := t.TempDir()

	// Test CSV with empty list
	csvPath := filepath.Join(tmpDir, "empty.csv")
	err := ExportToCSV([]models.Favorite{}, csvPath)
	if err != nil {
		t.Fatalf("ExportToCSV with empty list failed: %v", err)
	}

	// Verify CSV has header
	file, err := os.Open(csvPath)
	if err != nil {
		t.Fatalf("Failed to open CSV: %v", err)
	}
	defer func() { _ = file.Close() }()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("Failed to read CSV: %v", err)
	}

	if len(records) != 1 { // Only header
		t.Errorf("Expected 1 record (header), got %d", len(records))
	}

	// Test JSON with empty list
	jsonPath := filepath.Join(tmpDir, "empty.json")
	err = ExportToJSON([]models.Favorite{}, jsonPath)
	if err != nil {
		t.Fatalf("ExportToJSON with empty list failed: %v", err)
	}

	// Verify JSON is valid empty array
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("Failed to read JSON: %v", err)
	}

	var parsed []models.Favorite
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if len(parsed) != 0 {
		t.Errorf("Expected 0 favorites, got %d", len(parsed))
	}
}

// Helper function to compare slices
func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
