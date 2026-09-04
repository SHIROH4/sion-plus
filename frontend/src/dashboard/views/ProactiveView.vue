<template>
  <div class="proactive-view">
    <header class="page-header">
      <h1 class="page-title">主动系统</h1>
      <span v-if="store.status.last_action" class="last-action">
        上次: {{ store.status.last_action }} · {{ formatTime(store.status.last_tick) }}
      </span>
    </header>

    <!-- Mode Switch -->
    <GlassCard padding="md" radius="lg" :loading="store.loading">
      <label class="section-label">决策模式</label>
      <div class="mode-switch">
        <button
          v-for="mode in modes"
          :key="mode"
          class="mode-btn"
          :class="{ active: store.status.mode === mode }"
          @click="store.switchMode(mode)"
        >
          <span class="mode-name">{{ store.modeLabels[mode] }}</span>
          <span class="mode-desc">{{ store.modeDesc[mode] }}</span>
        </button>
      </div>
    </GlassCard>

    <GlassCard v-if="store.evaluation" padding="md" radius="lg">
      <label class="section-label">近 30 天策略观测</label>
      <div class="metric-grid">
        <div><strong>{{ store.evaluation.opportunities }}</strong><span>决策机会</span></div>
        <div><strong>{{ store.evaluation.delivered }}</strong><span>实际投递</span></div>
        <div><strong>{{ percent(store.evaluation.feedback_rate) }}</strong><span>显式反馈率</span></div>
        <div><strong>{{ percent(store.evaluation.negative_rate) }}</strong><span>负反馈率</span></div>
        <div><strong>{{ percent(store.evaluation.reply_candidate_rate) }}</strong><span>候选回复率*</span></div>
        <div><strong>{{ store.evaluation.shadow_different }}/{{ store.evaluation.shadow_compared }}</strong><span>影子策略分歧</span></div>
        <div><strong>{{ store.evaluation.shadow_matched_reward.toFixed(2) }}</strong><span>匹配回放奖励 (n={{ store.evaluation.shadow_matched_feedback }})</span></div>
      </div>
      <p class="metric-note">* 仅表示 15 分钟内出现下一条用户消息，不作为满意度或训练奖励。</p>
    </GlassCard>

    <!-- Action Catalog -->
    <GlassCard
      padding="md"
      radius="lg"
      :loading="store.loading"
      :empty="store.actions.length === 0"
      empty-icon="🎯"
      empty-text="暂无决策动作"
      :error="store.error"
      @retry="store.fetch()"
    >
      <label class="section-label">决策动作目录 ({{ store.actions.length }})</label>
      <div class="actions-grid">
        <div
          v-for="action in store.actions"
          :key="action.name"
          class="action-card"
          :class="`cat-${action.category}`"
        >
          <div class="action-header">
            <span class="action-icon">{{ store.categoryIcons[action.category] }}</span>
            <span class="action-name">{{ action.name }}</span>
            <span class="outcome-badge" :class="`outcome-${action.outcome_type}`">
              {{ store.outcomeLabels[action.outcome_type] }}
            </span>
          </div>
          <div class="action-body">
            <span v-if="action.trigger" class="action-trigger">{{ action.trigger }}</span>
            <span v-if="action.action" class="action-desc">{{ action.action }}</span>
          </div>
          <div class="action-weights">
            <span class="weight" title="社交权重">💬{{ action.weight_social.toFixed(1) }}</span>
            <span class="weight" title="关怀权重">💗{{ action.weight_care.toFixed(1) }}</span>
            <span class="weight" title="好奇权重">🧠{{ action.weight_curious.toFixed(1) }}</span>
            <span class="weight" title="安静权重">🔇{{ action.weight_quiet.toFixed(1) }}</span>
            <span class="weight" title="探索权重">🔍{{ action.weight_explore.toFixed(1) }}</span>
          </div>
          <div class="action-footer">
            <span class="cat-badge">{{ store.categoryLabels[action.category] }}</span>
            <span v-if="action.night_safe" class="night-safe">🌙 深夜安全</span>
          </div>
        </div>
      </div>
    </GlassCard>

    <GlassCard padding="md" radius="lg" :empty="store.decisions.length === 0" empty-icon="🧾" empty-text="尚无主动决策记录">
      <label class="section-label">决策审计（最近 8 条）</label>
      <div class="decision-list">
        <article v-for="decision in store.decisions" :key="decision.decision_id" class="decision-row">
          <div><strong>{{ decision.action }}</strong><span class="decision-meta">{{ decision.policy_version }} · {{ formatTime(decision.created_at) }}</span></div>
          <span class="decision-state" :class="decision.state">{{ stateLabel(decision.state) }}</span>
          <p v-if="decision.content">{{ decision.content }}</p>
        </article>
      </div>
    </GlassCard>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import GlassCard from '../components/GlassCard.vue'
import { useProactiveStore } from '../stores/proactive'

const store = useProactiveStore()
const modes = ['normal', 'frequent', 'focus', 'off']

function formatTime(ts: number): string {
  if (!ts) return '—'
  return new Date(ts * 1000).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

function percent(value: number): string { return `${(value * 100).toFixed(1)}%` }

function stateLabel(state: string): string {
  return ({ silent: '静默', blocked: '门控拦截', failed: '失败', delivered: '待反馈', resolved: '已结算', expired: '已过期' } as Record<string, string>)[state] || state
}

onMounted(() => store.fetch())
</script>

<style scoped>
.proactive-view {
  display: flex;
  flex-direction: column;
  height: calc(100vh - var(--titlebar-height) - var(--page-padding) * 2);
  overflow-y: auto;
  gap: 16px;
}

.page-header {
  display: flex; align-items: center; justify-content: space-between;
  flex-shrink: 0;
}
.page-title { font-size: var(--text-xl); font-weight: 600; }
.last-action { font-size: var(--text-xs); color: var(--text-tertiary); }

.section-label {
  display: block; font-size: var(--text-sm); font-weight: 500;
  color: var(--text-secondary); margin-bottom: 10px;
}
.metric-grid { display:grid; grid-template-columns:repeat(auto-fit,minmax(120px,1fr)); gap:8px; }
.metric-grid div { display:flex; flex-direction:column; gap:2px; padding:10px; border-radius:var(--radius-md); background:rgba(255,255,255,.45); }
.metric-grid strong { font-size:var(--text-lg); color:var(--text-primary); }
.metric-grid span,.metric-note { font-size:10px; color:var(--text-tertiary); }
.metric-note { margin:8px 0 0; }

/* Mode Switch */
.mode-switch { display: flex; gap: 8px; }
.mode-btn {
  flex: 1; display: flex; flex-direction: column; align-items: center;
  padding: 12px 8px; border: 2px solid rgba(0,0,0,0.06); border-radius: var(--radius-md);
  background: rgba(255,255,255,0.5); cursor: pointer; font-family: inherit;
  transition: all var(--duration-fast);
}
.mode-btn:hover { border-color: var(--color-accent); }
.mode-btn.active { border-color: var(--color-accent); background: rgba(247,119,186,0.08); }
.mode-name { font-size: var(--text-sm); font-weight: 600; color: var(--text-primary); }
.mode-desc { font-size: 11px; color: var(--text-tertiary); margin-top: 4px; }

/* Actions Grid */
.actions-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  gap: 10px;
}
.action-card {
  padding: 14px; border-radius: var(--radius-md);
  background: rgba(255,255,255,0.6); backdrop-filter: blur(6px);
  display: flex; flex-direction: column; gap: 8px;
  border-left: 3px solid rgba(0,0,0,0.08);
}
.action-card.cat-social { border-left-color: #3b82f6; }
.action-card.cat-care { border-left-color: #f778ba; }
.action-card.cat-learning { border-left-color: #8b5cf6; }
.action-card.cat-none { border-left-color: rgba(0,0,0,0.08); }

.action-header { display: flex; align-items: center; gap: 6px; }
.action-icon { font-size: 16px; }
.action-name { font-size: var(--text-sm); font-weight: 600; color: var(--text-primary); flex: 1; }
.outcome-badge { font-size: 10px; padding: 1px 6px; border-radius: var(--radius-sm); }
.outcome-speak { background: rgba(52,211,153,0.1); color: var(--color-success); }
.outcome-action { background: rgba(59,130,246,0.1); color: #3b82f6; }
.outcome-silent { background: rgba(0,0,0,0.04); color: var(--text-tertiary); }

.action-body { display: flex; flex-direction: column; gap: 2px; }
.action-trigger { font-size: 11px; color: var(--text-tertiary); }
.action-desc { font-size: var(--text-xs); color: var(--text-secondary); }

.action-weights { display: flex; gap: 8px; }
.weight { font-size: 11px; color: var(--text-tertiary); }

.action-footer { display: flex; align-items: center; gap: 6px; }
.cat-badge { font-size: 10px; padding: 1px 6px; border-radius: var(--radius-sm); background: rgba(0,0,0,0.04); color: var(--text-tertiary); }
.night-safe { font-size: 10px; color: #6366f1; }
.decision-list { display:flex; flex-direction:column; gap:8px; }
.decision-row { position:relative; padding:10px 12px; border-radius:var(--radius-md); background:rgba(255,255,255,.45); }
.decision-row strong { font-size:var(--text-xs); color:var(--text-primary); }
.decision-meta { margin-left:8px; font-size:10px; color:var(--text-tertiary); }
.decision-row p { margin:5px 0 0; color:var(--text-secondary); font-size:var(--text-xs); line-height:1.45; }
.decision-state { position:absolute; top:10px; right:12px; font-size:10px; color:#d97706; }
.decision-state.resolved { color:var(--color-success); }
.decision-state.blocked,.decision-state.failed { color:var(--color-error); }
.decision-state.silent { color:var(--text-tertiary); }
</style>
