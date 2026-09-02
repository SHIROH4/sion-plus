package memory

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// SelfModelStore persists the AI's evolving self-description.
// Combines UserModel and SelfModel in a single JSON file.
type SelfModelStore struct {
	mu    sync.RWMutex
	path  string
	model *SelfModelBundle
}

// SelfModelBundle contains both user model and self model.
type SelfModelBundle struct {
	UserModel string `json:"user_model"`
	SelfModel string `json:"self_model"`
	Version   int    `json:"version"`
	UpdatedAt int64  `json:"updated_at"`
}

func NewSelfModelStore(dataDir string) *SelfModelStore {
	return &SelfModelStore{
		path: filepath.Join(dataDir, "self_model.json"),
	}
}

func (s *SelfModelStore) Load(ctx context.Context) (*SelfModelBundle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.model != nil {
		return s.model, nil
	}

	data, err := os.ReadFile(s.path)
	if err != nil {
		s.model = &SelfModelBundle{Version: 1}
		return s.model, nil
	}

	var bundle SelfModelBundle
	if err := json.Unmarshal(data, &bundle); err != nil {
		s.model = &SelfModelBundle{Version: 1}
		return s.model, nil
	}

	s.model = &bundle
	log.Printf("[SelfModelStore] loaded v%d (user=%d chars, self=%d chars)",
		bundle.Version, len(bundle.UserModel), len(bundle.SelfModel))
	return s.model, nil
}

func (s *SelfModelStore) Save(ctx context.Context, bundle *SelfModelBundle) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	bundle.Version++
	bundle.UpdatedAt = time.Now().Unix()
	s.model = bundle

	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}

	if err := os.WriteFile(s.path, data, 0644); err != nil {
		return err
	}

	log.Printf("[SelfModelStore] saved v%d", bundle.Version)
	return nil
}

// UserModel returns the current user model text.
func (s *SelfModelStore) UserModel() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.model == nil {
		return ""
	}
	return s.model.UserModel
}

// SelfModel returns the current self model text.
func (s *SelfModelStore) SelfModel() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.model == nil {
		return ""
	}
	return s.model.SelfModel
}

// Current returns the full bundle (may be nil if never loaded).
func (s *SelfModelStore) Current() *SelfModelBundle {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.model
}
