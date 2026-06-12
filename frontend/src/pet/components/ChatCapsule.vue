<template>
  <div class="floating-input" :class="{ visible: isStreaming || hovered }">
    <div class="input-row">
      <input
        ref="inputRef"
        v-model="text"
        type="text"
        :disabled="isStreaming"
        placeholder="和 Sion 说点什么..."
        @keydown="onKeydown"
      />
      <button :disabled="isStreaming || !text.trim()" @click="doSend">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><path d="M22 2 11 13M22 2l-7 20-4-9-9-4 20-7z"/></svg>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'

const props = defineProps<{
  isStreaming?: boolean
  hovered?: boolean
  error?: string | null
}>()

const emit = defineEmits<{
  send: [text: string]
  stop: []
}>()

const text = ref('')
const inputRef = ref<HTMLInputElement>()

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    doSend()
  }
}

function doSend() {
  const t = text.value.trim()
  if (!t || props.isStreaming) return
  emit('send', t)
  text.value = ''
}
</script>

<style scoped>
.floating-input {
  position: absolute;
  bottom: 16px;
  left: 50%;
  transform: translateX(-50%);
  width: 88%;
  max-width: 300px;
  z-index: 25;
  opacity: 0;
  pointer-events: none;
  transition: opacity 0.2s ease;
}
.floating-input.visible {
  opacity: 1;
  pointer-events: auto;
}

.input-row {
  display: flex;
  align-items: center;
  gap: 6px;
}

input {
  flex: 1;
  height: 34px;
  border-radius: 17px;
  border: 1px solid rgba(255,255,255,0.3);
  background: rgba(255,255,255,0.88);
  padding: 0 14px;
  font-size: 13px;
  font-family: inherit;
  outline: none;
  color: #333;
  box-sizing: border-box;
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
}
input::placeholder { color: #999; }

button {
  width: 34px; height: 34px;
  border-radius: 50%;
  background: #f778ba;
  color: #fff;
  border: none;
  cursor: pointer;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  transition: opacity 0.15s;
}
button:disabled { opacity: 0.35; cursor: not-allowed; }
</style>
