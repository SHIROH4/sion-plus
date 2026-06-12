import type { NavItem, PersonalityTraits, LLMParameters } from './types'

export const ACCENT_COLOR = '#f778ba'
export const ACCENT_COLOR_HOVER = '#f960ae'
export const ACCENT_COLOR_ACTIVE = '#e6509e'

export const NAV_ITEMS: NavItem[] = [
  { key: 'chat', label: 'Chat', icon: '💬' },
  { key: 'memory', label: 'Memory', icon: '🧠' },
  { key: 'emotion', label: 'Emotion', icon: '💗' },
  { key: 'tools', label: 'Tools', icon: '🔧' },
  { key: 'proactive', label: 'Proactive', icon: '🎯' },
  { key: 'personality', label: 'Personality', icon: '🎭' },
  { key: 'llm-config', label: 'LLM Config', icon: '⚙️' },
  { key: 'logs', label: 'Logs', icon: '📋' },
  { key: 'health', label: 'Health', icon: '❤️' },
]

export const DEFAULT_PERSONALITY_TRAITS: PersonalityTraits = {
  warmth: 8,
  playfulness: 7,
  formality: 3,
  curiosity: 6,
  empathy: 8,
}

export const DEFAULT_LLM_PARAMETERS: LLMParameters = {
  temperature: 0.8,
  topP: 0.95,
  maxTokens: 2048,
  frequencyPenalty: 0.3,
  presencePenalty: 0.3,
}

export const EMOTION_LABELS: Record<string, string> = {
  joy: '开心',
  sadness: '悲伤',
  anger: '生气',
  fear: '害怕',
  surprise: '惊讶',
  disgust: '厌恶',
  neutral: '平静',
}

export const LOG_LEVEL_COLORS: Record<string, string> = {
  debug: '#8b949e',
  info: '#3b82f6',
  warn: '#f59e0b',
  error: '#ef4444',
}

export const PET_WINDOW = {
  width: 400,
  height: 500,
} as const

export const CAPSULE_COLLAPSED_WIDTH = 200
export const CAPSULE_COLLAPSED_HEIGHT = 36
export const CAPSULE_EXPANDED_WIDTH = 360
export const CAPSULE_EXPANDED_MAX_HEIGHT = 200
