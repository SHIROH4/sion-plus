package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/SHIROH4/sion-plus/internal/domain/types"
	derr "github.com/SHIROH4/sion-plus/internal/domain/errors"
	"github.com/SHIROH4/sion-plus/internal/port"
)

// AppDataDir returns the OS-appropriate application data directory.
//
//	macOS:   ~/Library/Application Support/Sion
//	Windows: %APPDATA%/Sion
//	Linux:   $XDG_DATA_HOME/sion  or  ~/.local/share/sion
func AppDataDir() string {
	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support", "Sion")
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "Sion")
	default:
		if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
			return filepath.Join(xdg, "sion")
		}
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".local", "share", "sion")
	}
}

var _ port.ConfigManager = (*ConfigManagerImpl)(nil)

// ConfigManagerImpl is the YAML-file backed configuration manager.
type ConfigManagerImpl struct {
	mu       sync.RWMutex
	path     string
	dataDir  string
	cfg      *port.AppConfig
	onChange []func(*port.AppConfig)
}

// NewConfigManager creates a ConfigManagerImpl for the given data directory.
// Use NewDefaultConfigManager() for production (auto-detects OS path).
// Use this constructor in tests to point at a temp directory.
func NewConfigManager(dataDir string) *ConfigManagerImpl {
	return &ConfigManagerImpl{
		dataDir: dataDir,
		path:    filepath.Join(dataDir, "config.yaml"),
	}
}

// DataDir returns the data directory path.
func (m *ConfigManagerImpl) DataDir() string {
	return m.dataDir
}

// Load reads config.yaml, creates defaults if missing, caches and returns.
// Thread-safe: caches on first call, returns copy on subsequent calls.
func (m *ConfigManagerImpl) Load() (*port.AppConfig, error) {
	// Fast path: return cached copy
	m.mu.RLock()
	if m.cfg != nil {
		c := *m.cfg
		m.mu.RUnlock()
		return &c, nil
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock
	if m.cfg != nil {
		c := *m.cfg
		return &c, nil
	}

	cfg, err := m.readOrCreate()
	if err != nil {
		return nil, fmt.Errorf("config load: %w", err)
	}

	if err := m.Validate(cfg); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}

	m.cfg = cfg
	c := *cfg
	return &c, nil
}

// Save writes the config to disk and notifies change listeners.
func (m *ConfigManagerImpl) Save(cfg *port.AppConfig) error {
	if err := m.Validate(cfg); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.writeFile(cfg); err != nil {
		return err
	}
	m.cfg = cfg

	for _, h := range m.onChange {
		go h(cfg)
	}
	return nil
}

// Validate checks that required fields are present and values are in valid ranges.
func (m *ConfigManagerImpl) Validate(cfg *port.AppConfig) error {
	if cfg == nil {
		return fmt.Errorf("%w: config is nil", derr.ErrConfigInvalid)
	}

	var failures []derr.ValidationFailure

	if len(cfg.Providers) == 0 {
		failures = append(failures, derr.ValidationFailure{
			Field: "providers", Message: "at least one LLM provider is required",
		})
	}
	for i, p := range cfg.Providers {
		if p.BaseURL == "" {
			failures = append(failures, derr.ValidationFailure{
				Field: fmt.Sprintf("providers[%d].base_url", i),
				Message: "base_url is required for each provider",
			})
		}
		if p.ChatModel == "" {
			failures = append(failures, derr.ValidationFailure{
				Field: fmt.Sprintf("providers[%d].chat_model", i),
				Message: "chat_model is required for each provider",
			})
		}
	}

	if cfg.Proactive.BaseIntervalSec < 60 {
		failures = append(failures, derr.ValidationFailure{
			Field: "proactive.base_interval_sec", Value: cfg.Proactive.BaseIntervalSec,
			Message: "must be >= 60 seconds",
		})
	}
	if cfg.Proactive.MaxDailyActions < 1 || cfg.Proactive.MaxDailyActions > 100 {
		failures = append(failures, derr.ValidationFailure{
			Field: "proactive.max_daily_actions", Value: cfg.Proactive.MaxDailyActions,
			Message: "must be between 1 and 100",
		})
	}

	if len(failures) > 0 {
		return &derr.ValidationError{Fields: failures}
	}
	return nil
}

// OnChange registers a callback for config changes. Returns an unsubscribe function.
func (m *ConfigManagerImpl) OnChange(handler func(*port.AppConfig)) func() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onChange = append(m.onChange, handler)
	idx := len(m.onChange) - 1
	return func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		// Mark slot as nil (avoid index shifting)
		if idx < len(m.onChange) {
			m.onChange[idx] = nil
		}
	}
}

// ── Plugin Config ──

func (m *ConfigManagerImpl) GetPluginConfig(pluginName string) (map[string]any, error) {
	cfg, err := m.Load()
	if err != nil {
		return nil, err
	}
	if cfg.Plugins == nil {
		return map[string]any{}, nil
	}
	c, ok := cfg.Plugins[pluginName]
	if !ok {
		return map[string]any{}, nil
	}
	return c, nil
}

func (m *ConfigManagerImpl) SetPluginConfig(pluginName string, pc map[string]any) error {
	cfg, err := m.Load()
	if err != nil {
		return err
	}
	if cfg.Plugins == nil {
		cfg.Plugins = make(map[string]map[string]any)
	}
	cfg.Plugins[pluginName] = pc
	return m.Save(cfg)
}

// ── Migration (stub — full implementation in migration.go) ──

func (m *ConfigManagerImpl) RunMigrations(ctx context.Context) (*port.MigrationResult, error) {
	return &port.MigrationResult{
		FromVersion: types.SchemaVersionCurrent,
		ToVersion:   types.SchemaVersionCurrent,
		Skipped:     1,
	}, nil
}

func (m *ConfigManagerImpl) NeedsMigration(ctx context.Context) (bool, error) {
	return false, nil
}

// ── Internal ──

func (m *ConfigManagerImpl) readOrCreate() (*port.AppConfig, error) {
	if err := os.MkdirAll(m.dataDir, 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	data, err := os.ReadFile(m.path)
	if os.IsNotExist(err) {
		cfg := defaultConfig()
		if err := m.writeFile(cfg); err != nil {
			return nil, fmt.Errorf("write default config: %w", err)
		}
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	expanded := os.ExpandEnv(string(data))
	var cfg port.AppConfig
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}

func (m *ConfigManagerImpl) writeFile(cfg *port.AppConfig) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	// Atomic write: write to tmp, then rename
	tmpPath := m.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmpPath, m.path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename config: %w", err)
	}
	return nil
}

func defaultConfig() *port.AppConfig {
	return &port.AppConfig{
		Version: 1,
		Providers: []port.LLMProviderConfig{
			{
				Name: "deepseek", BaseURL: "https://api.deepseek.com",
				ChatModel: "deepseek-chat", Enabled: true, Priority: 1,
				MaxRetries: 2, TimeoutSec: 30,
			},
		},
		Routes: port.LLMRoutes{Default: "deepseek"},
		Evidence: types.EvidenceConfig{
			ReinHalfLifeDays: 60, DispHalfLifeDays: 14,
			ConfirmedThreshold: 1.0, PromotedThreshold: 2.0,
			ComboThreshold: 3, ComboBonus: 0.5, BaseReinDelta: 0.5,
		},
		Retrieval: port.RetrievalConfig{
			BM25Budget: 10, CosineBudget: 10, RRFK: 60, FinalBudget: 5,
		},
		Proactive: port.ProactiveConfig{
			ChatEnabled: true, VisionEnabled: true,
			BaseIntervalSec: 300, MaxDailyActions: 20,
		},
	}
}
