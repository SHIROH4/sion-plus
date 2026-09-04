<template>
  <div class="chat-view">
    <div class="chat-messages">
      <GlassCard
        padding="md"
        radius="lg"
        :empty="store.messages.length === 0 && !store.isStreaming"
        empty-icon="💬"
        empty-text="No messages yet. Say hi to Sion!"
      >
        <div class="messages-list" ref="messagesContainer">
          <ChatBubble
            v-for="msg in store.messages"
            :key="msg.id"
            :role="msg.role"
            :content="msg.content"
            :timestamp="msg.timestamp"
            :streaming="msg.streaming"
            :decision-id="msg.decisionId"
            @feedback="store.feedback"
          />
        </div>
      </GlassCard>
    </div>
    <div class="chat-input-bar">
      <div class="chat-input-wrapper glass-card">
        <input
          v-model="inputText"
          class="chat-input"
          placeholder="和 Sion 聊天..."
          @keydown.enter.exact="handleKeydown"
          @compositionstart="composing = true"
          @compositionend="composing = false"
          :disabled="store.isStreaming"
          maxlength="2000"
        />
        <button
          v-if="!store.isStreaming"
          class="chat-send-btn"
          :class="{ disabled: !inputText.trim() }"
          :disabled="!inputText.trim()"
          @click="handleSend"
        >
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
            <path d="M22 2 11 13M22 2l-7 20-4-9-9-4 20-7z"/>
          </svg>
        </button>
        <button
          v-else
          class="chat-stop-btn"
          @click="store.stop()"
        >
          <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor">
            <rect x="4" y="4" width="16" height="16" rx="2" />
          </svg>
          Stop
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, nextTick, onMounted } from 'vue'
import GlassCard from '../components/GlassCard.vue'
import ChatBubble from '../components/ChatBubble.vue'
import { useChatStore } from '../stores/chat'

const store = useChatStore()
const inputText = ref('')
const messagesContainer = ref<HTMLDivElement>()
const composing = ref(false)

// Scroll to bottom on mount (new messages since last visit)
onMounted(() => nextTick(scrollToBottom))

function handleKeydown(e: KeyboardEvent) {
  if (composing.value || e.isComposing) return
  handleSend()
}

function handleSend() {
  const text = inputText.value.trim()
  if (!text) return
  store.send(text)
  inputText.value = ''
}

// Auto-scroll on new messages AND during streaming
watch(
  () => store.messages.length,
  () => nextTick(scrollToBottom),
)
watch(
  () => {
    const last = store.messages[store.messages.length - 1]
    return last?.streaming ? last.content.length : -1
  },
  () => nextTick(scrollToBottom),
)

function scrollToBottom() {
  if (messagesContainer.value) {
    messagesContainer.value.scrollTop = messagesContainer.value.scrollHeight
  }
}
</script>

<style scoped>
.chat-view {
  display: flex;
  flex-direction: column;
  height: calc(100vh - var(--titlebar-height) - var(--page-padding) * 2);
  gap: 16px;
}

.chat-messages {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.chat-messages :deep(.glass-card-wrapper) {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.messages-list {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 16px;
}

.chat-input-bar {
  flex-shrink: 0;
}

.chat-input-wrapper {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 16px;
  border-radius: var(--radius-lg);
  background: var(--glass-card);
  backdrop-filter: blur(var(--blur-heavy)) saturate(1.2);
  -webkit-backdrop-filter: blur(var(--blur-heavy)) saturate(1.2);
  box-shadow:
    var(--shadow-card),
    inset 0 0.5px 1px rgba(255,255,255,0.4);
}

.chat-input {
  flex: 1;
  border: none;
  background: transparent;
  color: var(--text-primary);
  font-size: var(--text-sm);
  font-family: inherit;
  outline: none;
  padding: 8px 0;
}

.chat-input::placeholder {
  color: var(--text-tertiary);
}

.chat-send-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  border: none;
  border-radius: var(--radius-full);
  background: var(--color-accent);
  color: white;
  cursor: pointer;
  transition:
    background var(--duration-fast) var(--easing-standard),
    transform var(--duration-fast) var(--easing-standard),
    opacity var(--duration-fast) var(--easing-standard);
}

.chat-send-btn:hover:not(.disabled) {
  background: var(--color-accent-hover);
  transform: scale(1.08);
}

.chat-send-btn.disabled {
  opacity: 0.35;
  cursor: not-allowed;
}

.chat-stop-btn {
  display: flex;
  align-items: center;
  gap: 6px;
  height: 36px;
  padding: 0 16px;
  border: none;
  border-radius: var(--radius-full);
  background: var(--color-danger);
  color: white;
  font-size: var(--text-sm);
  font-family: inherit;
  cursor: pointer;
}
</style>
