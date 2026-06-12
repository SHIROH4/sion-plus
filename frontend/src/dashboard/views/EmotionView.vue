<template>
  <div class="emotion-view">
    <header class="page-header">
      <h1 class="page-title">情绪</h1>
      <span v-if="store.current" class="emotion-badge" :style="{ background: emotionColor }">
        {{ emotionLabel }}
      </span>
    </header>

    <div class="emotion-grid">
      <!-- 8D Radar Chart -->
      <GlassCard
        padding="md"
        radius="lg"
        :loading="store.loading"
        :empty="!store.loading && !store.current"
        empty-icon="🎯"
        empty-text="暂无情绪数据"
        :error="store.error"
        @retry="store.startPolling(3000)"
      >
        <label class="section-label">8维内部情绪雷达</label>
        <v-chart
          v-if="store.current"
          :option="radarOption"
          :autoresize="true"
          style="height: 350px"
        />
      </GlassCard>

      <!-- 8D Timeline -->
      <GlassCard
        padding="md"
        radius="lg"
        :empty="store.history.length < 2"
        empty-icon="📈"
        empty-text="正在收集情绪数据..."
      >
        <label class="section-label">情绪维度时间线</label>
        <v-chart
          v-if="store.history.length >= 2"
          :option="timelineOption"
          :autoresize="true"
          style="height: 320px"
        />
      </GlassCard>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'
import VChart from 'vue-echarts'
import { use } from 'echarts/core'
import { RadarChart, LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, LegendComponent, RadarComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import type { EChartsOption } from 'echarts'
import GlassCard from '../components/GlassCard.vue'
import { useEmotionStore } from '../stores/emotion'

use([RadarChart, LineChart, GridComponent, TooltipComponent, LegendComponent, RadarComponent, CanvasRenderer])

const store = useEmotionStore()

const emotionLabelMap: Record<string, string> = {
  joy: '开心', sadness: '悲伤', anger: '生气',
  fear: '害怕', surprise: '惊讶', disgust: '厌恶', neutral: '平静',
}

const emotionColorMap: Record<string, string> = {
  joy: '#34d399', sadness: '#3b82f6', anger: '#ef4444',
  fear: '#f59e0b', surprise: '#f778ba', disgust: '#8b949e', neutral: '#596579',
}

const emotionColor = computed(() => emotionColorMap[store.current?.primary || 'neutral'] || '#596579')
const emotionLabel = computed(() => emotionLabelMap[store.current?.primary || 'neutral'] || '平静')

const dimLabels: Record<string, string> = {
  affection: '亲密度', worry: '担心度', curiosity: '好奇心', sleepiness: '困倦度',
  playfulness: '贪玩度', loneliness: '孤独感', confidence: '自信心', annoyance: '恼怒度',
}

const dimColors: Record<string, string> = {
  affection: '#f778ba', worry: '#f59e0b', curiosity: '#3b82f6', sleepiness: '#8b5cf6',
  playfulness: '#34d399', loneliness: '#6366f1', confidence: '#14b8a6', annoyance: '#ef4444',
}

const dimOrder = ['affection', 'worry', 'curiosity', 'sleepiness', 'playfulness', 'loneliness', 'confidence', 'annoyance']

const radarOption = computed((): EChartsOption => {
  const v = store.current?.vector
  if (!v) return {}
  return {
    radar: {
      center: ['50%', '55%'],
      radius: '70%',
      indicator: dimOrder.map(k => ({ name: dimLabels[k], max: 1 })),
      axisName: { color: '#596579', fontSize: 12 },
      splitArea: { areaStyle: { color: ['rgba(0,0,0,0.01)', 'rgba(0,0,0,0.03)'] } },
    },
    series: [{
      type: 'radar',
      data: [{
        value: dimOrder.map(k => v[k as keyof typeof v] as number),
        name: '当前状态',
        areaStyle: { color: 'rgba(247,119,186,0.15)' },
        lineStyle: { color: '#f778ba', width: 2 },
        itemStyle: { color: '#f778ba' },
      }],
      symbol: 'circle',
      symbolSize: 5,
    }],
  }
})

const timelineOption = computed((): EChartsOption => {
  const data = store.history
  if (data.length < 2) return {}
  return {
    grid: { top: 10, right: 20, bottom: 30, left: 50 },
    xAxis: {
      type: 'category',
      data: data.map((_, i) => `${i + 1}`),
      axisLine: { lineStyle: { color: 'rgba(0,0,0,0.08)' } },
      axisLabel: { color: '#8b949e', fontSize: 10 },
    },
    yAxis: {
      type: 'value', min: 0, max: 1,
      axisLine: { lineStyle: { color: 'rgba(0,0,0,0.08)' } },
      splitLine: { lineStyle: { color: 'rgba(0,0,0,0.04)' } },
    },
    legend: {
      show: true, bottom: 0,
      textStyle: { color: '#596579', fontSize: 10 },
      data: dimOrder.map(k => dimLabels[k]),
      type: 'scroll',
    },
    tooltip: { trigger: 'axis' },
    series: dimOrder.map(k => ({
      name: dimLabels[k],
      type: 'line',
      data: data.map(d => d.vector[k as keyof typeof d.vector] as number),
      smooth: true,
      symbol: 'none',
      lineStyle: { color: dimColors[k], width: 1.5 },
    })),
  } as EChartsOption
})

onMounted(() => store.startPolling(3000))
onUnmounted(() => store.stopPolling())
</script>

<style scoped>
.emotion-view {
  display: flex;
  flex-direction: column;
  height: calc(100vh - var(--titlebar-height) - var(--page-padding) * 2);
  overflow-y: auto;
  gap: 20px;
}

.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-shrink: 0;
}

.page-title {
  font-size: var(--text-xl);
  font-weight: 600;
}

.emotion-badge {
  padding: 4px 14px;
  border-radius: var(--radius-full);
  color: white;
  font-size: var(--text-sm);
  font-weight: 500;
}

.emotion-grid {
  display: flex;
  flex-direction: column;
  gap: var(--card-gap);
}

.section-label {
  display: block;
  font-size: var(--text-sm);
  font-weight: 500;
  color: var(--text-secondary);
  margin-bottom: 8px;
}
</style>
