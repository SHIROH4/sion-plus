<template>
  <div class="memory-view">
    <header class="page-header">
      <h1 class="page-title">记忆系统</h1>
    </header>

    <!-- Stats Bar -->
    <div class="stats-bar">
      <div class="stat-item">
        <span class="stat-value">{{ store.stats.total }}</span>
        <span class="stat-label">总事实</span>
      </div>
      <div class="stat-item confirmed">
        <span class="stat-value">{{ store.stats.confirmed }}</span>
        <span class="stat-label">已确认</span>
      </div>
      <div class="stat-item pending">
        <span class="stat-value">{{ store.stats.pending }}</span>
        <span class="stat-label">待确认</span>
      </div>
      <div class="stat-item">
        <span class="stat-value">{{ store.topics.length }}</span>
        <span class="stat-label">话题</span>
      </div>
    </div>

    <!-- Filters -->
    <div class="filter-bar">
      <n-select
        :value="store.filterEntity"
        @update:value="(v: string) => store.setFilter(v, store.filterSource, store.filterType)"
        :options="entityOptions"
        placeholder="全部实体"
        size="small"
        style="width: 120px"
      />
      <n-select
        :value="store.filterSource"
        @update:value="(v: string) => store.setFilter(store.filterEntity, v, store.filterType)"
        :options="sourceOptions"
        placeholder="全部来源"
        size="small"
        style="width: 120px"
      />
      <n-select
        :value="store.filterType"
        @update:value="(v: string) => store.setFilter(store.filterEntity, store.filterSource, v)"
        :options="typeOptions"
        placeholder="全部类型"
        size="small"
        style="width: 120px"
      />
      <div class="filter-spacer" />
      <n-button size="small" @click="store.fetchAll(true)" :loading="store.loading">
        刷新
      </n-button>
    </div>

    <!-- Facts Grid -->
    <GlassCard
      padding="md"
      radius="lg"
      :loading="store.loading"
      :empty="!store.loading && store.facts.length === 0"
      empty-icon="🧠"
      empty-text="暂无记忆。多和 Sion 聊天，她会记住关于你的事。"
      :error="store.error"
      @retry="store.fetchAll(true)"
    >
      <div class="facts-grid">
        <div
          v-for="fact in store.facts"
          :key="fact.id"
          class="fact-card"
          :class="{ 'low-confidence': fact.evidence.reinforcement - fact.evidence.disputation < 0.3 }"
        >
          <div class="fact-header">
            <span class="entity-badge" :class="`entity-${fact.entity}`">
              {{ entityLabel(fact.entity) }}
            </span>
            <span class="source-badge" :class="`source-${fact.source_tier}`">
              {{ sourceLabel(fact.source_tier) }}
            </span>
          </div>
          <p class="fact-content">{{ fact.content }}</p>
          <div class="fact-meta">
            <span class="meta-item">{{ scopeIcon(fact.temporal_scope) }} {{ scopeLabel(fact.temporal_scope) }}</span>
            <span class="meta-item importance">{{ '★'.repeat(fact.importance) }}</span>
            <span class="meta-item type-tag">{{ typeLabel(fact.memcell_type) }}</span>
          </div>
          <div class="evidence-bar">
            <div
              class="evidence-fill"
              :style="{ width: `${Math.max(0, Math.min(100, (fact.evidence.reinforcement - fact.evidence.disputation) * 100))}%` }"
            />
            <span class="evidence-text">{{ ((fact.evidence.reinforcement - fact.evidence.disputation) * 100).toFixed(0) }}%</span>
          </div>
        </div>
      </div>
    </GlassCard>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import GlassCard from '../components/GlassCard.vue'
import { useMemoryStore } from '../stores/memory'

const store = useMemoryStore()

const entityOptions = [
  { label: '全部实体', value: '' },
  { label: '👤 用户', value: 'master' },
  { label: '🐱 猫娘', value: 'neko' },
  { label: '💞 关系', value: 'relationship' },
]

const sourceOptions = [
  { label: '全部来源', value: '' },
  { label: '💬 亲述', value: 'explicit' },
  { label: '👁 观察', value: 'observed' },
  { label: '🧠 推论', value: 'inferred' },
]

const typeOptions = [
  { label: '全部类型', value: '' },
  { label: '事实', value: 'fact' },
  { label: '偏好', value: 'prefer' },
  { label: '事件', value: 'event' },
  { label: '情绪', value: 'emotion' },
  { label: '技能', value: 'skill' },
  { label: '关系', value: 'relation' },
]

function entityLabel(e: string) {
  const m: Record<string, string> = { master: '👤 用户', neko: '🐱 猫娘', relationship: '💞 关系' }
  return m[e] || e
}
function sourceLabel(s: string) {
  const m: Record<string, string> = { explicit: '亲述', observed: '观察', inferred: '推论' }
  return m[s] || s
}
function scopeLabel(s: string) {
  const m: Record<string, string> = { pattern: '持续', state: '当前', episode: '一次' }
  return m[s] || s
}
function scopeIcon(s: string) {
  const m: Record<string, string> = { pattern: '🔁', state: '📍', episode: '📌' }
  return m[s] || ''
}
function typeLabel(t: string) {
  const m: Record<string, string> = { fact: '事实', prefer: '偏好', event: '事件', emotion: '情绪', skill: '技能', relation: '关系' }
  return m[t] || t
}

onMounted(() => store.fetchAll(true))
</script>

<style scoped>
.memory-view {
  display: flex;
  flex-direction: column;
  height: calc(100vh - var(--titlebar-height) - var(--page-padding) * 2);
  overflow-y: auto;
  gap: 16px;
}

.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-shrink: 0;
}
.page-title { font-size: var(--text-xl); font-weight: 600; }

.stats-bar { display: flex; gap: 12px; flex-shrink: 0; }

.stat-item {
  flex: 1; display: flex; flex-direction: column; align-items: center;
  padding: 12px 8px; border-radius: var(--radius-md);
  background: rgba(255,255,255,0.6); backdrop-filter: blur(8px);
}
.stat-value { font-size: var(--text-xl); font-weight: 700; color: var(--text-primary); }
.stat-label { font-size: 11px; color: var(--text-tertiary); margin-top: 2px; }
.stat-item.confirmed .stat-value { color: var(--color-success); }
.stat-item.pending .stat-value { color: var(--color-warning); }

.filter-bar { display: flex; gap: 8px; align-items: center; flex-shrink: 0; }
.filter-spacer { flex: 1; }

.facts-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); gap: 10px; }
.fact-card {
  padding: 14px; border-radius: var(--radius-md);
  background: rgba(255,255,255,0.65); backdrop-filter: blur(6px);
  display: flex; flex-direction: column; gap: 8px; transition: opacity 0.2s;
}
.fact-card.low-confidence { opacity: 0.55; }

.fact-header { display: flex; align-items: center; gap: 6px; }
.entity-badge { font-size: 11px; padding: 2px 8px; border-radius: var(--radius-sm); font-weight: 500; }
.entity-master { background: rgba(59,130,246,0.12); color: #3b82f6; }
.entity-neko { background: rgba(247,119,186,0.12); color: #f778ba; }
.entity-relationship { background: rgba(139,92,246,0.12); color: #8b5cf6; }

.source-badge { font-size: 10px; padding: 2px 6px; border-radius: var(--radius-sm); border: 1px solid; }
.source-explicit { color: var(--color-success); border-color: rgba(52,211,153,0.3); }
.source-observed { color: var(--color-info); border-color: rgba(59,130,246,0.3); }
.source-inferred { color: var(--text-tertiary); border-color: rgba(0,0,0,0.08); }

.fact-content { font-size: var(--text-sm); color: var(--text-primary); line-height: 1.55; }
.fact-meta { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; margin-top: auto; }
.meta-item { font-size: 11px; color: var(--text-tertiary); }
.meta-item.importance { color: #f59e0b; letter-spacing: 1px; }
.type-tag { padding: 1px 6px; border-radius: var(--radius-sm); background: rgba(0,0,0,0.04); }

.evidence-bar { height: 4px; border-radius: 2px; background: rgba(0,0,0,0.06); position: relative; }
.evidence-fill { height: 100%; border-radius: 2px; background: linear-gradient(90deg, var(--color-danger), var(--color-warning), var(--color-success)); transition: width 0.3s; }
.evidence-text { position: absolute; right: 0; top: -15px; font-size: 10px; color: var(--text-tertiary); }
</style>
