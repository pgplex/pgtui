# pgtui User Guide

A complete guide to using pgtui, the terminal UI for PostgreSQL.

## Table of Contents

- [Getting Started](#getting-started)
- [Connecting to Databases](#connecting-to-databases)
- [Navigating the Interface](#navigating-the-interface)
- [Browsing Data](#browsing-data)
- [Searching and Filtering](#searching-and-filtering)
- [JSONB Viewer](#jsonb-viewer)
- [Command Palette](#command-palette)
- [SQL Editor](#sql-editor)
- [Query Favorites](#query-favorites)
- [Keyboard Reference](#keyboard-reference)

---

## Getting Started

Launch pgtui from your terminal:

```bash
pgtui
```

The interface has two main panels:
- **Left panel**: Database tree (databases, schemas, tables)
- **Right panel**: Data view (table contents, query results)

Press `Tab` to switch between panels, `?` for help, `q` to quit.

---

## Connecting to Databases

### Auto-Discovery

On startup, pgtui shows a connection dialog with:
- **Recent connections**: Your connection history
- **Discovered instances**: Local PostgreSQL instances found automatically

Use `↑/↓` to navigate, `Enter` to connect.

### Manual Connection

Press `m` to switch to manual mode and enter:
- Host (default: localhost)
- Port (default: 5432)
- Database (default: postgres)
- User (default: postgres)
- Password

Use `Tab` to move between fields, `Enter` to connect.

### Search Connections

Press `/` in the connection dialog to search across all connections by name, host, database, or user.

---

## Navigating the Interface

### Tree View (Left Panel)

Browse your database structure:

```
▾ mydb (active)
  ▾ public
    • users (1,234 rows)
    • orders (5,678 rows)
  ▸ other_schema
```

| Key | Action |
|-----|--------|
| `j/↓` | Move down |
| `k/↑` | Move up |
| `l/→` | Expand node |
| `h/←` | Collapse node |
| `Enter` | Select table (load data) |
| `g` | Jump to top |
| `G` | Jump to bottom |
| `Space` | Toggle expand/collapse |

### Panel Navigation

| Key | Action |
|-----|--------|
| `Tab` | Switch between panels |
| `Ctrl+K` | Open command palette |
| `?` | Show/hide help |
| `q` | Quit |

---

## Browsing Data

### Table View (Right Panel)

When you select a table, its data appears in the right panel with:
- Column headers with data types
- Row numbers
- Sort indicators
- Current cell highlighting

### Navigation

| Key | Action |
|-----|--------|
| `j/↓` | Move down one row |
| `k/↑` | Move up one row |
| `h` | Move left one column |
| `l` | Move right one column |
| `H` | Jump half screen left |
| `L` | Jump half screen right |
| `0` | Jump to first column |
| `$` | Jump to last column |
| `gg` | Jump to first row |
| `G` | Jump to last row |
| `5j` | Move 5 rows down (vim-style) |

### Sorting

| Key | Action |
|-----|--------|
| `s` | Sort by current column (toggle ASC/DESC) |
| `S` | Toggle NULLS FIRST/LAST |

### Structure Tabs

View table schema information:

| Key | View |
|-----|------|
| `1` | Data (table contents) |
| `2` | Columns (types, constraints) |
| `3` | Constraints (PK, FK, unique) |
| `4` | Indexes |

---

## Searching and Filtering

### Quick Search

Press `/` to open search:

| Key | Action |
|-----|--------|
| `/` | Open search |
| `Tab` | Toggle Local/Table search mode |
| `Enter` | Apply search |
| `n` | Next match |
| `N` | Previous match |
| `Esc` | Close search |

**Local search**: Searches visible rows in current view.
**Table search**: Queries database with WHERE clause.

### Filter Builder

Press `f` to open the interactive filter builder:

| Key | Action |
|-----|--------|
| `↑/↓` | Navigate conditions |
| `a/n` | Add new condition |
| `d/x` | Delete condition |
| `Enter` | Apply filter |
| `Esc` | Cancel |

### Quick Actions

| Key | Action |
|-----|--------|
| `Ctrl+F` | Create filter from current cell |
| `Ctrl+R` | Clear all filters |

---

## JSONB Viewer

Press `J` on a JSONB cell to open the interactive viewer.

### Features

- Collapsible tree structure
- Syntax highlighting by type
- Search within JSON
- Copy values

### Navigation

| Key | Action |
|-----|--------|
| `j/↓` | Move down |
| `k/↑` | Move up |
| `l/→` | Expand node |
| `h/←` | Collapse node |
| `Space` | Toggle expand/collapse |
| `/` | Search |
| `Esc` | Close viewer |

---

## Command Palette

Press `Ctrl+K` to open the command palette.

### Search Modes

| Prefix | Mode |
|--------|------|
| (none) | Search commands and tables |
| `>` | Commands only |
| `@` | Tables/views only |
| `#` | Query history only |

### Available Commands

| Command | Description |
|---------|-------------|
| Connect | Open connection dialog |
| Disconnect | Close current connection |
| Refresh | Reload current view |
| Query Editor | Open SQL editor |
| Query History | Browse past queries |
| Favorites | Manage saved queries |
| Help | Show keyboard shortcuts |
| Settings | Configure pgtui |

### Navigation

| Key | Action |
|-----|--------|
| `↓/Ctrl+N` | Next result |
| `↑/Ctrl+P` | Previous result |
| `Enter` | Execute command |
| `Esc` | Close palette |

---

## SQL Editor

### Opening the Editor

- Press `Ctrl+K` then select "Query Editor"
- Or use Quick Query from command palette

### Features

- Multi-line SQL editing
- Query history (use `↑/↓` to browse)
- External editor support
- Adjustable height

### Result Tabs

Query results appear in tabs:
- Auto-named based on SQL
- Shows execution time
- Up to 10 tabs
- Click to switch between results

---

## Query Favorites

Save frequently used queries for quick access.

### Managing Favorites

1. Press `Ctrl+K` and select "Favorites"
2. Browse, execute, or manage saved queries

### Actions

| Key | Action |
|-----|--------|
| `Enter` | Execute favorite |
| `y` | Copy query |
| `e` | Edit favorite |
| `d` | Delete favorite |
| `/` | Search favorites |

### Export

Export favorites via command palette:
- Export to CSV
- Export to JSON

---

## Keyboard Reference

### Global

| Key | Action |
|-----|--------|
| `Ctrl+K` | Command palette |
| `Tab` | Switch panels |
| `?` | Toggle help |
| `c` | Connection dialog |
| `r/F5` | Refresh |
| `d` | Disconnect |
| `q` | Quit |

### Navigation (Vim-style)

| Key | Action |
|-----|--------|
| `h/j/k/l` | Left/Down/Up/Right |
| `gg` | Top |
| `G` | Bottom |
| `Ctrl+D` | Half page down |
| `Ctrl+U` | Half page up |
| `0/$` | First/Last column |

### Data Operations

| Key | Action |
|-----|--------|
| `/` | Search |
| `f` | Filter builder |
| `s` | Sort column |
| `J` | JSONB viewer |
| `1-4` | Structure tabs |

### Dialogs

| Key | Action |
|-----|--------|
| `Enter` | Confirm/Select |
| `Esc` | Cancel/Close |
| `Tab` | Next field |
| `Shift+Tab` | Previous field |

---

## Configuration

Configuration files are stored in `~/.config/pgtui/`:

| File | Purpose |
|------|---------|
| `config.yaml` | Settings |
| `connection_history.yaml` | Recent connections |
| `favorites.yaml` | Saved queries |

### Example config.yaml

```yaml
ui:
  theme: "default"
  mouse_enabled: true
  panel_width_ratio: 25

general:
  default_limit: 100

performance:
  query_timeout: 30000
```

---

## Mouse Support

pgtui supports mouse interactions:

- **Click**: Select items, switch tabs, navigate
- **Scroll**: Scroll through data and lists
- **Double-click**: Expand/collapse tree nodes

Mouse support can be disabled in `config.yaml`:

```yaml
ui:
  mouse_enabled: false
```
