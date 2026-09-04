<template>
  <div
    class="chat-bubble"
    :class="[`role-${role}`, { streaming }]"
  >
    <div class="bubble-content">
      {{ content }}<span v-if="streaming" class="streaming-cursor animate-cursor-blink">|</span>
    </div>
    <span v-if="timestamp" class="bubble-time">{{ formatTime(timestamp) }}</span>
    <div v-if="decisionId" class="feedback-actions">
      <button @click="$emit('feedback', decisionId, 'helpful')">有帮助</button>
      <button @click="$emit('feedback', decisionId, 'irrelevant')">内容无关</button>
      <button @click="$emit('feedback', decisionId, 'bad_timing')">时机不对</button>
      <button @click="$emit('feedback', decisionId, 'wrong_tone')">表达不适</button>
      <button @click="$emit('feedback', decisionId, 'snooze')">稍后</button>
      <button @click="$emit('feedback', decisionId, 'stop')">别再提醒</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { ProactiveFeedbackKind } from '@/shared/types'

const props = withDefaults(defineProps<{
  role: 'user' | 'ai' | 'system'
  content: string
  timestamp?: number
  streaming?: boolean
  decisionId?: string
}>(), {
  streaming: false,
})

defineEmits<{ feedback: [decisionId: string, kind: ProactiveFeedbackKind] }>()

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
.feedback-actions { display:flex; flex-wrap:wrap; gap:6px; margin-top:8px; }
.feedback-actions button { border:0; border-radius:10px; padding:3px 7px; cursor:pointer; font:inherit; font-size:10px; color:var(--text-secondary); background:rgba(255,255,255,.45); }
.feedback-actions button:hover { background:rgba(247,119,186,.18); color:var(--color-accent); }
</style>
