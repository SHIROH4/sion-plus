<template>
  <div class="tools-view">
    <header class="page-header">
      <h1 class="page-title">工具系统</h1>
      <span class="tool-count">{{ store.tools.length }} 个工具</span>
    </header>

    <!-- Category Filter -->
    <div class="filter-bar">
      <button
        v-for="cat in store.categories"
        :key="cat"
        class="filter-btn"
        :class="{ active: store.filterCategory === cat }"
        @click="store.setFilter(cat)"
      >
        {{ store.categoryLabels[cat] }}
      </button>
    </div>

    <!-- Tool Cards -->
    <GlassCard
      padding="md"
      radius="lg"
      :loading="store.loading"
      :empty="!store.loading && store.filteredTools.length === 0"
      empty-icon="🔧"
      empty-text="暂无已注册的工具"
      :error="store.error"
      @retry="store.fetch()"
    >
      <div class="tools-list">
        <div
          v-for="tool in store.filteredTools"
          :key="tool.name"
          class="tool-card"
          :class="{ dangerous: tool.dangerous }"
          @click="toggleExpand(tool.name)"
        >
          <div class="tool-header">
            <div class="tool-left">
              <span class="tool-icon">{{ toolIcon(tool.name) }}</span>
              <div>
                <span class="tool-name">{{ tool.name }}</span>
                <span v-if="tool.dangerous" class="danger-badge">⚠️ 危险</span>
              </div>
            </div>
            <span class="expand-arrow">{{ expanded.has(tool.name) ? '▾' : '▸' }}</span>
          </div>
          <p class="tool-desc">{{ tool.description || '暂无描述' }}</p>
          <!-- Expandable Parameters -->
          <div v-if="expanded.has(tool.name)" class="tool-params">
            <div class="params-title">参数</div>
            <div
              v-for="(schema, key) in (tool.parameters?.properties || {}) as Record<string, Record<string, unknown>>"
              :key="key"
              class="param-item"
            >
              <code class="param-name">{{ key }}</code>
              <span class="param-type">{{ schema.type }}</span>
              <span v-if="isRequired(tool.parameters?.required, key)" class="param-required">必填</span>
              <span v-if="schema.description" class="param-desc">{{ schema.description }}</span>
            </div>
          </div>
        </div>
      </div>
    </GlassCard>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import GlassCard from '../components/GlassCard.vue'
import { useToolsStore } from '../stores/tools'

const store = useToolsStore()
const expanded = ref(new Set<string>())

function toggleExpand(name: string) {
  if (expanded.value.has(name)) expanded.value.delete(name)
  else expanded.value.add(name)
}

function toolIcon(name: string): string {
  const m: Record<string, string> = {
    web_search: '🌐', exec_command: '💻', read_file: '📖',
    write_file: '✏️', edit_file: '📝', computer_use: '🖥', browser: '🌍',
  }
  return m[name] || '🔧'
}

function isRequired(required: unknown, key: string): boolean {
  if (!Array.isArray(required)) return false
  return required.includes(key)
}

onMounted(() => store.fetch())
</script>

<style scoped>
.tools-view {
  display: flex;
  flex-direction: column;
  height: calc(100vh - var(--titlebar-height) - var(--page-padding) * 2);
  overflow-y: auto;
  gap: 16px;
}

.page-header {
  display: flex; align-items: center; justify-content: space-between;
  flex-shrink: 0;
}
.page-title { font-size: var(--text-xl); font-weight: 600; }
.tool-count { font-size: var(--text-sm); color: var(--text-tertiary); }

.filter-bar { display: flex; gap: 6px; flex-shrink: 0; }
.filter-btn {
  padding: 5px 14px; border: 1px solid rgba(0,0,0,0.06); border-radius: var(--radius-full);
  background: rgba(255,255,255,0.5); color: var(--text-secondary);
  font-size: var(--text-xs); font-family: inherit; cursor: pointer;
  transition: all var(--duration-fast);
}
.filter-btn.active {
  background: var(--color-accent); color: white; border-color: transparent;
}

.tools-list { display: flex; flex-direction: column; gap: 8px; }
.tool-card {
  padding: 16px; border-radius: var(--radius-md);
  background: rgba(255,255,255,0.6); backdrop-filter: blur(6px);
  cursor: pointer; transition: all var(--duration-fast);
}
.tool-card:hover { background: rgba(255,255,255,0.8); }
.tool-card.dangerous { border-left: 3px solid var(--color-danger); }

.tool-header {
  display: flex; align-items: center; justify-content: space-between;
  margin-bottom: 6px;
}
.tool-left { display: flex; align-items: center; gap: 8px; }
.tool-icon { font-size: 18px; }
.tool-name { font-size: var(--text-sm); font-weight: 600; color: var(--text-primary); }

.danger-badge {
  font-size: 10px; padding: 1px 6px; border-radius: var(--radius-sm);
  background: rgba(239,68,68,0.1); color: var(--color-danger);
  margin-left: 6px; font-weight: 500;
}

.expand-arrow { color: var(--text-tertiary); font-size: 12px; }
.tool-desc { font-size: var(--text-xs); color: var(--text-secondary); margin: 0 0 0 26px; }

.tool-params {
  margin: 10px 0 0 26px; padding: 10px; border-radius: var(--radius-sm);
  background: rgba(0,0,0,0.03);
}
.params-title { font-size: 11px; color: var(--text-tertiary); margin-bottom: 6px; text-transform: uppercase; letter-spacing: 0.5px; }
.param-item { margin-bottom: 6px; font-size: 12px; }
.param-name { font-family: var(--font-mono); font-size: 11px; background: rgba(0,0,0,0.04); padding: 1px 4px; border-radius: 3px; }
.param-type { color: var(--text-tertiary); margin: 0 6px; font-size: 11px; }
.param-required { color: var(--color-danger); font-size: 10px; font-weight: 500; }
.param-desc { color: var(--text-secondary); display: block; margin-top: 2px; font-size: 11px; }
</style>
