import { defineStore } from 'pinia'
import { ref, readonly } from 'vue'
import type { ChatMessage, ProactiveFeedbackKind } from '@/shared/types'
import { sendMessageStream, sendProactiveFeedback, subscribeToSSE, getChatHistory } from '@/shared/api'

let messageCounter = 0
function genId() { return `msg_${Date.now()}_${++messageCounter}` }

export const useChatStore = defineStore('chat', () => {
  const messages = ref<ChatMessage[]>([])
  const isStreaming = ref(false)
  const error = ref<string | null>(null)
  const localRequestIds = new Set<string>()

  // Load persisted history on init (retry if backend not ready)
  async function loadHistory() {
    try {
      const history = await getChatHistory()
      if (history.length > 0) {
        messages.value = history.map(m => ({
          id: genId(),
          role: (m.role === 'assistant' ? 'ai' : m.role) as ChatMessage['role'],
          content: m.content,
          timestamp: m.created_at * 1000,
        }))
      }
    } catch {
      // Backend not ready yet, retry once after delay
      setTimeout(async () => {
        try {
          const history = await getChatHistory()
          if (history.length > 0) {
            messages.value = history.map(m => ({
              id: genId(),
              role: (m.role === 'assistant' ? 'ai' : m.role) as ChatMessage['role'],
              content: m.content,
              timestamp: m.created_at * 1000,
            }))
          }
        } catch { /* */ }
      }, 3000)
    }
  }
  loadHistory()

  async function send(text: string) {
    if (!text.trim() || isStreaming.value) return
    error.value = null
    const clientMessageId = genId()
    localRequestIds.add(clientMessageId)

    messages.value.push({
      id: genId(), role: 'user', content: text.trim(), timestamp: Date.now(),
    })

    const aiId = genId()
    messages.value.push({
      id: aiId, role: 'ai', content: '', timestamp: Date.now(), streaming: true,
    })
    isStreaming.value = true

    await sendMessageStream(
      text.trim(),
      (token: string) => {
        const msg = messages.value.find(m => m.id === aiId)
        if (msg) msg.content += token
      },
      (result) => {
        const msg = messages.value.find(m => m.id === aiId)
        if (msg) {
          msg.content = result.response
          msg.emotion = result.emotion
          msg.source = result.source
          msg.streaming = false
        }
        isStreaming.value = false
      },
      (err: string) => {
        console.error('[stream] error:', err)
        error.value = err
        const msg = messages.value.find(m => m.id === aiId)
        if (msg) { msg.streaming = false }
        isStreaming.value = false
      },
      'dashboard',
      clientMessageId,
    )
  }

  function stop() {
    isStreaming.value = false
    const last = [...messages.value].reverse().find(m => m.streaming)
    if (last) { last.streaming = false; last.source = 'interrupted' }
  }

  function clear() { messages.value = []; error.value = null }

  // ── SSE listener ─────────────────────────────────────────────
  subscribeToSSE(['chat-message'], (_topic, data) => {
    const msg = data as { role?: string; content?: string; decision_id?: string; client_message_id?: string }
    // Normalize role: backend sends "assistant", local type is "ai"
    const role = msg.role === 'assistant' ? 'ai' : msg.role === 'user' ? 'user' : null
    if (!role || !msg.content) return

    // The streaming view already renders both sides of its own request.  Match
    // the server's correlation id so a proactive event cannot make its echo
    // appear as a second assistant message.
    if (msg.client_message_id && localRequestIds.has(msg.client_message_id)) return
    // Dedup by role + content
    const last = messages.value[messages.value.length - 1]
    if (last && last.role === role && last.content === msg.content) return

    messages.value.push({
      id: genId(),
      role: role as ChatMessage['role'],
      content: msg.content,
      timestamp: Date.now(),
      decisionId: msg.decision_id,
    })
  })

  async function feedback(decisionId: string, kind: ProactiveFeedbackKind) {
    try { await sendProactiveFeedback(decisionId, kind) } catch (err) { error.value = String(err) }
  }

  return {
    messages: readonly(messages),
    isStreaming: readonly(isStreaming),
    error: readonly(error),
    send, stop, clear, feedback,
  }
})
