package main

import (
	"context"
	"fmt"
	"log"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"
	"github.com/pgplex/pgtui/internal/app"
	"github.com/pgplex/pgtui/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Warning: Could not load config: %v (using defaults)\n", err)
		cfg = config.GetDefaults()
	}

	// Initialize bubblezone for mouse support
	zone.NewGlobal()

	// Create context for connection management
	ctx := context.Background()
	_ = ctx // Context will be used in later tasks for discovery

	app := app.New(cfg)

	opts := []tea.ProgramOption{tea.WithAltScreen()}
	if cfg.UI.MouseEnabled {
		opts = append(opts, tea.WithMouseCellMotion())
	}

	p := tea.NewProgram(app, opts...)
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v\n", err)
		os.Exit(1)
	}
}
