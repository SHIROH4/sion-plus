package learning

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/shirohania/sion/internal/domain/cognition"
	"github.com/shirohania/sion/internal/port"
)

// LearnerImpl implements port.Learner using the DPO domain logic.
type LearnerImpl struct {
	mu sync.Mutex

	// Drive records waiting for batch learning
	records []cognition.DriveRecord
	nextID  int

	// Learned weight matrix
	weights *cognition.WeightMatrix

	// Configuration
	cfg cognition.DPOConfig

	// Scheduling
	lastLearnAt time.Time
	learnEvery  time.Duration
}

var _ port.Learner = (*LearnerImpl)(nil)

// NewLearner creates a Learner with the canonical weight matrix and default config.
func NewLearner(cfg cognition.DPOConfig) *LearnerImpl {
	if cfg.MinSamples == 0 {
		cfg = cognition.DefaultDPOConfig()
	}
	return &LearnerImpl{
		weights:     cognition.NewWeightMatrix(),
		cfg:         cfg,
		lastLearnAt: time.Now(),
		learnEvery:  6 * time.Hour,
	}
}

// SetLearnInterval overrides the default 6-hour learning interval.
func (l *LearnerImpl) SetLearnInterval(d time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.learnEvery = d
}

// RecordDrive stores a drive snapshot at decision time.
func (l *LearnerImpl) RecordDrive(action string, drives *port.DriveSnapshot, reward float64) int {
	l.mu.Lock()
	defer l.mu.Unlock()

	id := l.nextID
	l.nextID++

	record := cognition.DriveRecord{
		Action:  action,
		Reward:  reward,
		Social:  drives.Social,
		Care:    drives.Care,
		Curious: drives.Curious,
		Quiet:   drives.Quiet,
		Explore: drives.Explore,
		At:      time.Now().Unix(),
	}
	l.records = append(l.records, record)
	return id
}

// UpdateReward updates the reward for a previously recorded drive entry.
// Since we use a simple slice (not a map by ID), we search by ID.
func (l *LearnerImpl) UpdateReward(driveID int, reward float64) {
	l.mu.Lock()
	defer l.mu.Unlock()

	for i := range l.records {
		if i == driveID {
			l.records[i].Reward = reward
			return
		}
	}
}

// ShouldLearn returns true if enough time has passed and enough samples exist.
func (l *LearnerImpl) ShouldLearn() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if time.Since(l.lastLearnAt) < l.learnEvery {
		return false
	}
	return len(l.records) >= l.cfg.MinSamples
}

// BatchLearn runs a DPO weight update on all accumulated drive records.
// Returns the number of actions that had their weights updated.
func (l *LearnerImpl) BatchLearn(ctx context.Context) int {
	l.mu.Lock()
	records := make([]cognition.DriveRecord, len(l.records))
	copy(records, l.records)
	l.mu.Unlock()

	if len(records) < l.cfg.MinSamples {
		return 0
	}

	deltas := cognition.DPOUpdate(l.weights, records, l.cfg)
	if deltas == nil {
		return 0
	}

	// Purge processed records (keep last 10% for continuity)
	l.mu.Lock()
	keep := len(l.records) / 10
	if keep < l.cfg.MinSamples {
		keep = l.cfg.MinSamples
	}
	if keep > len(l.records) {
		keep = len(l.records)
	}
	l.records = l.records[len(l.records)-keep:]
	l.lastLearnAt = time.Now()
	l.mu.Unlock()

	log.Printf("[Learner] batch update: %d actions updated, %d records retained", len(deltas), keep)
	return len(deltas)
}

// Audit checks the weight matrix and learning health.
func (l *LearnerImpl) Audit(ctx context.Context) (*port.AuditResult, error) {
	l.mu.Lock()
	records := make([]cognition.DriveRecord, len(l.records))
	copy(records, l.records)
	l.mu.Unlock()

	result := cognition.AuditWeights(l.weights, records)
	return &port.AuditResult{
		StuckActions:   result.StuckActions,
		DriftWarning:   result.DriftWarning,
		Recommendation: result.Recommendation,
	}, nil
}

// CurrentWeights returns the current weight matrix for introspection.
func (l *LearnerImpl) CurrentWeights() *cognition.WeightMatrix {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.weights
}

// RecordCount returns the number of pending drive records.
func (l *LearnerImpl) RecordCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.records)
}
