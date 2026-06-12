<template>
  <n-config-provider :theme="theme" :theme-overrides="themeOverrides" :locale="zhCN" :date-locale="dateZhCN">
    <div class="dashboard-shell">
      <TitleBar />
      <div class="dashboard-body">
        <Sidebar />
        <main class="dashboard-content">
          <router-view v-slot="{ Component }">
            <transition name="page" mode="out-in">
              <component :is="Component" />
            </transition>
          </router-view>
        </main>
      </div>
    </div>
  </n-config-provider>
</template>

<script setup lang="ts">
import { darkTheme, zhCN, dateZhCN } from 'naive-ui'
import type { GlobalThemeOverrides } from 'naive-ui'
import TitleBar from './components/TitleBar.vue'
import Sidebar from './components/Sidebar.vue'

const theme = darkTheme

const themeOverrides: GlobalThemeOverrides = {
  common: {
    fontFamily: "Inter, -apple-system, PingFang SC, Microsoft YaHei, sans-serif",
    bodyColor: 'transparent',
    cardColor: 'rgba(255,255,255,0.75)',
    modalColor: 'rgba(255,255,255,0.85)',
    popoverColor: 'rgba(255,255,255,0.85)',
    textColor1: '#1f2329',
    textColor2: '#596579',
    textColor3: '#8b949e',
    borderColor: 'rgba(0,0,0,0.06)',
    dividerColor: 'rgba(0,0,0,0.04)',
    primaryColor: '#f778ba',
    primaryColorHover: '#f960ae',
    primaryColorPressed: '#e6509e',
    primaryColorSuppl: '#f778ba',
    successColor: '#34d399',
    warningColor: '#f59e0b',
    errorColor: '#ef4444',
    infoColor: '#3b82f6',
    borderRadius: '12px',
    fontSize: '14px',
    heightSmall: '28px',
    heightMedium: '36px',
    heightLarge: '44px',
  },
  Input: {
    color: 'rgba(255,255,255,0.65)',
    colorFocus: 'rgba(255,255,255,0.85)',
    border: 'rgba(0,0,0,0.06)',
    borderFocus: '#f778ba',
    borderHover: 'rgba(0,0,0,0.12)',
    borderRadius: '10px',
    boxShadowFocus: '0 0 0 2px rgba(247,119,186,0.25)',
  },
  Button: {
    borderRadiusMedium: '10px',
  },
  Slider: {
    fill: '#f778ba',
    fillHover: '#f960ae',
  },
  Tabs: {
    tabTextColorActiveLine: '#f778ba',
    tabTextColorHoverLine: '#f960ae',
    barColor: '#f778ba',
  },
}
</script>

<style>
/* Global: all page views use explicit viewport height for reliable scrolling */
.page-view {
  height: calc(100vh - var(--titlebar-height) - var(--page-padding) * 2) !important;
  overflow-y: auto !important;
}
</style>

<style scoped>
.dashboard-shell {
  display: flex;
  flex-direction: column;
  width: 100%;
  height: 100%;
  background: transparent;
}

.dashboard-body {
  display: flex;
  flex: 1;
  overflow: hidden;
}

.dashboard-content {
  flex: 1;
  min-height: 0;
  padding: var(--page-padding);
  overflow-y: auto;
  background: transparent;
}
</style>
