/**
 * Sion API Layer — HTTP REST client
 *
 * Go backend HTTP endpoints:
 *   POST /api/v1/chat            → {response, emotion, source}
 *   POST /api/v1/chat/stream     → SSE: event:token / event:done / event:error
 *   GET  /api/v1/emotion         → {primary, intensity, vector: {affection, worry, ...}}
 *   GET  /api/v1/health          → {status, modules:{}}
 *   GET  /api/v1/stats           → {messages_today}
 *   GET  /api/v1/screen          → {app_name, app_category, window_title}
 *   POST /api/v1/proactive/mode  → body:{mode}, response:{mode, interval_sec}
 *   GET  /api/events?topics=...  → SSE event:emotion
 */

// ═══════════════════════════════════════════════════════════
//  API base URL
// ═══════════════════════════════════════════════════════════

// In Electron production (file://), use absolute URL.
// In dev mode, Vite proxies /api → :8080.
const API_HOST = window.location.protocol === 'file:'
  ? 'http://127.0.0.1:8080'
  : ''
const BASE = `${API_HOST}/api/v1`

// ═══════════════════════════════════════════════════════════
//  Wire types (match backend JSON exactly)
// ═══════════════════════════════════════════════════════════

export interface ChatResult {
  response: string
  emotion: string
  source: string
}

export interface EmotionData {
  primary: string
  intensity: number
  vector: {
    affection: number; worry: number; curiosity: number; sleepiness: number
    playfulness: number; loneliness: number; confidence: number; annoyance: number
  }
}

export interface HealthData {
  status: string
  modules: Record<string, string>
}

export interface ScreenData {
  app_name: string
  app_category: string
  window_title: string
}

export interface StatsData {
  messages_today: number
}

// ═══════════════════════════════════════════════════════════
//  HTTP helpers
// ═══════════════════════════════════════════════════════════

const API_TIMEOUT_MS = 30000

class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message)
    this.name = 'ApiError'
  }
}

function fetchWithTimeout(url: string, init?: RequestInit): Promise<Response> {
  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), API_TIMEOUT_MS)
  return fetch(url, { ...init, signal: controller.signal }).finally(() => clearTimeout(timer))
}

async function httpGet<T>(path: string): Promise<T> {
  const res = await fetchWithTimeout(`${BASE}${path}`)
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new ApiError(res.status, (body as { error?: string }).error || `HTTP ${res.status}`)
  }
  return res.json() as Promise<T>
}

async function httpPost<T>(path: string, body: unknown): Promise<T> {
  const res = await fetchWithTimeout(`${BASE}${path}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({}))
    throw new ApiError(res.status, (err as { error?: string }).error || `HTTP ${res.status}`)
  }
  return res.json() as Promise<T>
}

// ═══════════════════════════════════════════════════════════
//  Public API
// ═══════════════════════════════════════════════════════════

// ── Chat ──────────────────────────────────────────────────

export async function sendMessage(message: string): Promise<ChatResult> {
  return httpPost<ChatResult>('/chat', { message })
}

export async function sendMessageStream(
  message: string,
  onToken: (token: string) => void,
  onDone: (result: ChatResult) => void,
  onError: (err: string) => void,
  source: string = 'dashboard',
): Promise<void> {
  try {
    // Bypass Vite proxy — it buffers SSE, killing streaming.
    // Go backend has CORS * so cross-origin from :5173 to :8080 works.
    const streamBase = window.location.protocol === 'file:' ? 'http://127.0.0.1:8080' : 'http://127.0.0.1:8080'
    const res = await fetch(`${streamBase}/api/v1/chat/stream`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ message, source }),
    })
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: `HTTP ${res.status}` }))
      onError((err as { error?: string }).error || 'Stream request failed')
      return
    }
    const reader = res.body?.getReader()
    if (!reader) { onError('No response body'); return }

    const decoder = new TextDecoder()
    let buffer = ''
    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      const lines = buffer.split('\n')
      buffer = lines.pop() || ''
      let eventType = ''
      for (const line of lines) {
        if (line.startsWith('event: ')) {
          eventType = line.slice(7).trim()
        } else if (line.startsWith('data: ')) {
          try {
            const data = JSON.parse(line.slice(6))
            switch (eventType) {
              case 'token': if (data.token) onToken(data.token); break
              case 'done':
                onDone({ response: data.response || '', emotion: data.emotion || 'neutral', source: data.source || 'stream' })
                return
              case 'error': onError(data.error || 'Stream error'); return
            }
          } catch { /* skip */ }
          eventType = ''
        }
      }
    }
  } catch (e) {
    onError(e instanceof Error ? e.message : 'Stream connection failed')
  }
}

// ── Emotion ───────────────────────────────────────────────

export async function getCurrentEmotion(): Promise<EmotionData> {
  return httpGet<EmotionData>('/emotion')
}

// ── Health ────────────────────────────────────────────────

export async function getHealthStatus(): Promise<HealthData> {
  return httpGet<HealthData>('/health')
}

// ── Screen ────────────────────────────────────────────────

export async function getScreenStatus(): Promise<ScreenData> {
  return httpGet<ScreenData>('/screen')
}

// ── Stats ─────────────────────────────────────────────────

export async function getStats(): Promise<StatsData> {
  return httpGet<StatsData>('/stats')
}

// ── Proactive ─────────────────────────────────────────────

export async function getProactiveStatus(): Promise<import('@/shared/types').ProactiveStatus> {
  const res = await fetch('http://127.0.0.1:8080/api/v1/proactive/status')
  if (!res.ok) return { mode: 'off', interval_sec: 0, last_action: '', last_tick: 0 }
  return res.json()
}

export async function getProactiveActions(): Promise<import('@/shared/types').ProactiveAction[]> {
  const res = await fetch('http://127.0.0.1:8080/api/v1/proactive/actions')
  if (!res.ok) return []
  const data = await res.json()
  return (data as { actions: import('@/shared/types').ProactiveAction[] }).actions || []
}

export async function setProactiveMode(mode: string): Promise<void> {
  await fetch('http://127.0.0.1:8080/api/v1/proactive/mode', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ mode }),
  })
}

// ── Chat History ──────────────────────────────────────────

export interface HistoryMessage {
  role: string
  content: string
  created_at: number
}

export async function getChatHistory(limit = 50): Promise<HistoryMessage[]> {
  const host = window.location.protocol === 'file:' ? 'http://127.0.0.1:8080' : 'http://127.0.0.1:8080'
  const res = await fetch(`${host}/api/v1/chat/history`)
  if (!res.ok) return []
  const data = await res.json()
  return (data as { messages: HistoryMessage[] }).messages || []
}

// ── SSE Events ────────────────────────────────────────────

export function subscribeToSSE(
  topics: string[],
  onEvent: (topic: string, data: Record<string, unknown>) => void,
): () => void {
  const url = `http://127.0.0.1:8080/api/events?topics=${topics.join(',')}`
  let es = new EventSource(url)
  let stopped = false

  for (const topic of topics) {
    es.addEventListener(topic, (e: MessageEvent) => {
      try { onEvent(topic, JSON.parse(e.data) as Record<string, unknown>) } catch { /* */ }
    })
  }

  es.onerror = () => {
    if (stopped) return
    es.close()
    setTimeout(() => {
      if (stopped) return
      es = new EventSource(url)
      for (const topic of topics) {
        es.addEventListener(topic, (e: MessageEvent) => {
          try { onEvent(topic, JSON.parse(e.data) as Record<string, unknown>) } catch { /* */ }
        })
      }
    }, 3000)
  }

  return () => { stopped = true; es.close() }
}

// ── Logs ───────────────────────────────────────────────────

export async function getLogs(): Promise<import('@/shared/types').LogEntry[]> {
  const res = await fetch('http://127.0.0.1:8080/api/v1/logs')
  if (!res.ok) return []
  const data = await res.json()
  return (data as { logs: import('@/shared/types').LogEntry[] }).logs || []
}

// ── Tools ──────────────────────────────────────────────────

export async function getTools(): Promise<import('@/shared/types').ToolInfo[]> {
  const res = await fetch('http://127.0.0.1:8080/api/v1/tools')
  if (!res.ok) return []
  const data = await res.json()
  return (data as { tools: import('@/shared/types').ToolInfo[] }).tools || []
}

// ── Memory ─────────────────────────────────────────────────

export async function getMemoryFacts(params?: {
  entity?: string; source_tier?: string; type?: string
}): Promise<{ facts: import('@/shared/types').FactEntry[] }> {
  const qs = new URLSearchParams()
  if (params?.entity) qs.set('entity', params.entity)
  if (params?.source_tier) qs.set('source_tier', params.source_tier)
  if (params?.type) qs.set('type', params.type)
  const host = 'http://127.0.0.1:8080'
  const res = await fetch(`${host}/api/v1/memory/facts?${qs}`)
  if (!res.ok) return { facts: [] }
  return res.json()
}

export async function getMemoryTopics(): Promise<{ topics: import('@/shared/types').Topic[] }> {
  const res = await fetch('http://127.0.0.1:8080/api/v1/memory/topics')
  if (!res.ok) return { topics: [] }
  return res.json()
}

export async function getMemoryStats(): Promise<import('@/shared/types').MemoryStats> {
  const res = await fetch('http://127.0.0.1:8080/api/v1/memory/stats')
  if (!res.ok) return { total: 0, confirmed: 0, pending: 0, by_entity: {}, by_source: {}, by_type: {} }
  return res.json()
}

// ═══════════════════════════════════════════════════════════
//  Stubs — backend not yet implemented
// ═══════════════════════════════════════════════════════════

export async function createMemory(_entry: unknown): Promise<unknown> { throw new Error('Not implemented') }
export async function updateMemory(_id: string, _entry: unknown): Promise<void> { throw new Error('Not implemented') }
export async function deleteMemory(_id: string): Promise<void> { throw new Error('Not implemented') }
// ── Personality ────────────────────────────────────────────

export async function getPersonalityConfig(): Promise<import('@/shared/types').PersonalityConfig> {
  const res = await fetch('http://127.0.0.1:8080/api/v1/personality')
  if (!res.ok) throw new Error('Failed to load')
  return res.json()
}

export async function savePersonalityConfig(cfg: import('@/shared/types').PersonalityConfig): Promise<void> {
  const res = await fetch('http://127.0.0.1:8080/api/v1/personality', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(cfg),
  })
  if (!res.ok) throw new Error('Failed to save')
}

// ── LLM Config ─────────────────────────────────────────────

export async function getLLMFullConfig(): Promise<import('@/shared/types').LLMFullConfig> {
  const res = await fetch('http://127.0.0.1:8080/api/v1/llm-config')
  if (!res.ok) throw new Error('Failed to load')
  return res.json()
}

export async function saveLLMFullConfig(cfg: import('@/shared/types').LLMFullConfig): Promise<void> {
  const res = await fetch('http://127.0.0.1:8080/api/v1/llm-config', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(cfg),
  })
  if (!res.ok) throw new Error('Failed to save')
}
