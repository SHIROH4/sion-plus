package memory

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/shirohania/sion/internal/port"

	_ "modernc.org/sqlite"
)

// SQLiteStore implements port.MemoryStore backed by SQLite in WAL mode.
// Thread-safe: all public methods are guarded by a sync.Mutex.
//
// File layout:
//
//	sqlite_store.go   — struct, constructor, Session, utility helpers
//	sqlite_schema.go  — migrate() with all CREATE TABLE / INDEX / FTS5
//	sqlite_facts.go   — Facts, Diaries, Reflections, Strategies CRUD + scanners
//	sqlite_history.go — Messages, Episodes, Topics, Threads, Outcomes, Drives, Stats, Maintenance
type SQLiteStore struct {
	mu   sync.Mutex
	db   *sql.DB
	path string
}

// NewSQLiteStore opens (or creates) the SQLite database at dbPath,
// runs migrations, and enables WAL mode + foreign keys.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	// Ensure parent directory exists (SQLite won't create it).
	if dir := filepath.Dir(dbPath); dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}

	dsn := dbPath + "?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=5000"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite open: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	store := &SQLiteStore{db: db, path: dbPath}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite migrate: %w", err)
	}
	return store, nil
}

// Close closes the database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// Session returns nil — the SessionBuffer is in-memory and wired by the app layer.
func (s *SQLiteStore) Session() port.SessionBuffer {
	return nil
}

// ── Utility helpers ─────────────────────────────────────────────────

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullableInt64(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

// float32ToBlob encodes []float32 to little-endian binary for BLOB storage.
// Uses crude fixed-point (×1e6); will be replaced by sqlite-vec extension later.
func float32ToBlob(vec []float32) []byte {
	if len(vec) == 0 {
		return nil
	}
	buf := make([]byte, len(vec)*4)
	for i, v := range vec {
		bits := uint32(int32(v * 1e6))
		buf[i*4] = byte(bits)
		buf[i*4+1] = byte(bits >> 8)
		buf[i*4+2] = byte(bits >> 16)
		buf[i*4+3] = byte(bits >> 24)
	}
	return buf
}

func blobToFloat32(blob []byte) []float32 {
	if len(blob) == 0 || len(blob)%4 != 0 {
		return nil
	}
	vec := make([]float32, len(blob)/4)
	for i := range vec {
		bits := uint32(blob[i*4]) | uint32(blob[i*4+1])<<8 | uint32(blob[i*4+2])<<16 | uint32(blob[i*4+3])<<24
		vec[i] = float32(int32(bits)) / 1e6
	}
	return vec
}

// escapeFTS5 builds an FTS5-safe MATCH query string.
// Single words are quoted as phrases; multi-word queries become OR-connected terms.
func escapeFTS5(q string) string {
	q = strings.TrimSpace(q)
	if q == "" {
		return `""`
	}
	// Split into tokens and OR them together for multi-word queries
	words := strings.Fields(q)
	var parts []string
	for _, w := range words {
		// Escape double-quotes within each word
		w = strings.ReplaceAll(w, `"`, `""`)
		parts = append(parts, `"`+w+`"`)
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return strings.Join(parts, " OR ")
}
