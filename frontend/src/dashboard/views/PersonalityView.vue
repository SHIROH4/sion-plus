<template>
  <div class="page-view">
    <header class="page-header">
      <h1 class="page-title">人格配置</h1>
      <n-button
        v-if="store.isDirty"
        size="small"
        :style="{ '--n-color': '#f778ba', '--n-color-hover': '#f960ae', '--n-text-color': 'white' }"
        @click="store.save()"
      >
        保存
      </n-button>
    </header>

    <div class="personality-grid">
      <!-- System Prompt -->
      <GlassCard padding="md" radius="lg" :loading="store.loading" :error="store.error" @retry="store.fetch()">
        <label class="field-label">系统提示词</label>
        <n-input
          type="textarea"
          :value="store.config.system_prompt"
          @update:value="(v: string) => { store.config.system_prompt = v; store.markDirty() }"
          :rows="8"
          placeholder="You are Sion, a friendly AI catgirl companion..."
          :style="{ '--n-border': 'rgba(0,0,0,0.06)', '--n-border-focus': '#f778ba' }"
        />
      </GlassCard>

      <!-- Traits -->
      <GlassCard padding="md" radius="lg" :loading="store.loading" :error="store.error" @retry="store.fetch()">
        <label class="field-label">性格特质</label>
        <div class="traits-list">
          <div v-for="(value, trait) in store.config.traits" :key="trait" class="trait-row">
            <span class="trait-name">{{ store.traitLabels[trait] || trait }}</span>
            <n-slider
              :value="value"
              @update:value="(v: number) => { store.config.traits[trait] = v; store.markDirty() }"
              :min="0" :max="10" :step="1"
              :style="{ '--n-fill': '#f778ba', '--n-fill-hover': '#f960ae' }"
              style="flex: 1"
            />
            <span class="trait-value">{{ value }}</span>
          </div>
        </div>
      </GlassCard>

      <!-- 说话风格 & 背景故事 -->
      <GlassCard padding="md" radius="lg" :loading="store.loading" :error="store.error" @retry="store.fetch()">
        <div class="extra-fields">
          <div class="field-group">
            <label class="field-label">说话风格</label>
            <n-input
              :value="store.config.speaking_style"
              @update:value="(v: string) => { store.config.speaking_style = v; store.markDirty() }"
              placeholder="Casual, friendly, uses ~ and emoji..."
              :style="{ '--n-border': 'rgba(0,0,0,0.06)', '--n-border-focus': '#f778ba' }"
            />
          </div>
          <div class="field-group">
            <label class="field-label">背景故事</label>
            <n-input
              type="textarea"
              :value="store.config.background"
              @update:value="(v: string) => { store.config.background = v; store.markDirty() }"
              :rows="4"
              placeholder="Sion's backstory..."
              :style="{ '--n-border': 'rgba(0,0,0,0.06)', '--n-border-focus': '#f778ba' }"
            />
          </div>
        </div>
      </GlassCard>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import GlassCard from '../components/GlassCard.vue'
import { usePersonalityStore } from '../stores/personality'

const store = usePersonalityStore()

onMounted(() => store.fetch())
</script>

<style scoped>
.page-view {
  display: flex;
  flex-direction: column;
  min-height: 100%;
  gap: 20px;
}

.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.page-title {
  font-size: var(--text-xl);
  font-weight: 600;
}

.personality-grid {
  display: flex;
  flex-direction: column;
  gap: var(--card-gap);
}

.field-label {
  display: block;
  font-size: var(--text-sm);
  font-weight: 500;
  color: var(--text-secondary);
  margin-bottom: 10px;
}

.traits-list {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.trait-row {
  display: flex;
  align-items: center;
  gap: 14px;
}

.trait-name {
  width: 100px;
  font-size: var(--text-sm);
  color: var(--text-primary);
  text-transform: capitalize;
}

.trait-value {
  width: 24px;
  text-align: right;
  font-size: var(--text-sm);
  font-weight: 500;
  color: var(--color-accent);
}

.extra-fields {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.field-group {
  display: flex;
  flex-direction: column;
}
</style>
