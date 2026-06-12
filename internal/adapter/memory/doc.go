// Package memory implements port.MemoryStore, port.EvidenceEngine, port.MemoryRecall.
//
// Files:
//   sqlite_store.go   — SQLite WAL mode, all CRUD for facts/diaries/strategies/threads/outcomes
//   session_buffer.go — ring buffer with time-based eviction (port.SessionBuffer)
//   evidence_engine.go — signal application, archive sweep, protect/unprotect (port.EvidenceEngine)
//   recall.go         — BM25 + vector cosine + RRF fusion hybrid search (port.MemoryRecall)
//   compressor.go     — multi-level inline compression with archive marker persistence
//   search.go         — FTS5 full-text search for keyword matching
//   vector.go         — sqlite-vec extension integration for ANN search

package memory

// TODO (module 11): Implement SQLiteStore
// TODO (module 14): Implement HybridRecall (BM25 + cosine + RRF)
// TODO (module 15): Implement EvidenceEngineImpl
