# Sion v2.3 — Frontend Design Handoff

> Produced by `frontend-design-ui-ux` skill.
> Target: Vue 3.5 + Naive UI 2.44 + Pinia 3 + Vite 5 + Wails v3
> Aesthetic: Apple HIG × Google M3 × N.E.K.O dark glass
> Accent: #f778ba (Sion pink)

---

## Step 0: Discovery — User & Context

### Target Users
- **Primary**: 桌面用户（macOS/Windows），日常使用电脑时 Sion 作为桌面伴侣常驻
- **Usage context**: 用户在工作/浏览/编码时，Pet 窗口漂浮在桌面角落；需要完整功能时打开 Dashboard
- **Device**: 桌面/笔记本，1920×1080 及以上分辨率，暗色桌面壁纸

### Product Goal
Sion 是一个 AI 猫娘桌面伴侣。她的存在感来自 Live2D 形象 + 主动说话，功能性来自 Dashboard 控制台。

### Key Constraints
- 双窗口 Wails v3 桌面应用（非浏览器）
- Pet 窗口透明无边框，浮在桌面壁纸上
- Dashboard 窗口有自定义标题栏，玻璃态暗色主题
- 前端必须能独立开发（Vite dev server 回退模式，无 Wails 时也能跑）

### Accessibility Expectations
- 所有交互元素可键盘访问（Tab/Enter/Escape）
- `prefers-reduced-motion` 遵守
- 文字对比度 4.5:1（正常文本）/ 3:1（大文本）
- 屏幕阅读器友好（aria-label on icon buttons）

---

## Step 1: Flows & States

### Flow 1: Chat（核心流程）

```
用户输入消息 → 发送到 Go RuntimeService
    │
    ├── 成功 → 流式 token 逐个到达
    │           │
    │           ├── Pet 窗口: 胶囊显示逐字预览 + 语音气泡
    │           └── Dashboard: 消息列表实时更新
    │           │
    │           └── 完成 → Emotion 更新 + Proactive 分析
    │
    ├── 网络错误 → 消息标记 failed + 重试按钮
    └── LLM 错误 → 显示错误消息气泡
```

### Flow 2: Dashboard 导航

```
Dashboard 启动 → 默认显示 Chat 页面
    │
    ├── 点击侧边栏项 → 页面切换动画（300ms pageEnter/pageExit）
    │                   内容区加载对应页面组件
    │
    └── 每个页面独立状态（不互相影响）
```

### Flow 3: Pet ↔ Dashboard 联动

```
Dashboard 打开 → Pet 窗口齿轮 hover 可见
    │
    ├── Pet 发消息 → Dashboard Chat 页面也显示
    ├── Dashboard 发消息 → Pet 胶囊显示流式预览
    └── Emotion 变化 → 两个窗口同步（Go 1s 轮询推送）
```

### State Coverage

Every data-displaying area must handle:

| State | Chat Messages | Memory List | Emotion Display | Tools List | Logs Table |
|-------|--------------|-------------|-----------------|------------|------------|
| **Loading** | — | Skeleton rows | Skeleton pulse | Skeleton rows | Skeleton rows |
| **Empty** | "喵~ 你好！我是 Sion ✨" 欢迎消息 | "暂无记忆数据" | — (emotion always has data) | — (tools always registered) | "暂无日志" |
| **Error** | "连接失败" + retry | "加载失败" + retry | Fallback to neutral | — | "加载失败" + retry |
| **Streaming** | 左粉色边框 + ... 脉冲 | — | — | — | — |

---

## Step 2: Component Specifications

### Component Tree

```
Dashboard App
├── TitleBar
├── Sidebar
│   ├── Brand (logo + version)
│   ├── NavItem[] (9 items)
│   └── EmotionIndicator (dot + name + VAD)
├── ContentArea (page transition wrapper)
│   ├── ChatView
│   │   ├── MessageList
│   │   │   └── MessageBubble[]
│   │   └── ChatInput (textarea + send button)
│   ├── MemoryView
│   │   ├── StatCard[] (4 stats)
│   │   └── EvidenceList
│   ├── EmotionView
│   │   ├── PrimaryDisplay (dot + name + intensity)
│   │   ├── VADBar[] (3 progress bars)
│   │   └── DimensionChip[] (8 emotion tags)
│   ├── ToolsView
│   │   ├── StatCard[] (4 stats)
│   │   ├── ToolItem[] (7 tools)
│   │   └── RateLimiterBar
│   ├── ProactiveView
│   │   ├── StatCard[] (4 stats)
│   │   ├── ModeSelector (radio group)
│   │   └── DecisionTable
│   ├── PersonalityView
│   │   ├── PromptEditor (textarea)
│   │   └── PersonaStateList
│   ├── LLMConfigView
│   │   ├── StatCard[] (4 stats)
│   │   ├── EnvVarList
│   │   └── ChannelTags
│   ├── LogsView
│   │   ├── LevelFilter (radio group)
│   │   └── LogTable
│   └── HealthView
│       ├── StatCard[] (4 stats)
│       ├── ModuleStatusList
│       └── BuildInfo

Pet App
├── Live2DCanvas (PixiJS + Cubism)
│   └── SpeechBubble
├── FloatingCapsule
│   ├── PillState (default: glass pill)
│   └── InputState (expanded: input + send btn)
├── MinimizeBall
└── GearButton
```

### Component Spec: FloatingCapsule

**Purpose**: Pet 窗口的聊天输入胶囊。N.E.K.O compact capsule 风格。

**Variants**:
- `pill`: 默认药丸状态，玻璃态圆角条
- `input`: 展开输入状态，输入框 + 发送按钮

**States**:

| State | Visual | Behavior |
|-------|--------|----------|
| Default (pill) | 玻璃态药丸，文字 "和 Sion 说点什么…" | 点击 → input |
| Streaming (pill) | 逐字流式预览 + 右侧 ... 脉冲点 | 不可点击（等待完成） |
| Hover (pill) | translateY(-1px), 阴影加深, 粉色发光边框出现 | cursor: pointer |
| Input (bar) | 展开的输入框 + 粉色圆形发送按钮 | 聚焦输入框 |
| Sending | 发送按钮 disabled, 输入框清空, 缩回 pill | — |
| Disabled | opacity 0.4, cursor: not-allowed | — |

**Props**:
```typescript
interface FloatingCapsuleProps {
  streaming: boolean
  previewText: string
  onSend: (text: string) => void
}
```

**States (internal)**:
```typescript
type CapsuleState = 'default' | 'input'
```

**Animation**:
| Trigger | Animation | Duration | Easing |
|---------|-----------|----------|--------|
| pill → input | barIn: scale(0.96→1) + opacity(0→1) | 220ms | spring |
| input → pill | barOut: reverse | 160ms | ease-in |
| glyph reveal | glyphIn: opacity(0.58→1) | 90ms | ease-out |
| dot pulse | dotPulse: translateY(0→-3px→0) | 1.2s | infinite |

**Accessibility**:
- Role: `combobox` (when expanded)
- aria-label: "聊天输入"
- Enter: send
- Escape: collapse to pill

**Visual Spec (CSS)**:
```css
.capsule-pill {
  border-radius: 999px;
  padding: 10px 24px;
  /* 3-layer glass */
  background: linear-gradient(180deg, rgba(31,48,66,0.72), rgba(8,17,30,0.62));
  backdrop-filter: blur(20px) saturate(1.26) brightness(1.06);
  border: 1px solid rgba(255,255,255,0.07);
  box-shadow: 0 0 0 1px rgba(208,233,255,0.08), 0 4px 20px rgba(0,0,0,0.35);
  /* inner highlight */
  &::after { /* linear-gradient highlight */ }
}
.capsule-bar {
  /* same glass + pink glow ring */
  border-color: rgba(247,119,186,0.18);
}
.send-btn {
  width: 34px; height: 34px; border-radius: 50%;
  background: linear-gradient(135deg, #f778ba, #e8659e);
  box-shadow: 0 2px 8px rgba(247,119,186,0.35);
}
```

### Component Spec: Sidebar

**Purpose**: Dashboard 导航栏。

**Content**:
- Brand: 🐱 + "Sion" + "v2.3"
- Nav: 9 items (Chat/Memory/Emotion/Tools/Proactive/Personality/LLM/Logs/Health)
- Footer: EmotionIndicator

**States**:

| State | Visual |
|-------|--------|
| Default | 文字 #9aa0ac, 透明背景 |
| Hover | 文字变亮, bg rgba(255,255,255,0.03), translateX(2px) |
| Active | 文字 #f778ba, bg rgba(247,119,186,0.1), 左边 3px 粉色条 |

**Visual**:
```css
.sidebar {
  width: 210px;
  background: rgba(28,28,46,0.92);
  backdrop-filter: blur(24px) saturate(160%);
  border-right: 1px solid rgba(255,255,255,0.05);
}
```

### Component Spec: MessageBubble

**Purpose**: 单条聊天消息。

**Variants**: `user` | `assistant`

**States**: Default | Streaming (assistant only: 左粉色边框 + 脉冲点)

**Props**:
```typescript
interface MessageBubbleProps {
  role: 'user' | 'assistant'
  text: string
  time: Date
  streaming?: boolean
}
```

**Visual**:
```css
.msg.user {
  align-self: flex-end;
  background: linear-gradient(135deg, #1a3a6e, #264a8a);
  color: #d8e6ff;
  border-bottom-right-radius: 4px;
  animation: slideFromRight 150ms ease-out;
}
.msg.assistant {
  align-self: flex-start;
  background: #2d2d3c;
  color: #e8eaed;
  border-bottom-left-radius: 4px;
  animation: slideFromLeft 250ms ease-out;
}
```

### Component Spec: StatCard

**Purpose**: 统计数据展示卡片。

**Variants**: 默认 | 小号（用于 build info 等）

**Props**:
```typescript
interface StatCardProps {
  label: string
  value: string | number
  size?: 'default' | 'small'
}
```

**Visual**:
```css
.stat-card {
  padding: 18px;
  background: rgba(45,45,60,0.55);
  border: 1px solid rgba(255,255,255,0.06);
  border-radius: 10px;
  box-shadow: inset 0 1px 0 rgba(255,255,255,0.04);
}
.stat-value { font-size: 24px; font-weight: 700; color: #f778ba; }
.stat-label { font-size: 11px; color: #9aa0ac; text-transform: uppercase; }
```

### Component Spec: PageShell

**Purpose**: 每个 Dashboard 页面的统一外壳。

**Content**: 页面标题 + Naive UI `<n-card>` 包裹内容

**Visual**:
```css
.page {
  height: 100%;
  padding: 24px 32px;
}
.page h2 { font-size: 20px; font-weight: 700; margin-bottom: 20px; }
```

---

## Step 3: Design Tokens

### Colors

```typescript
const colors = {
  // Background hierarchy (dark navy, NOT pure black)
  page:     '#1a1a2e',
  card:     '#252538',
  hover:    'rgba(255,255,255,0.04)',
  input:    'rgba(255,255,255,0.04)',
  
  // Text hierarchy
  textPrimary:   '#e8eaed',
  textSecondary: '#9aa0ac',
  textDisabled:  'rgba(255,255,255,0.25)',
  
  // Accent (Sion pink)
  accent:        '#f778ba',
  accentSoft:    'rgba(247,119,186,0.12)',
  accentGlow:    'rgba(247,119,186,0.18)',
  accentGradient:'linear-gradient(135deg, #f778ba, #e8659e)',
  
  // Semantic
  success: '#34d399',
  warning: '#fbbf24',
  danger:  '#f87171',
  info:    '#60a5fa',
  
  // Glass
  glassHeavy:  'blur(48px) saturate(180%)',
  glassMedium: 'blur(24px) saturate(160%)',
  glassLight:  'blur(18px) saturate(1.22) brightness(1.08)',
  glassCapsule:'blur(20px) saturate(1.26) brightness(1.06)',
  
  // Borders
  borderSubtle:  'rgba(255,255,255,0.06)',
  borderDefault: 'rgba(255,255,255,0.08)',
  borderAccent:  'rgba(247,119,186,0.18)',
}
```

### Typography

```
10px / 400  — 极小标签（tag, badge）
11px / 400  — 辅助文字、时间戳
12px / 400  — 正文辅助、placeholder、菜单项
13px / 400  — 正文默认阅读尺寸
14px / 600  — 小标题、section header
16px / 700  — 卡片标题
20px / 700  — 页面标题（h2）
Font: -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif
Mono: "SF Mono", "Cascadia Code", "Fira Code", monospace
```

### Spacing (4px grid)

```
4, 8, 12, 16, 20, 24, 32, 40, 48

Component inner padding: 12-16px
Card padding:            18-20px
Card gap:                10-12px
Page content padding:    24px 32px
Sidebar width:           210px
```

### Radius

```
6px   — 小元素（tag, badge, chip）
8px   — 中等元素（button, input）
10px  — 卡片、面板
14px  — 大卡片
18px  — 特大面板
999px — 药丸/胶囊/圆形按钮
```

### Shadows (depth system)

```css
--shadow-xs:  0 1px 2px rgba(0,0,0,0.2);
--shadow-sm:  0 2px 8px rgba(0,0,0,0.25);
--shadow-md:  0 4px 16px rgba(0,0,0,0.3);
--shadow-lg:  0 8px 32px rgba(0,0,0,0.35);
/* Surface highlight — N.E.K.O signature */
--highlight-top: inset 0 1px 0 rgba(255,255,255,0.06);
--glow-ring: 0 0 0 1px rgba(208,233,255,0.08);
```

### Motion

```css
--ease-spring: cubic-bezier(0.22, 1, 0.36, 1);
--ease-out:    cubic-bezier(0, 0, 0.2, 1);
--ease-in:     cubic-bezier(0.4, 0, 1, 1);

Duration:
  instant: 100ms  (hover color change)
  fast:    150ms  (hover transform, button press)
  normal:  220ms  (component enter/exit)
  slow:    300ms  (page transition)
```

### Keyframe Animations (required)

```
slideFromLeft    — message enter (assistant)
slideFromRight   — message enter (user)
glyphIn          — streaming text character reveal (90ms, opacity 0.58→1)
barIn / barOut   — capsule expand/collapse
pageIn / pageOut — page transition (scale+blur+translateY)
dotPulse         — streaming indicator (1.2s infinite)
```

---

## Step 4: Handoff

### Implementation Target

**Agent**: Vue 3 frontend engineer (this conversation)

**Framework**: Vue 3.5 Composition API + `<script setup>` + TypeScript

**Component Library**: Naive UI 2.44 — use `<n-config-provider>` with dark theme, `<n-card>`, `<n-button>`, `<n-tag>`, `<n-table>`, `<n-statistic>`, `<n-progress>`, `<n-menu>`, `<n-input>`, `<n-radio-group>`, `<n-skeleton>`, `<n-space>`, `<n-divider>`, `<n-descriptions>`

**State Management**: Pinia 3 composition stores — `useEmotionStore` (shared), per-page stores as needed

**File Structure**:
```
src/
├── assets/styles/
│   ├── variables.css    ← Design tokens as CSS custom properties
│   └── animations.css   ← Keyframe definitions
├── dashboard/
│   ├── main.ts          ← Naive UI ConfigProvider + Pinia + mount
│   ├── DashboardApp.vue ← n-config-provider + Layout
│   ├── TitleBar.vue     ← Custom titlebar
│   ├── Sidebar.vue      ← n-menu sidebar
│   ├── ChatView.vue     ← MessageList + Input
│   ├── MemoryView.vue
│   ├── EmotionView.vue
│   ├── ToolsView.vue
│   ├── ProactiveView.vue
│   ├── PersonalityView.vue
│   ├── LLMConfigView.vue
│   ├── LogsView.vue
│   └── HealthView.vue
├── pet/
│   ├── main.ts          ← Pet app mount
│   ├── PetApp.vue       ← Live2D + Capsule + Gear
│   └── FloatingCapsule.vue ← Standalone capsule component
├── components/
│   └── live2d/
│       └── Live2DCanvas.vue ← PixiJS + Cubism rendering
├── shared/
│   ├── wails.ts         ← Wails v3 Go bridge
│   ├── types.ts         ← Shared TypeScript types
│   └── stores/
│       └── emotion.ts   ← Shared emotion Pinia store
```

### Naive UI ConfigProvider Setup

```typescript
// main.ts
import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { darkTheme, zhCN, dateZhCN } from 'naive-ui'

const app = createApp(DashboardApp)
app.use(createPinia())

// Naive UI — dark theme + Chinese locale + Sion pink accent
const themeOverrides = {
  common: {
    primaryColor: '#f778ba',
    primaryColorHover: '#ff8fcb',
    primaryColorPressed: '#e8659e',
    primaryColorSuppl: '#f778ba',
  },
}
```

### Implementation Order

1. **CSS Foundation** — `variables.css` + `animations.css` with all tokens above
2. **Dashboard Shell** — `main.ts` (Naive UI setup) + `DashboardApp.vue` + `TitleBar.vue` + `Sidebar.vue`
3. **Chat Page** — Most important. `ChatView.vue` with full message list + input + streaming
4. **Data Pages** — Emotion, Tools, Health (simple display pages)
5. **Detail Pages** — Memory, Proactive, Personality, LLM, Logs
6. **Pet Window** — `PetApp.vue` + `FloatingCapsule.vue` + update `Live2DCanvas.vue`
7. **Integration** — Wire Go RuntimeService calls, test dual-window communication

### Acceptance Criteria

- [ ] Dashboard opens with TitleBar + Sidebar + Chat page
- [ ] Chat: send message → streaming response with animation
- [ ] All 9 pages navigate with page transition animation
- [ ] Sidebar active state: pink bar + pink text on current page
- [ ] StatCards render with correct colors and spacing
- [ ] Pet window: transparent background, Live2D visible, capsule functional
- [ ] Capsule: pill state → click → input state → send → pill with streaming preview
- [ ] Glassmorphism: blur effects visible on all surfaces
- [ ] All animations 60fps, obey prefers-reduced-motion
- [ ] Dark theme looks premium — deep navy, soft glass, pink accent
- [ ] TypeScript: zero errors
- [ ] Vite build: both entries (pet.html + index.html) produce correct output
