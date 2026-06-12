// Package emotion contains pure domain logic for the emotional model:
//   - state.go: PAD computation, 8D vector → primary emotion inference
//   - smoothing.go: EMA blending with personality-modulated alphas
//   - decay.go: homeostatic decay toward neutral points
//   - circadian.go: sleepiness curve, sleep schedule learning
//
// All functions are PURE — no IO, no external dependencies beyond domain/types.
package emotion
