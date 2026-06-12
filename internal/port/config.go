package port

import (
	"context"

	"github.com/SHIROH4/sion-plus/internal/domain/types"
)

// ── Config Manager ──

// ConfigManager manages application configuration with versioning,
// migration support, and hot-reload notification.
// Implementation: adapter/config/config_manager.go
type ConfigManager interface {
	// Load reads the config file, or returns defaults if not found.
	Load() (*AppConfig, error)

	// Save writes the config to disk and notifies listeners.
	Save(cfg *AppConfig) error

	// GetPluginConfig returns a plugin's config subtree.
	GetPluginConfig(pluginName string) (map[string]any, error)

	// SetPluginConfig updates a plugin's config subtree.
	SetPluginConfig(pluginName string, cfg map[string]any) error

	// Validate checks whether the config is semantically valid.
	Validate(cfg *AppConfig) error

	// OnChange registers a hot-reload callback.
	OnChange(handler func(*AppConfig)) (unsubscribe func())

	// DataDir returns the app's data directory (~/.sion/).
	DataDir() string

	// ── Migration ──

	// RunMigrations executes all pending data migrations.
	RunMigrations(ctx context.Context) (*MigrationResult, error)

	// NeedsMigration returns true if any data is behind SchemaVersionCurrent.
	NeedsMigration(ctx context.Context) (bool, error)
}

// ── Migrations ──

type MigrationResult struct {
	Applied     int      `json:"applied"`
	Skipped     int      `json:"skipped"`
	Errors      []string `json:"errors"`
	FromVersion int      `json:"from_version"`
	ToVersion   int      `json:"to_version"`
}

// ── App Config ──

type AppConfig struct {
	Version   int                       `yaml:"version"`
	Providers []LLMProviderConfig       `yaml:"providers"`
	Routes    LLMRoutes                 `yaml:"routes"`
	Evidence  types.EvidenceConfig      `yaml:"evidence"`
	Retrieval RetrievalConfig           `yaml:"retrieval"`
	Proactive ProactiveConfig           `yaml:"proactive"`
	User      UserConfig                `yaml:"user"`
	WarmStart WarmStartConfig           `yaml:"warm_start"`
	Plugins   map[string]map[string]any `yaml:"plugins"`
}

type UserConfig struct {
	Name      string   `yaml:"name"`
	TechStack []string `yaml:"tech_stack"`
}

type WarmStartConfig struct {
	Personality PersonalityWarmStart `yaml:"personality"`
	KnownFacts  []string             `yaml:"known_facts"`
}

type PersonalityWarmStart struct {
	AnnoyanceSensitivity float64 `yaml:"annoyance_sensitivity"`
	AffectionWarmth      float64 `yaml:"affection_warmth"`
	WorryTendency        float64 `yaml:"worry_tendency"`
}

type RetrievalConfig struct {
	BM25Budget   int     `yaml:"bm25_budget"`    // default 10
	CosineBudget int     `yaml:"cosine_budget"`   // default 10
	RRFK         float64 `yaml:"rrf_k"`           // default 60
	FinalBudget  int     `yaml:"final_budget"`    // default 5
}

type ProactiveConfig struct {
	ChatEnabled     bool `yaml:"chat_enabled"`
	VisionEnabled   bool `yaml:"vision_enabled"`
	BaseIntervalSec int  `yaml:"base_interval_sec"` // default 300
	MaxDailyActions int  `yaml:"max_daily_actions"` // default 20
}
