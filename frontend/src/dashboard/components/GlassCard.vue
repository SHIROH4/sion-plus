<template>
  <div class="glass-card-wrapper" :class="[`padding-${padding}`, `radius-${radius}`]">
    <!-- Loading -->
    <template v-if="loading">
      <n-skeleton :text="true" :repeat="3" />
      <n-skeleton :text="true" style="width: 60%" />
    </template>

    <!-- Empty -->
    <template v-else-if="empty">
      <div class="empty-state">
        <span class="empty-icon">{{ emptyIcon }}</span>
        <p class="empty-text">{{ emptyText }}</p>
        <n-button
          v-if="emptyActionLabel"
          size="small"
          :style="{ '--n-color': '#f778ba', '--n-color-hover': '#f960ae', '--n-text-color': 'white' }"
          @click="$emit('emptyAction')"
        >
          {{ emptyActionLabel }}
        </n-button>
      </div>
    </template>

    <!-- Error -->
    <template v-else-if="error">
      <div class="error-state">
        <div class="error-header">
          <span class="error-icon">⚠️</span>
          <span class="error-text">{{ error }}</span>
        </div>
        <n-button
          v-if="retryable"
          size="small"
          type="error"
          @click="$emit('retry')"
        >
          Retry
        </n-button>
      </div>
    </template>

    <!-- Content -->
    <template v-else>
      <slot />
    </template>
  </div>
</template>

<script setup lang="ts">
withDefaults(defineProps<{
  padding?: 'sm' | 'md' | 'lg'
  radius?: 'md' | 'lg' | 'xl'
  loading?: boolean
  empty?: boolean
  emptyIcon?: string
  emptyText?: string
  emptyActionLabel?: string
  error?: string | null
  retryable?: boolean
}>(), {
  padding: 'md',
  radius: 'lg',
  loading: false,
  empty: false,
  emptyIcon: '📭',
  emptyText: 'Nothing here yet',
  error: null,
  retryable: true,
})

defineEmits<{
  retry: []
  emptyAction: []
}>()
</script>

<style scoped>
.glass-card-wrapper {
  background: var(--glass-card);
  backdrop-filter: blur(var(--blur-heavy)) saturate(1.2);
  -webkit-backdrop-filter: blur(var(--blur-heavy)) saturate(1.2);
  box-shadow:
    var(--shadow-card),
    inset 0 0.5px 1px rgba(255, 255, 255, 0.4);
}

.padding-sm { padding: 12px; }
.padding-md { padding: var(--card-padding); }
.padding-lg { padding: 32px; }

.radius-md { border-radius: var(--radius-md); }
.radius-lg { border-radius: var(--radius-lg); }
.radius-xl { border-radius: var(--radius-xl); }

/* Empty state */
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  padding: 32px 16px;
}

.empty-icon {
  font-size: 48px;
  opacity: 0.4;
}

.empty-text {
  font-size: var(--text-sm);
  color: var(--text-tertiary);
}

/* Error state */
.error-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  padding: 24px;
}

.error-header {
  display: flex;
  align-items: center;
  gap: 8px;
}

.error-icon {
  font-size: 18px;
}

.error-text {
  font-size: var(--text-sm);
  color: var(--color-danger);
}
</style>
