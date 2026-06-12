import { defineStore } from 'pinia'
import { ref, readonly } from 'vue'
import type { EmotionState, EmotionSnapshot } from '@/shared/types'
import { subscribeToSSE } from '@/shared/api'

const API = 'http://127.0.0.1:8080'

export const useEmotionStore = defineStore('emotion', () => {
  const current = ref<EmotionState | null>(null)
  const history = ref<EmotionSnapshot[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  let pollTimer: ReturnType<typeof setInterval> | null = null
  let unsubSSE: (() => void) | null = null

  function pushToHistory(state: EmotionState) {
    history.value.push({ ...state, timestamp: Date.now() })
    if (history.value.length > 200) {
      history.value = history.value.slice(-200)
    }
  }

  async function fetchCurrent() {
    try {
      const res = await fetch(`${API}/api/v1/emotion`)
      if (!res.ok) return
      const data = await res.json()
      current.value = {
        primary: data.primary,
        intensity: data.intensity,
        vector: data.vector,
      }
      pushToHistory(current.value)
      error.value = null
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'Failed to fetch emotion'
    }
  }

  function startPolling(intervalMs = 3000) {
    stopPolling()

    // SSE for real-time updates
    try {
      unsubSSE = subscribeToSSE(['emotion'], (_topic, data) => {
        const e: EmotionState = {
          primary: data.primary as string,
          intensity: data.intensity as number,
          vector: data.vector as EmotionState['vector'],
        }
        current.value = e
        pushToHistory(e)
      })
    } catch { /* */ }

    loading.value = true
    fetchCurrent().finally(() => { loading.value = false })
    pollTimer = setInterval(fetchCurrent, intervalMs)
  }

  function stopPolling() {
    if (pollTimer) { clearInterval(pollTimer); pollTimer = null }
    if (unsubSSE) { unsubSSE(); unsubSSE = null }
  }

  return {
    current: readonly(current),
    history: readonly(history),
    loading: readonly(loading),
    error: readonly(error),
    startPolling,
    stopPolling,
  }
})
