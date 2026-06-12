# Sion v2.3 — Frontend Redesign Specification

## 0. Discovery

### Target Users
- 桌面用户 (macOS/Windows)，追求桌面美化与效率工具
- AI 伴侣使用者：需要情感交互、记忆连续性、个性化配置
- 二次元/Live2D 爱好者：期望高品质猫娘互动体验

### Product Goal
Sion 是一个 AI 猫娘桌面伴侣 — Pet 窗口提供 Live2D 视觉陪伴与聊天交互，Dashboard 提供完整的控制与管理能力。

### Device & Usage Context
- **设备**: 桌面 (macOS/Windows)，非移动设备
- **窗口**: Pet 窗口常驻桌面（透明无边框，Always-on-Top 可选），Dashboard 按需打开
- **交互模式**: Pet 窗口轻量即时聊天 + Dashboard 深度配置管理
- **技术约束**: Wails v3 壳提供窗口管理，Go 后端处理 LLM/记忆/情绪，前端纯 SPA

### Design System Constraints
- Naive UI 2.44 为基础组件库，darkTheme + 自定义 themeOverrides
- Pet 窗口脱离 Naive UI 体系，纯手写 CSS 玻璃态
- Dashboard 窗口混合 Naive UI 组件 + 自定义玻璃态样式
- 仅桌面端，无需响应式断点（窗口最小 900×600 即可）

### Accessibility Baseline
- 键盘导航 (Tab/Enter/Escape/方向键)
- 屏幕阅读器友好标签 (aria-label on glass elements)
- 色彩对比度 ≥ 4.5:1 (正文) / 3:1 (大文本/UI)

---

## 1. Flows & States

### Flow A: Chat Interaction (Pet Window)

```
[Pet Window Open]
      │
      ▼
┌─────────────────────┐
│  Live2D Idle State  │  ← Mao 模型呼吸动画 + 随机动作
│  Capsule: Collapsed │  ← 底部居中胶囊，显示 "和 Sion 说点什么..."
└─────────────────────┘
      │ 用户点击胶囊 / 打字触发
      ▼
┌─────────────────────┐
│  Capsule: Expanded  │  ← 胶囊展开为输入区，Live2D 上移 40px
│  输入框 + 发送按钮   │
└─────────────────────┘
      │ 用户输入 + 回车/点击发送
      ▼
┌─────────────────────┐
│  Capsule: Streaming │  ← 发送按钮变为止损按钮 [⏹]
│  AI 文字逐字揭示     │  ← 粉色光标闪烁，Live2D 切换说话表情
└─────────────────────┘
      │ 流式完成 / 用户终止
      ▼
┌─────────────────────┐
│  AI Response Done   │  ← 文字渐隐 → 胶囊回缩为 Collapsed
│  Live2D 表情同步    │  ← Mao 切换到对应 emotion 表情
└─────────────────────┘
```

#### States
| State | Capsule Visual | Live2D | Input |
|-------|---------------|--------|-------|
| **Idle** | 折叠药丸，半透明玻璃，placeholder 文字 | 呼吸动画 + 随机动作 | 隐藏 |
| **Focused** | 胶囊展开，高度增加，blur(48px) 全效 | 上移 40px，注视状态 | 输入框可见，聚焦 |
| **Streaming** | 文字逐字出现，粉色光标 1s blink | 说话嘴型 + emotion 表情 | 变为 Stop 按钮 |
| **Error** | 错误文字红色，3s 后回 Idle | 困惑表情 | 恢复发送 |
| **Empty** | 同 Idle | 同 Idle | — |

#### Decision Points
- **发送判断**: 空输入 → 不发送，保持 Expanded；有内容 → 进入 Streaming
- **终止判断**: 用户点击 Stop → 显示已生成部分，标记 `source: "interrupted"`；流式完成 → 完整展示
- **Emotion 同步**: `chatResponse.emotion` 映射到 Mao 表情集 (exp_01 ~ exp_08)

---

### Flow B: Dashboard Navigation

```
[Dashboard Window Open]
      │
      ▼
┌─────────────────────┐
│  Custom Title Bar   │  ← 窗口标题 "Sion" + 最小化/最大化/关闭
├──────────┬──────────┤
│ Sidebar  │ Content  │  ← 默认 Chat 页面
│ 9 nav    │ Area     │
│ items    │          │
└──────────┴──────────┘
```

#### Sidebar Navigation Items (9 Pages)
1. **Chat** — 聊天记录历史，左侧会话列表 + 右侧消息区
2. **Memory** — 记忆管理，卡片网格展示 AI 记忆条目，支持 CRUD
3. **Emotion** — 情绪可视化，VAD 空间 3D 散点 + 时序折线图
4. **Tools** — 工具/MCP 管理，工具列表 + 启用/禁用 toggle
5. **Proactive** — 主动交互规则配置，触发条件 + 动作编辑器
6. **Personality** — 人设编辑器，系统提示词 + 特征 slider
7. **LLM Config** — 模型配置，Provider/Model/API Key/参数
8. **Logs** — 运行日志，虚拟滚动日志列表 + 级别过滤
9. **Health** — 系统健康，CPU/内存/WS 连接状态 + 组件状态面板

#### State: Each Page
Each page must handle:

| State | Design |
|-------|--------|
| **Loading** | 骨架屏 (Naive UI Skeleton) + 毛玻璃卡片 |
| **Empty** | 居中插图 + 引导文字 + 操作按钮 |
| **Data** | 数据正常展示 |
| **Error** | 错误卡片 + 重试按钮 + 错误详情折叠 |
| **Streaming** (Chat only) | AI 回复逐字揭示 + 停止按钮 |

---

### Flow C: Configuration Changes (LLM Config, Personality, etc.)

```
[Config Page]
      │
      ▼
┌─────────────────────┐
│  表单 / 配置区       │  ← Naive UI Form + Input/Select/Slider
│  "保存" 按钮 disabled │  ← 无变更时
└─────────────────────┘
      │ 用户修改任意字段
      ▼
┌─────────────────────┐
│  "保存" 按钮 enabled  │  ← 粉色主色，表示有未保存变更
│  "重置" 按钮出现     │
└─────────────────────┘
      │ 点击保存
      ▼
┌─────────────────────┐
│  保存中 (Loading)    │  ← 按钮 loading spinner
└─────────────────────┘
      │
      ├── Success → Toast 顶部弹出 "已保存" ✓ → 按钮恢复 disabled
      └── Error   → Toast 顶部弹出错误信息 → 按钮保持 enabled
```

---

## 2. Component Specifications

### 2.1 Pet Window Components

---

#### Component: ChatCapsule

**Purpose**: Pet 窗口底部聊天交互胶囊 — 折叠/展开/流式三态药丸

**Variants**:
- `collapsed`: 36px 高，200px 宽药丸，placeholder 文字 "和 Sion 说点什么..."
- `expanded`: 展开为输入区，包含输入框 + 发送按钮
- `streaming`: AI 回复逐字揭示 + 停止按钮

**Props**:
```typescript
interface ChatCapsuleProps {
  state: 'collapsed' | 'expanded' | 'streaming'
  streamingText?: string
  isStreaming?: boolean
  error?: string | null
}

interface ChatCapsuleEmits {
  (e: 'send', text: string): void
  (e: 'stop'): void
  (e: 'focus'): void
  (e: 'blur'): void
}
```

**States**:
| State | Visual | Behavior |
|-------|--------|----------|
| Collapsed | 36px 高药丸，rgba(255,255,255,0.45) + blur(48px)，文字 #34536c | 点击展开 |
| Expanded | 高度自适应(48-200px)，输入框 + 粉色发送按钮 | 回车/点击发送 |
| Streaming | 文字逐字 reveal，粉色光标 blink(1s)，发送→停止按钮 | 点击停止 |
| Error | 错误文字 #ef4444，3s 自动清除 | 同 Expanded |

**3-Layer Pseudo-Element Glass**:
```css
/* Layer 1: Base background */
background: rgba(255, 255, 255, 0.45);
/* Layer 2: ::before — blur layer */
backdrop-filter: blur(48px);
-webkit-backdrop-filter: blur(48px);
/* Layer 3: ::after — subtle inner highlight */
box-shadow: 
  inset 0 0.5px 1px rgba(255,255,255,0.6),   /* top highlight */
  inset 0 -0.5px 1px rgba(0,0,0,0.04),        /* bottom shadow */
  0 2px 16px rgba(0,0,0,0.08),                /* outer shadow */
  0 0 0 1px rgba(255,255,255,0.2);            /* outer ring */
```

**Animations**:
| Trigger | Animation | Duration | Easing |
|---------|-----------|----------|--------|
| expand | height 48-200px, width 200-360px, opacity 0.45→0.65 | 250ms | cubic-bezier(0.22,1,0.36,1) |
| collapse | reverse expand | 200ms | same |
| char reveal | opacity 0→1, translateY(2px→0) | 50ms/char | ease-out |
| cursor blink | opacity 1→0→1 | 1s loop | steps(1) |
| send→stop icon | rotate + crossfade | 200ms | cubic-bezier(0.22,1,0.36,1) |

**Edge Cases**:
- 超长输入: 输入框自动增高到 200px 上限，超出滚动
- 流式中途切换窗口: 保持 streaming 状态，不中断
- 透明窗口点击穿透: 胶囊区域必须 `pointer-events: auto`

---

#### Component: PetLive2D

**Purpose**: PixiJS + Cubism SDK 渲染 Mao Live2D 模型

**Props**:
```typescript
interface PetLive2DProps {
  emotion?: string       // 映射到 exp_01 ~ exp_08
  motion?: string        // 随机/指定动作
  talking?: boolean      // 嘴型开关
}
```

**States**:
| State | Visual | Trigger |
|-------|--------|---------|
| Idle | 呼吸动画，随机动作 (mtn_01~04) | 无交互 5s 后 |
| Listening | 注视状态，头微倾 | 胶囊 Expanded |
| Speaking | 嘴型动画 + emotion 表情 | Streaming |
| Reacting | 特殊动作 (special_01~03) | 情绪突变 / 主动触发 |

**Implementation Notes**:
- 初始化 PixiJS Application，透明背景，`width: 400, height: 500`
- 模型路径: `/model/Mao/Mao.model3.json`
- 纹理目录: `/model/Mao/Mao.2048/`
- 表情映射: emotion → exp 编号 (后端返回的 primary emotion → expression index)
- 动作池: mtn_01~04 随机，special_01~03 条件触发

---

### 2.2 Dashboard Window Components

---

#### Component: TitleBar

**Purpose**: 自定义窗口标题栏，替代系统原生标题栏

**Props**:
```typescript
interface TitleBarProps {
  title?: string        // default: "Sion"
  alwaysOnTop?: boolean
}
```

**Layout**:
```
┌──────────────────────────────────────────────┐
│  ● Sion                    [📌] [─] [□] [✕] │
└──────────────────────────────────────────────┘
```
- 左侧: 拖拽区域 + 应用名称
- 右侧: Always-on-Top 图钉 / 最小化 / 最大化 / 关闭
- 高度: 38px
- 背景: rgba(255,255,255,0.65) + blur(32px)
- `-webkit-app-region: drag` on bar, `no-drag` on buttons

---

#### Component: Sidebar

**Purpose**: 9 页面导航侧边栏

**Props**:
```typescript
interface SidebarProps {
  activeKey: string
  collapsed?: boolean    // 可折叠为仅图标，default false
}
```

**Visual Spec**:
```
┌──────────────┐
│  💬 Chat     │  ← active: 左边粉色 3px 条 + bg rgba(247,119,186,0.12)
│  🧠 Memory   │  ← hover: 右移 4px + bg rgba(0,0,0,0.04)
│  💗 Emotion  │
│  🔧 Tools    │
│  🎯 Proactive│
│  🎭 Personality│
│  ⚙️ LLM Config│
│  📋 Logs     │
│  ❤️ Health   │
└──────────────┘
```
- 宽度: 200px
- 背景: rgba(255,255,255,0.55) + blur(32px)
- 右侧 1px 分割线: rgba(0,0,0,0.06)
- 导航项: 44px 高, 12px 圆角, 12px 左右 padding
- Active 指示器: 左侧 3px #f778ba, 背景 rgba(247,119,186,0.12)
- Hover: translateX(4px), 背景 rgba(0,0,0,0.04)
- 图标: 18px, #596579 (inactive) → #f778ba (active)
- 文字: 14px, #1f2329 (active) / #596579 (inactive)

**Animations**:
| Trigger | Animation | Duration | Easing |
|---------|-----------|----------|--------|
| hover | translateX(0→4px), bg fade | 200ms | cubic-bezier(0.22,1,0.36,1) |
| active change | 粉色条 slide + bg fade | 200ms | same |
| collapse/expand | width 200↔64px | 250ms | cubic-bezier(0.22,1,0.36,1) |

---

#### Component: GlassCard

**Purpose**: 通用毛玻璃卡片容器，Dashboard 内容区主要容器

**Props**:
```typescript
interface GlassCardProps {
  padding?: 'sm' | 'md' | 'lg'   // default: 'md' = 24px
  radius?: 'md' | 'lg' | 'xl'    // default: 'lg' = 16px
  blur?: 'light' | 'heavy'       // default: 'heavy' = blur(48px)
  loading?: boolean
  empty?: boolean
  emptyText?: string
  error?: string | null
  onRetry?: () => void
}
```

**States**:
| State | Visual |
|-------|--------|
| Default | rgba(255,255,255,0.75) + blur(48px) + 16px radius |
| Loading | 骨架屏 (Naive UI Skeleton) 透明度叠加 |
| Empty | 居中: 60px 灰色图标 + "暂无数据" + 可选操作按钮 |
| Error | 红色左边框 + 错误文字 + 重试按钮 |

**Glass Spec**:
```css
background: rgba(255, 255, 255, 0.75);
backdrop-filter: blur(48px) saturate(1.2);
-webkit-backdrop-filter: blur(48px) saturate(1.2);
border-radius: 16px;
box-shadow: 
  0 2px 24px rgba(0,0,0,0.06),
  0 0 0 0.5px rgba(255,255,255,0.4) inset;
```

---

#### Component: ChatBubble

**Purpose**: 聊天消息气泡

**Variants**:
- `user`: 浅蓝渐变，靠右排列
- `ai`: 白色 + 阴影，靠左排列
- `system`: 居中灰色小字

**Props**:
```typescript
interface ChatBubbleProps {
  role: 'user' | 'ai' | 'system'
  content: string
  timestamp?: number
  streaming?: boolean
  emotion?: string
}
```

**Visual Spec — User Bubble**:
```css
background: linear-gradient(135deg, #667eea, #764ba2);
color: #ffffff;
border-radius: 18px 18px 4px 18px;  /* 右下小角 */
max-width: 75%;
align-self: flex-end;
box-shadow: 0 2px 8px rgba(102,126,234,0.3);
```

**Visual Spec — AI Bubble**:
```css
background: rgba(255,255,255,0.85);
color: #1f2329;
border-radius: 18px 18px 18px 4px;  /* 左下小角 */
max-width: 75%;
align-self: flex-start;
box-shadow: 0 1px 4px rgba(0,0,0,0.08);
backdrop-filter: blur(24px);
```

**Animations**:
| Trigger | Animation | Duration |
|---------|-----------|----------|
| Enter (new bubble) | slideIn: translateY(12px→0), opacity 0→1 | 300ms |
| Streaming | 逐字 reveal (同 ChatCapsule) | 50ms/char |

---

#### Component: EmotionChart

**Purpose**: 情绪 VAD (Valence-Arousal-Dominance) 可视化

**Sub-components**:
- `VADScatter3D`: 3D 散点图 (可旋转) — 使用 ECharts/CSS 3D
- `EmotionTimeline`: 时序折线图 — 横轴时间，纵轴强度
- `EmotionBadge`: 当前主情绪标签 — 粉色胶囊

**States**: Loading / Empty (无历史数据) / Data / Error

---

#### Component: ConfigForm

**Purpose**: 配置表单通用组件 (复用于 LLM Config, Personality, Proactive)

**States**: Default (保存 disabled) / Dirty (保存 enabled) / Saving / Saved ✓ / Error

---

## 3. Design Tokens

### Colors

```typescript
const colors = {
  // Brand / Accent
  accent: {
    DEFAULT: '#f778ba',
    hover: '#f960ae',
    active: '#e6509e',
    muted: 'rgba(247,119,186,0.12)',  // subtle bg
  },

  // Glass Backgrounds
  glass: {
    light: 'rgba(255,255,255,0.45)',   // Pet capsule
    card: 'rgba(255,255,255,0.75)',    // Dashboard cards
    sidebar: 'rgba(255,255,255,0.55)', // Sidebar
    titlebar: 'rgba(255,255,255,0.65)',// Title bar
    hover: 'rgba(0,0,0,0.04)',         // Sidebar hover
    active: 'rgba(247,119,186,0.12)',  // Sidebar active
  },

  // Text
  text: {
    primary: '#1f2329',
    secondary: '#596579',
    tertiary: '#8b949e',
    capsule: '#34536c',                // Pet capsule text
    inverse: '#ffffff',                 // On accent/dark bg
  },

  // Semantic
  semantic: {
    success: '#34d399',
    warning: '#f59e0b',
    danger: '#ef4444',
    info: '#3b82f6',
  },

  // Chat Bubbles
  bubble: {
    user: 'linear-gradient(135deg, #667eea, #764ba2)',
    ai: '#2d2d3c',    // Actually: rgba(255,255,255,0.85) glass
    system: '#8b949e',
  },

  // Shadows
  shadow: {
    capsule: '0 2px 16px rgba(0,0,0,0.08), 0 0 0 1px rgba(255,255,255,0.2)',
    card: '0 2px 24px rgba(0,0,0,0.06)',
    sidebar: '1px 0 0 rgba(0,0,0,0.06)',
    button: '0 1px 3px rgba(0,0,0,0.08)',
  },
}
```

### Typography

```typescript
const typography = {
  fontFamily: {
    sans: ['Inter', '-apple-system', 'BlinkMacSystemFont', 'Segoe UI', 'PingFang SC', 'Microsoft YaHei', 'sans-serif'],
    mono: ['JetBrains Mono', 'SF Mono', 'Cascadia Code', 'monospace'],
  },
  fontSize: {
    xs: '12px',    // Caption, timestamps
    sm: '13px',    // Sidebar items, secondary text
    base: '14px',  // Body text, chat messages
    lg: '16px',    // Card titles
    xl: '20px',    // Page titles
    '2xl': '28px', // Dashboard hero
  },
  fontWeight: {
    normal: '400',
    medium: '500',
    semibold: '600',
  },
  lineHeight: {
    tight: '1.25',
    normal: '1.5',
    relaxed: '1.75',
  },
}
```

### Spacing Rhythm

```typescript
const spacing = {
  unit: 4,              // Base unit in px
  sidebarWidth: 200,    // Sidebar width in px
  sidebarCollapsed: 64,
  titleBarHeight: 38,
  pagePadding: 32,      // Content area padding
  cardPadding: 24,      // Card internal padding
  cardGap: 16,          // Card grid gap
  navItemHeight: 44,    // Sidebar item height
}
```

### Motion

```typescript
const motion = {
  duration: {
    fast: 150,
    normal: 200,
    slow: 300,
    emphasis: 500,
  },
  easing: {
    standard: 'cubic-bezier(0.22, 1, 0.36, 1)', // Apple-style overshoot ease
    enter: 'cubic-bezier(0, 0, 0.2, 1)',         // ease-out
    exit: 'cubic-bezier(0.4, 0, 1, 1)',           // ease-in
  },
}
```

### Border Radius

```typescript
const radius = {
  sm: '6px',
  md: '10px',
  lg: '16px',       // Cards
  xl: '24px',        // Large containers
  full: '9999px',    // Capsules, pills, avatar
}
```

### Glass Levels

```typescript
const glass = {
  none: { bg: 'rgba(255,255,255,1)', blur: '0px' },
  subtle: { bg: 'rgba(255,255,255,0.85)', blur: '16px' },
  standard: { bg: 'rgba(255,255,255,0.65)', blur: '32px' },
  heavy: { bg: 'rgba(255,255,255,0.45)', blur: '48px' },   // N.E.K.O style
  extreme: { bg: 'rgba(255,255,255,0.25)', blur: '64px' },
}
```

### Z-Index Layers

```typescript
const zIndex = {
  base: 0,
  content: 10,
  sidebar: 20,
  titlebar: 30,
  capsule: 40,        // Pet capsule floats above Live2D
  dropdown: 50,
  modal: 60,
  toast: 70,
}
```

---

## 4. Naive UI Theme Overrides

基于 `darkTheme` 预设，覆盖为 Apple 白色毛玻璃风格：

```typescript
const themeOverrides = {
  common: {
    fontFamily: 'Inter, -apple-system, PingFang SC, Microsoft YaHei, sans-serif',
    // 将 darkTheme 的暗色反转为亮色毛玻璃
    bodyColor: '#ffffff00',           // 页面透明 → 透壁纸
    cardColor: 'rgba(255,255,255,0.75)',
    modalColor: 'rgba(255,255,255,0.85)',
    popoverColor: 'rgba(255,255,255,0.85)',
    // Text
    textColor1: '#1f2329',
    textColor2: '#596579',
    textColor3: '#8b949e',
    // Borders
    borderColor: 'rgba(0,0,0,0.06)',
    dividerColor: 'rgba(0,0,0,0.04)',
    // Accent
    primaryColor: '#f778ba',
    primaryColorHover: '#f960ae',
    primaryColorPressed: '#e6509e',
    primaryColorSuppl: '#f778ba',
    // Semantic
    successColor: '#34d399',
    warningColor: '#f59e0b',
    errorColor: '#ef4444',
    infoColor: '#3b82f6',
    // Misc
    borderRadius: '12px',
    fontSize: '14px',
    heightSmall: '28px',
    heightMedium: '36px',
    heightLarge: '44px',
  },
  // Form components
  Input: {
    color: 'rgba(255,255,255,0.65)',
    colorFocus: 'rgba(255,255,255,0.85)',
    border: 'rgba(0,0,0,0.06)',
    borderFocus: '#f778ba',
    borderHover: 'rgba(0,0,0,0.12)',
    borderRadius: '10px',
    boxShadowFocus: '0 0 0 2px rgba(247,119,186,0.25)',
  },
  Button: {
    borderRadiusMedium: '10px',
  },
  Slider: {
    fill: '#f778ba',
    fillHover: '#f960ae',
  },
  Tabs: {
    tabTextColorActiveLine: '#f778ba',
    tabTextColorHoverLine: '#f960ae',
    barColor: '#f778ba',
  },
}
```

---

## 5. File Structure

```
frontend/src/
├── dashboard/
│   ├── main.ts                      # Dashboard 入口，创建 Vue app + Pinia + Naive UI
│   ├── App.vue                      # 根布局: TitleBar + Sidebar + Router
│   ├── router.ts                    # Vue Router (9 routes)
│   ├── stores/
│   │   ├── chat.ts                  # 聊天状态 (消息列表, 流式状态)
│   │   ├── emotion.ts               # 情绪数据
│   │   ├── memory.ts                # 记忆数据
│   │   ├── config.ts                # LLM/Personality 配置
│   │   ├── tools.ts                 # 工具/MCP 状态
│   │   ├── proactive.ts             # 主动交互规则
│   │   ├── logs.ts                  # 日志数据
│   │   └── health.ts                # 系统健康
│   ├── views/
│   │   ├── ChatView.vue
│   │   ├── MemoryView.vue
│   │   ├── EmotionView.vue
│   │   ├── ToolsView.vue
│   │   ├── ProactiveView.vue
│   │   ├── PersonalityView.vue
│   │   ├── LLMConfigView.vue
│   │   ├── LogsView.vue
│   │   └── HealthView.vue
│   └── components/
│       ├── TitleBar.vue
│       ├── Sidebar.vue
│       ├── GlassCard.vue
│       ├── ChatBubble.vue
│       ├── ConfigForm.vue
│       ├── StatusBadge.vue
│       └── SkeletonLoader.vue
├── pet/
│   ├── main.ts                      # Pet 窗口入口
│   ├── App.vue                      # PetView + ChatCapsule 布局
│   └── components/
│       ├── PetLive2D.vue            # PixiJS + Cubism 渲染
│       ├── ChatCapsule.vue          # 三层玻璃胶囊
│       └── StreamingText.vue        # 逐字揭示文本
├── shared/
│   ├── types.ts                     # 共享 TypeScript 类型
│   ├── constants.ts                 # 颜色、配置常量
│   ├── api.ts                       # Wails Go 绑定封装
│   └── composables/
│       ├── useChat.ts               # 聊天逻辑 (发送/接收/流式)
│       ├── useEmotion.ts            # 情绪数据拉取
│       └── useGlass.ts              # 玻璃态工具 (blur 强度计算)
└── styles/
    ├── variables.css                # CSS 自定义属性
    ├── glass.css                    # 玻璃态 mixin 类
    ├── animations.css               # @keyframes 动画
    └── global.css                   # 全局重置 + 基础样式
```

---

## 6. Implementation Handoff

### Target
- **Framework**: Vue 3.5 SPA (纯客户端渲染，Wails v3 壳)
- **Agent**: Manual implementation (非 React/Next.js)
- **UI Library**: Naive UI 2.44 + 手写 CSS 玻璃态
- **State**: Pinia 3 stores
- **Routing**: Vue Router 4 (hash mode for Wails file:// compatibility)

### Implementation Phases

**Phase 1 — Foundation**
1. CSS variables + glass utilities in `src/styles/`
2. Shared types + Wails API encapsulation in `src/shared/`
3. Naive UI ConfigProvider with themeOverrides

**Phase 2 — Dashboard Shell**
4. TitleBar + Sidebar layout
5. Vue Router with 9 route placeholders
6. GlassCard component
7. Pinia stores scaffold

**Phase 3 — Pet Window**
8. PetLive2D component (PixiJS init + Cubism SDK)
9. ChatCapsule (3-layer glass, 3 states)
10. StreamingText (char-by-char reveal)

**Phase 4 — Dashboard Pages**
11. ChatView (message list + ChatBubble + streaming)
12. MemoryView (card grid + CRUD)
13. EmotionView (EmotionChart + timeline)
14. ToolsView (toggle list)
15. ProactiveView (rule editor)
16. PersonalityView (prompt editor + sliders)
17. LLMConfigView (provider/model/params form)
18. LogsView (virtual scroll log list)
19. HealthView (status cards + metrics)

**Phase 5 — Polish**
20. All loading/empty/error states
21. 60fps animation tuning
22. Keyboard navigation
23. aria-label audit

### Acceptance Criteria
- [ ] Pet 窗口: Live2D 模型正确渲染，3 表情+动作切换流畅
- [ ] Pet 窗口: 胶囊 3 态切换动画 60fps，流式文字逐字揭示
- [ ] Dashboard: 侧边栏 9 页导航，hover/active 动画完整
- [ ] Dashboard: 每页 loading/empty/error/data 四态覆盖
- [ ] 毛玻璃: blur(48px) 在所有页面生效，壁纸透视可感知
- [ ] 粉色 #f778ba 仅用于交互元素，无滥用
- [ ] Naive UI 组件风格与自定义玻璃态视觉一致
- [ ] 所有动画 cubic-bezier(0.22,1,0.36,1) 150-300ms
- [ ] 键盘 Tab/Enter/Escape 导航正常
- [ ] Wails Go API 调用正常 (chat/emotion)
