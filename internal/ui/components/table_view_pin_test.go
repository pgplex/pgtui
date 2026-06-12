package components

import (
	"strings"
	"testing"

	zone "github.com/lrstanley/bubblezone"
	"github.com/pgplex/pgtui/internal/ui/theme"
)

func init() {
	zone.NewGlobal()
}

func TestTableView_PinUnpin(t *testing.T) {
	th := theme.GetTheme("default")
	tv := NewTableView(th)

	// Set up test data
	columns := []string{"id", "name"}
	rows := [][]string{
		{"1", "Alice"},
		{"2", "Bob"},
		{"3", "Charlie"},
	}
	tv.SetData(columns, rows, 3)
	tv.Width = 80
	tv.Height = 20

	// Select row 1
	tv.SelectedRow = 1

	// Pin row
	err := tv.TogglePin()
	if err != nil {
		t.Fatalf("failed to pin: %v", err)
	}

	if len(tv.PinnedRows) != 1 {
		t.Fatalf("expected 1 pinned row, got %d", len(tv.PinnedRows))
	}

	if tv.PinnedRows[0] != 1 {
		t.Fatalf("expected row 1 pinned, got %d", tv.PinnedRows[0])
	}

	// Verify data copied
	if tv.PinnedData[0][1] != "Bob" {
		t.Fatalf("expected Bob, got %s", tv.PinnedData[0][1])
	}

	// Unpin
	err = tv.TogglePin()
	if err != nil {
		t.Fatalf("failed to unpin: %v", err)
	}

	if len(tv.PinnedRows) != 0 {
		t.Fatalf("expected 0 pinned rows, got %d", len(tv.PinnedRows))
	}
}

func TestTableView_MaxPinnedRows(t *testing.T) {
	th := theme.GetTheme("default")
	tv := NewTableView(th)
	tv.MaxPinnedRows = 2

	columns := []string{"id"}
	rows := [][]string{{"1"}, {"2"}, {"3"}}
	tv.SetData(columns, rows, 3)

	// Pin 2 rows
	tv.SelectedRow = 0
	if err := tv.TogglePin(); err != nil {
		t.Fatalf("failed to pin row 0: %v", err)
	}
	tv.SelectedRow = 1
	if err := tv.TogglePin(); err != nil {
		t.Fatalf("failed to pin row 1: %v", err)
	}

	// Try to pin 3rd row
	tv.SelectedRow = 2
	err := tv.TogglePin()
	if err == nil {
		t.Fatal("expected error when exceeding max pinned rows")
	}

	if len(tv.PinnedRows) != 2 {
		t.Fatalf("expected 2 pinned rows, got %d", len(tv.PinnedRows))
	}
}

func TestTableView_PinnedRowsInView(t *testing.T) {
	th := theme.GetTheme("default")
	tv := NewTableView(th)

	columns := []string{"id", "name"}
	rows := [][]string{
		{"1", "Alice"},
		{"2", "Bob"},
	}
	tv.SetData(columns, rows, 2)
	tv.Width = 80
	tv.Height = 20

	// Pin a row
	tv.SelectedRow = 0
	if err := tv.TogglePin(); err != nil {
		t.Fatalf("failed to pin: %v", err)
	}

	// Render view - verify it doesn't panic
	view := tv.View()

	// Verify pinned count is shown in status
	if !strings.Contains(view, "1 pinned") {
		t.Errorf("expected '1 pinned' in view, got:\n%s", view)
	}
}

func TestTableView_IsPinned(t *testing.T) {
	th := theme.GetTheme("default")
	tv := NewTableView(th)

	columns := []string{"id"}
	rows := [][]string{{"1"}, {"2"}, {"3"}}
	tv.SetData(columns, rows, 3)

	tv.SelectedRow = 1
	_ = tv.TogglePin()

	if !tv.IsPinned(1) {
		t.Fatal("row 1 should be pinned")
	}

	if tv.IsPinned(0) {
		t.Fatal("row 0 should not be pinned")
	}

	if tv.IsPinned(2) {
		t.Fatal("row 2 should not be pinned")
	}
}

func TestTableView_ClearPins(t *testing.T) {
	th := theme.GetTheme("default")
	tv := NewTableView(th)

	columns := []string{"id"}
	rows := [][]string{{"1"}, {"2"}, {"3"}}
	tv.SetData(columns, rows, 3)

	// Pin multiple rows
	tv.SelectedRow = 0
	_ = tv.TogglePin()
	tv.SelectedRow = 1
	_ = tv.TogglePin()

	if tv.GetPinnedCount() != 2 {
		t.Fatalf("expected 2 pinned rows, got %d", tv.GetPinnedCount())
	}

	// Clear all pins
	tv.ClearPins()

	if tv.GetPinnedCount() != 0 {
		t.Fatalf("expected 0 pinned rows after clear, got %d", tv.GetPinnedCount())
	}
}
