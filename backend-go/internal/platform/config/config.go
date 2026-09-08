package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server        ServerConfig
	Database      DatabaseConfig
	CORS          CORSConfig
	Log           LogConfig
	Tracing       TracingConfig
	Storage       StorageConfig
	Bocha         BochaConfig
	CrossBoardRel CrossBoardRelationConfig
}

// CrossBoardRelationConfig holds the global budget and thresholds for
// cross-board relation discovery (add-evidence-backed-cross-board-relations).
// Per-board on/off is stored on semantic_labels.relation_auto_discovery_enabled;
// these values govern every run.
type CrossBoardRelationConfig struct {
	AutoMaxSourcesPerBrief int     `mapstructure:"auto_max_sources_per_brief"` // auto trigger: sources per new brief (-1 = disabled; unset/0 = default)
	MaxSearchesPerRun      int     `mapstructure:"max_searches_per_run"`       // web_search calls per run
	MaxFetchesPerRun       int     `mapstructure:"max_fetches_per_run"`        // fetch_page calls per run
	MaxLoopsPerRun         int     `mapstructure:"max_loops_per_run"`          // scout loop ceiling
	RunTimeoutSeconds      int     `mapstructure:"run_timeout_seconds"`        // whole-run timeout
	ResolveThreshold       float64 `mapstructure:"resolve_threshold"`          // top-1 minimum score
	ResolveMargin          float64 `mapstructure:"resolve_margin"`             // required top1-top2 gap
	DismissCooldownDays    int     `mapstructure:"dismiss_cooldown_days"`      // same-hash re-suggest block
	ConfirmedTTLHours      int     `mapstructure:"confirmed_ttl_hours"`        // confirm expiry horizon
	BriefMaxRelations      int     `mapstructure:"brief_max_relations"`        // brief injection count budget
	BriefMaxRelationRunes  int     `mapstructure:"brief_max_relation_runes"`   // brief injection char budget
}

type LogConfig struct {
	Level string        `mapstructure:"level"`
	File  LogFileConfig `mapstructure:"file"`
}

type LogFileConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	Path       string `mapstructure:"path"`
	MaxSizeMB  int    `mapstructure:"max_size_mb"`
	MaxBackups int    `mapstructure:"max_backups"`
	MaxAgeDays int    `mapstructure:"max_age_days"`
	Compress   bool   `mapstructure:"compress"`
}

type ServerConfig struct {
	Port string
	Mode string // debug, release, test
}

type DatabaseConfig struct {
	Driver   string
	DSN      string
	Postgres PostgresConfig

	// AllowDestructiveMigrations controls whether destructive migrations
	// (TRUNCATE/DROP) execute. Defaults to false (production-safe). Set via
	// env MIGRATIONS_ALLOW_DESTRUCTIVE=1 for dev/test environments that need
	// historical data cleanup. See db-migration-safety capability.
	AllowDestructiveMigrations bool
}

type PostgresConfig struct {
	MaxIdleConns           int `mapstructure:"max_idle_conns"`
	MaxOpenConns           int `mapstructure:"max_open_conns"`
	ConnMaxLifetimeMinutes int `mapstructure:"conn_max_lifetime_minutes"`
	ConnMaxIdleTimeMinutes int `mapstructure:"conn_max_idle_time_minutes"`
}

type CORSConfig struct {
	Origins      []string
	Methods      []string
	AllowHeaders []string
}

type TracingConfig struct {
	SampleRatio    float64 `mapstructure:"sample_ratio"`
	InstrumentGORM bool    `mapstructure:"instrument_gorm"`
	InstrumentHTTP bool    `mapstructure:"instrument_http"`
}

// StorageConfig holds filesystem storage locations.
type StorageConfig struct {
	// IconDir is the root directory for locally downloaded feed icons (the
	// feeds/ subdirectory lives inside it). Defaults to data/icons.
	IconDir string `mapstructure:"icon_dir"`
}

// BochaConfig configures the Bocha (bochaai.com) web-search backend used by the
// data-enrichment web_search tool. When APIKey is empty the wiring falls back
// to NoopWebSearcher (graceful degradation). Endpoint defaults to the Bocha
// general-search (raw web results) endpoint.
type BochaConfig struct {
	APIKey   string `mapstructure:"api_key"`  // empty → degrade to Noop
	Endpoint string `mapstructure:"endpoint"` // default https://api.bochaai.com/v1/web-search
}

var AppConfig *Config

func LoadConfig(configPath string) error {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(configPath)
	viper.AddConfigPath(".")
	viper.AddConfigPath("./configs")

	// Set defaults
	viper.SetDefault("server.port", "5000")
	viper.SetDefault("server.mode", "debug")
	viper.SetDefault("database.driver", "postgres")
	viper.SetDefault("database.dsn", "host=127.0.0.1 user=postgres password=postgres dbname=syntopica port=5432 sslmode=disable TimeZone=Asia/Shanghai")
	viper.SetDefault("database.postgres.max_idle_conns", 5)
	viper.SetDefault("database.postgres.max_open_conns", 25)
	viper.SetDefault("database.postgres.conn_max_lifetime_minutes", 60)
	viper.SetDefault("database.postgres.conn_max_idle_time_minutes", 10)
	viper.SetDefault("cors.origins", []string{"http://localhost:3000", "http://localhost:3000"})
	viper.SetDefault("cors.methods", []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"})
	viper.SetDefault("cors.allow_headers", []string{"Content-Type", "Authorization"})

	viper.SetDefault("log.level", "debug")
	viper.SetDefault("log.file.enabled", true)
	viper.SetDefault("log.file.path", "logs/app.log")
	viper.SetDefault("log.file.max_size_mb", 50)
	viper.SetDefault("log.file.max_backups", 30)
	viper.SetDefault("log.file.max_age_days", 30)
	viper.SetDefault("log.file.compress", true)

	viper.SetDefault("tracing.sample_ratio", 0.05)
	viper.SetDefault("tracing.instrument_gorm", true)
	viper.SetDefault("tracing.instrument_http", true)

	viper.SetDefault("storage.icon_dir", "data/icons")

	viper.SetDefault("bocha.api_key", "")
	viper.SetDefault("bocha.endpoint", "https://api.bochaai.com/v1/web-search")

	// Cross-board relation discovery budgets (add-evidence-backed-cross-board-relations).
	viper.SetDefault("cross_board_rel.auto_max_sources_per_brief", 3)
	viper.SetDefault("cross_board_rel.max_searches_per_run", 4)
	viper.SetDefault("cross_board_rel.max_fetches_per_run", 2)
	viper.SetDefault("cross_board_rel.max_loops_per_run", 6)
	viper.SetDefault("cross_board_rel.run_timeout_seconds", 300)
	viper.SetDefault("cross_board_rel.resolve_threshold", 0.62)
	viper.SetDefault("cross_board_rel.resolve_margin", 0.08)
	viper.SetDefault("cross_board_rel.dismiss_cooldown_days", 14)
	viper.SetDefault("cross_board_rel.confirmed_ttl_hours", 720)
	viper.SetDefault("cross_board_rel.brief_max_relations", 3)
	viper.SetDefault("cross_board_rel.brief_max_relation_runes", 1200)

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return err
		}
		fmt.Println("Config file not found, using defaults")
	}

	AppConfig = &Config{}

	if err := viper.Unmarshal(AppConfig); err != nil {
		return err
	}

	applyEnvOverrides(AppConfig)

	return nil
}

func applyEnvOverrides(cfg *Config) {
	if cfg == nil {
		return
	}

	if value := strings.TrimSpace(os.Getenv("SERVER_PORT")); value != "" {
		cfg.Server.Port = value
	}

	if value := strings.TrimSpace(os.Getenv("SERVER_MODE")); value != "" {
		cfg.Server.Mode = value
	}

	if value := strings.TrimSpace(os.Getenv("DATABASE_DRIVER")); value != "" {
		cfg.Database.Driver = value
	}

	if value := strings.TrimSpace(os.Getenv("DATABASE_DSN")); value != "" {
		cfg.Database.DSN = value
	}

	if value := strings.TrimSpace(os.Getenv("CORS_ORIGINS")); value != "" {
		cfg.CORS.Origins = splitCommaSeparated(value)
	}

	// MIGRATIONS_ALLOW_DESTRUCTIVE=1 enables destructive migrations (TRUNCATE/DROP).
	// Defaults to false: production never sets this, refusing destructive migrations.
	cfg.Database.AllowDestructiveMigrations = os.Getenv("MIGRATIONS_ALLOW_DESTRUCTIVE") == "1"

	// Tracing (defaults via viper SetDefault above; env overrides when set).
	// Invalid or out-of-range values fall back to the default with a warning:
	// full sampling kept otel_spans at a steady ~4.3GB (820k spans/day), so the
	// safe fallback is the low default, never the caller's bad value.
	if v := strings.TrimSpace(os.Getenv("TRACE_SAMPLE_RATIO")); v != "" {
		if r, err := strconv.ParseFloat(v, 64); err == nil && r >= 0.0 && r <= 1.0 {
			cfg.Tracing.SampleRatio = r
		} else {
			fmt.Printf("Config: invalid TRACE_SAMPLE_RATIO %q (want 0.0-1.0), keeping default %.2f\n", v, cfg.Tracing.SampleRatio)
		}
	}
	if v := os.Getenv("TRACE_INSTRUMENT_GORM"); v != "" {
		cfg.Tracing.InstrumentGORM = v != "0"
	}
	if v := os.Getenv("TRACE_INSTRUMENT_HTTP"); v != "" {
		cfg.Tracing.InstrumentHTTP = v != "0"
	}
	if value := strings.TrimSpace(os.Getenv("STORAGE_ICON_DIR")); value != "" {
		cfg.Storage.IconDir = value
	}

	// Bocha web-search backend (data-enrichment web_search). Empty key → Noop.
	if v := strings.TrimSpace(os.Getenv("BOCHA_API_KEY")); v != "" {
		cfg.Bocha.APIKey = v
	}
	if v := strings.TrimSpace(os.Getenv("BOCHA_ENDPOINT")); v != "" {
		cfg.Bocha.Endpoint = v
	}
}

// defaultCrossBoardRelationConfig mirrors the viper defaults above; used when
// AppConfig has not been loaded (tests, wiring order) so discovery budgets are
// never zero-valued by accident.
func defaultCrossBoardRelationConfig() CrossBoardRelationConfig {
	return CrossBoardRelationConfig{
		AutoMaxSourcesPerBrief: 3,
		MaxSearchesPerRun:      4,
		MaxFetchesPerRun:       2,
		MaxLoopsPerRun:         6,
		RunTimeoutSeconds:      300,
		ResolveThreshold:       0.62,
		ResolveMargin:          0.08,
		DismissCooldownDays:    14,
		ConfirmedTTLHours:      720,
		BriefMaxRelations:      3,
		BriefMaxRelationRunes:  1200,
	}
}

// EffectiveCrossBoardRelationConfig returns the loaded config, falling back to
// defaults for zero fields (config not loaded / partial yaml).
func EffectiveCrossBoardRelationConfig() CrossBoardRelationConfig {
	def := defaultCrossBoardRelationConfig()
	if AppConfig == nil {
		return def
	}
	got := AppConfig.CrossBoardRel
	// Auto budget: negative = explicit global disable (effective 0);
	// zero = unset → default. Keeps "预算为零跳过" expressible.
	if got.AutoMaxSourcesPerBrief < 0 {
		got.AutoMaxSourcesPerBrief = 0
	} else if got.AutoMaxSourcesPerBrief == 0 {
		got.AutoMaxSourcesPerBrief = def.AutoMaxSourcesPerBrief
	}
	if got.MaxSearchesPerRun <= 0 {
		got.MaxSearchesPerRun = def.MaxSearchesPerRun
	}
	if got.MaxFetchesPerRun <= 0 {
		got.MaxFetchesPerRun = def.MaxFetchesPerRun
	}
	if got.MaxLoopsPerRun <= 0 {
		got.MaxLoopsPerRun = def.MaxLoopsPerRun
	}
	if got.RunTimeoutSeconds <= 0 {
		got.RunTimeoutSeconds = def.RunTimeoutSeconds
	}
	if got.ResolveThreshold <= 0 || got.ResolveThreshold >= 1 {
		got.ResolveThreshold = def.ResolveThreshold
	}
	if got.ResolveMargin <= 0 || got.ResolveMargin >= 1 {
		got.ResolveMargin = def.ResolveMargin
	}
	if got.DismissCooldownDays <= 0 {
		got.DismissCooldownDays = def.DismissCooldownDays
	}
	if got.ConfirmedTTLHours <= 0 {
		got.ConfirmedTTLHours = def.ConfirmedTTLHours
	}
	if got.BriefMaxRelations <= 0 {
		got.BriefMaxRelations = def.BriefMaxRelations
	}
	if got.BriefMaxRelationRunes <= 0 {
		got.BriefMaxRelationRunes = def.BriefMaxRelationRunes
	}
	return got
}

func splitCommaSeparated(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		result = append(result, trimmed)
	}
	return result
}
