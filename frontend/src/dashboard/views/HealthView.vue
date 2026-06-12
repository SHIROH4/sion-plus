<template>
  <div class="health-view">
    <header class="page-header">
      <h1 class="page-title">系统健康</h1>
    </header>

    <!-- Overall Status -->
    <GlassCard padding="md" radius="lg">
      <div class="overall-status" :class="{ degraded: !store.allOk }">
        <span class="status-dot" />
        <span class="status-text">
          {{ store.allOk ? '全部模块运行正常' : '部分模块异常' }}
        </span>
        <span class="status-count">{{ store.moduleList.length }} 个模块</span>
      </div>
    </GlassCard>

    <!-- System Stats -->
    <div class="stats-row">
      <div class="stat-card">
        <span class="stat-val">{{ store.health.cpu_cores }}</span>
        <span class="stat-lbl">CPU 核心</span>
      </div>
      <div class="stat-card">
        <span class="stat-val">{{ store.health.mem_used_mb }}MB</span>
        <span class="stat-lbl">内存使用</span>
      </div>
      <div class="stat-card">
        <span class="stat-val">{{ store.health.goroutines }}</span>
        <span class="stat-lbl">协程数</span>
      </div>
      <div class="stat-card">
        <span class="stat-val">{{ store.formatUptime(store.health.uptime_sec) }}</span>
        <span class="stat-lbl">运行时长</span>
      </div>
    </div>

    <!-- Module Cards -->
    <GlassCard
      padding="md"
      radius="lg"
      :loading="store.loading"
      :empty="store.moduleList.length === 0"
      empty-icon="❤️"
      empty-text="暂无模块数据"
      :error="store.error"
    >
      <div class="modules-grid">
        <div
          v-for="mod in store.moduleList"
          :key="mod.name"
          class="module-card"
          :class="{ error: !mod.ok }"
        >
          <span class="module-icon">{{ mod.icon }}</span>
          <div class="module-info">
            <span class="module-name">{{ mod.label }}</span>
            <span class="module-status" :class="mod.ok ? 'ok' : 'err'">
              {{ mod.ok ? '🟢 正常' : '🔴 异常' }}
            </span>
          </div>
          <span v-if="mod.message" class="module-msg">{{ mod.message }}</span>
        </div>
      </div>
    </GlassCard>
  </div>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import GlassCard from '../components/GlassCard.vue'
import { useHealthStore } from '../stores/health'

const store = useHealthStore()

onMounted(() => store.startPolling(5000))
onUnmounted(() => store.stopPolling())
</script>

<style scoped>
.health-view {
  display: flex;
  flex-direction: column;
  height: calc(100vh - var(--titlebar-height) - var(--page-padding) * 2);
  overflow-y: auto;
  gap: 16px;
}

.page-header { display: flex; align-items: center; justify-content: space-between; flex-shrink: 0; }
.page-title { font-size: var(--text-xl); font-weight: 600; }

/* Overall */
.overall-status {
  display: flex; align-items: center; gap: 12px; padding: 8px 0;
}
.status-dot {
  width: 14px; height: 14px; border-radius: 50%; background: var(--color-success);
  box-shadow: 0 0 8px rgba(52,211,153,0.4);
  animation: pulse-glow 2s infinite;
}
.degraded .status-dot { background: var(--color-danger); box-shadow: 0 0 8px rgba(239,68,68,0.4); }
.status-text { font-size: var(--text-base); font-weight: 500; color: var(--text-primary); }
.status-count { font-size: var(--text-xs); color: var(--text-tertiary); }

/* Stats row */
.stats-row { display: flex; gap: 12px; flex-shrink: 0; }
.stat-card {
  flex: 1; display: flex; flex-direction: column; align-items: center;
  padding: 12px 8px; border-radius: var(--radius-md);
  background: rgba(255,255,255,0.6); backdrop-filter: blur(8px);
}
.stat-val { font-size: var(--text-lg); font-weight: 700; color: var(--text-primary); }
.stat-lbl { font-size: 11px; color: var(--text-tertiary); margin-top: 2px; }

/* Module Cards */
.modules-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(180px, 1fr)); gap: 12px; }
.module-card {
  padding: 16px; border-radius: var(--radius-md);
  background: rgba(255,255,255,0.6); backdrop-filter: blur(6px);
  display: flex; flex-direction: column; gap: 8px;
}
.module-card.error { border: 1px solid rgba(239,68,68,0.2); }
.module-icon { font-size: 28px; }
.module-info { display: flex; align-items: center; justify-content: space-between; }
.module-name { font-size: var(--text-sm); font-weight: 600; color: var(--text-primary); }
.module-status { font-size: var(--text-xs); }
.module-status.ok { color: var(--color-success); }
.module-status.err { color: var(--color-danger); }
.module-msg { font-size: 11px; color: var(--color-danger); word-break: break-all; }
</style>
