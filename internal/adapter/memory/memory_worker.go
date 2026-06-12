package memory

import (
	"context"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"github.com/shirohania/sion/internal/domain/types"
	"github.com/shirohania/sion/internal/port"
)

// MemoryWorker orchestrates background memory operations.
// Single goroutine pool (size=3 per v2.0 design) receives wake signals
// from OnAfterChat and processes them in priority order.
//
// LLM hooks are injected when the corresponding executors are available.
// Until wired, each step is a no-op — the skeleton compiles and runs.
type MemoryWorker struct {
	store    *SQLiteStore
	evidence *EvidenceEngine
	recall   *Recall
	buffer   *SessionBuffer
	compress *Compressor
	eventLog *EventLog

	// Wake channel — OnAfterChat sends here
	wakeCh chan struct{}

	// LLM hooks (injected later)
	extractFactsFn    func(ctx context.Context, messages []types.Message) ([]types.FactEntry, error)
	detectSignalsFn   func(ctx context.Context, newFacts, existingFacts []types.FactEntry) ([]SignalResult, error)
	reflectAndDiaryFn func(ctx context.Context, facts []types.FactEntry, msgs []types.Message) (*ReflectAndDiaryResult, error)
	identityBuilder   *IdentityBuilder

	// State
	mu                   sync.Mutex
	running              bool
	processing           bool   // CAS flag: true when a worker is processing a wake signal
	turnsSinceExtraction int
	lastExtractionAt     time.Time
	lastReflectionAt     time.Time
	lastArchiveSweep     time.Time
	stopCh               chan struct{}
	wg                   sync.WaitGroup

	// Emotional spike → force diary on next wake
	forceDiaryOnNextWake bool

	// Reflection promotion tracking (for promote_blocked detection)
	promoteFailures map[int64]int // reflectionID → consecutive failure count

	// Config (set by Start)
	extractEveryN int

	// Emotion modulation (v2.1)
	lastValence float64 // previous evaluation valence, for change detection
	lastArousal float64 // previous arousal, for importance modulation
}

// SignalResult is the output of SignalDetection LLM.
type SignalResult struct {
	EntryID int64  `json:"entry_id"`
	Type    string `json:"type"` // "reinforce"|"contradict"
	Source  string `json:"source"`
}

// ReflectAndDiaryResult is the joint output of Reflection+Diary LLM call.
type ReflectAndDiaryResult struct {
	Reflections []types.ReflectionEntry
	Diary       *types.DiaryEntry
}

// MemoryWorkerConfig tunes worker behaviour.
type MemoryWorkerConfig struct {
	PoolSize          int           // goroutine pool size (default 3)
	ExtractEveryN     int           // extract facts every N turns (default 10)
	ExtractIdleMin    int           // or when idle for N minutes (default 5)
	ReflectionMaxGap  time.Duration // max gap between reflections (default 4h)
	MaintenanceTick   time.Duration // maintenance interval (default 30min)
	ArchiveTick       time.Duration // archive sweep interval (default 1h)
}

// DefaultWorkerConfig returns sensible defaults.
func DefaultWorkerConfig() MemoryWorkerConfig {
	return MemoryWorkerConfig{
		PoolSize:         3,
		ExtractEveryN:    3,
		ExtractIdleMin:   5,
		ReflectionMaxGap: 4 * time.Hour,
		MaintenanceTick:  30 * time.Minute,
		ArchiveTick:      1 * time.Hour,
	}
}

// NewMemoryWorker creates a worker bound to the memory stack.
func NewMemoryWorker(
	store *SQLiteStore,
	evidence *EvidenceEngine,
	recall *Recall,
	buffer *SessionBuffer,
	compress *Compressor,
	cfg MemoryWorkerConfig,
) *MemoryWorker {
	w := &MemoryWorker{
		store:      store,
		evidence:   evidence,
		recall:     recall,
		buffer:     buffer,
		compress:   compress,
		wakeCh:         make(chan struct{}, 64),
		stopCh:         make(chan struct{}),
		promoteFailures: make(map[int64]int),
		lastExtractionAt: time.Now(),
		lastReflectionAt: time.Now(),
		lastArchiveSweep: time.Now(),
	}
	return w
}

// ── LLM hook setters ────────────────────────────────────────────────

func (w *MemoryWorker) SetExtractFactsHook(fn func(ctx context.Context, messages []types.Message) ([]types.FactEntry, error)) {
	w.extractFactsFn = fn
}

func (w *MemoryWorker) SetDetectSignalsHook(fn func(ctx context.Context, newFacts, existingFacts []types.FactEntry) ([]SignalResult, error)) {
	w.detectSignalsFn = fn
}

func (w *MemoryWorker) SetIdentityBuilder(b *IdentityBuilder) { w.identityBuilder = b }

func (w *MemoryWorker) SetReflectAndDiaryHook(fn func(ctx context.Context, facts []types.FactEntry, msgs []types.Message) (*ReflectAndDiaryResult, error)) {
	w.reflectAndDiaryFn = fn
}

// Store returns the underlying SQLiteStore (for testing/inspection).
func (w *MemoryWorker) Store() *SQLiteStore { return w.store }

// SetEventLog wires an EventLog for audit trail + funnel analytics.
func (w *MemoryWorker) SetEventLog(el *EventLog) {
	w.eventLog = el
}

// UpdateEmotionState feeds the latest emotion PAD values into the worker.
//   - arousal > 0.6 → boosts fact importance by +2 (high emotional salience)
//   - valence change > 0.3 → triggers immediate diary + reflection (emotional spike)
//   - combined valence swing + high arousal → diary always generated
func (w *MemoryWorker) UpdateEmotionState(valence, arousal float64) {
	w.mu.Lock()
	valenceChange := math.Abs(valence - w.lastValence)
	w.lastValence = valence
	w.lastArousal = arousal
	w.mu.Unlock()

	// Emotional spike detection: valence swing + high arousal → force diary
	if valenceChange > 0.3 && arousal > 0.6 {
		w.forceDiaryOnNextWake = true
		log.Printf("[MemoryWorker] emotional spike detected (ΔV=%.2f, A=%.2f) — diary forced on next wake",
			valenceChange, arousal)
		w.Wake()
	} else if valenceChange > 0.3 {
		w.Wake()
	}
}

// currentArousalBoost returns +2 if arousal > 0.6, 0 otherwise.
// Used by fact extraction to modulate importance based on emotional salience.
func (w *MemoryWorker) currentArousalBoost() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.lastArousal > 0.6 {
		return 2
	}
	return 0
}

// ── Lifecycle ───────────────────────────────────────────────────────

// Start launches the background goroutine pool.
func (w *MemoryWorker) Start(ctx context.Context, cfg MemoryWorkerConfig) {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return
	}
	w.running = true
	w.extractEveryN = cfg.ExtractEveryN
	w.turnsSinceExtraction = cfg.ExtractEveryN - 1 // trigger on first turn
	w.mu.Unlock()

	for i := 0; i < cfg.PoolSize; i++ {
		w.wg.Add(1)
		go w.loop(ctx, cfg, i)
	}

	// Maintenance and archive tickers
	w.wg.Add(1)
	go w.maintenanceLoop(ctx, cfg)

	log.Printf("[MemoryWorker] started with pool_size=%d", cfg.PoolSize)
}

// Stop gracefully shuts down all goroutines.
func (w *MemoryWorker) Stop() {
	w.mu.Lock()
	w.running = false
	w.mu.Unlock()

	close(w.stopCh)
	w.wg.Wait()
	log.Println("[MemoryWorker] stopped")
}

// Wake signals the worker to process a new conversation turn.
// Non-blocking: if the channel is full, the signal is dropped
// (the worker will pick up the work on the next maintenance tick).
func (w *MemoryWorker) Wake() {
	select {
	case w.wakeCh <- struct{}{}:
	default:
	}
}

// ── Main loop ───────────────────────────────────────────────────────

func (w *MemoryWorker) loop(ctx context.Context, cfg MemoryWorkerConfig, workerID int) {
	defer w.wg.Done()

	for {
		select {
		case <-w.stopCh:
			return
		case <-ctx.Done():
			return
		case <-w.wakeCh:
			w.processWake(ctx, cfg)
		case <-time.After(5 * time.Minute):
			// Idle timeout: run extraction if enough turns have accumulated
			w.mu.Lock()
			idle := time.Since(w.lastExtractionAt).Minutes()
			turns := w.turnsSinceExtraction
			w.mu.Unlock()

			if idle >= float64(cfg.ExtractIdleMin) && turns > 0 {
				w.processWake(ctx, cfg)
			}
		}
	}
}

func (w *MemoryWorker) maintenanceLoop(ctx context.Context, cfg MemoryWorkerConfig) {
	defer w.wg.Done()

	maintenanceTicker := time.NewTicker(cfg.MaintenanceTick)
	archiveTicker := time.NewTicker(cfg.ArchiveTick)
	defer maintenanceTicker.Stop()
	defer archiveTicker.Stop()

	for {
		select {
		case <-w.stopCh:
			return
		case <-ctx.Done():
			return
		case <-maintenanceTicker.C:
			w.runMaintenance(ctx)
		case <-archiveTicker.C:
			w.runArchiveSweep(ctx)
		}
	}
}

// ── Processing ──────────────────────────────────────────────────────

func (w *MemoryWorker) processWake(ctx context.Context, cfg MemoryWorkerConfig) {
	// CAS guard: skip if another worker is already processing
	w.mu.Lock()
	if w.processing {
		w.mu.Unlock()
		return
	}
	w.processing = true
	w.mu.Unlock()
	defer func() {
		w.mu.Lock()
		w.processing = false
		w.mu.Unlock()
	}()

	// Phase 1: Fact extraction
	newFacts := w.runFactExtraction(ctx)
	if len(newFacts) > 0 {
		log.Printf("[MemoryWorker] extracted %d new facts", len(newFacts))
	}

	// Phase 2: Signal detection (if new facts)
	if len(newFacts) > 0 {
		w.runSignalDetection(ctx, newFacts)
	}

	// Phase 3: Reflection + Diary (if enough unabsorbed facts)
	w.runReflectionAndDiary(ctx, cfg)

	// Phase 4: L0 compression
	w.runCompression(ctx, DefaultCompressorConfig())

	w.mu.Lock()
	w.turnsSinceExtraction = 0
	w.lastExtractionAt = time.Now()
	w.mu.Unlock()
}

func (w *MemoryWorker) runFactExtraction(ctx context.Context) []types.FactEntry {
	if w.extractFactsFn == nil {
		return nil
	}

	msgs, err := w.store.LoadHistory(ctx, 50)
	if err != nil || len(msgs) == 0 {
		return nil
	}
	log.Printf("[MemoryWorker] extracting from %d messages...", len(msgs))

	typedMsgs := make([]types.Message, len(msgs))
	for i, m := range msgs {
		typedMsgs[i] = types.Message{Role: m.Role, Content: m.Content, CreatedAt: m.CreatedAt}
	}

	facts, err := w.extractFactsFn(ctx, typedMsgs)
	if err != nil {
		log.Printf("[MemoryWorker] extractFactsFn failed: %v", err)
		return nil
	}

	// Persist extracted facts
	arousalBoost := w.currentArousalBoost()
	for i := range facts {
		facts[i].SignalProcessed = false
		facts[i].Source = "chat"
		if arousalBoost > 0 && facts[i].Importance+arousalBoost <= 10 {
			facts[i].Importance += arousalBoost
		}
		if err := w.store.SaveFact(ctx, &facts[i]); err != nil {
			log.Printf("[MemoryWorker] SaveFact failed: %v", err)
		}
	}

	return facts
}

func (w *MemoryWorker) runSignalDetection(ctx context.Context, newFacts []types.FactEntry) {
	if w.detectSignalsFn == nil {
		return
	}

	// Get existing active facts as signal targets
	existing, err := w.store.ListActiveFacts(ctx, 0)
	if err != nil {
		return
	}

	results, err := w.detectSignalsFn(ctx, newFacts, existing)
	if err != nil {
		return
	}

	// Apply signals
	for _, r := range results {
		sigType := portSignalType(r.Type)
		_, err := w.evidence.ApplySignal(ctx, r.EntryID, port.EvidenceSignal{
			EntryID: r.EntryID,
			Type:   sigType,
			Source: r.Source,
		})
		if err != nil {
			log.Printf("[MemoryWorker] ApplySignal failed: %v", err)
		}
	}

	// Mark new facts as processed
	ids := make([]int64, len(newFacts))
	for i, f := range newFacts {
		ids[i] = f.ID
	}
	if err := w.store.MarkFactsProcessed(ctx, ids); err != nil {
		log.Printf("[MemoryWorker] MarkFactsProcessed failed: %v", err)
	}
}

func (w *MemoryWorker) runReflectionAndDiary(ctx context.Context, cfg MemoryWorkerConfig) {
	if w.reflectAndDiaryFn == nil {
		return
	}

	w.mu.Lock()
	gap := time.Since(w.lastReflectionAt)
	arousal := w.lastArousal
	w.mu.Unlock()

	// Emotional spike → bypass all thresholds, always generate diary
	w.mu.Lock()
	forceDiary := w.forceDiaryOnNextWake
	w.forceDiaryOnNextWake = false
	w.mu.Unlock()

	if !forceDiary {
		// Normal path: skip if too recent and not high arousal
		if gap < cfg.ReflectionMaxGap/2 && arousal < 0.6 {
			return
		}
	}

	unabsorbed, err := w.store.ListUnabsorbedFacts(ctx, 5, 50)
	if err != nil || (!forceDiary && len(unabsorbed) < 5) {
		return
	}

	// If forcing diary with few facts, log and proceed anyway
	if forceDiary && len(unabsorbed) < 3 {
		log.Printf("[MemoryWorker] emotional diary skipped — insufficient facts (%d)", len(unabsorbed))
		return
	}

	msgs, _ := w.store.LoadHistory(ctx, 20)
	typedMsgs := make([]types.Message, len(msgs))
	for i, m := range msgs {
		typedMsgs[i] = types.Message{Role: m.Role, Content: m.Content, CreatedAt: m.CreatedAt}
	}

	result, err := w.reflectAndDiaryFn(ctx, unabsorbed, typedMsgs)
	if err != nil || result == nil {
		return
	}

	// Persist reflections
	sourceIDs := make([]int64, len(unabsorbed))
	for i, f := range unabsorbed {
		sourceIDs[i] = f.ID
	}
	for i := range result.Reflections {
		result.Reflections[i].SourceFactIDs = sourceIDs
		if _, err := w.store.SaveReflection(ctx, &result.Reflections[i]); err != nil {
			log.Printf("[MemoryWorker] SaveReflection failed: %v", err)
		}
	}

	// Persist diary
	if result.Diary != nil {
		if err := w.store.SaveDiary(ctx, result.Diary); err != nil {
			log.Printf("[MemoryWorker] SaveDiary failed: %v", err)
		}
	}

	// Mark source facts as absorbed
	if err := w.store.MarkFactsAbsorbed(ctx, sourceIDs); err != nil {
		log.Printf("[MemoryWorker] MarkFactsAbsorbed failed: %v", err)
	}

	w.mu.Lock()
	w.lastReflectionAt = time.Now()
	w.mu.Unlock()
}

func (w *MemoryWorker) runCompression(ctx context.Context, cfg CompressorConfig) {
	if w.compress == nil {
		return
	}
	result, err := w.compress.Run(ctx, cfg)
	if err != nil || result == nil {
		return
	}

	// Archive trimmed messages
	if len(result.TrimmedMsgs) > 0 {
		if err := w.store.SaveHistory(ctx, result.TrimmedMsgs); err != nil {
			log.Printf("[MemoryWorker] archive trimmed msgs failed: %v", err)
		}
	}
}

// ── Periodic maintenance ────────────────────────────────────────────

func (w *MemoryWorker) runMaintenance(ctx context.Context) {
	// Run forgetting (expired state facts, sub-zero sweep)
	if err := w.store.RunForgetting(ctx); err != nil {
		log.Printf("[MemoryWorker] RunForgetting failed: %v", err)
	}

	// Reflection lifecycle: scan + auto-promote pending→confirmed→promoted
	w.runPromotionSweep(ctx)
	// Detect contradiction among promoted reflections
	w.detectContradictions(ctx)
	// Build identity from promoted reflections
	if w.identityBuilder != nil {
		if err := w.identityBuilder.BuildIdentity(ctx); err != nil {
			log.Printf("[MemoryWorker] IdentityBuilder: %v", err)
		}
	}
}

func (w *MemoryWorker) runArchiveSweep(ctx context.Context) {
	if w.evidence == nil {
		return
	}
	n, err := w.evidence.ArchiveSweep(ctx)
	if err != nil {
		log.Printf("[MemoryWorker] ArchiveSweep failed: %v", err)
	} else if n > 0 {
		log.Printf("[MemoryWorker] ArchiveSweep: archived %d facts", n)
	}
	w.mu.Lock()
	w.lastArchiveSweep = time.Now()
	w.mu.Unlock()
}

// ── Reflection state machine (v2.1: complete 7-state) ──────────────

func (w *MemoryWorker) autoConfirmReflections(ctx context.Context) {
	const autoConfirmDays = 3
	cutoff := time.Now().AddDate(0, 0, -autoConfirmDays).Unix()

	pending, err := w.store.ListReflectionsByStatus(ctx, []string{"pending"}, 100)
	if err != nil {
		return
	}

	for _, r := range pending {
		if r.CreatedAt < cutoff {
			if types.CanTransitionReflection(types.ReflPending, types.ReflConfirmed) {
				_ = w.transitionReflection(ctx, r.ID, string(types.ReflPending), string(types.ReflConfirmed), "auto-confirmed after 3 days")
			}
		}
	}
}

// promoteReflections promotes confirmed reflections to persona.
// Called during maintenance. Merges reflections with high reinforcement.
func (w *MemoryWorker) promoteReflections(ctx context.Context) {
	const promoteThreshold = 2.0

	confirmed, err := w.store.ListReflectionsByStatus(ctx, []string{"confirmed"}, 50)
	if err != nil {
		return
	}

	for _, r := range confirmed {
		if r.Reinforcement < promoteThreshold {
			continue
		}

		isNew := false
		w.mu.Lock()
		if _, ok := w.promoteFailures[r.ID]; !ok {
			w.promoteFailures[r.ID] = 0
			isNew = true
		}
		failCount := w.promoteFailures[r.ID]
		w.mu.Unlock()

		if isNew || failCount == 0 {
			if types.CanTransitionReflection(types.ReflConfirmed, types.ReflPromoted) {
				if err := w.transitionReflection(ctx, r.ID, string(types.ReflConfirmed), string(types.ReflPromoted), ""); err != nil {
					w.mu.Lock()
					w.promoteFailures[r.ID]++
					if w.promoteFailures[r.ID] >= 3 {
						_ = w.transitionReflection(ctx, r.ID, string(types.ReflConfirmed), string(types.ReflPromoteBlocked),
							fmt.Sprintf("failed promotion %d times", w.promoteFailures[r.ID]))
					}
					w.mu.Unlock()
				} else {
					w.mu.Lock()
					delete(w.promoteFailures, r.ID)
					w.mu.Unlock()

					// After promotion, merge into persona-like storage (absorb source facts)
					w.mergeReflectionIntoPersona(ctx, &r)
				}
			}
		}
	}
}

// transitionReflection performs a validated state transition and logs the event.
func (w *MemoryWorker) transitionReflection(ctx context.Context, id int64, from, to, feedback string) error {
	if err := w.store.UpdateReflectionStatus(ctx, id, to, feedback); err != nil {
		return err
	}
	if w.eventLog != nil {
		_ = w.eventLog.LogReflectionStateChanged(ctx, id, from, to, feedback)
	}
	return nil
}

// mergeReflectionIntoPersona absorbs a promoted reflection into the persona (L2 user/self model).
// Marks source facts as absorbed and logs the merge event.
func (w *MemoryWorker) mergeReflectionIntoPersona(ctx context.Context, r *types.ReflectionEntry) {
	// Mark source facts as absorbed
	if len(r.SourceFactIDs) > 0 {
		if err := w.store.MarkFactsAbsorbed(ctx, r.SourceFactIDs); err != nil {
			log.Printf("[MemoryWorker] MarkFactsAbsorbed failed: %v", err)
		}
		// Log absorption events
		if w.eventLog != nil {
			for _, fid := range r.SourceFactIDs {
				_ = w.eventLog.LogFactAbsorbed(ctx, fid, r.ID)
			}
		}
	}

	// Transition to merged
	_ = w.transitionReflection(ctx, r.ID, string(types.ReflPromoted), string(types.ReflMerged), "")
}

// ── OnAfterChat hook ────────────────────────────────────────────────

// OnAfterChat is called by the app layer after each conversation turn.
// Appends messages to L0, saves to history, and wakes the worker.
func (w *MemoryWorker) OnAfterChat(ctx context.Context, userMsg, assistantMsg string) {
	now := time.Now().Unix()

	// L0
	w.buffer.Append(types.Message{Role: types.RoleUser, Content: userMsg, CreatedAt: now})
	w.buffer.Append(types.Message{Role: types.RoleAssistant, Content: assistantMsg, CreatedAt: now})

	// Persistent history
	_ = w.store.SaveHistory(ctx, []types.Message{
		{Role: types.RoleUser, Content: userMsg, CreatedAt: now},
		{Role: types.RoleAssistant, Content: assistantMsg, CreatedAt: now},
	})

	// Wake worker
	w.mu.Lock()
	w.turnsSinceExtraction++
	turns := w.turnsSinceExtraction
	everyN := w.extractEveryN
	w.mu.Unlock()

	if turns >= everyN {
		w.Wake()
	}
}

// ── Helpers ─────────────────────────────────────────────────────────

func portSignalType(t string) port.EvidenceSignalType {
	switch t {
	case "reinforce":
		return port.SignalUserFact
	case "contradict":
		return port.SignalContradiction
	default:
		return port.EvidenceSignalType(t)
	}
}
