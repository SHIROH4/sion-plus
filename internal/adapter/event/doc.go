// Package event implements port.EventBus.
//
// Files:
//   event_bus.go — channel-based pub/sub with panic recovery per subscriber

package event

// TODO (module 6): Implement EventBusImpl
//   - Publish(): non-blocking, recover per-subscriber panics
//   - Subscribe(): returns unsubscribe func
//   - SubscribePattern(): prefix-based topic matching
