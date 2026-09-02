package port

// ── Event Bus ──

// EventBus is the inter-module publish-subscribe mechanism.
// All cross-module asynchronous communication goes through the EventBus.
// Direct module-to-module calls are for synchronous operations only.
type EventBus interface {
	// Publish emits an event to all matching subscribers.
	// Non-blocking: if a subscriber panics, it is recovered and others proceed.
	Publish(topic string, payload any)

	// Subscribe registers a handler and returns an unsubscribe function.
	Subscribe(topic string, handler func(payload any)) (unsubscribe func())

	// SubscribePattern registers for topics matching a prefix pattern.
	// e.g., SubscribePattern("cognition:*") matches "cognition:tick", "cognition:decision"
	SubscribePattern(pattern string, handler func(topic string, payload any)) (unsubscribe func())
}

// ── Standard Topic Constants ──
// Use these constants instead of raw strings to avoid typos.

const (
	TopicUserActive            = "user:active"
	TopicUserIdle              = "user:idle"
	TopicUserWorking           = "user:working"
	TopicAppChanged            = "perception:app_changed"
	TopicChatSent              = "chat:sent"
	TopicChatReceived          = "chat:received"
	TopicMemoryUpdated         = "memory:updated"
	TopicMemoryForgotten       = "memory:forgotten"
	TopicMemoryEvidenceChanged = "memory:evidence_changed"
	TopicEmotionChanged        = "emotion:changed"
	TopicEmotionSpike          = "emotion:spike"
	TopicCareAction            = "care:action"
	TopicCareTrigger           = "care:trigger"
	TopicDecisionMade          = "cognition:decision"
	TopicCognitionTick         = "cognition:tick"
	TopicPlaybackStart         = "playback:start"
	TopicPlaybackEnd           = "playback:end"
	TopicProactiveSubmitted    = "proactive:submitted"
	TopicProactiveDelivered    = "proactive:delivered"
	TopicProactiveRejected     = "proactive:rejected"
	TopicPluginEvent           = "plugin:event"
	TopicConfigChanged         = "config:changed"
)
