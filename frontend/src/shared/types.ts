// === Chat ===
export interface ChatMessage {
  id: string
  role: 'user' | 'ai' | 'system'
  content: string
  timestamp: number
  emotion?: string
  source?: string // 'llm' | 'interrupted' | 'system'
  streaming?: boolean
}

export interface ChatResponse {
  response: string
  emotion: string
  source: string
}

// === Emotion ===
export interface EmotionVector {
  affection: number    // 0~1 亲密度
  worry: number        // 0~1 担心度
  curiosity: number    // 0~1 好奇心
  sleepiness: number   // 0~1 困倦度
  playfulness: number  // 0~1 贪玩度
  loneliness: number   // 0~1 孤独感
  confidence: number   // 0~1 自信心
  annoyance: number    // 0~1 恼怒度
}

export interface EmotionState {
  primary: string      // joy, sadness, anger, fear, surprise, disgust, neutral
  intensity: number    // 0 to 1
  vector: EmotionVector
}

export interface EmotionSnapshot extends EmotionState {
  timestamp: number
}

// === Memory ===
export interface FactEntry {
  id: number
  entity: string        // "master" | "neko" | "relationship"
  relation_type: string
  content: string
  source_tier: 'explicit' | 'observed' | 'inferred'
  temporal_scope: 'pattern' | 'state' | 'episode'
  importance: number    // 1-10
  memcell_type: string  // fact|prefer|event|emotion|skill|relation
  evidence: { reinforcement: number; disputation: number }
  archived: boolean
  created_at: number
}

export interface Topic {
  id: number
  name: string
  count: number
}

export interface MemoryStats {
  total: number
  confirmed: number
  pending: number
  by_entity: Record<string, number>
  by_source: Record<string, number>
  by_type: Record<string, number>
}

// === Tools ===
export interface ToolInfo {
  name: string
  description: string
  parameters: Record<string, unknown>
  dangerous: boolean
}

// === Proactive ===
export interface ProactiveStatus {
  mode: string          // "normal"|"frequent"|"focus"|"off"
  interval_sec: number
  last_action: string
  last_tick: number
}

export interface ProactiveAction {
  name: string
  category: string      // "social"|"care"|"learning"|"none"
  outcome_type: string  // "speak"|"action"|"silent"
  night_safe: boolean
  weight_social: number
  weight_care: number
  weight_curious: number
  weight_quiet: number
  weight_explore: number
  trigger: string
  action: string
}

// === Personality ===
export interface PersonalityConfig {
  name: string
  system_prompt: string
  traits: Record<string, number>
  speaking_style: string
  background: string
}

// === LLM Config ===
export interface LLMProviderConfig {
  name: string
  base_url: string
  api_key: string
  chat_model: string
  embed_model?: string
  enabled: boolean
  priority: number
  max_retries: number
  timeout_sec: number
}

export interface LLMRoutes {
  default: string
  chat: string
  emotion: string
  memory: string
  vision: string
  summary: string
  signal: string
  search: string
}

export interface LLMFullConfig {
  providers: LLMProviderConfig[]
  routes: LLMRoutes
}

// === Logs ===
export interface LogEntry {
  level: 'debug' | 'info' | 'warn' | 'error'
  source: string
  message: string
  time: number  // unix milliseconds
}

// === Health ===
export interface HealthStatus {
  status: string
  modules: Record<string, string>
  cpu_cores: number
  mem_used_mb: number
  mem_total_mb: number
  goroutines: number
  uptime_sec: number
}

// === UI States ===
export type AsyncState = 'idle' | 'loading' | 'success' | 'error'

export interface AsyncWrapper<T> {
  data: T | null
  state: AsyncState
  error: string | null
}

// === Navigation ===
export type DashboardPage =
  | 'chat'
  | 'memory'
  | 'emotion'
  | 'tools'
  | 'proactive'
  | 'personality'
  | 'llm-config'
  | 'logs'
  | 'health'

export interface NavItem {
  key: DashboardPage
  label: string
  icon: string
}
