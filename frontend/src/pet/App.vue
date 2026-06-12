<template>
  <div class="pet-window">
    <PetLive2D
      ref="petRef"
      :emotion="currentEmotion"
      @resize="onResize"
      @interaction="onInteraction"
    />
    <div
      class="input-zone"
      @mouseenter="hovered = true"
      @mouseleave="hovered = false"
    >
      <ChatCapsule
        :is-streaming="sending"
        :hovered="hovered"
        :error="sendError"
        @send="handleSend"
        @stop="handleStop"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import PetLive2D from './components/PetLive2D.vue'
import ChatCapsule from './components/ChatCapsule.vue'

const API = 'http://127.0.0.1:8080'

const petRef = ref<InstanceType<typeof PetLive2D>>()
const hovered = ref(false)
const sending = ref(false)
const sendError = ref<string | null>(null)
const currentEmotion = ref('neutral')
let eventSource: EventSource | null = null

// ── SSE: all incoming messages → bubble display ──
function connectSSE() {
  eventSource = new EventSource(`${API}/api/events?topics=chat-message,proactive`)

  eventSource.addEventListener('chat-message', (e: MessageEvent) => {
    try {
      const msg = JSON.parse(e.data)
      if (msg.role === 'assistant' && msg.content) {
        petRef.value?.showBubble(msg.content)
      }
    } catch {}
  })

  eventSource.addEventListener('proactive', (e: MessageEvent) => {
    try {
      const msg = JSON.parse(e.data)
      if (msg.message) {
        petRef.value?.showBubble(msg.message)
      }
    } catch {}
  })

  eventSource.onerror = () => {
    eventSource?.close()
    setTimeout(connectSSE, 3000)
  }
}

onMounted(() => connectSSE())
onUnmounted(() => eventSource?.close())

// ── Send message to backend ──
async function handleSend(text: string) {
  if (!text.trim() || sending.value) return
  petRef.value?.hideBubble()
  sending.value = true
  sendError.value = null

  try {
    const res = await fetch(`${API}/api/v1/chat`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ message: text.trim(), source: 'pet' }),
    })
    if (!res.ok) throw new Error(`HTTP ${res.status}`)
  } catch (e) {
    sendError.value = e instanceof Error ? e.message : '发送失败'
  } finally {
    sending.value = false
  }
}

function handleStop() {
  sending.value = false
}

function onResize(w: number, h: number) {
  const api = (window as any).electronAPI
  if (api?.resizeWindow) api.resizeWindow(w, h)
}

function onInteraction(_part: string) {}
</script>

<style scoped>
.pet-window {
  position: relative;
  width: 100vw;
  height: 100vh;
  overflow: hidden;
  background: transparent;
}

.input-zone {
  position: absolute;
  bottom: 0;
  left: 0;
  right: 0;
  height: 80px;
  z-index: 20;
}
</style>
