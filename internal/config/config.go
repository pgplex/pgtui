package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config holds all application configuration
type Config struct {
	General     GeneralConfig     `mapstructure:"general"`
	UI          UIConfig          `mapstructure:"ui"`
	Editor      EditorConfig      `mapstructure:"editor"`
	Data        DataConfig        `mapstructure:"data"`
	History     HistoryConfig     `mapstructure:"history"`
	Performance PerformanceConfig `mapstructure:"performance"`
}

type GeneralConfig struct {
	AutoConnectLast       bool `mapstructure:"auto_connect_last"`
	ConfirmDestructiveOps bool `mapstructure:"confirm_destructive_ops"`
	DefaultLimit          int  `mapstructure:"default_limit"`
}

type UIConfig struct {
	Theme             string `mapstructure:"theme"`
	MouseEnabled      bool   `mapstructure:"mouse_enabled"`
	PanelWidthRatio   int    `mapstructure:"panel_width_ratio"`
	ShowBreadcrumbs   bool   `mapstructure:"show_breadcrumbs"`
	CommandPaletteKey string `mapstructure:"command_palette_key"`
}

type EditorConfig struct {
	TabSize      int  `mapstructure:"tab_size"`
	UseSpaces    bool `mapstructure:"use_spaces"`
	AutoComplete bool `mapstructure:"auto_complete"`
	FormatOnSave bool `mapstructure:"format_on_save"`
}

type DataConfig struct {
	VirtualScrollBuffer  int  `mapstructure:"virtual_scroll_buffer"`
	MaxCellDisplayLength int  `mapstructure:"max_cell_display_length"`
	JSONBAutoFormat      bool `mapstructure:"jsonb_auto_format"`
	LargeTableThreshold  int  `mapstructure:"large_table_threshold"`
	PrefetchThreshold    int  `mapstructure:"prefetch_threshold"`
	PrefetchSize         int  `mapstructure:"prefetch_size"`
	MaxPinnedRows        int  `mapstructure:"max_pinned_rows"`
}

type HistoryConfig struct {
	Enabled           bool `mapstructure:"enabled"`
	MaxEntries        int  `mapstructure:"max_entries"`
	Persist           bool `mapstructure:"persist"`
	SaveFailedQueries bool `mapstructure:"save_failed_queries"`
}

type PerformanceConfig struct {
	ConnectionPoolSize int `mapstructure:"connection_pool_size"`
	QueryTimeout       int `mapstructure:"query_timeout"`
	MetadataCacheTTL   int `mapstructure:"metadata_cache_ttl"`
}

// GetDefaults returns a Config with all default values
func GetDefaults() *Config {
	return &Config{
		General: GeneralConfig{
			AutoConnectLast:       false,
			ConfirmDestructiveOps: true,
			DefaultLimit:          100,
		},
		UI: UIConfig{
			Theme:             "default",
			MouseEnabled:      true,
			PanelWidthRatio:   25,
			ShowBreadcrumbs:   true,
			CommandPaletteKey: "ctrl+k",
		},
		Editor: EditorConfig{
			TabSize:      2,
			UseSpaces:    true,
			AutoComplete: true,
			FormatOnSave: false,
		},
		Data: DataConfig{
			VirtualScrollBuffer:  100,
			MaxCellDisplayLength: 100,
			JSONBAutoFormat:      true,
			LargeTableThreshold:  1000000,
			PrefetchThreshold:    50,
			PrefetchSize:         100,
			MaxPinnedRows:        5,
		},
		History: HistoryConfig{
			Enabled:           true,
			MaxEntries:        1000,
			Persist:           true,
			SaveFailedQueries: true,
		},
		Performance: PerformanceConfig{
			ConnectionPoolSize: 10,
			QueryTimeout:       30000,
			MetadataCacheTTL:   300,
		},
	}
}

// Load loads configuration from files
func Load() (*Config, error) {
	v := viper.New()

	// Set config name and type
	v.SetConfigName("config")
	v.SetConfigType("yaml")

	// Add config paths in priority order
	// 1. User config directory
	if configDir, err := os.UserConfigDir(); err == nil {
		v.AddConfigPath(filepath.Join(configDir, "pgtui"))
	}

	// 2. Current directory
	v.AddConfigPath(".")

	// 3. Default config directory
	v.AddConfigPath("./config")

	// Set defaults from default.yaml
	v.SetDefault("general.auto_connect_last", false)
	v.SetDefault("general.confirm_destructive_ops", true)
	v.SetDefault("general.default_limit", 100)
	v.SetDefault("ui.theme", "default")
	v.SetDefault("ui.mouse_enabled", true)
	v.SetDefault("ui.panel_width_ratio", 25)
	v.SetDefault("ui.show_breadcrumbs", true)
	v.SetDefault("ui.command_palette_key", "ctrl+k")
	v.SetDefault("editor.tab_size", 2)
	v.SetDefault("editor.use_spaces", true)
	v.SetDefault("editor.auto_complete", true)
	v.SetDefault("editor.format_on_save", false)
	v.SetDefault("data.virtual_scroll_buffer", 100)
	v.SetDefault("data.max_cell_display_length", 100)
	v.SetDefault("data.jsonb_auto_format", true)
	v.SetDefault("data.large_table_threshold", 1000000)
	v.SetDefault("data.prefetch_threshold", 50)
	v.SetDefault("data.prefetch_size", 100)
	v.SetDefault("data.max_pinned_rows", 5)
	v.SetDefault("history.enabled", true)
	v.SetDefault("history.max_entries", 1000)
	v.SetDefault("history.persist", true)
	v.SetDefault("history.save_failed_queries", true)
	v.SetDefault("performance.connection_pool_size", 10)
	v.SetDefault("performance.query_timeout", 30000)
	v.SetDefault("performance.metadata_cache_ttl", 300)

	// Read config (it's okay if file doesn't exist, we have defaults)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config: %w", err)
		}
	}

	// Unmarshal into struct
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	return &cfg, nil
}

// GetConfigPath returns the user config directory path
func GetConfigPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "pgtui"), nil
}
