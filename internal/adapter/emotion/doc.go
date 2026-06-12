// Package emotion implements port.EmotionStateManager (persistence + lifecycle) and
// port.EmotionEvaluator (LLM + rule-based).
//
// Files:
//   emotion_store.go   — SQLite persistence for EmotionState + EmotionVector
//   llm_evaluator.go   — Kardia-R1 style LLM evaluation (port.EmotionEvaluator)
//   rule_evaluator.go  — regex-based fallback evaluator (port.EmotionEvaluator)
//   cache.go           — simhash cache for identical recent turns

package emotion

// TODO (module 17): Implement EmotionStateManager adapter
//   - Load/Save: read/write emotion state from SQLite
//   - Background decay goroutine (5min ticker)
//   - NotifyActivity: reunion effect, activity hour tracking
//   - LearnPersonality: adjust from outcome history
