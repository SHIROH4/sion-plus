<template>
  <header class="titlebar glass-titlebar">
    <div class="titlebar-spacer" />
    <span class="titlebar-title">Sion</span>
  </header>
</template>

<script setup lang="ts">
// Window controls handled natively by Electron (hiddenInset titlebar)
</script>

<style scoped>
.titlebar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: var(--titlebar-height);
  padding: 0 12px;
  flex-shrink: 0;
  -webkit-app-region: drag;
  user-select: none;
}

.titlebar-spacer {
  flex: 1;
}

.titlebar-title {
  font-size: var(--text-sm);
  font-weight: 500;
  color: var(--text-secondary);
}

.titlebar-right {
  display: flex;
  align-items: center;
  gap: 4px;
  -webkit-app-region: no-drag;
}

.titlebar-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--text-secondary);
  cursor: pointer;
  transition: background var(--duration-fast) var(--easing-standard);
}

.titlebar-btn:hover {
  background: var(--glass-hover);
  color: var(--text-primary);
}

.titlebar-btn.active {
  color: var(--color-accent);
  background: var(--glass-active);
}

.titlebar-btn-close:hover {
  background: var(--color-danger);
  color: white;
}
</style>
