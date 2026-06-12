import { defineStore } from 'pinia'
import { ref, readonly } from 'vue'
import type { PersonalityConfig } from '@/shared/types'
import { getPersonalityConfig, savePersonalityConfig } from '@/shared/api'

export const usePersonalityStore = defineStore('personality', () => {
  const config = ref<PersonalityConfig>({
    name: 'Sion',
    system_prompt: '',
    traits: { warmth: 8, playfulness: 7, formality: 3, curiosity: 6, empathy: 8 },
    speaking_style: '',
    background: '',
  })
  const loading = ref(false)
  const error = ref<string | null>(null)
  const isDirty = ref(false)

  const traitLabels: Record<string, string> = {
    warmth: '温暖度', playfulness: '贪玩度', formality: '正式度', curiosity: '好奇心', empathy: '共情力',
  }

  async function fetch() {
    loading.value = true
    error.value = null
    try {
      const data = await getPersonalityConfig()
      Object.assign(config.value, data)
    } catch (e) {
      error.value = e instanceof Error ? e.message : '加载失败'
    } finally {
      loading.value = false
    }
  }

  function markDirty() { isDirty.value = true }

  async function save() {
    loading.value = true
    error.value = null
    try {
      await savePersonalityConfig({ ...config.value })
      isDirty.value = false
    } catch (e) {
      error.value = e instanceof Error ? e.message : '保存失败'
    } finally {
      loading.value = false
    }
  }

  return {
    config,
    loading: readonly(loading),
    error: readonly(error),
    isDirty: readonly(isDirty),
    traitLabels,
    fetch, markDirty, save,
  }
})
