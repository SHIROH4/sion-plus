import { ref, readonly } from 'vue'
import type { ChatMessage } from '@/shared/types'
import { sendMessageStream, subscribeToSSE } from '@/shared/api'

const messages = ref<ChatMessage[]>([])
const isStreaming = ref(false)
const streamingText = ref('')
const chatError = ref<string | null>(null)

let messageCounter = 0
function genId() { return `msg_${Date.now()}_${++messageCounter}` }

let sseStarted = false

function startSSE() {
  if (sseStarted) return
  sseStarted = true

  // Cross-window chat sync (messages from dashboard)
  subscribeToSSE(['chat-message'], (_topic, data) => {
    const msg = data as { role?: string; content?: string }
    if (!msg.content || !msg.role) return

    // Skip while we're sending our own message
    if (isStreaming.value) return

    // Skip own role=user (already added locally)
    if (msg.role === 'user') return

    // Dedup
    const last = messages.value[messages.value.length - 1]
    if (last && last.role === 'ai' && last.content === msg.content) return

    messages.value.push({
      id: genId(),
      role: 'ai',
      content: msg.content,
      timestamp: Date.now(),
      source: 'remote',
    })
  })

}

export function useChat() {
  startSSE()

  async function send(text: string) {
    if (!text.trim() || isStreaming.value) return
    chatError.value = null

    messages.value.push({
      id: genId(), role: 'user', content: text.trim(), timestamp: Date.now(),
    })

    const aiId = genId()
    messages.value.push({
      id: aiId, role: 'ai', content: '', timestamp: Date.now(), streaming: true,
    })
    isStreaming.value = true
    streamingText.value = ''

    await sendMessageStream(
      text.trim(),
      (token: string) => {
        streamingText.value += token
        const msg = messages.value.find(m => m.id === aiId)
        if (msg) msg.content = streamingText.value
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
        streamingText.value = ''
      },
      (err: string) => {
        chatError.value = err
        const msg = messages.value.find(m => m.id === aiId)
        if (msg) { msg.streaming = false; if (!msg.content) msg.content = '抱歉~ ' + err }
        isStreaming.value = false
        streamingText.value = ''
      },
      'pet',
    )
  }

  function stop() {
    isStreaming.value = false
    const last = [...messages.value].reverse().find(m => m.streaming)
    if (last) { last.streaming = false; last.source = 'interrupted' }
  }

  function clearMessages() { messages.value = []; chatError.value = null }

  return {
    messages: readonly(messages),
    isStreaming: readonly(isStreaming),
    streamingText: readonly(streamingText),
    chatError: readonly(chatError),
    send, stop, clearMessages,
  }
}
