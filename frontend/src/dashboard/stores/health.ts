import { defineStore } from 'pinia'
import { ref, readonly, computed } from 'vue'
import type { HealthStatus } from '@/shared/types'
import { getHealthStatus } from '@/shared/api'

const moduleIcons: Record<string, string> = { llm: '🧠', memory: '💾', emotion: '💗' }
const moduleLabels: Record<string, string> = { llm: 'LLM', memory: '记忆', emotion: '情绪' }

export const useHealthStore = defineStore('health', () => {
  const health = ref<HealthStatus>({
    status: 'ok', modules: {},
    cpu_cores: 0, mem_used_mb: 0, mem_total_mb: 0, goroutines: 0, uptime_sec: 0,
  })
  const loading = ref(false)
  const error = ref<string | null>(null)
  let pollTimer: ReturnType<typeof setInterval> | null = null

  const allOk = computed(() => {
    const m = health.value.modules
    return Object.keys(m).length > 0 && Object.values(m).every(v => v === 'ok')
  })

  const moduleList = computed(() => {
    return Object.entries(health.value.modules).map(([name, status]) => ({
      name,
      label: moduleLabels[name] || name,
      icon: moduleIcons[name] || '📦',
      ok: status === 'ok',
      message: status !== 'ok' ? status : undefined,
    }))
  })

  function formatUptime(s: number): string {
    const h = Math.floor(s / 3600)
    const m = Math.floor((s % 3600) / 60)
    return h > 0 ? `${h}h ${m}m` : `${m}m`
  }

  async function fetch() {
    try {
      health.value = await getHealthStatus()
      error.value = null
    } catch (e) {
      error.value = e instanceof Error ? e.message : '获取状态失败'
    }
  }

  function startPolling(intervalMs = 5000) {
    stopPolling()
    loading.value = true
    fetch().finally(() => { loading.value = false })
    pollTimer = setInterval(fetch, intervalMs)
  }

  function stopPolling() {
    if (pollTimer) { clearInterval(pollTimer); pollTimer = null }
  }

  return {
    health: readonly(health),
    loading: readonly(loading),
    error: readonly(error),
    allOk, moduleList, formatUptime,
    startPolling, stopPolling,
  }
})
