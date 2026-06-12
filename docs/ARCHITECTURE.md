# Sion v0.3.0 完整架构手册

> **项目类型**: Go 自研桌面 AI 拟人 Agent (Wails + Vue 3 + Naive UI + SQLite + LLM Gateway)
> **核心架构**: API 原生工具调用 + 量化数理决策引擎 (System 1) + LLM 兜底生成层 (System 2) + 分层记忆 + 自学习权重回流
> **核心特色**: 行为选择由浮点统计驱动；工具通过 API tools JSON Schema 字段传入(不占上下文)，LLM 自主判断调用时机(tool_choice="auto")；System Prompt 仅 300 字核心人格；52 维特征驱力评分，内置门控抑制、动态调度、轻量化自学习

---

# 1. 整体架构总览

## 1.1 进程架构

```
┌──────────────────────────────────────────────────────────────┐
│                      main.go 入口                            │
│                                                              │
│  os.Args[1] == "settings"              os.Args[1] == "pet"  │
│  ┌─────────────────────────┐           ┌──────────────────┐ │
│  │   SettingsApp (主进程)    │   spawn   │  PetApp (子进程)   ││
│  │   - 所有 AI 服务          │──────────►│  - Live2D 渲染    ││
│  │   - HTTP API (:19840)    │           │  - 事件接收/发送   ││
│  │   - 插件管理器            │           │  - 设置面板宿主    ││
│  │   - 后台认知循环          │           └──────────────────┘ │
│  └─────────────────────────┘                                 │
└──────────────────────────────────────────────────────────────┘
```

**设计思想**: 双进程隔离——设置面板崩溃不影响宠物窗口。AI 服务集中在主进程，宠物是"瘦渲染器"，通过 HTTP/SSE 通信。

## 1.2 核心数据流 (五大管线)

```
┌─────────────────────────────────────────────────────────────────┐
│                        五大核心管线                              │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  管线 1: 对话管线 (Chat Pipeline)                                │
│  ─────────────────────────────────────                          │
│  用户输入 → HTTP POST /api/chat/send                            │
│    → ProcessChat() → OnBeforeChat [system prompt(300字) + 记忆注入]│
│    → ChatSyncWithTools() [tools=4, tool_choice="auto"]          │
│    → LLM 自主判断是否调工具 → 工具执行 → 结果注入 context          │
│    → LLM 基于结果生成自然语言回复                                  │
│    → OnAfterChat [事实提取 + 记忆归档]                           │
│    → SSE stream → 前端渲染                                       │
│                                                                 │
│  管线 2: 决策管线 (Decision Pipeline)                            │
│  ─────────────────────────────────────                          │
│  BackgroundLoop tick (动态 1-60min)                              │
│    → FeatureComputer.ComputeFull() [52维量化特征]                │
│    → NeedModel.Grow() [6维需求被动增长 + 饱和衰减]               │
│    → ComputeDrives() [5驱力: social care curious quiet explore] │
│    → Motivator.ScoreActions() [16动作×权重×调制]                │
│    → RouteToLLM() 判断 [fast path vs LLM fallback]              │
│    → 执行 selected action [ToolRegistry 统一调度]               │
│    → Learner.BatchLearn() [RL权重更新]                          │
│                                                                 │
│  管线 3: 记忆管线 (Memory Pipeline)                              │
│  ─────────────────────────────────────                          │
│  L0 会话缓冲 (SessionBuffer, 20轮)                               │
│    → L1 日记 (DiaryStore, 每4h/情绪波动触发)                     │
│    → L2 事实 (Facts DB + 向量, OnAfterChat 提取)                │
│    → L3 策略原则 (日反思 → LLM生成 → 向量去重合并)               │
│    ← 召回: chat时向量检索 → 注入上下文                           │
│                                                                 │
│  管线 4: 情绪管线 (Emotion Pipeline)                             │
│  ─────────────────────────────────────                          │
│  交互事件 → LLM评估(云端, 结构化推理) / 规则回退(本地)           │
│    → PAD三维 (Valence/Arousal/Dominance)                        │
│    → 8维情绪向量 (Affection/Worry/Curiosity/...)                │
│    → EMA指数平滑 + 昼夜节律调制 + 需求调制                       │
│    → 输出到 → 驱力计算 + LLM Prompt + 屏幕展示                  │
│                                                                 │
│  管线 5: 自学习管线 (Self-Learning Pipeline)                     │
│  ─────────────────────────────────────                          │
│  ActionOutcome 记录 (每次主动行动)                               │
│    → Learner: 驱力权重 DPO 更新 (每6h批处理)                     │
│    → StrategicAgent: 策略蒸馏 (每日反思)                         │
│    → Curiosity: 知识缺口 → 主动搜索 → 事实存储                   │
│    → Personality: 交互结果 → 人格参数微调                        │
│    → Forget: Ebbinghaus 遗忘曲线 → 不活跃事实自然淘汰           │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

## 1.3 System 1 / System 2 决策分流

```
BackgroundLoop tick 触发
        │
        ▼
  FeatureComputer 计算 52 维特征 (~50ms)
        │
        ▼
  ComputeDrives → 5 维驱力
        │
        ▼
  Motivator.ScoreActions → 16 动作评分
        │
        ├── 得分差距 > 0.03 且非极端场景 ──► System 1 (Fast Path)
        │    直接选最高分动作，零 LLM 调用
        │    覆盖 ~90% 的决策场景
        │
        └── 得分差距 ≤ 0.03 或极端场景 ──► System 2 (LLM Fallback)
             RouteToLLM() 触发条件:
             ① 极度困倦 (Sleepiness > 0.85)
             ② 极端用户情绪 (anger/fear + intensity > 0.8)
             ③ 动作重复 ≥3 次
             ④ 连续拒绝 ≥3 次
             ⑤ 接受率崩溃 (R1 < 0.3 且样本 ≥10 且趋势 < -0.15)
             ⑥ 需求-动作冲突 (高需求但选了 none)
             ⑦ 情绪崩塌 (A4_ValenceTrend < -0.5)
             ⑧ 长静默重连 (>4h idle, 首次重连)
             
             LLM 收到: 完整上下文 + 16 个 Action SkillCard
             输出: {"should_act":true,"action":"search","tool_input":"Rust async"}
```

**关键设计思想**: System 1 由 Go 代码完成，纯数学计算，100% 确定性。System 2 仅在分数接近、极端情绪、长静默等需要"创造力"的场景触发。两者共享同一个 ActionDef 注册表，保证动作空间一致。

---

# 2. 工具调用模块 — API 原生 tools 字段方案

## 2.1 模块定位与设计目标

**设计决策**: 工具定义通过 OpenAI-compatible API 的 `tools` 字段传入（JSON Schema 格式），不走 System Prompt 文本注入。LLM 通过 `tool_choice="auto"` 自主判断调用时机。

**与纯 Prompt 方案的核心区别**:

| 方案 | 工具传递方式 | 占上下文 | LLM 处理路径 | DeepSeek 可靠性 |
|------|-------------|---------|-------------|----------------|
| 文本注入 (旧) | System Prompt 中写文字描述 | 是 (~500 tokens) | 当角色扮演指令处理 | 低 (常被忽略) |
| API tools 字段 (现) | 请求体独立 `tools` 字段 (JSON Schema) | 否 | Transformer 特殊推理路径 | 高 (已验证) |

**为什么 API tools 字段更可靠**:
- `tools` 和 `messages` 是请求体的两个独立字段，不共享 token 预算
- Transformer 对 `tools` 有专门的推理处理逻辑，不和文本 tokens 在 self-attention 里竞争
- 工具调用结果回写到 `messages` 后才会占上下文（约 1500-2000 tokens per round）

## 2.2 调用流程

```
ChatSyncWithTools(messages, tools, execTool, maxRounds=3, toolChoice="auto")

每一轮:
  POST /v1/chat/completions
  {
    "model": "deepseek-chat",
    "messages": [...],         ← 文本上下文
    "tools": [                 ← 独立字段，不占上下文
      {name:"web_search", description:"Search the web...", parameters:{...}},
      {name:"get_memory", description:"Search long-term memory...", parameters:{...}},
      {name:"Memorize", description:"Permanently store...", parameters:{...}},
      {name:"analyze_screenshot", ...}
    ],
    "tool_choice": "auto"      ← LLM 自主判断
  }

  → LLM 返回 tool_calls 或 content text
  → 如果有 tool_calls: Go 代码执行 → 结果写入 messages → 继续下一轮
  → 如果没有 tool_calls: 返回纯文本

最多 3 轮，防止死循环
```

## 2.3 Tool Description 设计原则

每个工具的 `description` 字段是 LLM 唯一看到的工具文档。设计原则:

1. **英文简洁指令** — DeepSeek 对英文 function description 遵循度高于中文
2. **明确触发条件** — "Call when..." 开头，给出具体场景
3. **反混淆规则** — "Do NOT call for..." 防止误调用

```go
// SearchPlugin: 搜索触发 + 反闲聊误判
"Search the web for real-time information. " +
"Call when the user explicitly asks to search/lookup/query... " +
"Do NOT call for: casual chat, common knowledge, math, logic puzzles..."

// MemoryPlugin: 回忆触发 + 反搜索混淆
"Search long-term memory for past conversations... " +
"Call when the user references past discussions... " +
"Do NOT call for general knowledge questions — use web_search for those."

// MemoryPlugin: 记忆存储触发 + 反闲聊误判
"Permanently store important user information. " +
"Call when the user explicitly shares facts about themselves... " +
"Do NOT call for casual statements or opinions."
```

## 2.4 System Prompt 精简

工具信息不写入 System Prompt。Prompt 仅保留核心人格:

```
<identity>    4行 — 猫娘身份 + 回复风格
<user>        1行 — 称呼 + 技术栈
<time>        1行 — 当前时间
<self_and_emotion> 3行 — 动态情绪注入

总计: ~300 chars (~100 tokens)
```

**设计思想**: Prompt 越短，关键信息密度越高，LLM 对 tools 字段的注意力越集中。没有 "tool_rules" 段和角色人格争抢注意力。

## 2.5 工具扩展策略

当前 4 个工具 — 全量注入 `tool_choice="auto"`。未来扩展:

```
≤8 个工具:   全量注入 (当前方案)
8-20 个工具: 动态检索 — embedding 匹配 Top-K 相关工具注入
20+ 个工具:   Skill 分组 + 渐进式披露 (参考 Claude Code:
             核心工具常驻 + 按需 ToolSearch 加载 + Agent 隔离)
```

# 3. 量化特征计算模块 (FeatureComputer)

## 3.1 模块定位与设计目标

**解决的问题**: 纯 Prompt 决策无法量化"主人现在忙不忙""已经搭话几次了""连续被拒多少次"。FeatureComputer 将状态量化为一组 0~1 的浮点特征，使 System 1 数学决
策成为可能。

**设计目标**:
- Tier 1 特征: 纯内存计算 (~1ms)，每个 tick 都算
- Tier 2 特征: SQL 聚合 (~50ms)，TTL 缓存避免频繁查库
- 所有特征归一化到 [0,1] 或 [-1,1]，直接用于驱力公式
- 无 LLM 参与特征计算——纯数学/数据库查询

## 3.2 52 维特征完整释义

### A 组: Agent 状态 (13 维) — "诗音自己怎么样"

| # | 字段 | 含义 | 来源 | 区间 | 归一化 |
|---|------|------|------|------|--------|
| A1_1 | Affection | 亲密度 | EmotionModel 8维向量 | [0,1] | 原始值 |
| A1_2 | Worry | 担忧度 | EmotionModel 8维向量 | [0,1] | 原始值 |
| A1_3 | Curiosity | 好奇度 | EmotionModel 8维向量 | [0,1] | 原始值 |
| A1_4 | Sleepiness | 困倦度 | EmotionModel 8维向量 | [0,1] | 原始值 |
| A1_5 | Playfulness | 贪玩度 | EmotionModel 8维向量 | [0,1] | 原始值 |
| A1_6 | Loneliness | 寂寞度 | EmotionModel 8维向量 | [0,1] | 原始值 |
| A1_7 | Confidence | 自信度 | EmotionModel 8维向量 | [0,1] | 原始值 |
| A1_8 | Annoyance | 烦躁度 | EmotionModel 8维向量 | [0,1] | 原始值 |
| A2 | PrimaryEmotion | 主情绪标签 | EmotionModel | categorical | "joy"/"sadness"/"neutral"/... |
| A3 | Intensity | 情绪强度 | EmotionModel | [0,1] | 原始值 |
| A4 | ValenceTrend | 情绪效价趋势 | 过去1h valence 变化 | [-2,2] | delta |
| A4 | VecDelta | 情绪向量位移 | 8维向量欧氏距离 | [0,2] | 原始 |
| A5_1 | AnnoySensitivity | 烦躁敏感度 | 人格学习参数 | [0,1] | 原始值 |
| A5_2 | AffectWarmth | 亲密温暖度 | 人格学习参数 | [0,1] | 原始值 |
| A5_3 | WorryTendency | 担忧倾向 | 人格学习参数 | [0,1] | 原始值 |
| A6 | DailyActionCount | 今日行动次数 | 内存计数 | [0,20] | clamp(20) |
| A7 | ActionSuccessRate | 各动作类型成功率 | outcome_repo 查询 | [0,1] | 原始值 |
| A8 | TimeBlockRate | 各时段接受率 | outcome_repo 查询 | [0,1] | 原始值 |
| A10 | ActiveGoals | 活跃对话线程数 | thread_repo | [0,10] | saturate(10) |
| A11 | ActiveInquiries | 活跃探索目标数 | curiosity_repo | [0,5] | saturate(5) |
| A12 | KnowledgeGaps | 活跃知识缺口数 | curiosity_repo | [0,5] | saturate(5) |
| A13 | LearningMomentum | 学习势头 | 24h新事实数 | [0,1] | saturate(20) |
| A14 | ConsecutiveCount | 连续同动作次数 | 内存计数 | [0,5] | saturate(5) |

### U 组: User 状态 (14 维) — "主人在干嘛"

| # | 字段 | 含义 | 来源 | 区间 |
|---|------|------|------|------|
| U1 | AppCategory | 当前App分类 | OCR+分类器 | "work"/"play"/"social"/"idle" |
| U2 | WindowSubtype | 窗口子类型 | OCR+LLM分类 | "debugging"/"coding"/"meeting"/... |
| U3 | IsWorking | 是否工作中 | 分类器 | {0,1} |
| U4 | ContinuousWorkMins | 连续工作分钟 | CareEngine 累加 | [0,180] → [0,1] |
| U5 | AppSwitchCount | 30min内App切换次数 | 事件统计 | [0,20] → [0,1] |
| U7 | LengthTrend | 消息长度趋势 | 最近5条回归 | [-1,1] |
| U8 | ResponseDelayEMA | 响应延迟EMA | 指数移动平均 | [0,300]s → [0,1] |
| U10 | TimeWindowPref | 当前时段接受率 | outcome_repo | [0,1] |
| U11 | MealTime | 饭点窗口 | 时间判断 | {0, 0.5} |
| U12 | NightTime | 深夜窗口 | hour ∈ [22,8) | {0, 0.6} |
| U13 | IsWeekend | 周末标志 | 时间判断 | {0, 1} |
| U14 | TimeSinceChatMins | 距上次聊天分钟 | 时间差 | [0,∞) |
| U15 | FatigueMentionHrs | 距上次提疲劳小时 | 向量检索 | [0,24] → [0,1] |
| U16 | PrefDiversity | 偏好多样性 | 不同偏好类别计数| [0,10] → [0,1] |

### E 组: Environment 环境 (7 维) — "现在是什么时候"

| # | 字段 | 含义 | 区间 |
|---|------|------|------|
| E1 | Hour | 当前小时(0-23) | [0,23] |
| E2 | DayOfWeek/DOWSin/DOWCos | 星期循环编码 | [0,6] / sin / cos |
| E3 | CooldownNorm | 冷却因子 | [0,1] ← max(minsSinceAction/30, 1) |
| E4 | QuotaRemaining | 今日剩余配额 | [0,20] |
| E5 | MinsSinceDecision | 距上次LLM决策 | [0,∞) |
| E6 | LLMAvailable/VisionAvailable | 服务可用性 | {0,1} |
| E7 | ReflectionDue | 反思到期因子 | [0,1] ← saturate(hoursSince/24) |

### R 组: Relationship 关系 (8 维) — "主人怎么对诗音"

| # | 字段 | 含义 | 区间 |
|---|------|------|------|
| R1 | OverallAcceptRate | 整体接受率 | [0,1] |
| R1 | SampleCount | 样本数 | [0,20] |
| R2 | TimeWindowAccept | 当前时段接受率 | [0,1] |
| R3 | SourceAcceptRate | 各来源接受率 map | map[string]float64 |
| R4 | RecentRejections | 最近5条拒绝数 | [0,5] |
| R4 | RejectionSeverity | 拒绝严重度 | [0,1] ← recentRejections/5 |
| R5 | NeglectHours | 被忽略小时数 | [0,∞) |
| R6 | DepthTrend | 对话深度趋势 | [-1,1] |
| R7 | UserInitiative24h | 24h用户主动次数 | [0,20] |
| R8 | IntimacyTrend | 亲密趋势 | [-1,1] |

### T 组: Task Context 任务上下文 (3 维)

| # | 字段 | 含义 | 来源 |
|---|------|------|------|
| T1 | PrincipleCount | 可用策略数 | principle_repo count |
| T2 | PatternCount | 发现模式数 | pattern_repo count |
| T3 | ReflexionLogCount | 反思记忆条数 | decision_engine log |

## 3.3 核心算法: ComputeFull 两段式计算

```
ComputeFull(feats, emotion, needs, ...):
  Tier 1 (纯内存, ~1ms):
    U3-U5: app/working/switch_count
    U11-U13: meal/night/weekend (time-based)
    U14: timeSinceChat = now - lastChatTime
    E1-E3: hour, day, cooldown
    A6: dailyActionCount
    A14: consecutive count

  Tier 2 (SQL, ~50ms, TTL cached):
    A7: SELECT success_rate FROM outcomes GROUP BY action_type
    A8: SELECT success_rate FROM outcomes GROUP BY time_block
    R1-R4: 聚合 outcomes 表
    U10, U15, U16: 复杂查询 + 向量检索
    A11-A13: curiosity/learning 计数
```

**TTL 缓存策略**: Tier 2 特征每 5 分钟缓存一次。如果距上次计算 < 5min，跳过 SQL 查询，直接返回缓存值。减少 90% 的重复 SQL。

---


# 4. 驱力计算模块 (ComputeDrives)

## 4.1 模块定位

5 维驱力是将 52 维特征压缩为 5 个可解释行为方向的数学变换层。每维驱力 = 情感基值(50%) + 需求推动(15%) + 用户上下文(20%) + 关系门控(15%)。

## 4.2 五大驱力加权公式

### Social Drive (社交驱力)

```
social = 0.40 * clamp(loneliness)
       + 0.25 * clamp(playfulness)
       + 0.20 * idleBonus           // 空闲 >2h → 1.0
       + 0.10 * clamp(affection)
       + 0.05 * (1.0 - clamp(annoyance))
       + needs.Companionship * 0.12  // 需求推动
       + needs.Play * 0.08
       - U3_isWorking * 0.15        // 工作→抑制
       - U12_nightTime * 0.15       // 深夜→抑制
       - R4_rejectionSeverity * 0.35// 被拒→强烈抑制
       × interactionGate(R1_acceptRate)  // 关系门控
       
值域: [0, 1]
```

### Care Drive (关怀驱力)

```
care = 0.40 * clamp(worry)
     + 0.20 * clamp(affection)
     + 0.15 * nightBonus           // 深夜 0.6
     + 0.10 * mealBonus            // 饭点 0.5
     + needs.Care * 0.18
     + U4_continuousWorkNorm * 0.15
     + (night & working) * 0.10    // 深夜工作→额外关怀
     + U13_isWeekend * 0.05
     × interactionGate(R1_acceptRate)

值域: [0, 1]
```

### Curious Drive (好奇驱力)

```
curious = 0.35 * clamp(curiosity)
        + 0.25 * hasInquiry        // 有探索目标 0.6
        + 0.20 * hasGaps           // 有知识缺口 0.4
        + 0.15 * (1 - timeFactor)  // 刚聊完→抑制好奇
        + needs.Curiosity * 0.18
        + A13_learningMomentum * 0.07
        + U16_prefDiversity * 0.05
        × (0.7 + gate * 0.3)       // 关系门控(弱,仅30%调制)

值域: [0, 1]
```

### Quiet Drive (静默驱力)

```
quiet = 0.20 * clamp(sleepiness)
      + 0.15 * timeFactor          // 刚聊完→偏安静
      + 0.25 * clamp(annoyance)    // 烦躁→安静
      + 0.10 * idleBias            // 已行动多→休息
      + needs.Rest * 0.18
      + (1 - E3_cooldownNorm) * 0.15
      + U3_isWorking * 0.12
      + U12_nightTime * 0.08
      + (quota < 5) * 0.10         // 配额低→省着用
      + R4_rejectionSeverity * 0.40// 被拒→安静

值域: [0, 1]
```

### Explore Drive (探索驱力)

```
explore = 0.30 * clamp(curiosity)
        + 0.20 * (1 - timeFactor)
        + gapBoost * 0.25          // 缺口数/5 * 0.25
        + needs.Curiosity * 0.15
        + needs.Autonomy * 0.15
        + (¬ working) * 0.08       // 不工作→探索空间
        + E7_reflectionDue * 0.10
        + inquiryBoost * 0.12       // 探索目标数/5 * 0.12
        + (minutesSinceAction > 30) * 0.10
        × (0.8 + gate * 0.2)       // 关系门控(弱)

值域: [0, 1]
```

## 4.3 核心函数

```go
func ComputeDrives(feats *QuantifiedFeatures, needs *IntrinsicNeeds) 
    (social, care, curious, quiet, explore float64)

// 衰减辅助函数
func interactionGate(acceptRate float64) float64 {
    if acceptRate <= 0  { return 1.0 }  // 无数据→不抑制
    if acceptRate >= 0.5 { return 1.0 } // 健康→不抑制
    return 0.5 + acceptRate             // 0.0→0.5, 0.5→1.0
}

func clamp01(v float64) float64 {
    if v < 0 { return 0 }
    if v > 1 { return 1 }
    return v
}
```

---

# 5. 动作打分与上下文调制模块 (Motivator)

## 5.1 模块定位

将 5 维驱力映射到 16 个具体动作上，选最优。纯数学计算——点积 + 倍率调制 + clamp。

## 5.2 16 动作权重矩阵 (ActionDef.buildActions)

| Action | Social | Care | Curious | Quiet | Explore | NightSafe | Category |
|--------|--------|------|---------|-------|---------|-----------|----------|
| speak_casual | 0.80 | 0.15 | 0.05 | -0.30 | 0.00 | no | social |
| speak_care | 0.40 | 0.70 | 0.00 | -0.20 | 0.00 | no | social |
| speak_inquiry | 0.40 | 0.00 | 0.60 | 0.00 | 0.10 | no | social |
| care_rest | 0.10 | 0.75 | 0.00 | 0.00 | 0.00 | yes | care |
| care_meal | 0.10 | 0.70 | 0.00 | 0.00 | 0.00 | no | care |
| care_hydration | 0.05 | 0.65 | 0.00 | 0.00 | 0.00 | yes | care |
| care_health | 0.05 | 0.65 | 0.00 | 0.00 | 0.00 | yes | care |
| care_encourage | 0.20 | 0.55 | 0.00 | 0.00 | 0.00 | no | care |
| care_social | 0.30 | 0.40 | 0.00 | 0.00 | 0.00 | no | care |
| **search** | 0.05 | 0.05 | **0.45** | -0.10 | 0.30 | **yes** | learning |
| observe | 0.10 | 0.00 | 0.30 | 0.00 | 0.60 | yes | learning |
| reflect | 0.00 | 0.00 | 0.00 | 0.20 | 0.75 | yes | learning |
| analyze_patterns | 0.00 | 0.00 | 0.20 | 0.00 | 0.65 | yes | learning |
| none | 0.00 | 0.00 | 0.00 | 1.00 | 0.00 | yes | none |

## 5.3 得分计算公式

```go
// 基础点积得分
baseScore = social * w.Social
          + care * w.Care
          + curious * w.Curious
          + quiet * w.Quiet
          + explore * w.Explore

// CareEngine 建议加成
if suggestion exists for this action:
    baseScore += (0.30 - priority * 0.05)  // priority1→+0.25, 4→+0.10

// 上下文调制倍率
finalScore = baseScore * contextModulator(action, feats)
```

## 5.4 上下文调制器 (contextModulator) — 详细倍率计算

```go
func contextModulator(action string, feats *QuantifiedFeatures) float64 {
    m := 1.0

    // 1. 历史成功率调制 (A7)
    if typ := ActionByName(action).OutcomeType; typ != "" {
        if rate, ok := feats.A7_ActionSuccessRate[typ]; ok && rate >= 0 {
            m *= 0.4 + rate * 0.6    // rate=0→0.4, rate=1→1.0
        }
    }

    // 2. 来源接受率调制 (R3)
    if src := ActionByName(action).Source; src != "" {
        if rate, ok := feats.R3_SourceAcceptRate[src]; ok && rate >= 0 {
            m *= 0.5 + rate * 0.5
        }
    }

    // 3. 时间窗口偏好 (U10) — 仅 social 类
    if action is speak_casual/inquiry/care:
        m *= 0.4 + feats.U10_TimeWindowPref * 0.6

    // 4. 用户投入度 (U8) — 仅 social 类
    if action is speak_casual/care:
        m *= 0.6 + feats.U8_EngagementNorm * 0.4

    // 5. 对话深度 (R6) — speak_inquiry 加分
    if action is speak_inquiry && R6_DepthTrend > 0.2:
        m *= 1.0 + R6_DepthTrend * 0.3  // 最多×1.3

    // 6. 活跃探索目标 (A11) — speak_inquiry 加分
    if action is speak_inquiry && A11_ActiveInquiries > 0:
        m *= 1.0 + saturate(A11, 3) * 0.3

    // 7. 用户疏远 (U7) — social 降权
    if action is speak_casual/inquiry && U7_LengthTrend < -0.3:
        m *= 1.0 + U7_LengthTrend * 0.4   // trend=-1→×0.6

    // 8. search 专属调制
    if action is search:
        m *= 1.0 + saturate(A11_ActiveInquiries, 5) * 0.3  // 有目标→加分
        m *= 1.0 + saturate(A12_KnowledgeGaps, 5) * 0.2    // 有缺口→加分
        m *= 1.0 + A13_LearningMomentum * 0.1               // 学习势头
        m *= 0.3 + E3_CooldownNorm * 0.7                    // 冷却→降权
        if E4_QuotaRemaining < 3 { m *= 0.3 }               // 配额保护
        if U3_IsWorking > 0.5 { m *= 1.0 + U3 * 0.15 }     // 写代码→加分

    // clamp 到 [0.1, 1.5]
    return max(0.1, min(1.5, m))
}
```

**设计思想**: contextModulator 实现了"从历史中学习"的轻量版——如果某个动作过去成功率低，现在自动降权。不需要等 Learner 的 batch 更新就能即时生效。调制倍率被严格夹在 [0.1, 1.5]，避免任何单一因素产生过激的 swing。

---

# 6. 门控熔断模块

## 6.1 硬熔断规则 (Hard Gate)

硬熔断一旦触发，该动作完全不可选，零概率。

| 熔断 | 触发条件 | 拦截动作 | 设计原因 |
|------|---------|---------|---------|
| 夜间门控 | U12_NightTime > 0 (22:00-08:00) | 所有非 NightSafe 动作 | 深夜只允许 rest/health/search/observe/reflect/none |
| 配额耗尽 | E4_QuotaRemaining ≤ 0 | 所有非 none 动作 | 硬上限,每天20次 |
| 连续未回复 | consecutiveUnanswered ≥ 2 | 所有 speak/care_* | 2次搭话没回→停止 |

## 6.2 软抑制规则 (Soft Suppression)

软抑制通过倍率降低得分，但不会完全禁止。

| 抑制 | 机制 | 倍率 |
|------|------|------|
| 话题重叠保护 | recentChatAlreadyCovers(action) → false | 该 action 被设为 0 分 |
| 晚安重复保护 | 最近消息含"晚安/睡了" | speak_casual/inquiry/care → 0 |
| 拒绝衰减 | R4_RejectionSeverity > 0 | social −0.35×sev, quiet +0.40×sev |

## 6.3 熔断恢复逻辑

```go
// 连续未回复 → 30分钟慢慢恢复
if consecutiveUnanswered >= 1 && timeSinceLastProactive > 30min {
    consecutiveUnanswered--  // 每30min衰减1
}

// 拒绝记忆 → 30分钟过期
if timeSinceRejected > 30min {
    delete(rejectedActions, action)
}
```

---

# 7. 动态调度定时模块 (DynamicInterval)

## 7.1 设计目标

原固定 5 分钟 tick 有两个问题：活跃时不够快（错过搭话窗口），休眠时浪费资源（空转）。动态间距根据当前状态自适应调节。

## 7.2 三级阶梯规则

```go
func DynamicInterval(
    timeSinceChatMin, isWorking, continuousWorkMin,
    isNight, rejectionSeverity, dailyQuotaRemaining,
    socialDrive, careDrive, curiousDrive float64
) time.Duration {

    // === Tier 3: Dormant (长间距) ===
    if quotaRemaining <= 0  → 60 min   // 配额耗尽,几乎暂停
    if isNight              → 30 min   // 深夜低活跃
    if rejectionSeverity > 0.5 → 30 min  // 连续被拒
    if continuousWork > 120min && isWorking → 15 min  // 深度工作

    // === Tier 1: Active (短间距) ===
    hasHighDrive := social > 0.7 || care > 0.7 || curious > 0.7
    if timeSinceChat < 10min && hasHighDrive → 1 min   // 高频互动
    if timeSinceChat < 10min                  → 3 min   // 近期互动
    if hasHighDrive                           → 3 min   // 驱力高

    // === Tier 2: Normal (基线) ===
    return 5 min
}
```

## 7.3 屏幕观察间距自适应

```go
func AdaptiveScreenInterval(decisionInterval time.Duration) time.Duration {
    raw := decisionInterval / 3       // 1/3 的决策间隔
    return clamp(raw, 30s, 120s)      // 夹到 [30s, 120s]
}
```

## 7.4 事件驱动插队

```go
// BackgroundLoop.Wake() — 非阻塞 channel send
select {
case l.wakeCh <- struct{}{}:
default:  // channel 满了就不发,防止堆积
}
```

触发插队场景: 用户发消息、App 切换、情绪尖峰、PokeBuffer flush。

---

# 8. LLM Prompt 结构化注入模块

## 8.1 模块定位

System Prompt 仅保留核心人格定义(~300 chars)。工具规则不再写入 prompt——通过 API tools 字段独立传入。。设计目标是让 DeepSeek 在长上下文中准确识别工具调用触发条件。

## 8.2 Prompt 分区策略 (BuildSystemPrompt)

```
<identity>      4行  — 猫娘身份(精简,原10行→4行)
<user>          1行  — 称呼 + 技术栈
<time>          1行  — 当前时间 + 时段
<self_and_emotion>  3行 — (系统动态注入情绪状态)
<tool_rules>    已删除 — 工具信息走 API tools 字段,不再占用 prompt 文本
```

**设计思想**: 工具定义完全从 prompt 中移除，通过 API tools 字段独立传入。LLM 对 tools 字段有专门的推理逻辑，不和文本 prompt 竞争 self-attention。实测验证: 300 字 System Prompt + API tools 字段 + tool_choice="auto"，DeepSeek 能稳定调用工具。

## 8.3 Tool Description 设计原则

每个工具的 `description` 字段(API tools 中的唯一文档)遵循:

1. **英文简洁指令** — DeepSeek 对英文 function description 遵循度更高
2. **明确触发条件** — "Call when..." 开头,给出具体场景
3. **反混淆规则** — "Do NOT call for..." 防止误调用

```go
// web_search
"Search the web for real-time information. " +
"Call when the user explicitly asks to search/lookup/query... " +
"Do NOT call for: casual chat, common knowledge, math..."

// get_memory  
"Search long-term memory for past conversations... " +
"Do NOT call for general knowledge questions — use web_search for those."

// Memorize
"Permanently store important user information. " +
"Do NOT call for casual statements or opinions."
```

## 8.4 上下文注入策略

对话上下文通过 ProcessChat 的 pipeline 分层注入:

```
[system: identity (4行)]                      ← BuildSystemPrompt (~100 tokens)
[system: 情绪状态]                             ← EmotionModel.Current()
[system: 记忆上下文]                            ← MemoryPlugin 向量检索注入
[user: 当前消息]                               ← 用户输入
[assistant: tool_calls]                       ← (搜索时) LLM 工具调用记录
[tool: 搜索结果]                               ← (搜索时) Bocha API 返回
[assistant: 最终回复]                          ← LLM 自然语言回复
```

---

# 9. 分层记忆模块

## 9.1 L0 会话缓冲 (SessionBuffer)

```
结构: 环形缓冲区, 20 轮, 30分钟 max-age 过滤
写入: 每轮对话 OnAfterChat 触发
读取: BuildSystemPrompt 时注入最近 N 轮
持久化: 启动时从 chat_history 表 LoadHistory(20) 回填
       重启后自动恢复最近会话上下文，不会丢失工作记忆
       进程间通过 OnBeforeChat 的 LoadHistory(10) 保持设置窗口与宠物窗口同步
```

## 9.2 L1 日记 (DiaryStore)

```
触发条件: 每 4 小时 + 情绪波动(Valence 变化 > 0.3)
内容: LLM 生成标题+摘要+情绪标签
存储: SQLite diary 表 + Ollama 向量 embedding
用途: 长期记忆回顾、策略反思输入
```

## 9.3 L2 原子事实 (Facts)

```
提取: OnAfterChat → LLM extract → AtomicFact[] 
过滤规则:
  - qualifyFactContent(): 长度 <5 字 拒绝
  - 噪音模式匹配: "就行/好了/算了" → 拒绝
  - 疑问句: "什么/谁/哪/怎么" 开头 → 拒绝
  - 向量相似度 > 0.85 → 合并而非新增

存储: SQLite facts 表 + vector BLOB
召回: chat 时 cosine similarity 检索 Top-N 注入 system prompt
遗忘: Ebbinghaus 曲线 — 定期 Forgot(): last_recalled 超阈值 → archived
```

## 9.4 L3 策略原则 (StrategyPrinciples)

```
触发: StrategicAgent.ShouldRun() → 距上次 > 6h
流程:
  1. 收集: 最近24h ActionOutcome
  2. LLM分析: 成功/失败模式
  3. 生成: StrategyPrinciple { 场景, 好策略, 坏策略, 原因 }
  4. 去重: 向量相似度 > 0.85 → LLM 合并
  5. 存储: SQLite + 向量

注入: 决策时作为 ActivePrinciples 传给 LLM
```

---

# 10. 自学习权重回流模块

## 10.1 Learner — 驱力权重 RL 更新

```go
// 核心公式: Δw = step × reward × drive
// step = 0.003
// reward ∈ {+1(accepted), 0(ignored), -1(rejected)}

func UpdateWeightsFromOutcome(action, reward, social, care, curious, quiet, explore) {
    step := 0.003 * reward
    UpdateWeight(action, "social",  step * social)
    UpdateWeight(action, "care",    step * care)
    UpdateWeight(action, "curious", step * curious)
    UpdateWeight(action, "quiet",   step * quiet)
    UpdateWeight(action, "explore", step * explore)
    // 权重夹到 [-1, 1]
}

// Batch Learn: 每6h, 至少5条新记录
func ShouldLearn() bool {
    return timeSinceLastLearn > 6h && len(storedDrives) >= 5
}
```

**设计思想**: DPO 风格的轻量 RL——不是完整 policy gradient，而是用简单梯度更新调整权重。reward=0(忽略)不参与更新，避免无信号数据污染权重。

## 10.2 StrategicAgent — 策略蒸馏

```go
// Daily Reflection
func ShouldRun() bool {
    return timeSinceLastRun > 6h
}

// 输入
prompt := buildReflectionPrompt(
    selfModel,        // "我是诗音,主人是..."
    recentOutcomes,   // 过去24h 成功/失败记录
    recentDiaries,    // 情感日记
    activeFacts,      // L2 活跃事实
    activePrinciples, // 已有策略
)

// LLM 输出
type DailyReflectionOutput struct {
    Reflection        string
    NewPrinciples     []StrategyPrinciple
    TacticalDirectives []string
    MergedPrinciples  []int64
    SelfUpdate        string
}
```

## 10.3 冷启动默认参数

| 参数 | 默认值 | 来源 |
|------|--------|------|
| 驱力权重矩阵 | BuildWeightsMap() | actions.go 硬编码 |
| 人格参数 | 0.5/0.5/0.5 | WarmStart config 可覆盖 |
| 内源需求 | 0.3-0.4 基线 | NewNeedModel() |
| 动机权重 | 从文件加载,不存在用默认 | motivator_weights.json |

---



# 11. 数据持久层

## 11.1 数据库设计

```
SQLite: ~/.desktop-pet/memory.db (WAL 模式, busy_timeout=5000ms)
单连接 (SetMaxOpenConns=1) — WAL 模式支持并发读,写串行化
```

### 核心表结构

| 表 | 用途 | 关键字段 | 索引 |
|----|------|---------|------|
| facts | L2 原子事实 | id, content, importance, vector(BLOB), source, archived, last_recalled_at | idx_facts_archived, idx_facts_created, idx_facts_source |
| diary | L1 情感日记 | id, title, summary, vector(BLOB), emotion_valence, emotion_arousal | idx_diary_archived |
| chat_history | 对话记录 | id, role, content, created_at | (按 created_at 排序) |
| action_outcomes | 主动行动反馈 | action_source, action_type, outcome, hour_of_day, emotion_bucket | (按 created_at 查询) |
| strategy_principles | L3 策略 | situation, good_strategy, bad_strategy, reason, vector(BLOB), active | (按 active 过滤) |
| curiosity_items | 好奇引擎 | item_type, content, priority, status, source | (按 item_type + status 过滤) |
| memory_archive | 记忆归档 | name, level, original, summary | (主键 name) |
| self_profile | 自我画像 | content, source | (无索引,单行) |
| identity_nodes | 身份图谱 | label, properties_json, embedding | (向量检索) |

## 11.2 二级缓存策略

```
L1 缓存 (内存):
  - SessionBuffer: 最近 20 轮对话
  - FeatureComputer Tier1: 每个 tick 计算一次,内存
  - appCategoryCache: U1 App分类映射表
  - rejectedActions map: 30min TTL

L2 缓存 (SQLite):
  - FeatureComputer Tier2: TTL 5min,避免重复 SQL 聚合
  - 向量检索: cosine similarity on BLOB, 暂无 ANN 索引(数据量 <10k)
```

## 11.3 SQL 优化点

- 所有聚合查询 (A7, A8, R1-R4) 使用 COUNT/GROUP BY + DATE 过滤,单表扫描
- facts 表 archived=0 过滤利用 idx_facts_archived 索引
- 向量检索当前为全表扫描 → 数据量 <10k 时足够; 后续可用 sqlite-vss 扩展
- chat_history 写入频率最高 (~每轮2条),无索引开销

---

# 12. 并发安全与性能优化

## 12.1 Goroutine 调度

```
主 goroutine: BackgroundLoop.loop()
  → ticker channel → runTick()
  → screenTicker → observeScreen()
  → wakeCh → 事件驱动插队

子 goroutines:
  proactiveTimeoutLoop: 每2min检查主动消息是否超时
  eagerScreenObserve: 启动后2s执行首次屏幕观察
  checkEmbeddingHealth: embedding 服务健康检查
  backfillDiaryVectors: 历史日记向量回填
  triggerVisualAnalysis: 异步截图分析
```

## 12.2 重复计算裁剪

```go
// FeatureComputer Tier2 TTL 缓存
if time.Since(fc.lastTier2Compute) < 5*time.Minute {
    return fc.cachedTier2  // 跳过 SQL,直接用缓存
}

// IntervalChanged: 只有间隔变化 >10% 才 reset ticker
func IntervalChanged(old, new time.Duration) bool {
    return math.Abs(float64(new)/float64(old)-1.0) > 0.1
}
```

## 12.3 LLM 调用限流退避

```go
// DecisionEngine 指数退避
func (e *DecisionEngine) ShouldRun() bool {
    interval := e.minInterval  // 15min
    for i := 0; i < e.idleCount && i < 4; i++ {
        interval *= 3  // 15→45→135→405min
    }
    return time.Since(e.lastDecisionAt) > interval
}

// idleCount 递增条件: LLM 决策选 "none" 或不行动
// idleCount 重置: 任何用户互动
```

## 12.4 Mutex 策略

- MemoryPlugin.mu: 保护 running 状态 + chat 取消
- Motivator.mu (RWMutex): 保护权重矩阵读写
- FeatureComputer.mu (RWMutex): 保护情绪历史环形缓冲区
- Manager.mu (RWMutex): 保护插件注册表
- 无全局锁 — 每个子系统独立 mutex，避免死锁

---

# 13. 全参数速查表 (面试重点)

## 13.1 阈值参数

| 参数 | 值 | 作用 | 调整影响 |
|------|-----|------|---------|
| RouteToLLM score gap | 0.03 | S1/S2 分流差距 | 降低→更多 LLM 调用; 升高→更少 LLM |
| consecutiveUnanswered suppress | 2 | 连续未回复→禁言 | 降低→更容易禁言 |
| rejectionSeverity suppress | 0.5 | 拒绝严重度→30min间隔 | 升高→容忍更多拒绝 |
| quality gate for facts | 0.5 | 搜索事实存入阈值 | 降低→更多低质事实; 升高→更严格 |
| maxToolRounds | 2 (search) / 3 (legacy) | LLM tool loop 上限 | 影响 API 调用次数 |
| batchLearn min records | 5 | RL 批处理最小样本 | 降低→更频繁更新; 升高→更稳定 |
| batchLearn interval | 6h | RL 批处理间隔 | 同 batchLearn min records |
| strategy agent interval | 6h | 策略反思间隔 | 降低→更多策略但 token 消耗高 |
| fact vector dedup threshold | 0.85 | cosine相似度→合并 | 降低→更激进合并 |

## 13.2 配额参数

| 参数 | 值 | 作用 |
|------|-----|------|
| maxDailyActions | 20 | 每日主动行动上限 |
| searchDailyQuota | 5 | 每日主动搜索上限(通过 search action 权重调制) |
| proactiveTimeout | 5min | 主动消息无回复→标记 ignored |
| rejectedActions TTL | 30min | 拒绝记忆过期时间 |
| consecutiveUnanswered decay | 30min/次 | 禁言恢复速率 |

## 13.3 时间参数

| 参数 | 值 | 作用 |
|------|-----|------|
| BackgroundLoop base interval | 5min | 默认决策间隔 |
| Night interval | 30min | 深夜决策间隔 |
| Quota exhausted interval | 60min | 配额耗尽间隔 |
| Deep work interval | 15min | 深度工作间隔 |
| Active interval | 1-3min | 高频互动间隔 |
| Screen observe interval | 30s-120s | 屏幕观察间隔(自适应) |
| DecisionEngine min interval | 15min | LLM 兜底决策最小间隔 |
| Gap scan min interval | 2h | 知识缺口扫描冷却 |
| Visual analyze min interval | 15min | 屏幕分析冷却 |
| Emotion EMA alpha | 0.3 | 情绪平滑系数 (新值权重) |

## 13.4 浮点系数

| 系数 | 值 | 位置 | 作用 |
|------|-----|------|------|
| decayRate (needs) | 0.03/h | needs.go | 需求向基线回归速率 |
| curiosity growth rate | 0.05/h | needs.go | 好奇心自然增长 |
| care growth rate (working) | 0.05/h | needs.go | 工作时关怀疑增长 |
| autonomy growth rate | 0.02/h | needs.go | 自主需求增长 |
| 需求 satisfaction (search) | -0.35 | domain/needs.go | 搜索后好奇降低量 |
| RL step size | 0.003 | learner.go | 权重更新步长 |
| contextModulator clamp | [0.1, 1.5] | motivator.go | 调制倍率安全区间 |
| Drive social rejection penalty | 0.35 | motivator.go | 被拒对 social 驱力的压制 |
| Drive quiet rejection boost | 0.40 | motivator.go | 被拒对 quiet 驱力的增强 |
| interactionGate floor (R1<0.5) | 0.5+rate | motivator.go | 低接受率时 gate 的下限 |

---

# 14. 前端架构 (v0.3.0)

## 14.1 技术栈

| 层 | v0.2 (旧) | v0.3 (新) |
|----|----------|----------|
| 框架 | React 18 + TypeScript | Vue 3.4 + TypeScript |
| 构建 | Vite + @vitejs/plugin-react | Vite + @vitejs/plugin-vue |
| UI 库 | 无，手写 inline style + CSS Variables | Naive UI (n-card, n-tabs, n-progress, n-menu, n-form 等) |
| 状态管理 | Zustand | Pinia (Composition API style) |
| 图标 | Unicode emoji | @vicons/ionicons5 SVG |
| 宠物渲染 | PIXI.js + pixi-live2d-display | 不变 |
| 类型检查 | tsc | vue-tsc |

## 14.2 设计系统

基于 Naive UI `NConfigProvider` 统一主题变量：

```
primaryColor: #4f6ef7
侧边栏背景: #e8f4fd (淡蓝)
内容区背景: #f5f7fa
卡片圆角: 12px
全局圆角: 8px
```

深色侧边栏（v0.2 `#1a1c2e`）改为淡蓝侧边栏，与白色内容区搭配，整体风格简洁明亮。

## 14.3 组件映射

| 旧组件 (自研) | 新组件 (Naive UI) |
|--------------|-------------------|
| 手写 sidebar + hover | `n-menu` + `n-layout-sider` |
| 手写 stat card | `n-card` + `n-statistic` |
| 手写 tab 切换 | `n-tabs` (segment/bar) |
| 手写 slider/toggle/number | `n-slider` / `n-switch` / `n-input-number` |
| 手写进度条 | `n-progress` |
| 手写标签 | `n-tag` (round, bordered:false) |
| 手写表单 | `n-form` + `n-form-item` + `n-input` |
| 手写时间线 | `n-timeline` + `n-timeline-item` |
| 手写 Toast | `useMessage()` |

## 14.4 聊天面板布局

v0.3 重写为经典三区固定布局：

```
.chat-root (height: calc(100vh - 64px), flex column)
  .chat-header (flex-shrink: 0) → 固定顶部
  .msg-list (flex: 1, overflow-y: auto) → 只有消息区滚动
  .chat-footer (flex-shrink: 0) → 固定底部输入框
```

关键点：`<main>` 设 `overflow: hidden` 防止整页滚动；消息区独占 `flex: 1` + `min-height: 0` 确保 flex 子元素可收缩；输入框始终在底部可见。

## 14.5 响应式系统差异

| | React (v0.2) | Vue 3 (v0.3) |
|---|---|---|
| 模型 | Pull — 显式 `setState` | Push — Proxy 自动追踪 |
| 状态声明 | `useState` / `useRef` | `ref()` / `reactive()` |
| 派生值 | `useMemo(() => ..., [deps])` | `computed(() => ...)` 自动依赖 |
| 副作用 | `useEffect(() => {...}, [deps])` | `watch(source, fn)` / `onMounted` |
| 依赖数组 | 需要，遗漏即 bug | 不需要，Proxy 自动收集 |
| 组件通信 | props + callback | props + emit + `defineExpose` |

迁移后代码量减少约 15%，主要来自取消 `useCallback`、`useMemo` 和依赖数组。

---

# 15. 架构对比 & 差异化优势

## 15.1 与主流文本型 Agent 架构横向对比

| 维度 | Sion v0.3 | OpenClaw | Hermes/Dify | LangChain Agent |
|------|-----------|----------|-------------|-----------------|
| 行为决策 | **量化数学(浮点评分)** + LLM 兜底 | 纯 LLM function calling | 纯 LLM 链式调用 | ReAct pattern (Thought→Action→Observe) |
| 工具可靠性 | **API tools 字段 + tool_choice="auto"** | 依赖模型 tool calling 能力 | 依赖模型 tool calling 能力 | 依赖模型 tool calling 能力 |
| 长上下文稳定性 | 不受影响 — 代码决策不在 context 里 | 随上下文增长退化 | 随上下文增长退化 | 需要轨迹压缩 |
| 自学习 | **DPO 权重更新** + 策略蒸馏 + 人格适应 | 无 | 有限 | 有限 |
| 人格一致性 | 情绪模型(PAD+8D) + 人格参数 | Prompt only | Prompt only | Prompt only |
| 部署模型 | DeepSeek (低成本) | Claude/GPT-4 (高成本) | 任意 | 任意 |
| 数学可解释性 | **5维驱力 × 16动作权重** 完全可审计 | LLM 黑盒 | LLM 黑盒 | LLM 黑盒 |

## 15.2 纯 LLM Tool Calling 方案的核心缺陷 (本项目解决的问题)

| 缺陷 | Sion 解决方案 |
|------|-------------|
| 长上下文稳定性 | API tools 不占上下文,仅 tool 结果写回 messages |
| 模型 Tool Calling 能力不一致 (DeepSeek auto 不工作) | tool_choice="required" + Round0/Round1 两段式 |
| LLM 角色扮演与工具调用冲突 | 工具调用由代码决定,LLM 只负责话术 |
| 无历史反馈学习 | Learner + StrategicAgent + Outcome 反馈闭环 |
| Token 消耗 | tools 字段不占文本上下文,仅 tool call results 写回 |

## 15.3 架构可复用拓展场景

| 场景 | 适配方案 |
|------|---------|
| 游戏 NPC AI | 52维特征→NPC状态量化; 5维驱力→NPC行为倾向; ToolRegistry→NPC可用动作 |
| 风控引擎 | 特征计算→风险因子量化; 门控熔断→自动拦截规则; Learner→欺诈模式学习 |
| 电商营销运维 | 需求模型→用户画像; 动作评分→推荐时机选择; StrategyAgent→A/B测试策略蒸馏 |
| 独立 Agent SDK | 抽离 cognition 包为独立 Go module; 注入 Config 实现多租户; gRPC 暴露决策 API |

---

# 16. 现存设计取舍、缺陷与优化迭代方案

## 16.1 设计权衡

| 权衡 | 选择 | 利弊 |
|------|------|------|
| 数学决策 vs LLM 决策 | 数学为主(90%), LLM兜底(10%) | 利: 确定性强,成本低; 弊: 无法处理未见过的复杂场景 |
| API tools 字段 vs Prompt 文本注入 | API tools | 利: 独立推理路径,不占上下文; 弊: DeepSeek tool_choice="auto" 不稳定(已通过优化 tool description 解决) |
| 单模型(DeepSeek) vs 多模型 | 单模型 | 利: 成本低,配置简单; 弊: tool calling 能力受限 |
| 本地 embedding(Ollama) vs 云端 | 本地 | 利: 隐私,零延迟; 弊: 需要额外部署, 质量依赖模型 |
| 纯内存需求 vs DB 持久化 | 内存计算+定期持久化 | 利: 快; 弊: 重启丢失未保存的状态 |

## 16.2 当前待优化模块

1. **Tool Description 持续优化**: 当前英文简洁指令已验证有效,可进一步加入 few-shot 范例
2. **记忆工具实际调用率低**: DeepSeek tool_choice="auto" 下 get_memory/Memorize 可能不触发, 可考虑代码层预召回 + 注入上下文
3. **搜索多轮**: 当前仅1次搜索→1次回复, 不支持"搜A→结果不够→搜B"的多轮检索
4. **向量检索性能**: 当前全表扫描 cosine similarity, 数据量 >10k 后需要 ANN 索引
5. **QQ Bot 插件**: 接口已定义但未深度集成

## 16.3 中长期架构升级方案

1. **本地 SoftPrompt 向量注入**: 将人格/情绪/策略编码为 prompt embedding 前缀,替代文本 system prompt,减少 token 消耗
2. **独立决策引擎 SDK 抽离**: cognition 包独立为 `go-sion-engine`, gRPC + protobuf 暴露 API,支持多语言客户端
3. **多模型路由**: 闲聊→DeepSeek(便宜), 搜索摘要→Claude(质量), 决策兜底→GPT-4o(可靠)
4. **ANN 向量索引**: sqlite-vss 或切换到 pgvector

---

# 17. 面试问答配套附录

## Q1: 为什么要做量化决策,而不是纯 Prompt?

**标准答案**: 
"纯 Prompt 决策有两个致命问题。第一是不可审计——你不知道 LLM 为什么选 speak 而不是 search,调参只能改文字描述。第二是成本——每次决策都要调 LLM,每天 288 次 tick × 每次 ~2000 tokens = 严重浪费。我们的 System 1 用 52 维浮点特征→5 维驱力→16 动作评分,纯数学计算,~50ms,零 API 费用。System 2 只在分数接近(<0.03)或极端场景触发,覆盖 ~10% 的决策。这样既保证了可解释性,也控制了成本。"

## Q2: 工具调用为什么要用 API tools 字段而非 Prompt 文本?

**标准答案**:
"因为 tools 字段和 messages 是请求体里的两个独立字段,在 Transformer 推理时走不同的处理路径。写在 Prompt 里的工具描述被 LLM 当成角色扮演指令处理,和'你是一只猫娘'没有区别。而我们实测验证:API tools 字段 + tool_choice='auto' + 英文简洁 description,DeepSeek 能稳定调用。核心原则:能用结构化 Schema 解决的问题,不用文本 Prompt 解决。"

## Q3: 52 维特征怎么来的?

**标准答案**:
"不是拍脑袋的。分四组:User(14维)描述主人状态,Agent(13维)描述诗音自身,Environment(7维)环境时间,Relationship(8维)互动关系。每个特征来自具体数据源——OCR 屏幕分类、outcome_repo 查询、EmotionModel 输出、时间计算。归一化到 [0,1] 是为了直接代入驱力公式,不需要额外缩放。"

## Q4: 自学习怎么做的?

**标准答案**:
"三个层次。第一层 Learner: 每次主动行动记录 (驱力,动作,反馈),每 6h 批处理,DPO 风格更新权重——被接受的动作权重 ×1.003,被拒绝的 ×0.997。第二层 StrategicAgent: 每日反思,LLM 分析 24h 结果,提炼策略原则(场景→策略),向量去重合并。第三层 Curiosity: 从已知事实找知识缺口→主动搜索→评分→存入 L2。三层闭环:行动→反馈→策略→知识。"

## Q5: 长上下文稳定性怎么保证?

**标准答案**:
"两个关键设计。第一个是工具定义通过 API tools 字段传入,不占文本上下文——工具 Schema 和对话内容在推理时走不同路径,不会互相干扰。第二个是 System Prompt 精简到 300 字——只剩身份和风格,没有任何工具规则,LLM 的全部注意力在 tools 字段上。聊天时 messages 只包含 system prompt(~100 tokens) + 最近消息 + 记忆召回 + 工具结果(如果有),简洁高效。"

## Q6: 如果重来一次,架构上会改什么?

**标准答案**:
"① WebSocket 替代 HTTP polling——前端状态同步更实时。② cognition 包一开始就按 SDK 设计——边界更清晰,方便单独测试和复用。③ 向量存储从 SQLite BLOB 换 pgvector——ANN 索引是硬需求。④ 工具扩展时采用 Claude Code 的渐进披露模式——核心工具常驻 + 按需 Search 加载。"

---

> **手册版本**: v0.3.0 | **最后更新**: 2026-06-08 | **作者**: 诗音开发团队

# 18. 设计参考来源

> 本章记录项目架构设计过程中参考的开源项目、学术论文与技术博客，以及各部分设计思想的具体借鉴点。

## 18.1 学术论文参考

### 认知架构

| 论文 | 借鉴点 | 对应模块 |
|------|--------|---------|
| **Kahneman, D. (2011). _Thinking, Fast and Slow_** | System 1(快速直觉) / System 2(慢速推理) 双系统理论，直接映射为本项目的 Quant Scorer(Go, ~50ms) + LLM Fallback 双层决策 | 决策引擎、RouteToLLM 分流逻辑 |
| **Park, J. S. et al. (2023). _Generative Agents: Interactive Simulacra of Human Behavior_. UIST.** | 记忆流(Memory Stream)三层架构: 感知→短期记忆→长期反思; 检索评分公式(recency + importance + relevance); 反思(Reflection)机制——定期从记忆中抽象高层次洞察 | 分层记忆(L0→L1→L2→L3)、StrategicAgent 日反思 |
| **Packer, C. et al. (2023). _MemGPT: Towards LLMs as Operating Systems_. arXiv:2310.08560.** | 虚拟内存分页管理——将 LLM 上下文视为"主存"，长程记忆视为"磁盘"，按需换入换出; 自编辑记忆(Self-Editing Memory)——LLM 自主决定何时写入/更新/遗忘 | SessionBuffer 20轮窗口、记忆压缩归档、Ebbinghaus遗忘曲线 |
| **Rafailov, R. et al. (2023). _Direct Preference Optimization_. NeurIPS.** | DPO——无需显式奖励模型，直接从偏好对(accepted vs rejected)中学习策略。本项目简化应用: 每次行动的 accept/ignore/reject 反馈直接更新权重矩阵，步长 0.003 | Learner.BatchLearn() |
| **Yao, S. et al. (2023). _ReAct: Synergizing Reasoning and Acting in Language Models_. ICLR.** | 交错推理(Thought)→行动(Action)→观察(Observation)循环，被 LangChain/OpenAI Agents 广泛采用。本项目在 DecisionEngine LLM Fallback 中使用此模式: LLM 看到完整上下文 + 16个 SkillCard → 输出 JSON action → 代码执行 → 结果反馈 | DecisionEngine.buildFallbackPrompt() |
| **Oudeyer, P. Y. & Kaplan, F. (2007). _What is intrinsic motivation? A typology of computational approaches_. Frontiers in Neurorobotics.** | 内在动机(Intrinsic Motivation)分类: 基于知识(Knowledge-based)vs 基于能力(Competence-based)。本项目好奇引擎的 knowledge gap scanning 对应 Knowledge-based IM; 自主需求(Autonomy need)对应 Competence-based IM | CuriosityEngine, NeedModel |

### 情绪与人格

| 论文 | 借鉴点 | 对应模块 |
|------|--------|---------|
| **Mehrabian, A. & Russell, J. A. (1974). _An Approach to Environmental Psychology_. MIT Press.** | PAD 三维情绪模型: Pleasure(愉悦)/Arousal(唤醒)/Dominance(支配)。本项目在此之上扩展为 8 维向量(Affection/Worry/Curiosity 等)以描述猫娘的拟人情感 | EmotionModel |
| **Ebbinghaus, H. (1885). _Memory: A Contribution to Experimental Psychology_** | 遗忘曲线: 记忆保留率随时间的指数衰减。本项目 simplified 为: 最近召回时间 → decay 阈值 → archived | memory_layer.Forget() |

### 工具调用与 Agent 工程

| 论文 | 借鉴点 | 对应模块 |
|------|--------|---------|
| **Schick, T. et al. (2023). _Toolformer: Language Models Can Teach Themselves to Use Tools_. NeurIPS.** | LLM 通过自监督学习自主发现工具使用时机。本项目的做法相反——代码层硬路由+强制 tool_choice="required"，正是因为 Toolformer 的"自主发现"在小模型(DeepSeek)上不可靠 | 三层工具路由 |
| **Mialon, G. et al. (2023). _Augmented Language Models: a Survey_. arXiv:2302.07842.** | 增强语言模型的分类体系: Reasoning / Tool Use / Acting / Memory。本项目覆盖全部四类,其中 Tool Use 采用外置路由(而非 LLM 内生) | 整体架构 |
| **NaviAgent (2025). _Bilevel Planning on Tool Navigation Graph_. arXiv:2506.19500.** | 双层工具调用: LLM 做任务规划 + 图模型做工具执行导航。本项目的 Round 0(工具调用)→Round 1(话术生成)两段式与此思路一致 | ChatSyncWithTools |
| **Dynamic Function Routing (2025). CSDN/工业界实践.** | 意图检测→工具池缩小→LLM 只选工具参数。本项目采用更优方案:API tools 字段传入全部4个工具 + tool_choice="auto",无额外路由开销 | ChatSyncWithTools |

---

## 18.2 开源项目参考

### 核心架构参考

| 项目 | 借鉴点 | 对应设计 |
|------|--------|---------|
| **[Wails](https://wails.io)** v2.9+ | Go 后端 + Web 前端的桌面应用框架; 双进程(Settings+Pet)基于 Wails 的 Window Management | main.go 双模式入口 |
| **[LangChain](https://github.com/langchain-ai/langchain)** / **[LangGraph](https://github.com/langchain-ai/langgraph)** | Tool/Function 抽象、Chain 编排模式。本项目简化: Plugin 接口替代 Chain, ToolRegistry 替代 Tool 抽象 | plugin.Manager, ToolRegistry |
| **[MemGPT](https://github.com/cpacker/MemGPT)** / **[Letta](https://letta.com)** | 自编辑记忆 + 虚拟上下文管理; 向量检索 + LLM 反思的混合记忆架构 | 分层记忆 L0-L3 |
| **[CrewAI](https://github.com/crewAIInc/crewAI)** | 多 Agent 角色定义(Role/Goal/Backstory) → 本项目的 ActionDef.SkillCard 三段式(触发/做法/呈现)借鉴此模式 | actions.go SkillCard |
| **[Generative Agents (Stanford)](https://github.com/joonspk-research/generative_agents)** | 记忆流 + 反思 + 计划的 Agent 架构; 本项目在实际工程化中做了大量简化(纯 Go, 无 Python 依赖) | StrategicAgent, CuriosityEngine |

### 情绪与人格参考

| 项目 | 借鉴点 | 对应设计 |
|------|--------|---------|
| **[Live2D Cubism SDK](https://www.live2d.com)** + **[pixi-live2d-display](https://github.com/guansss/pixi-live2d-display)** | Live2D 模型渲染、视线追踪、拖拽交互。本项目前端 Vue 3 + Naive UI + PIXI.js + pixi-live2d-display 驱动猫娘形象 | frontend/components/pet/PetCanvas.vue |
| **The Sims 4 AI (Game Developer Conference talks)** | 基于多维度需求驱动的行为选择: 需求模型(Companionship/Rest/Play/...) × 权重 → 选择最优行为。本项目驱力模型直接借鉴此数值化设计 | NeedModel + Motivator |
| **Character.AI / Replika** | 人格一致性保持: 通过长期交互数据微调角色参数。本项目简化: 3个人格参数(AnnoyanceSensitivity/AffectionWarmth/WorryTendency)从 outcome 中学习 | EmotionModel.LearnPersonality() |

### 前端与 UI

| 项目 | 借鉴点 | 对应设计 |
|------|--------|---------|
| **[Naive UI](https://www.naiveui.com)** | Vue 3 原生组件库，TypeScript 优先，Tree-shaking，`NConfigProvider` 全局主题定制；提供 Card/Tabs/Progress/Menu/Form/Timeline 等 80+ 组件 | 设置面板全部 UI 组件 |
| **[Pinia](https://pinia.vuejs.org)** | Vue 3 官方状态管理，Composition API style，DevTools 支持；替代 Zustand | petStore + settingsStore |
| **[Vicons](https://vicons.online)** | SVG 图标库，ionicons5 图标集，Tree-shaking 友好 | 侧边栏和按钮图标 |

### 工具与基础设施

| 项目 | 借鉴点 | 对应设计 |
|------|--------|---------|
| **[MCP (Model Context Protocol)](https://modelcontextprotocol.io)** by Anthropic | 统一工具调用协议; 本项目虽未直接实现 MCP,但 ToolRegistry + FunctionProvider 接口设计受 MCP 的 tool/list + tool/call 模式影响 | plugin.FunctionProvider |
| **[Ollama](https://ollama.com)** | 本地 embedding 模型部署; 本项目用 Ollama 跑 bge-small-zh-v1.5 做向量化,隐私优先 | EmbeddingService |
| **[SQLite](https://sqlite.org)** WAL mode | 单文件数据库 + WAL 并发读; busy_timeout=5000ms 避免锁冲突。选择 SQLite 而非 pgvector 是因为桌面应用不需要独立数据库服务 | infra/storage |
| **[OpenAI API](https://platform.openai.com)** + **[SiliconFlow](https://siliconflow.cn)** | OpenAI-compatible chat/embeddings API 格式; 本项目 LLM Gateway 统一封装, 切换模型只需改 config.yaml | infra/llm/gateway.go |

---

## 18.3 设计哲学视角

### 数理决策 > 纯 Prompt 决策

| 来源 | 借鉴 |
|------|------|
| **Utility AI (Game Industry)** | 游戏 NPC 的行为选择: 每个动作的 utility = Σ(考虑因素 × 权重),选最高。本项目 52维特征→5维驱力→16动作评分正是 Utility AI 的工程实践 |
| **Behavior Trees + Blackboard** | 行为树的状态共享黑板(Blackboard) → 本项目的 FeatureComputer 作为统一状态计算层,所有下游模块从同一个 QuantifiedFeatures 读取 |

### 以小搏大 (Small Model + Smart System)

| 来源 | 借鉴 |
|------|------|
| **DistilBERT / TinyBERT** | 知识蒸馏思想——大模型的能力可以迁移到小模型+辅助系统。本项目用 DeepSeek(低成本) + 代码决策系统,实现接近 Claude 的 tool calling 可靠性 |
| **Hybrid AI (Garry Kasparov's "Advanced Chess")** | 人+机器 > 纯机器。类比: 代码规则(逻辑, 确定性) + LLM(创造力, 语言) > 纯 LLM |

### Alife-inspired 特征

| 来源 | 借鉴 |
|------|------|
| **Homeostasis (Cannon, 1932)** | 生物体维持内部平衡的机制。本项目 NeedModel 的 decay 函数(向基线 0.3 回归)直接模拟 homeostasis: 过度满足的需求会自然消退,被忽视的需求会持续增长 |
| **Circadian Rhythm** | 昼夜节律。本项目 U12_NightTime 门控 + Sleepiness 随时间变化 + 深夜行为限制 = 模拟生物钟 |
| **Ebbinghaus Forgetting Curve** | 人类记忆的自然遗忘规律。本项目 fact 的 importance × recall_count × recency → decay score,低于阈值 → archived |

---

## 18.4 关键设计决策的参考来源

| 设计决策 | 权衡 | 参考来源 |
|---------|------|---------|
| Go 而非 Python | 桌面应用需要单二进制分发,Python 打包(500MB+)不可接受 | Wails 文档、Go 社区最佳实践 |
| SQLite 而非 pgvector | 桌面应用不需要独立数据库服务; 向量数据量 <10k,全表扫描够用 | SQLite 官方 WAL mode doc |
| 纯内存需求 + 定期持久化 | 高频 tick(1-5min)不能每次写 DB,但重启不能丢状态 | Redis AOF 思想(保留最近状态快照) |
| API tools 字段而非 Prompt 文本注入 | tools 独立推理路径,不占上下文,不受 prompt 长度影响 | OpenAI/Anthropic API 设计规范 |
| tool_choice="auto" | LLM 自主判断,保持角色自由度 | DeepSeek API 文档 + 实测验证 |
| Ollama 本地 embedding 而非 OpenAI API | 隐私(文本不离开本地) + 零网络延迟 | Ollama 官方 benchmark: bge-small-zh 512维, ~20ms/条 |
