<template>
  <div ref="containerRef" class="live2d-container">
    <div v-if="status === 'loading'" class="live2d-status">加载中...</div>
    <div v-if="status === 'error'" class="live2d-status error">{{ errorMsg }}</div>
    <div ref="bubbleElRef" class="pet-bubble" />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch } from 'vue'

const props = withDefaults(defineProps<{ emotion?: string; talking?: boolean }>(), {
  emotion: 'neutral', talking: false,
})

const emit = defineEmits<{
  interaction: [part: string]
  resize: [w: number, h: number]
}>()

const containerRef = ref<HTMLDivElement>()
const bubbleElRef = ref<HTMLDivElement>()
const status = ref<'loading' | 'ready' | 'error'>('loading')
const errorMsg = ref('')

// ── Bubble (matching oyasumi-sion PetCanvas exactly) ──
const BUBBLE_HIDE_MS = 6000
const bubbleQueueRef = ref<string[]>([])
let bubbleTimer: ReturnType<typeof setTimeout> | null = null

function splitBubbleText(text: string): string[] {
  const MAX = 20
  if (text.length <= MAX) return text ? [text] : []
  const chunks: string[] = []
  let remaining = text
  while (remaining.length > 0) {
    if (remaining.length <= MAX) { chunks.push(remaining); break }
    let cut = -1
    for (const sep of ['。', '！', '？', '\n', '，', '、']) {
      const idx = remaining.lastIndexOf(sep, MAX)
      if (idx > MAX * 0.5) { cut = idx + 1; break }
    }
    if (cut < 0) cut = MAX
    chunks.push(remaining.slice(0, cut).trim())
    remaining = remaining.slice(cut).trim()
  }
  return chunks.filter(c => c.length > 0)
}

function showNextBubble() {
  if (bubbleTimer) { clearTimeout(bubbleTimer); bubbleTimer = null }
  const q = bubbleQueueRef.value
  if (q.length === 0) {
    bubbleElRef.value?.classList.remove('show')
    return
  }
  const el = bubbleElRef.value
  if (el) { el.textContent = q[0]; el.classList.add('show') }
  q.shift()
  timerRef.lastBubbleAt = Date.now()
  bubbleTimer = setTimeout(showNextBubble, 3000)
}

function stopBubbleQueue() {
  bubbleQueueRef.value = []
  if (bubbleTimer) { clearTimeout(bubbleTimer); bubbleTimer = null }
}

function showBubble(t: string) {
  timerRef.lastBubbleAt = Date.now()
  stopBubbleQueue()
  if (!t) return
  bubbleQueueRef.value = splitBubbleText(t)
  showNextBubble()
}

function showBubbleLive(t: string) {
  timerRef.lastBubbleAt = Date.now()
  stopBubbleQueue()
  const el = bubbleElRef.value
  if (el && t) { el.textContent = t; el.classList.add('show') }
}

function hideBubble() {
  timerRef.lastBubbleAt = 0
  stopBubbleQueue()
  bubbleElRef.value?.classList.remove('show')
}

defineExpose({ showBubble, showBubbleLive, hideBubble })

let app: any = null
let live2dModel: any = null
let idleTimer: ReturnType<typeof setTimeout> | null = null
let destroyed = false

const emotionMap: Record<string, string> = {
  joy: 'exp_02', sadness: 'exp_04', anger: 'exp_06',
  fear: 'exp_07', surprise: 'exp_05', disgust: 'exp_08', neutral: 'exp_01',
}

// ── Timers (gaze/expression/bubble auto-reset, like oyasumi-sion) ──
const timerRef = {
  lastExpressionAt: 0,
  lastBubbleAt: 0,
  lastGazeAt: 0,
  gazeCX: 0,
  gazeCY: 0,
}

onMounted(async () => { await initLive2D() })

onUnmounted(() => {
  destroyed = true
  if (idleTimer) clearTimeout(idleTimer)
  stopBubbleQueue()
  try { live2dModel?.destroy() } catch { /* */ }
  try { app?.destroy?.(true, { children: true, texture: true }) } catch { /* */ }
})

watch(() => props.emotion, (v) => {
  if (!live2dModel || !v) return
  try {
    timerRef.lastExpressionAt = Date.now()
    live2dModel.expression?.(emotionMap[v] || 'exp_01')
  } catch { /* */ }
})

async function initLive2D() {
  const el = containerRef.value
  if (!el) { showError('容器未找到'); return }

  const win = window as unknown as Record<string, unknown>
  await new Promise(r => setTimeout(r, 200))

  const PIXI = win.PIXI
  if (!PIXI) { showError('PixiJS 未加载'); return }
  const live2d = (PIXI as any)?.live2d
  const LM = live2d?.Live2DModel
  if (!LM?.from) { showError('Live2DModel 未找到'); return }
  if (!win.Live2DCubismCore) { showError('CubismCore 未加载'); return }

  try {
    app = new (PIXI as any).Application({
      backgroundAlpha: 0,
      transparent: true,
      useContextAlpha: 'notMultiplied',
      resizeTo: el,
      antialias: true,
      resolution: window.devicePixelRatio || 1,
      autoDensity: true,
    })
    el.appendChild(app.view as HTMLCanvasElement)

    live2dModel = await LM.from('/model/Mao/Mao.model3.json', {
      autoInteract: true,
      ticker: app.ticker,
    })
    if (!live2dModel || destroyed) return

    app.stage.addChildAt(live2dModel, 0)

    const cw = el.clientWidth
    const ch = el.clientHeight
    const s = (ch * 0.88) / live2dModel.height
    live2dModel.scale.set(s)
    live2dModel.anchor.set(0.5, 0.5)
    live2dModel.position.set(cw / 2, ch / 2)

    const ctrl = live2dModel.internalModel?.focusController
    if (ctrl) { ctrl.acceleration = 0.04; ctrl.deceleration = 0.08 }

    timerRef.gazeCX = cw / 2
    timerRef.gazeCY = ch / 2

    const b = live2dModel.getBounds()
    const margin = 16
    const newW = Math.ceil(b.width + margin * 2)
    const newH = Math.ceil(b.height + margin)
    emit('resize', newW, newH)

    const canvas = app.view as HTMLCanvasElement
    const DRAG_PX = 4
    const POKE_MS = 250
    const api = (window as any).electronAPI
    let drag: { sx: number; sy: number; st: number; active: boolean } | null = null

    const getPos = (e: PointerEvent) => {
      const r = canvas.getBoundingClientRect()
      return { x: e.clientX - r.left, y: e.clientY - r.top }
    }
    const updateGaze = () => { timerRef.lastGazeAt = Date.now() }

    canvas.addEventListener('pointerdown', (e: PointerEvent) => {
      const { x, y } = getPos(e)
      drag = { sx: x, sy: y, st: Date.now(), active: false }
    })

    canvas.addEventListener('pointermove', (e: PointerEvent) => {
      const { x, y } = getPos(e)
      try { live2dModel?.focus?.(x, y, false) } catch { /* */ }
      updateGaze()
      if (!drag) return
      if (!drag.active && (Math.abs(x - drag.sx) > DRAG_PX || Math.abs(y - drag.sy) > DRAG_PX)) {
        drag.active = true
        if (api) api.dragStart()
      }
    })

    canvas.addEventListener('pointerup', () => {
      if (!drag) return
      if (drag.active) { if (api) api.dragStop() }
      else if (Date.now() - drag.st < POKE_MS) {
        try { live2dModel?.internalModel?.motionManager?.startRandomMotion() } catch { /* */ }
      }
      drag = null
    })

    try {
      live2dModel.on('hit', (areas: string[]) => {
        if (areas?.length) {
          emit('interaction', areas[0])
          try { live2dModel.motion?.('TapBody', areas[0] === 'Head' ? 1 : 2) } catch { /* */ }
        }
      })
    } catch { /* */ }

    // ── Auto-reset timers (expression / gaze / bubble) ──
    const interval = setInterval(() => {
      if (destroyed) { clearInterval(interval); return }
      const now = Date.now()
      const t = timerRef
      if (t.lastExpressionAt > 0 && now - t.lastExpressionAt > 3000) {
        t.lastExpressionAt = 0
        try { live2dModel?.expression?.('exp_01') } catch { /* */ }
      }
      if (t.lastBubbleAt > 0 && now - t.lastBubbleAt > BUBBLE_HIDE_MS) {
        t.lastBubbleAt = 0
        stopBubbleQueue()
        bubbleElRef.value?.classList.remove('show')
      }
      if (t.lastGazeAt > 0 && now - t.lastGazeAt > 3000) {
        t.lastGazeAt = 0
        try { live2dModel?.focus?.(t.gazeCX, t.gazeCY, true) } catch { /* */ }
      }
    }, 200)

    const ro = new ResizeObserver(() => {
      if (!live2dModel) return
      const cw2 = el.clientWidth
      const ch2 = el.clientHeight
      live2dModel.position?.set(cw2 / 2, ch2 / 2)
      timerRef.gazeCX = cw2 / 2
      timerRef.gazeCY = ch2 / 2
    })
    ro.observe(el)

    const tick = () => {
      if (destroyed) return
      try { live2dModel?.internalModel?.motionManager?.startRandomMotion() } catch { /* */ }
      idleTimer = setTimeout(tick, 6000 + Math.random() * 10000)
    }
    idleTimer = setTimeout(tick, 4000)

    status.value = 'ready'
  } catch (e) {
    console.error('[PetLive2D] init error:', e)
    showError(`初始化失败: ${e instanceof Error ? e.message : String(e)}`)
  }
}

function showError(msg: string) {
  console.error('[PetLive2D]', msg)
  status.value = 'error'; errorMsg.value = msg
}
</script>

<style scoped>
.live2d-container {
  width: 100%; height: 100%;
  overflow: hidden;
  background: transparent;
  position: relative;
}

.live2d-status {
  position: absolute;
  bottom: 40px; left: 50%;
  transform: translateX(-50%);
  padding: 6px 16px;
  border-radius: 12px;
  background: rgba(255,255,255,0.85);
  color: #34536c;
  font-size: 12px;
  backdrop-filter: blur(8px);
  -webkit-backdrop-filter: blur(8px);
  z-index: 10;
  pointer-events: none;
}
.live2d-status.error {
  background: rgba(239,68,68,0.15);
  color: #dc2626;
}

.pet-bubble {
  position: fixed;
  top: 0;
  left: 50%;
  transform: translateX(-50%);
  max-width: 420px;
  padding: 10px 14px;
  background: rgba(255, 255, 255, 0.95);
  backdrop-filter: blur(8px);
  -webkit-backdrop-filter: blur(8px);
  border-radius: 18px;
  font-size: 14px;
  line-height: 1.5;
  color: #444;
  box-shadow: 0 4px 15px rgba(0, 0, 0, 0.1);
  border: 1px solid rgba(255, 255, 255, 0.5);
  word-break: break-word;
  pointer-events: none;
  z-index: 10;
  opacity: 0;
  transition: opacity 0.3s ease;
}

.pet-bubble::after {
  content: '';
  position: absolute;
  bottom: -8px;
  left: 50%;
  transform: translateX(-50%);
  border-left: 8px solid transparent;
  border-right: 8px solid transparent;
  border-top: 8px solid rgba(255, 255, 255, 0.95);
}

.pet-bubble.show {
  opacity: 1;
}
</style>
