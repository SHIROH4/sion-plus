import { defineStore } from 'pinia'
import { ref, readonly } from 'vue'
import type { ProactiveStatus, ProactiveAction, ProactiveDecision, ProactivePolicyEvaluation } from '@/shared/types'
import { getProactiveStatus, getProactiveActions, getProactiveDecisions, getProactiveEvaluation, setProactiveMode as setMode } from '@/shared/api'

export const useProactiveStore = defineStore('proactive', () => {
  const status = ref<ProactiveStatus>({ mode: 'normal', interval_sec: 60, last_action: '', last_tick: 0 })
  const actions = ref<ProactiveAction[]>([])
  const decisions = ref<ProactiveDecision[]>([])
  const evaluation = ref<ProactivePolicyEvaluation | null>(null)
  const loading = ref(false)
  const error = ref<string | null>(null)

  const modeLabels: Record<string, string> = {
    normal: '普通', frequent: '频繁', focus: '专注', off: '关闭',
  }

	const modeDesc: Record<string, string> = {
		normal: '标准决策节奏',
		frequent: '缩短决策间隔',
		focus: '延长间隔，减少打扰',
    off: '暂停主动搭话',
  }

  const categoryLabels: Record<string, string> = {
    social: '社交', care: '关怀', learning: '学习', none: '沉默',
  }

  const categoryIcons: Record<string, string> = {
    social: '💬', care: '💗', learning: '🧠', none: '🔇',
  }

  const outcomeLabels: Record<string, string> = {
    speak: '搭话', action: '行动', silent: '静默',
  }

  async function fetchStatus() {
    try {
      status.value = await getProactiveStatus()
    } catch { /* */ }
  }

  async function fetchActions() {
    try {
      actions.value = await getProactiveActions()
    } catch { /* */ }
  }

  async function fetchDecisions() {
    try { decisions.value = await getProactiveDecisions() } catch { /* */ }
  }

  async function fetchEvaluation() {
    try { evaluation.value = await getProactiveEvaluation() } catch { /* */ }
  }

  async function fetch() {
    loading.value = true
    error.value = null
    try {
      await Promise.all([fetchStatus(), fetchActions(), fetchDecisions(), fetchEvaluation()])
    } catch (e) {
      error.value = e instanceof Error ? e.message : '加载失败'
    } finally {
      loading.value = false
    }
  }

  async function switchMode(mode: string) {
    try {
      await setMode(mode)
      await fetchStatus()
    } catch { /* */ }
  }

  return {
    status: readonly(status), decisions: readonly(decisions), evaluation: readonly(evaluation),
    actions: readonly(actions),
    loading: readonly(loading),
    error: readonly(error),
    modeLabels, modeDesc, categoryLabels, categoryIcons, outcomeLabels,
    fetch, fetchStatus, fetchDecisions, switchMode,
  }
})
