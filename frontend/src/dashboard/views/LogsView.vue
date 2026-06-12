<template>
  <div class="logs-view">
    <header class="page-header">
      <h1 class="page-title">运行日志</h1>
      <div class="log-controls">
        <n-select
          :value="filterLevel"
          @update:value="(v: string) => filterLevel = v"
          :options="levelOptions"
          size="small"
          style="width: 100px"
        />
        <n-button size="small" @click="store.clear()">清空</n-button>
      </div>
    </header>

    <GlassCard
      padding="md"
      radius="lg"
      :loading="store.loading"
      :empty="filteredLogs.length === 0"
      empty-icon="📋"
      :empty-text="store.loading ? '加载中...' : '暂无日志'"
      :error="store.error"
    >
      <div class="logs-list">
        <div
          v-for="(log, i) in filteredLogsReversed"
          :key="i"
          class="log-row"
        >
          <span class="log-time">{{ formatTime(log.time) }}</span>
          <span class="log-level" :class="`level-${log.level}`">{{ levelLabel(log.level) }}</span>
          <span class="log-source">{{ log.source }}</span>
          <span class="log-message" :title="log.message">{{ log.message }}</span>
        </div>
      </div>
    </GlassCard>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import GlassCard from '../components/GlassCard.vue'
import { useLogsStore } from '../stores/logs'

const store = useLogsStore()
const filterLevel = ref('info')

const levelOptions = [
  { label: 'Debug', value: 'debug' },
  { label: 'Info', value: 'info' },
  { label: 'Warn', value: 'warn' },
  { label: 'Error', value: 'error' },
]

const levelOrder: Record<string, number> = { debug: 0, info: 1, warn: 2, error: 3 }

const levelLabel = (l: string) => ({ debug: 'DEBUG', info: 'INFO', warn: 'WARN', error: 'ERROR' }[l] || l)

const filteredLogs = computed(() => {
  const min = levelOrder[filterLevel.value] ?? 0
  return store.logs.filter(l => (levelOrder[l.level] ?? 0) >= min)
})

const filteredLogsReversed = computed(() => [...filteredLogs.value].reverse())

function formatTime(ts: number): string {
  return new Date(ts).toLocaleTimeString()
}

onMounted(() => store.startPolling(3000))
onUnmounted(() => store.stopPolling())
</script>

<style scoped>
.logs-view {
  display: flex;
  flex-direction: column;
  height: calc(100vh - var(--titlebar-height) - var(--page-padding) * 2);
  overflow-y: auto;
  gap: 16px;
}

.page-header { display: flex; align-items: center; justify-content: space-between; flex-shrink: 0; }
.page-title { font-size: var(--text-xl); font-weight: 600; }

.log-controls { display: flex; gap: 8px; align-items: center; }

.logs-list {
  flex: 1; overflow-y: auto;
  font-family: var(--font-mono); font-size: 12px;
}
.log-row {
  display: flex; gap: 10px; padding: 3px 0;
  border-bottom: 1px solid rgba(0,0,0,0.015);
}
.log-row:hover { background: rgba(0,0,0,0.015); }

.log-time { color: var(--text-tertiary); flex-shrink: 0; width: 72px; font-size: 11px; }
.log-level { width: 44px; font-weight: 600; flex-shrink: 0; font-size: 10px; }
.level-debug { color: #8b949e; }
.level-info  { color: #3b82f6; }
.level-warn  { color: #f59e0b; }
.level-error { color: #ef4444; }

.log-source { color: var(--text-secondary); width: 100px; flex-shrink: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 11px; }
.log-message { color: var(--text-primary); flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
