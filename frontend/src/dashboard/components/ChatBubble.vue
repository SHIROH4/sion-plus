<template>
  <div
    class="chat-bubble"
    :class="[`role-${role}`, { streaming }]"
  >
    <div class="bubble-content">
      {{ content }}<span v-if="streaming" class="streaming-cursor animate-cursor-blink">|</span>
    </div>
    <span v-if="timestamp" class="bubble-time">{{ formatTime(timestamp) }}</span>
  </div>
</template>

<script setup lang="ts">
const props = withDefaults(defineProps<{
  role: 'user' | 'ai' | 'system'
  content: string
  timestamp?: number
  streaming?: boolean
}>(), {
  streaming: false,
})

function formatTime(ts: number): string {
  const d = new Date(ts)
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}
</script>

<style scoped>
.chat-bubble {
  display: flex;
  flex-direction: column;
  max-width: 75%;
  padding: 10px 16px;
  font-size: var(--text-sm);
  line-height: 1.55;
  animation: slide-up var(--duration-slow) var(--easing-standard) both;
}

.role-user {
  align-self: flex-end;
  background: var(--bubble-user-bg);
  color: var(--bubble-user-text);
  border-radius: 18px 18px 4px 18px;
  box-shadow: var(--shadow-bubble-user);
}

.role-ai {
  align-self: flex-start;
  background: var(--bubble-ai-bg);
  color: var(--bubble-ai-text);
  border-radius: 18px 18px 18px 4px;
  box-shadow: var(--shadow-bubble-ai);
  backdrop-filter: blur(var(--blur-standard));
  -webkit-backdrop-filter: blur(var(--blur-standard));
}

.role-system {
  align-self: center;
  background: transparent;
  color: var(--text-tertiary);
  font-size: var(--text-xs);
  padding: 4px 12px;
}

.bubble-content {
  white-space: pre-wrap;
  word-break: break-word;
}

.streaming-cursor {
  color: var(--color-accent);
  font-weight: 600;
}

.bubble-time {
  font-size: 10px;
  color: inherit;
  opacity: 0.5;
  margin-top: 4px;
  align-self: flex-end;
}
</style>
