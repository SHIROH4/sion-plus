<template>
  <div class="llm-view">
    <header class="page-header">
      <h1 class="page-title">LLM 配置</h1>
      <n-button
        v-if="store.isDirty"
        size="small"
        :style="{ '--n-color': '#f778ba', '--n-color-hover': '#f960ae', '--n-text-color': 'white' }"
        @click="store.save()"
        :loading="store.loading"
      >
        保存并重载
      </n-button>
    </header>

    <!-- Routes -->
    <GlassCard padding="md" radius="lg" :loading="store.loading" :error="store.error" @retry="store.fetch()">
      <label class="section-label">模型路由</label>
      <div class="routes-grid">
        <div v-for="(label, key) in store.routeLabels" :key="key" class="route-item">
          <span class="route-label">{{ label }}</span>
          <n-select
            :value="store.routes[key as keyof typeof store.routes]"
            @update:value="(v: string) => { (store.routes as Record<string, string>)[key as string] = v; store.markDirty() }"
            :options="routeOptions"
            size="small"
            style="width: 140px"
          />
        </div>
      </div>
    </GlassCard>

    <!-- Providers -->
    <div v-for="(p, idx) in store.providers" :key="idx">
      <GlassCard padding="md" radius="lg">
        <div class="provider-header">
          <div class="provider-title">
            <n-switch
              :value="p.enabled"
              @update:value="(v: boolean) => { p.enabled = v; store.markDirty() }"
              size="small"
              :style="{ '--n-rail-color-active': '#f778ba' }"
            />
            <n-input
              :value="p.name"
              @update:value="(v: string) => { p.name = v; store.markDirty() }"
              placeholder="提供者名称"
              size="small"
              style="width: 120px"
            />
          </div>
          <n-button text size="tiny" type="error" @click="store.removeProvider(idx)">删除</n-button>
        </div>
        <div class="provider-fields">
          <div class="field-row">
            <label>URL</label>
            <n-input :value="p.base_url" @update:value="(v: string) => { p.base_url = v; store.markDirty() }" placeholder="https://api.openai.com" size="small" style="flex:1" />
          </div>
          <div class="field-row">
            <label>Key</label>
            <n-input type="password" :value="p.api_key" @update:value="(v: string) => { p.api_key = v; store.markDirty() }" placeholder="sk-..." size="small" style="flex:1" />
          </div>
          <div class="field-row">
            <label>模型</label>
            <n-input :value="p.chat_model" @update:value="(v: string) => { p.chat_model = v; store.markDirty() }" placeholder="deepseek-chat" size="small" style="width: 160px" />
            <label>优先级</label>
            <n-input-number :value="p.priority" @update:value="(v: number | null) => { p.priority = v || 1; store.markDirty() }" :min="1" :max="10" size="small" style="width: 80px" />
            <label>重试</label>
            <n-input-number :value="p.max_retries" @update:value="(v: number | null) => { p.max_retries = v || 0; store.markDirty() }" :min="0" :max="10" size="small" style="width: 70px" />
            <label>超时</label>
            <n-input-number :value="p.timeout_sec" @update:value="(v: number | null) => { p.timeout_sec = v || 30; store.markDirty() }" :min="5" :max="300" size="small" style="width: 80px" />
          </div>
        </div>
      </GlassCard>
    </div>

    <n-button dashed @click="store.addProvider()" style="align-self:center">+ 添加提供者</n-button>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import GlassCard from '../components/GlassCard.vue'
import { useLLMConfigStore } from '../stores/llmConfig'

const store = useLLMConfigStore()

const routeOptions = computed(() => {
  const names = store.providerNames()
  return names.map(n => ({ label: n, value: n }))
})

onMounted(() => store.fetch())
</script>

<style scoped>
.llm-view {
  display: flex;
  flex-direction: column;
  height: calc(100vh - var(--titlebar-height) - var(--page-padding) * 2);
  overflow-y: auto;
  gap: 16px;
}
.page-header { display: flex; align-items: center; justify-content: space-between; flex-shrink: 0; }
.page-title { font-size: var(--text-xl); font-weight: 600; }
.section-label { display: block; font-size: var(--text-sm); font-weight: 500; color: var(--text-secondary); margin-bottom: 10px; }
.routes-grid { display: flex; flex-wrap: wrap; gap: 12px; }
.route-item { display: flex; align-items: center; gap: 8px; }
.route-label { font-size: var(--text-sm); color: var(--text-primary); width: 36px; }
.provider-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 12px; }
.provider-title { display: flex; align-items: center; gap: 8px; }
.provider-fields { display: flex; flex-direction: column; gap: 8px; }
.field-row { display: flex; align-items: center; gap: 8px; }
.field-row label { font-size: var(--text-xs); color: var(--text-tertiary); width: 36px; flex-shrink: 0; }
</style>
