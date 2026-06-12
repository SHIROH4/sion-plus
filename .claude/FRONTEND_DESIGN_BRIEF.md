# Sion v2.3 Frontend Design Brief

## 产品背景

Sion 是一只 AI 猫娘桌面伴侣。她住在你的桌面上，有 Live2D 形象，能聊天、能感知你的屏幕活动、会主动关心你。

**两个窗口**：
- **Pet 窗口**（常驻）—— 透明无边框，猫娘 Live2D 形象浮在桌面上，底下一个聊天胶囊
- **Dashboard 窗口**（按需打开）—— 控制台，管理对话、记忆、情绪、工具、配置

## 审美方向

> 融合 Apple 的克制与 Google 的活力，以 N.E.K.O 的暗色玻璃态为载体。

**Apple — 学它的克制**：
- 大量留白，不要让元素挤在一起
- 毛玻璃效果（frosted glass），半透明表面叠加在深色背景上
- 字体层级清晰：标题 bold、正文 regular、辅助文字更小更淡
- 圆角统一且柔和（大圆角用于卡片，全圆角用于胶囊/按钮）
- 阴影用来暗示层级，不是装饰——近的元素阴影更浅更大，远的更深更小
- 动画有意图，不是随意弹跳：ease-out 入场、快速 hover 反馈、不打断用户

**Google Material Design 3 — 学它的色彩与表面**：
- 暗色主题不是纯黑，是有层次的海军蓝/深灰
- 表面（surface）有明确的高度层级——越靠近用户的越亮
- 主色调鲜明但克制——Sion 的粉色作为点缀，大面积留白
- 状态标签（success/warning/error）用语义色，对比清晰
- 图标简洁、几何化（Naive UI 自带）

**N.E.K.O — 学它的玻璃态和桌面感**:
- `backdrop-filter: blur(48px) saturate(180%)` 是它看起来"高级"的核心
- 每块表面顶部有一道微弱的 inset 高光 (`inset 0 1px 0 rgba(255,255,255,0.06)`)
- 外圈发光环 (`0 0 0 1px rgba(208,233,255,0.08)`) 让元素"浮"在背景上
- 胶囊药丸输入框是桌面伴侣的标志性交互

## 设计原则

1. **留白 > 填充**。不确定加什么时，加留白。
2. **玻璃 > 实色**。桌面应用的优势——能看到壁纸透过来，不要浪费。
3. **暗示 > 说明**。用阴影、模糊、透明度来表达层级，不要画边框。
4. **一种主色**。Sion 粉（#f778ba）用于交互元素和强调。其他用灰度。
5. **状态可见**。loading、empty、error、streaming 每个状态都要设计，不能留白屏。
6. **60fps 动画**。所有过渡用 `cubic-bezier(0.22, 1, 0.36, 1)`，150-300ms。没有低于 100ms 或超过 500ms 的。

## 色彩系统

```
底色层级（从深到浅）：
  Page bg:      #1a1a2e   ← 最底层，页面背景
  Card bg:      #252538   ← 卡片、面板
  Hover bg:     rgba(255,255,255,0.04)  ← hover 高亮

文字层级（从亮到暗）：
  Primary:      #e8eaed   ← 标题、正文
  Secondary:    #9aa0ac   ← 辅助文字、标签
  Disabled:     rgba(255,255,255,0.25)  ← 占位符、不可用文字

强调色（Sion 粉）：
  Primary:      #f778ba   ← 按钮、链接、选中态
  Soft:         rgba(247,119,186,0.12)  ← 选中背景
  Glow:         rgba(247,119,186,0.18)  ← 发光效果

语义色：
  Success:      #34d399   ← 成功、运行中
  Warning:      #fbbf24   ← 警告、降级
  Danger:       #f87171   ← 错误、危险操作
  Info:         #60a5fa   ← 中性信息

玻璃态：
  Heavy:        blur(48px) saturate(180%)   ← 大表面（标题栏、侧边栏）
  Medium:       blur(24px) saturate(160%)   ← 卡片、面板
  Light:        blur(18px) saturate(1.22) brightness(1.08)  ← 胶囊
```

## 字体阶梯

```
10px  — 极小标签、时间戳
11px  — 辅助文字、徽标
12px  — 正文辅助、placeholder
13px  — 正文（默认阅读尺寸）
14px  — 正文加强、菜单项
16px  — 小标题
18px  — 卡片标题
20px  — 页面标题
24px  — 大标题（Dashboard 页面名）
```

## 间距系统

基于 4px 网格：4, 8, 12, 16, 20, 24, 32, 40, 48

- 组件内部 padding: 12-16px
- 卡片 padding: 20px
- 卡片间距: 12px
- 页面内容区 padding: 24-32px
- 侧边栏宽度: 220px
- 聊天面板宽度: 380px
- Pet 窗口: 400×500

## Pet 窗口设计

### 布局
```
┌──────────────────────┐
│                      │
│                      │
│    Live2D Mao 模型    │  ← 透明背景，猫娘浮在壁纸上
│    (PixiJS 渲染)      │
│                      │
│                      │
│  ┌────────────────┐  │
│  │ 和 Sion 说点什么… │  │  ← 玻璃态胶囊药丸（默认状态）
│  └────────────────┘  │
│   🧶             ⚙   │  ← 最小化球 + 齿轮（hover 显示）
└──────────────────────┘
```

### 胶囊交互
- **默认态**: 圆角 999px 药丸，半透明玻璃态，显示 "和 Sion 说点什么…" 或流式回复预览
- **输入态**: 点击展开为输入框 + 发送按钮，粉色渐变圆形按钮
- **流式态**: 药丸显示逐字出现的最新回复（glyph-in 90ms 动画），右侧 ... 脉冲点
- **hover**: 药丸轻微放大（scale 1.01）+ 阴影加深 + 粉色发光边框出现
- **发送**: 输入框缩回药丸，药丸上显示流式预览

### 视觉效果
- 胶囊背景: `linear-gradient(180deg, rgba(31,48,66,0.72), rgba(8,17,30,0.62))`
- 胶囊模糊: `blur(20px) saturate(1.26) brightness(1.06)`
- 胶囊阴影: `0 0 0 1px rgba(208,233,255,0.08), 0 4px 20px rgba(0,0,0,0.35)`
- 内高光: `linear-gradient(180deg, rgba(255,255,255,0.1), transparent 45%)`
- 发送按钮: 粉色渐变圆形，hover 放大 1.06，按下缩小 0.95
- 最小化球: 30px 毛线球 emoji，hover 变粉色 + scale(1.12)
- 齿轮: 30px 半透明圆形，hover 变粉色

## Dashboard 窗口设计

### 布局
```
┌─ TitleBar (38px, 玻璃态, 可拖拽) ──────────────────────┐
│ 🐱 Sion Dashboard                      ─ □ ✕           │
├─ Sidebar (220px) ─┬─ Content Area ────────────────────┤
│                   │                                    │
│ 🐱 Sion v2.3      │  [当前选中页面的内容]                │
│ ─────────         │                                    │
│ 💬 Chat      ← active, 左边粉色 3px 条                 │
│ 🧠 Memory          │  ┌──────────────────────────────┐ │
│ 💗 Emotion         │  │  el-card                      │ │
│ 🔧 Tools           │  │  标题: Chat      12 messages  │ │
│ 💡 Proactive       │  │  ─────────────────────────── │ │
│ ✨ Personality     │  │  消息列表...                   │ │
│ ⚡ LLM Config      │  │  输入框           [发送]      │ │
│ 📋 Logs            │  └──────────────────────────────┘ │
│ 📊 Health          │                                    │
│ ─────────         │                                    │
│ ● neutral V0 A0   │                                    │
└────────────────────┴───────────────────────────────────┘
```

### 标题栏
- 38px 高，玻璃态 `blur(48px) saturate(180%)`
- 左侧: 🐱 + "Sion Dashboard" (13px, weight 650, 文字阴影)
- 右侧: 最小化/最大化/关闭按钮，CSS 绘制，hover 时亮起
- 整个标题栏 `-webkit-app-region: drag`，可拖拽窗口

### 侧边栏
- 220px 宽，玻璃态 `blur(24px) saturate(160%)`
- 顶部: Sion logo + 版本号
- 中部: 9 个导航项（Naive UI menu 组件）
  - 每个 item: 10px 间距 + 图标 + 文字
  - hover: 背景高亮 + translateX(2px) 右移
  - active: 粉色背景 + 左边 3px 粉色条 + 文字变粉
- 底部: 情绪指示器（圆点 + 情绪名 + VAD 值）

### 内容区
- Page bg 作为底色
- 每个页面包裹在 Naive UI `<n-card>` 中，玻璃态
- 页面标题: 20px, weight 700
- 统计数据用 `<n-statistic>`, 4 列 grid
- 按钮使用 Naive UI `<n-button>`, 粉色 primary
- 标签使用 `<n-tag>`, 语义色

### 9 个页面概要

**Chat**
- 消息列表（用户蓝色渐变气泡，AI 深灰气泡）
- 消息分组：同一角色 30s 内合并
- 每条消息：角色名 + 时间戳 + 正文
- 自带消息入场动画（user 从右滑入, AI 从左滑入）
- 底部输入区：textarea + 粉色发送按钮
- 流式响应：左边粉色 2px 边框 + ... 脉冲动画

**Memory**
- 4 个统计数字（Evidence / Reflections / Tier / Last Consolidation）
- 证据和反思列表：每条左边有类型标签（evidence=蓝, fact=绿, pattern=粉, reflection=黄），置信度百分比

**Emotion**
- 大情绪指示器：32px 彩色圆点 + 发光 + 情绪名 + 强度 %
- VAD 三维度进度条（valence 粉色，arousal 蓝色，dominance 粉色）
- 8 个情绪维度标签（joy, sadness, anger, fear, surprise, sleepy, worried, curious）

**Tools**
- 7 个已注册工具列表，每个显示名称(typewriter 字体)、描述、安全标签(safe=绿/danger=红)
- 危险工具有左边红色 2px 边框
- Rate Limiter 进度条（粉色渐变）

**Proactive**
- 4 个统计数字（Tick / Threshold / Today / Delivered）
- 模式切换（普通/频繁/专注/关闭），用 radio button
- 决策历史表格（Action 标签 / Reason / Result 标签 / Time）

**Personality**
- System Prompt 编辑器（textarea, 等宽字体，只读）
- Persona 状态列表（Tier, 级联清理, 上次重建, 证据引擎）

**LLM Config**
- 4 个统计（Tokens Today / Global Limit / Providers / Channels）
- 环境变量列表（KEY: value 形式，密钥掩码显示）
- 7 个路由通道标签

**Logs**
- 日志级别过滤器（All / Info / Warn / Error）
- 日志表格（时间 / 级别标签 / 模块 / 消息）
- Error 行红色背景，Warn 行黄色背景

**Health**
- 4 个实时统计（Uptime / Goroutines / Memory / Status）
- 9 个模块健康状态列表（ok=绿, degraded=黄）
- 构建信息（Version / Wails / Go / Platform）

## 状态设计

每个需要数据的区域必须覆盖以下状态：

| 状态 | 处理方式 |
|------|---------|
| **Loading** | 骨架屏（Naive UI `<n-skeleton>`）或轻量 spinner，不是空白 |
| **Empty** | 友好的空状态提示文字，带 emoji |
| **Error** | 紧凑错误提示 + 重试按钮 |
| **Streaming** | 逐字动画 + 闪烁光标或脉冲点 |

## 动画规范

- **入场**: 150-250ms ease-out，轻微 scale(0.98→1) + translateY(4px→0) + opacity fade
- **hover**: 100-150ms，translateY(-1px) + 阴影加深
- **按下**: scale(0.97) 50ms
- **页面切换**: 300ms，scale(0.98)+blur(4px) → scale(1)+blur(0)
- **流式文字**: 每字 90ms opacity 0→1
- **dotPulse**: 1.2s infinite，3 个点依次弹跳
- **遵守 `prefers-reduced-motion`**: 检测后全部动画 duration 归零

## 技术约束

- **框架**: Vue 3.5 Composition API + `<script setup>` + TypeScript strict
- **组件库**: Naive UI 2.44（暗色主题，ConfigProvider）
- **状态管理**: Pinia 3（composition API style stores）
- **构建**: Vite 5 多入口（pet.html + index.html）
- **桌面壳**: Wails v3（透明 frameless 窗口）
- **Live2D**: PixiJS 6 + pixi-live2d-display（仅 Pet 窗口加载）

## 实现优先级

1. **CSS 基础** — 色彩变量、字体、间距、动画关键帧、玻璃态 utility
2. **Dashboard 布局** — TitleBar + Sidebar + 路由框架 + 页面切换动画
3. **Chat 页面** — 消息列表、输入区、流式响应（核心功能）
4. **Emotion + Tools + Health 页面** — 数据展示型页面（简单）
5. **Memory + Proactive + Personality + LLM + Logs 页面** — 详情页面
6. **Pet 窗口** — Live2D + 胶囊组件（独立 Vue app）
7. **联动** — Pet ↔ Dashboard 通过 Go RuntimeService 通信

---

*本设计文档供 `frontend-design-ui-ux` skill 和实现 agent 使用。*
*审美参考: Apple Human Interface Guidelines + Google Material Design 3 + N.E.K.O*
*组件库: Naive UI 2.44 · 状态管理: Pinia 3 · 框架: Vue 3.5*
