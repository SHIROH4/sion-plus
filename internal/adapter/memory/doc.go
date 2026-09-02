// Package memory implements port.MemoryStore, port.EvidenceEngine, port.MemoryRecall.
//
// Files:
//   sqlite_store.go    — SQLite WAL mode, all CRUD for facts/diaries/strategies/threads/outcomes
//   session_buffer.go  — ring buffer with time-based eviction (port.SessionBuffer)
//   evidence_engine.go — signal application, archive sweep, protect/unprotect (port.EvidenceEngine)
//   recall.go          — BM25 + vector cosine + RRF fusion hybrid search (port.MemoryRecall)
//   compressor.go      — multi-level inline compression with archive marker persistence

package memory
