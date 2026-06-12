// Package memory contains pure domain logic for the memory system:
//   - evidence.go: dual-signal half-life decay, combo, status derivation
//   - forget.go: Ebbinghaus forgetting curve (importance × recall × recency)
//   - merge.go: strategy principle merging logic
//
// All functions are PURE — no IO, no external dependencies beyond domain/types.
package memory
