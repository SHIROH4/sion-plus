import { defineStore } from 'pinia'
import { ref, readonly } from 'vue'
import type { FactEntry, Topic, MemoryStats } from '@/shared/types'
import { getMemoryFacts, getMemoryTopics, getMemoryStats } from '@/shared/api'

export const useMemoryStore = defineStore('memory', () => {
  const facts = ref<FactEntry[]>([])
  const topics = ref<Topic[]>([])
  const stats = ref<MemoryStats>({ total: 0, confirmed: 0, pending: 0, by_entity: {}, by_source: {}, by_type: {} })
  const loading = ref(false)
  const error = ref<string | null>(null)
  const filterEntity = ref('')
  const filterSource = ref('')
  const filterType = ref('')
  let lastFetch = 0
  const CACHE_MS = 30000

  async function fetchFacts() {
    loading.value = true
    error.value = null
    try {
      const params: Record<string, string> = {}
      if (filterEntity.value) params.entity = filterEntity.value
      if (filterSource.value) params.source_tier = filterSource.value
      if (filterType.value) params.type = filterType.value
      const data = await getMemoryFacts(params)
      facts.value = data.facts
      lastFetch = Date.now()
    } catch (e) {
      error.value = e instanceof Error ? e.message : '加载失败'
    } finally {
      loading.value = false
    }
  }

  async function fetchTopics() {
    try {
      const data = await getMemoryTopics()
      topics.value = data.topics
    } catch { /* */ }
  }

  async function fetchStats() {
    try {
      stats.value = await getMemoryStats()
    } catch { /* */ }
  }

  async function fetchAll(force = false) {
    if (!force && Date.now() - lastFetch < CACHE_MS) return
    await Promise.all([fetchFacts(), fetchTopics(), fetchStats()])
  }

  function setFilter(entity: string, source: string, type: string) {
    filterEntity.value = entity
    filterSource.value = source
    filterType.value = type
    fetchFacts()
  }

  return {
    facts: readonly(facts),
    topics: readonly(topics),
    stats: readonly(stats),
    loading: readonly(loading),
    error: readonly(error),
    filterEntity: readonly(filterEntity),
    filterSource: readonly(filterSource),
    filterType: readonly(filterType),
    fetchAll, fetchFacts, setFilter,
  }
})
