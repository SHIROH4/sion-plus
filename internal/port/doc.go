// Package port defines all inter-module interfaces ("ports" in hexagonal architecture).
// These interfaces are the CONSTITUTION of the project:
//   - Every module depends ONLY on port interfaces, never on concrete implementations.
//   - Each interface has exactly ONE concern (interface segregation principle).
//   - All methods accept context.Context as the first parameter.
package port

// This file is intentionally empty — interfaces are split across topic-specific files.
