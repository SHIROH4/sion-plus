<template>
  <span class="streaming-text">
    <span
      v-for="(char, i) in chars"
      :key="i"
      class="streaming-char"
      :style="{ animationDelay: `${i * speed}ms` }"
    >{{ char }}</span>
    <span v-if="isActive" class="streaming-cursor animate-cursor-blink" />
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(defineProps<{
  text: string
  isActive?: boolean
  speed?: number     // ms per character delay
}>(), {
  isActive: false,
  speed: 40,
})

const chars = computed(() => props.text.split(''))
</script>

<style scoped>
.streaming-text {
  white-space: pre-wrap;
  word-break: break-word;
}

.streaming-char {
  animation: char-reveal 0ms ease-out both;
}

.streaming-cursor {
  display: inline;
  color: var(--color-accent);
  font-weight: 600;
  animation: cursor-blink 1s steps(1) infinite;
}
</style>
