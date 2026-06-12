package commands

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/pgplex/pgtui/internal/models"
)

// Command messages
type ConnectCommandMsg struct{}
type DisconnectCommandMsg struct{}
type RefreshCommandMsg struct{}
type QuickQueryCommandMsg struct{}
type QueryEditorCommandMsg struct{}
type HistoryCommandMsg struct{}
type FavoritesCommandMsg struct{}
type SettingsCommandMsg struct{}
type ExportFavoritesCSVMsg struct{}
type ExportFavoritesJSONMsg struct{}

// GetBuiltinCommands returns the list of built-in commands
func GetBuiltinCommands() []models.Command {
	return []models.Command{
		{
			ID:          "connect",
			Type:        models.CommandTypeAction,
			Label:       "Connect to Database",
			Description: "Open connection dialog",
			Icon:        "🔌",
			Tags:        []string{"connection", "database", "connect"},
			Action: func() tea.Msg {
				return ConnectCommandMsg{}
			},
		},
		{
			ID:          "disconnect",
			Type:        models.CommandTypeAction,
			Label:       "Disconnect",
			Description: "Close current connection",
			Icon:        "🔴",
			Tags:        []string{"connection", "disconnect", "close"},
			Action: func() tea.Msg {
				return DisconnectCommandMsg{}
			},
		},
		{
			ID:          "refresh",
			Type:        models.CommandTypeAction,
			Label:       "Refresh",
			Description: "Refresh current view",
			Icon:        "🔄",
			Tags:        []string{"view", "refresh", "reload"},
			Action: func() tea.Msg {
				return RefreshCommandMsg{}
			},
		},
		{
			ID:          "quick-query",
			Type:        models.CommandTypeAction,
			Label:       "Quick Query",
			Description: "Execute a quick SQL query",
			Icon:        "⚡",
			Tags:        []string{"query", "sql", "execute", "quick"},
			Action: func() tea.Msg {
				return QuickQueryCommandMsg{}
			},
		},
		{
			ID:          "query-editor",
			Type:        models.CommandTypeAction,
			Label:       "Query Editor",
			Description: "Open full query editor",
			Icon:        "📝",
			Tags:        []string{"query", "sql", "editor", "write"},
			Action: func() tea.Msg {
				return QueryEditorCommandMsg{}
			},
		},
		{
			ID:          "history",
			Type:        models.CommandTypeAction,
			Label:       "Query History",
			Description: "View query history",
			Icon:        "📜",
			Tags:        []string{"query", "history", "past"},
			Action: func() tea.Msg {
				return HistoryCommandMsg{}
			},
		},
		{
			ID:          "favorites",
			Type:        models.CommandTypeAction,
			Label:       "Favorites",
			Description: "Manage favorite queries",
			Icon:        "⭐",
			Tags:        []string{"favorites", "bookmarks", "saved"},
			Action: func() tea.Msg {
				return FavoritesCommandMsg{}
			},
		},
		{
			ID:          "help",
			Type:        models.CommandTypeAction,
			Label:       "Help",
			Description: "Show keyboard shortcuts",
			Icon:        "❓",
			Tags:        []string{"help", "shortcuts", "keybindings"},
			Action: func() tea.Msg {
				// Toggle help mode
				return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}}
			},
		},
		{
			ID:          "settings",
			Type:        models.CommandTypeAction,
			Label:       "Settings",
			Description: "Open settings",
			Icon:        "⚙️",
			Tags:        []string{"config", "settings", "preferences"},
			Action: func() tea.Msg {
				return SettingsCommandMsg{}
			},
		},
		{
			ID:          "export-favorites-csv",
			Type:        models.CommandTypeAction,
			Label:       "Export Favorites to CSV",
			Description: "Export all favorites to CSV file",
			Icon:        "📊",
			Tags:        []string{"export", "favorites", "csv"},
			Action: func() tea.Msg {
				return ExportFavoritesCSVMsg{}
			},
		},
		{
			ID:          "export-favorites-json",
			Type:        models.CommandTypeAction,
			Label:       "Export Favorites to JSON",
			Description: "Export all favorites to JSON file",
			Icon:        "📦",
			Tags:        []string{"export", "favorites", "json"},
			Action: func() tea.Msg {
				return ExportFavoritesJSONMsg{}
			},
		},
	}
}
