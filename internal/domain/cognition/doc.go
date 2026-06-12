// Package cognition contains pure domain logic for the decision engine:
//   - actions.go: 16-action registry with weight vectors and SkillCards
//   - drives.go: 52D features → 5D drives weighted formulas
//   - router.go: System 1 / System 2 decision routing (7 triggers)
//   - needs.go: 6D intrinsic need model with homeostatic dynamics
//   - features.go: Tier 1 feature computation (pure in-memory, ~1ms)
//   - motivator.go: action scoring (drive dot-product + context modulation)
//   - learner.go: DPO-style batch weight updates from action outcomes
//   - strategy.go: strategy reflection scheduling + outcome pattern analysis
//
// Tier 2 features (SQL-backed) live in adapter/memory/feature_store.go
// because they require database access.
//
// All functions are PURE — no IO, no external dependencies beyond domain/types.
package cognition
