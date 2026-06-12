# AI Assistant Development Guide

This document provides guidance for AI assistants (Claude, GPT, etc.) working on this codebase. It documents key patterns, common pitfalls, and project-specific conventions.

## Project Overview

pgtui is a terminal UI client for PostgreSQL built with:
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - TUI framework
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) - Terminal styling
- [pgx](https://github.com/jackc/pgx) - PostgreSQL driver

## Code Organization

```
pgtui/
├── cmd/pgtui/          # Main entry point
├── internal/
│   ├── app/             # Main application logic
│   ├── ui/components/   # Reusable UI components
│   ├── ui/theme/        # Color themes
│   ├── db/              # Database operations
│   ├── models/          # Data structures
│   └── ...              # Other internal packages
├── config/              # Default configuration
└── docs/                # Documentation
```

## Critical: Lipgloss Width Calculation

When using `lipgloss` borders with `Width()` or `MaxWidth()`, borders and padding render **outside** the content area. This causes overflow if not handled correctly.

### The Correct Pattern

Always use `GetHorizontalFrameSize()` to calculate available content width:

```go
// Define style FIRST
containerStyle := lipgloss.NewStyle().
    Border(lipgloss.RoundedBorder()).
    Padding(1, 2)

// Calculate content width
contentWidth := terminalWidth - containerStyle.GetHorizontalFrameSize()

// Render with calculated width
content := renderContent(contentWidth)
return containerStyle.Render(content)
```

### Common Mistake

```go
// DON'T DO THIS - width doesn't account for frame
containerStyle := lipgloss.NewStyle().
    Border(lipgloss.RoundedBorder()).
    Padding(1, 2).
    Width(80)  // Will overflow by 6 chars!
```

### Nested Components

Pass content width down through component hierarchy:

```go
// Parent calculates and passes width
contentWidth := maxWidth - parentStyle.GetHorizontalFrameSize()
childContent := child.Render(contentWidth)

// Child subtracts its own frame
childContentWidth := contentWidth - childStyle.GetHorizontalFrameSize()
```

## Performance Considerations

### Style Caching

Create styles once and reuse them. Don't create new `lipgloss.Style` objects in render loops:

```go
// GOOD: Cache styles in struct
type MyComponent struct {
    cachedStyles *myStyles
}

func (c *MyComponent) initStyles() {
    c.cachedStyles = &myStyles{
        header: lipgloss.NewStyle().Bold(true),
        // ...
    }
}

// BAD: Creating styles on every render
func (c *MyComponent) View() string {
    style := lipgloss.NewStyle().Bold(true)  // Don't do this
}
```

### Mouse Support

Use [bubblezone](https://github.com/lrstanley/bubblezone) for mouse click detection:

```go
// Mark clickable zones
zone.Mark("button-id", buttonContent)

// Check clicks in Update
if zone.Get("button-id").InBounds(mouseMsg) {
    // Handle click
}
```

Initialize bubblezone in tests:
```go
func init() {
    zone.NewGlobal()
}
```

## Conventions

### Error Handling

- Log errors but don't crash on non-critical failures
- Show user-friendly messages in UI overlays
- Use `log.Printf("Warning: ...")` for non-fatal errors

### Testing

- Initialize `zone.NewGlobal()` in test files that call `View()` methods
- Test UI at 80-char terminal width
- Run `go test ./...` before committing

### Commits

- Use conventional commit format: `feat:`, `fix:`, `perf:`, etc.
- Keep commits focused and atomic

## Common Tasks

### Adding a New UI Component

1. Create file in `internal/ui/components/`
2. Implement `tea.Model` interface (Init, Update, View)
3. Add style caching with `initStyles()` method
4. Add zone marks for mouse support if needed
5. Add tests with `zone.NewGlobal()` in init

### Adding a New Command

1. Add command type in `internal/commands/commands.go`
2. Register in command registry
3. Handle message in `internal/app/app.go` Update method

## Resources

- [Bubble Tea Documentation](https://github.com/charmbracelet/bubbletea)
- [Lip Gloss Documentation](https://github.com/charmbracelet/lipgloss)
- [Project Documentation](docs/INDEX.md)
