// Package sse implements Server-Sent Events for real-time frontend updates.
//
// Files:
//   broker.go — SSE broker: client connect/disconnect, topic-based fanout, heartbeat

package sse

// TODO: Implement SSE broker
//   - GET /api/events → SSE stream
//   - Clients subscribe to topics via query params: /api/events?topics=chat,emotion
//   - Broker fans out events to matching subscribers
//   - Heartbeat every 30s to detect dead connections
