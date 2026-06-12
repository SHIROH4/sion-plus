import { defineStore } from 'pinia'
import { ref, readonly } from 'vue'
import type { LLMProviderConfig, LLMRoutes, LLMFullConfig } from '@/shared/types'
import { getLLMFullConfig, saveLLMFullConfig } from '@/shared/api'

export const useLLMConfigStore = defineStore('llmConfig', () => {
  const providers = ref<LLMProviderConfig[]>([])
  const routes = ref<LLMRoutes>({
    default: '', chat: '', emotion: '', memory: '', vision: '', summary: '', signal: '', search: '',
  })
  const loading = ref(false)
  const error = ref<string | null>(null)
  const isDirty = ref(false)

  const routeLabels: Record<string, string> = {
    default: '默认', chat: '对话', emotion: '情绪', memory: '记忆',
    vision: '视觉', summary: '总结', signal: '信号', search: '搜索',
  }

  async function fetch() {
    loading.value = true
    error.value = null
    try {
      const data = await getLLMFullConfig()
      providers.value = data.providers || []
      routes.value = data.routes || { default: '', chat: '', emotion: '', memory: '', vision: '', summary: '', signal: '', search: '' }
      isDirty.value = false
    } catch (e) {
      error.value = e instanceof Error ? e.message : '加载失败'
    } finally {
      loading.value = false
    }
  }

  function markDirty() { isDirty.value = true }

  function addProvider() {
    providers.value.push({
      name: '', base_url: '', api_key: '', chat_model: '',
      enabled: true, priority: providers.value.length + 1,
      max_retries: 2, timeout_sec: 60,
    })
    markDirty()
  }

  function removeProvider(idx: number) {
    providers.value.splice(idx, 1)
    markDirty()
  }

  function providerNames() {
    return providers.value.filter(p => p.name).map(p => p.name)
  }

  async function save() {
    loading.value = true
    error.value = null
    try {
      const cfg: LLMFullConfig = {
        providers: [...providers.value],
        routes: { ...routes.value },
      }
      await saveLLMFullConfig(cfg)
      isDirty.value = false
    } catch (e) {
      error.value = e instanceof Error ? e.message : '保存失败'
    } finally {
      loading.value = false
    }
  }

  return {
    providers,
    routes,
    loading: readonly(loading),
    error: readonly(error),
    isDirty: readonly(isDirty),
    routeLabels,
    fetch, markDirty, addProvider, removeProvider, providerNames, save,
  }
})
