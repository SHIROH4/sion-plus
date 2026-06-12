// Package config implements port.ConfigManager.
//
// Files:
//   config_manager.go — YAML load/save, env var expansion, version migration, hot-reload
//   migration.go      — migration chain (map[int]MigrationFunc), version marker, atomic writes
//   defaults.go       — defaultConfig() with sensible defaults for all fields

package config

// TODO (module 7): Implement ConfigManagerImpl
// - Load(): read ~/.sion/config.yaml, expand ${ENV_VARS}, return AppConfig
// - Save(): marshal to YAML, atomic write (write tmp + rename)
// - RunMigrations(): sequential migration chain from current→SchemaVersionCurrent
// - Validate(): check required fields, valid ranges
// - OnChange(): callback registry for hot-reload
