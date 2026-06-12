<template>
  <nav class="sidebar glass-sidebar" aria-label="Main navigation">
    <div class="sidebar-items">
      <button
        v-for="item in navItems"
        :key="item.key"
        class="sidebar-item"
        :class="{ active: currentKey === item.key }"
        @click="navigate(item.key)"
        :aria-label="item.label"
        :aria-current="currentKey === item.key ? 'page' : undefined"
        :title="item.label"
      >
        <span class="sidebar-icon">{{ item.icon }}</span>
        <span class="sidebar-label">{{ item.label }}</span>
        <span v-if="currentKey === item.key" class="sidebar-indicator" />
      </button>
    </div>
  </nav>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { NAV_ITEMS } from '@/shared/constants'
import type { DashboardPage } from '@/shared/types'

const router = useRouter()
const route = useRoute()
const navItems = NAV_ITEMS

const currentKey = computed<DashboardPage>(() => {
  const name = route.name as string
  return (name || 'chat') as DashboardPage
})

function navigate(key: DashboardPage) {
  router.push({ name: key })
}
</script>

<style scoped>
.sidebar {
  display: flex;
  flex-direction: column;
  width: var(--sidebar-width);
  height: calc(100vh - var(--titlebar-height));
  padding: var(--page-padding) 8px;
  flex-shrink: 0;
}

.sidebar-items {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.sidebar-item {
  position: relative;
  display: flex;
  align-items: center;
  gap: 10px;
  height: var(--nav-item-height);
  padding: 0 12px;
  border: none;
  border-radius: var(--radius-md);
  background: transparent;
  color: var(--text-secondary);
  font-size: var(--text-sm);
  font-family: inherit;
  cursor: pointer;
  transition:
    background var(--duration-normal) var(--easing-standard),
    color var(--duration-normal) var(--easing-standard);
  overflow: hidden;
}

.sidebar-item:hover {
  background: var(--glass-hover);
  color: var(--text-primary);
}

.sidebar-item.active {
  color: var(--color-accent);
  background: var(--glass-active);
  font-weight: 500;
}

.sidebar-indicator {
  position: absolute;
  left: 0;
  top: 50%;
  transform: translateY(-50%);
  width: 3px;
  height: 20px;
  border-radius: 0 3px 3px 0;
  background: var(--color-accent);
}

.sidebar-icon {
  font-size: 18px;
  line-height: 1;
  flex-shrink: 0;
}

.sidebar-label {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>
