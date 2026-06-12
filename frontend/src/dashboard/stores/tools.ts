import { defineStore } from 'pinia'
import { ref, readonly, computed } from 'vue'
import type { ToolInfo } from '@/shared/types'
import { getTools } from '@/shared/api'

export const useToolsStore = defineStore('tools', () => {
  const tools = ref<ToolInfo[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)
  const filterCategory = ref('')

  const categories = ['', 'file', 'system', 'network', 'browser'] as const
  const categoryLabels: Record<string, string> = {
    '': '全部', file: '文件', system: '系统', network: '网络', browser: '浏览器',
  }

  function toolCategory(t: ToolInfo): string {
    const n = t.name
    if (n.includes('file') || n.includes('read') || n.includes('write') || n.includes('edit')) return 'file'
    if (n.includes('command') || n.includes('exec') || n.includes('bash')) return 'system'
    if (n.includes('search') || n.includes('web')) return 'network'
    if (n.includes('browser') || n.includes('computer')) return 'browser'
    return 'system'
  }

  const filteredTools = computed(() => {
    if (!filterCategory.value) return tools.value
    return tools.value.filter(t => toolCategory(t) === filterCategory.value)
  })

  async function fetch() {
    loading.value = true
    error.value = null
    try {
      tools.value = await getTools()
    } catch (e) {
      error.value = e instanceof Error ? e.message : '加载失败'
    } finally {
      loading.value = false
    }
  }

  function setFilter(cat: string) { filterCategory.value = cat }

  return {
    tools: readonly(tools),
    filteredTools: readonly(filteredTools),
    loading: readonly(loading),
    error: readonly(error),
    filterCategory: readonly(filterCategory),
    categories,
    categoryLabels,
    toolCategory,
    fetch, setFilter,
  }
})
