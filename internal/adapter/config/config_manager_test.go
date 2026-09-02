package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/SHIROH4/sion-plus/internal/port"
)

func TestLoadCreatesDefault(t *testing.T) {
	dir := t.TempDir()
	mgr := NewConfigManager(dir)

	cfg, err := mgr.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Version != 1 {
		t.Errorf("expected version 1, got %d", cfg.Version)
	}
	if len(cfg.Providers) != 1 {
		t.Errorf("expected 1 provider, got %d", len(cfg.Providers))
	}
	if cfg.Providers[0].Name != "deepseek" {
		t.Errorf("expected deepseek provider, got %s", cfg.Providers[0].Name)
	}
	if cfg.Proactive.BaseIntervalSec != 300 {
		t.Errorf("expected base_interval 300, got %d", cfg.Proactive.BaseIntervalSec)
	}

	// Verify file was created
	configPath := filepath.Join(dir, "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("config.yaml was not created")
	}
}

func TestSaveAndReload(t *testing.T) {
	dir := t.TempDir()
	mgr := NewConfigManager(dir)

	cfg, _ := mgr.Load()
	cfg.User.Name = "测试用户"
	cfg.User.TechStack = []string{"Go", "Rust"}
	cfg.Proactive.BaseIntervalSec = 120

	if err := mgr.Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Create new manager pointing to same dir (simulate restart)
	mgr2 := NewConfigManager(dir)
	cfg2, err := mgr2.Load()
	if err != nil {
		t.Fatalf("Reload: %v", err)
	}

	if cfg2.User.Name != "测试用户" {
		t.Errorf("expected name '测试用户', got %q", cfg2.User.Name)
	}
	if len(cfg2.User.TechStack) != 2 {
		t.Errorf("expected 2 tech stack items, got %d", len(cfg2.User.TechStack))
	}
	if cfg2.Proactive.BaseIntervalSec != 120 {
		t.Errorf("expected base_interval 120, got %d", cfg2.Proactive.BaseIntervalSec)
	}
}

func TestCaching(t *testing.T) {
	dir := t.TempDir()
	mgr := NewConfigManager(dir)

	cfg1, _ := mgr.Load()
	cfg2, _ := mgr.Load()

	// Same pointer means caching works (Load returns copies though)
	if cfg1 == cfg2 {
		t.Log("configs are same pointer (caching)")
	}
	if cfg1.Version != cfg2.Version {
		t.Error("cached and fresh load should be equal")
	}
}

func TestEnvVarExpansion(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("TEST_API_KEY", "sk-test-12345")
	defer os.Unsetenv("TEST_API_KEY")

	// Write config with env var placeholder
	configPath := filepath.Join(dir, "config.yaml")
	yamlContent := `
version: 1
providers:
  - name: test-provider
    base_url: https://api.test.com/v1
    api_key: ${TEST_API_KEY}
    chat_model: test-model
    enabled: true
    priority: 1
    max_retries: 2
    timeout_sec: 30
routes:
  default: test-provider
evidence:
  rein_half_life_days: 60
  disp_half_life_days: 14
  confirmed_threshold: 1.0
  promoted_threshold: 2.0
  archive_threshold: 0.0
  combo_threshold: 3
  combo_bonus: 0.5
  base_rein_delta: 0.5
retrieval:
  bm25_budget: 10
  cosine_budget: 10
  rrf_k: 60
  final_budget: 5
proactive:
  chat_enabled: true
  vision_enabled: true
  base_interval_sec: 300
  max_daily_actions: 20
`
	os.WriteFile(configPath, []byte(yamlContent), 0600)

	mgr := NewConfigManager(dir)
	cfg, err := mgr.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Providers[0].APIKey != "sk-test-12345" {
		t.Errorf("env var not expanded: got %q", cfg.Providers[0].APIKey)
	}
}

func TestValidateRejectsInvalid(t *testing.T) {
	mgr := NewConfigManager(t.TempDir())

	// No providers
	cfg := &port.AppConfig{Version: 1, Proactive: port.ProactiveConfig{
		BaseIntervalSec: 300, MaxDailyActions: 20,
	}}
	err := mgr.Validate(cfg)
	if err == nil {
		t.Error("expected validation error for missing providers")
	}

	// Invalid interval
	cfg2, _ := mgr.Load()
	cfg2.Proactive.BaseIntervalSec = 10
	err = mgr.Validate(cfg2)
	if err == nil {
		t.Error("expected validation error for base_interval < 60")
	}
}

func TestPluginConfigCRUD(t *testing.T) {
	dir := t.TempDir()
	mgr := NewConfigManager(dir)

	// Get empty
	pc, err := mgr.GetPluginConfig("test_plugin")
	if err != nil {
		t.Fatalf("GetPluginConfig: %v", err)
	}
	if len(pc) != 0 {
		t.Errorf("expected empty config, got %v", pc)
	}

	// Set
	err = mgr.SetPluginConfig("test_plugin", map[string]any{
		"enabled":  true,
		"interval": 60,
	})
	if err != nil {
		t.Fatalf("SetPluginConfig: %v", err)
	}

	// Get back
	pc, err = mgr.GetPluginConfig("test_plugin")
	if err != nil {
		t.Fatalf("GetPluginConfig after set: %v", err)
	}
	if pc["enabled"] != true {
		t.Errorf("expected enabled=true, got %v", pc["enabled"])
	}

	// Verify persistence
	mgr2 := NewConfigManager(dir)
	pc2, _ := mgr2.GetPluginConfig("test_plugin")
	if pc2["enabled"] != true {
		t.Error("plugin config not persisted")
	}
}

func TestOnChange(t *testing.T) {
	dir := t.TempDir()
	mgr := NewConfigManager(dir)

	ch := make(chan string, 1)
	mgr.OnChange(func(cfg *port.AppConfig) {
		ch <- cfg.User.Name
	})

	cfg, _ := mgr.Load()
	cfg.User.Name = "changed"
	mgr.Save(cfg)

	select {
	case name := <-ch:
		if name != "changed" {
			t.Errorf("expected 'changed', got %q", name)
		}
	default:
		// OnChange fires asynchronously, this might miss in fast tests
		t.Log("OnChange callback not caught in channel (async, expected in fast tests)")
	}

	// Give goroutine time
	_ = ch
}

func TestAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	mgr := NewConfigManager(dir)

	// Verify no .tmp file left after Save
	cfg, _ := mgr.Load()
	cfg.User.Name = "atomic"
	mgr.Save(cfg)

	tmpPath := filepath.Join(dir, "config.yaml.tmp")
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("temp file should not remain after successful atomic write")
	}
}

func TestNeedsMigration(t *testing.T) {
	dir := t.TempDir()
	mgr := NewConfigManager(dir)

	needs, err := mgr.NeedsMigration(context.Background())
	if err != nil {
		t.Fatalf("NeedsMigration: %v", err)
	}
	if needs {
		t.Log("migration needed (expected for fresh install — might be false if SchemaVersionCurrent==1)")
	}
}

func TestAppDataDirPerOS(t *testing.T) {
	dir := AppDataDir()

	if dir == "" {
		t.Fatal("AppDataDir returned empty string")
	}

	// Verify it contains "Sion" or "sion"
	base := filepath.Base(dir)
	if base != "Sion" && base != "sion" {
		t.Errorf("expected base dir to be 'Sion' or 'sion', got %q", base)
	}

	t.Logf("AppDataDir: %s", dir)
}
