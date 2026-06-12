import type { RouteRecordRaw } from 'vue-router'
import ChatView from './views/ChatView.vue'
import MemoryView from './views/MemoryView.vue'
import EmotionView from './views/EmotionView.vue'
import ToolsView from './views/ToolsView.vue'
import ProactiveView from './views/ProactiveView.vue'
import PersonalityView from './views/PersonalityView.vue'
import LLMConfigView from './views/LLMConfigView.vue'
import LogsView from './views/LogsView.vue'
import HealthView from './views/HealthView.vue'

export const routes: RouteRecordRaw[] = [
  { path: '/', redirect: '/chat' },
  { path: '/chat', name: 'chat', component: ChatView },
  { path: '/memory', name: 'memory', component: MemoryView },
  { path: '/emotion', name: 'emotion', component: EmotionView },
  { path: '/tools', name: 'tools', component: ToolsView },
  { path: '/proactive', name: 'proactive', component: ProactiveView },
  { path: '/personality', name: 'personality', component: PersonalityView },
  { path: '/llm-config', name: 'llm-config', component: LLMConfigView },
  { path: '/logs', name: 'logs', component: LogsView },
  { path: '/health', name: 'health', component: HealthView },
]
