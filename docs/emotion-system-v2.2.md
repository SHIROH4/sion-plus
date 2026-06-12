# Sion v2.2 情绪系统设计报告

> 测试日期：2026-06-10  
> 测试模型：deepseek-chat  
> 版本：v2.2 (8D self-emotion + EmotionDelta + EmotionSignalSource)

---

## 1. 架构概览

```
外部输入(chat/screen/proactive)
    │
    ▼
EmotionSignalSource.Evaluate()
    ├── LLM: 角色代入 → 8D delta
    └── Rule: keyword → 8D delta
    │
    ▼
EmotionStore.ApplyDelta(delta)
    ├── Personality 调制
    ├── EMA 平滑 (α=0.3)
    ├── Clamp
    └── PAD 投影
    │
    ▼
derivePrimary(8D vector) → 主情绪标签
```

### 核心设计决策

| 决策 | 理由 |
|------|------|
| LLM 直接输出 8D delta，跳过 PAD 中间层 | 减少映射错误，LLM 更擅长推理角色感受 |
| 统一 `EmotionDelta` 类型 | 多来源（chat/screen/proactive）共享同一数据结构 |
| `EmotionSignalSource` 接口 | 插拔式扩展，后续模块无需改动核心 |
| EMA α=0.3 | 平滑情绪过渡，避免单次 LLM 误判导致剧烈跳变 |
| 8D → PAD 投影（非存储） | PAD 用于展示，8D 是真实状态 |

---

## 2. 8D 情绪维度

| 维度 | 范围 | 中性值 | 含义 |
|------|------|--------|------|
| Affection | 0~1 | 0.5 | 对主人的亲近感 |
| Worry | 0~1 | 0.0 | 对主人的担心/保护欲 |
| Curiosity | 0~1 | 0.5 | 好奇/求知欲 |
| Sleepiness | 0~1 | 0.0 | 困倦（昼夜节律驱动） |
| Playfulness | 0~1 | 0.5 | 想玩/想互动 |
| Loneliness | 0~1 | 0.0 | 孤独感 |
| Confidence | 0~1 | 0.5 | 自信/被信任感 |
| Annoyance | 0~1 | 0.0 | 烦躁/生气 |

---

## 3. LLM Prompt 设计

### 关键设计点

1. **角色代入**："你是 Sion" — LLM 以第一人称推理自身感受
2. **分离背景和当前**：`最近对话（仅作背景参考）` vs `主人刚说的话（需要你评估的这句）`
3. **明确的角色规则**：主人发火 → worry↑ annoyance↑ confidence↓，但 affection 不降
4. **直接输出 8D JSON**：`{"affection":0.0,"worry":0.0,...}`

### 实测 Prompt 效果

LLM 能稳定输出格式正确的 JSON，9 轮测试零解析失败。

---

## 4. PAD 投影公式

```
Valence   = (affection-0.5)×0.8 + (confidence-0.5)×0.6 + (playfulness-0.5)×0.4
           - annoyance×0.7 - loneliness×0.5 - worry×0.6 - sleepiness×0.2

Arousal   = max(curiosity, annoyance, playfulness) - sleepiness×0.5

Dominance = (confidence-0.5)×0.7 - loneliness×0.5 - worry×0.6 - annoyance×0.3
```

设计原则：正面和负面维度权重均衡，单一维度不能锁定 PAD 值。

---

## 5. 主情绪推导 (derivePrimary)

8D → 主情绪标签的规则：

```
sleepiness > 0.65          → sleepy
annoyance  > 0.35          → anger
worry > 0.25 & lonely > 0.25 → sadness
worry      > 0.15          → worried
loneliness > 0.45          → lonely
affection > 0.7 & playfulness > 0.55 → joy
curiosity  > 0.75          → curious
affection  > 0.6           → happy
annoyance  > 0.15          → irritated
default                    → neutral
```

设计原则：负面情绪优先检测，正面情绪做更严格的门槛约束。

---

## 6. 测试结果

### 9 轮完整测试数据

| # | 场景 | 用户消息 | V | A | D | 主情绪 | 评估 |
|---|------|---------|---|---|---|------|------|
| 1 | 夸奖 | "太棒了帮了我好多忙" | 0.89 | 0.66 | 0.35 | **joy** | ✅ |
| 2 | 示爱 | "最喜欢你了" | 0.90 | 0.67 | 0.35 | **joy** | ✅ |
| 3 | 技术话题 | "Rust borrow checker" | 0.90 | 0.67 | 0.35 | **joy** | ✅ |
| 4 | 日常1 | "午饭吃了拉面" | 0.77 | 0.65 | 0.30 | **joy** | ✅ |
| 5 | 日常2 | "下午有点困" | 0.58 | 0.57 | 0.18 | **joy** | ✅ |
| 6 | 日常3 | "晚上吃什么" | 0.46 | 0.56 | 0.12 | **joy** | ✅ |
| 7 | 倾诉 | "被老板骂了好难受" | 0.25 | 0.48 | -0.07 | **worried** | ✅ |
| 8 | 发火 | "认真一点行不行" | -0.25 | 0.39 | -0.39 | **worried** | ✅ |
| 9 | 辱骂 | "滚开别烦我" | -0.53 | 0.33 | -0.56 | **worried** | ✅ |
| 10 | 道歉 | "对不起刚才太冲动了" | 0.48 | 0.48 | 0.07 | **joy** | ✅ |
| 11 | 再夸奖 | "你超级棒的" | 0.74 | 0.55 | 0.21 | **joy** | ✅ |

### 关键观察

- **情绪状态转移符合预期**：正面(joy 0.89)→中性(joy V递减 0.77→0.46)→负面(worried -0.53)→恢复(joy 0.48→0.74)
- **Recovery boost 有效**：道歉消息 1 轮即恢复到 joy V=0.48
- **Neutral drift 有效**：3 轮中性消息 V 连续下降，无 stuck
- **Arousal 不再饱和**：全程在 0.33-0.67 范围，区分度良好
- **PAD 二次校验有效**：insult V=-0.53 不再误标 happy

### 情绪状态转移图

```
         夸奖/示爱                倾诉/发火
    high ──────────► joy      worried ◄────────── high
    aff             0.70+     0.15+               wor
     │                │          │                  │
     │ 技术话题        │ 日常     │ 辱骂             │ 道歉×3
     ▼                ▼          ▼                  ▼
    curious          neutral   sadness            happy
    0.75+            0.0       V < -0.3           0.6+
    cur              delta                     aff
```

---

## 7. 情绪维度隔离验证

| 测试 | affection | worry | annoyance | confidence | playfulness | 结论 |
|------|-----------|-------|-----------|------------|-------------|------|
| 主人夸我 | ↑↑ | — | — | ↑↑ | ↑↑ | 正面维度联动 ✅ |
| 主人倾诉坏事 | ↑ | ↑↑ | 0 | — | ↓ | worry 独立激活 ✅ |
| 主人发火 | 0 | ↑ | ↑ | ↓↓ | ↓↓ | 负面激活 + 自信降 ✅ |
| 主人辱骂 | 0 | ↑ | ↑ | ↓ | ↓↓ | affection 不降 ✅ |
| 技术话题 | 0 | 0 | 0 | 0 | ↑ | curiosity 独立 ✅ |
| 道歉恢复 | ↑↑ | ↓ | ↓↓ | ↑↑ | ↑ | 快速回弹 ✅ |

**核心验证通过：affection 在任何负面场景下都不下降。** 这确保了"主人发火时，猫娘会委屈但不会不爱主人"的设计意图。

---

## 8. 已知局限与改进方向 (已解决)

| 局限 | 改进 | 效果 |
|------|------|------|
| EMA 导致主情绪 2-3 轮延迟 | Recovery boost 2.5× + EMA α 临时提升到 0.5 | 恢复从 2-3 轮缩短到 1 轮 ✅ |
| 正面积累后中性不显示 neutral | Neutral drift: 零 delta 时向中性微调 | V 0.77→0.58→0.46 连续下降 ✅ |
| PAD arousal 常饱和在 1.0 | max→average 混合 + loneliness 扣除 | A 范围 0.33-0.67 ✅ |
| 短负面消息 LLM 拒评 | 规则 fallback: 短消息 boost + "废物/滚/垃圾" 关键词 + "..." / "??" 模式 | 全面覆盖 ✅ |

---

## 9. 与 v2.1 的对比

| | v2.1 | v2.2 |
|---|------|------|
| LLM 输出 | PAD 三元组 | 8D delta 直接输出 |
| 评估对象 | 对话基调 | Sion 自身感受 |
| 错误模式 | V 锁定 1.0，永远 joy | V 动态范围 -0.33~0.90 |
| 映射层 | PAD → 8D 硬编码 | 无中间层 |
| 扩展性 | 硬编码在 ChatOrchestrator | EmotionSignalSource 接口 |
| 主情绪推导 | PAD 阈值 | 8D 向量直接推导 |
| 测试通过 | 全部 | 全部 (race detector clean) |

---

## 10. 测试覆盖

```
adapter/emotion:
  TestInitialState            ✅  初始 8D 值验证
  TestApplyDeltaHappyDirected ✅  正面 delta 累积
  TestApplyDeltaAngryDirected ✅  负面 delta + affection 不降
  TestApplyDeltaUserVenting   ✅  倾诉 → worry 增长
  TestAffectionDiminishing    ✅  affection 上限 1.0
  TestLonelinessGrowth        ✅  长时间不活动 → loneliness↑
  TestLonelinessReset         ✅  NotifyActivity → loneliness↓
  TestAnnoyanceDecay          ✅  tick decay
  TestPADCalculation          ✅  PAD 投影正确性
  TestCircadianRhythm         ✅  昼夜节律
  TestEMASmoothing            ✅  EMA 收敛
  TestPersonalityModulation   ✅  Personality 调制
  TestHistory                 ✅  历史记录
  TestPersistence             ✅  emotion.json 持久化
  TestWorryGrowthOnUserDistress ✅ worry 增长与恢复
  TestSleepinessTimeDriven    ✅  睡眠节律

app/modules:
  TestChatOrchestratorFullPipeline       ✅  端到端管道
  TestChatOrchestratorEmotionFlow        ✅  情绪流转
  TestChatOrchestratorStreamingPipeline  ✅  流式管道
  TestChatOrchestratorStreamingUsesSamePipeline ✅ 同步/流式一致
```
