import { defineStore } from 'pinia'
import { ref, readonly } from 'vue'
import type { LogEntry } from '@/shared/types'
import { getLogs } from '@/shared/api'

export const useLogsStore = defineStore('logs', () => {
  const logs = ref<LogEntry[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)
  let pollTimer: ReturnType<typeof setInterval> | null = null

  async function fetch() {
    try {
      logs.value = await getLogs()
    } catch (e) {
      error.value = e instanceof Error ? e.message : '加载失败'
    } finally {
      loading.value = false
    }
  }

  function startPolling(intervalMs = 3000) {
    stopPolling()
    loading.value = true
    fetch()
    pollTimer = setInterval(fetch, intervalMs)
  }

  function stopPolling() {
    if (pollTimer) { clearInterval(pollTimer); pollTimer = null }
  }

  function clear() { logs.value = [] }

  return {
    logs: readonly(logs),
    loading: readonly(loading),
    error: readonly(error),
    startPolling, stopPolling, clear,
  }
})
