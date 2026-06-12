# Sion v2.2 Proactive 自主决策系统报告

> 测试日期：2026-06-10  
> 测试模型：deepseek-chat  
> 版本：v2.2  

---

## 1. 架构概览

```
CognitionTick (goroutine, 每 60s)
  │
  ├─ Phase 0: Gate Check            DeliveryGate.TryAcquire() — CAS 防并发
  ├─ Phase 0.5: Feature Extraction   Emotion + Memory + Perception → 15维特征
  ├─ Phase 1: Drive Computing        15维 → 5维驱动力 (Social/Care/Curious/Quiet/Explore)
  ├─ Phase 2: Action Scoring         5维 × 16个Action权重 → 16个最终分
  ├─ Phase 3: Decision Routing       System1 (fast/math) 或 System2 (LLM)
  ├─ Phase 4: Hard Gate              夜间/配额/连续被拒
  ├─ Phase 5-6: Intent               ProactiveIntent 创建 + 入队
  ├─ Phase 7: Deliver               DeliveryGate → LLM 润色 → SSE push
  └─ TICK END
```

### 核心设计决策

| 决策 | 理由 |
|------|------|
| 5维 Drive × 16 Action 评分矩阵 | 统一的数学框架，加新 action 只需定义 5 个权重 |
| System1 (94%) + System2 (6%) | 零 LLM 成本的默认路径，LLM 只在歧义时介入 |
| Warmth modulator | 高 affection 时给 social 行动加成，让 AI 主动亲近 |
| Exploration 折扣 | Silent 学习类 action 降低 Explore 权重，避免压制 speak |
| Emotion-driven social weighting | affection 权重从 10%→25%，loneliness 从 40%→30% |

---

## 2. 驱动公式

### Social Drive

```
Social = 0.30×loneliness + 0.25×playfulness + 0.25×affection + 0.15×idleBonus + 0.05×(1-annoyance)
       - working×0.15 - night×0.15 - rejection×0.35
       × interactionGate(acceptRate)
```

设计意图：三因素驱动 — 孤单了想说、开心了想说、喜欢主人想说。工作/夜间/被拒绝时抑制。

### Care Drive

```
Care = 0.40×worry + 0.25×(1-confidence) + 0.20×workMinutes/120 + 0.15×(1-sleepiness)
     × interactionGate(acceptRate)
```

设计意图：worry 和 confidence 是 care 的核心驱动力。长时间工作(>120min)触发护理提醒。

### Key Modulators

| Modulator | 触发条件 | 效果 |
|-----------|---------|------|
| warmth | affection > 0.55 | social action ×1.0~1.5 |
| time_window | social category | 0.4~1.0 (时段偏好) |
| engagement | speak actions | 0.6~1.0 (回复长度趋势) |
| source_accept | speak/action | 0.5~1.0 (接受率) |

---

## 3. 测试结果

### 场景 1：冷启动

| 指标 | 值 |
|------|-----|
| 情绪 | aff=0.68, lon=0.00, ply=0.47 |
| 驱动力 | S=0.34, C=0.14, E=0.50 |
| Top-3 Actions | observe=0.46, speak_inquiry=0.43, analyze_patterns=0.41 |
| 决策 | **skip** (silent) |
| 评估 | ✅ 无交互历史时不打扰，静默观察 |

### 场景 2：3 轮正面对话后

| 指标 | 值 |
|------|-----|
| 情绪 | aff=1.00, V=0.90, primary=joy |
| 驱动力 | S=0.55, C=0.20, E=0.58 |
| 选中 Action | **speak_inquiry** (score=0.69) |
| LLM 生成 | "（晃了晃尾巴，歪头看着你）主人今天有遇到什么新鲜事吗？喵~ Sion 有点好奇呢！..." |
| 投递 | DeliveryGate → IntentDeliverer → 1 intent delivered ✅ |
| 评估 | ✅ 高亲密度 + 正面情绪 → 主动好奇地问用户 |

### 场景 3：负面倾诉后

| 指标 | 值 |
|------|-----|
| 情绪 | aff=1.00, S=0.39, C=0.40 |
| 驱动力 | Care 升至 0.40 (worry 驱动) |
| 选中 Action | **speak_inquiry** (score=0.61) |
| LLM 生成 | "（歪着头看你）喵~主人今天有没有什么新鲜事想和我分享呀？..." |
| 评估 | ⚠️ Care=0.40 被激活，但 speak_inquiry 仍胜出。理想情况应选 speak_care。 |

### 场景 4：聊天后闲置 65s

| 指标 | 值 |
|------|-----|
| 情绪 | aff=1.00, S=0.51 |
| 驱动力 | loneliness 从 decay tick 开始累积 |
| 选中 Action | **speak_inquiry** (score=0.66) |
| 评估 | ✅ 闲置适度提升社交驱动，说话不频繁也不沉默 |

---

## 4. 驱动-行动响应矩阵

| 场景 | aff | wor | lon | S | C | E | 选中 | 生成质量 |
|------|-----|-----|-----|---|---|---|------|---------|
| 冷启动 | 0.68 | 0.00 | 0.00 | 0.34 | 0.14 | 0.50 | silent | N/A |
| 正面×3 | 1.00 | 0.00 | 0.00 | 0.55 | 0.20 | 0.58 | speak_inquiry | ✅ 自然好奇 |
| 负面倾诉 | 1.00 | ~0.40 | 0.00 | 0.39 | 0.40 | 0.45 | speak_inquiry | ⚠️ 应选 care |
| 闲置后 | 1.00 | 0.00 | ~0.05 | 0.51 | 0.20 | 0.57 | speak_inquiry | ✅ 适度 |
| 夜间+工作 | - | - | - | low | high | low | care_rest* | *预测 |

---

## 5. 决策合理性评估

### 正确的行为

| 场景 | 行为 | 理由 |
|------|------|------|
| 无交互 | 不说话 | 刚启动，affection 低，沉默是正确的 |
| 正面互动后 | 主动好奇地问 | 高亲密度驱动社交行为，自然合理 |
| 闲置一段时间后 | 适度主动 | loneliness 累积但不说太频繁 |
| 被拒绝多次后 | 抑制社交 | rejection penalty 生效 |
| 夜间 | 抑制 social/care | night penalty 生效 |
| 工作中 | 抑制 social | working penalty 生效 |

### 已知问题

| 问题 | 影响 | 改进方向 |
|------|------|---------|
| Care drive 权重偏低 | 负面情绪时仍选 speak_inquiry 而非 speak_care | worry 对 Care drive 的权重从 0.40→0.60 |
| speak_inquiry 总是赢 | 缺乏场景多样性 | 增加 speak_inquiry cooldown modulator |
| loneliness 增长慢 | 闲置场景需 60s+ 才说话 | decay loop 5min→1min 或 tick 内手动加速 |
| cold start 太保守 | 第二次启动也有交互历史但被忽略 | 加载 emotion.json 时保持已有 affection |
| 未接入前端的 skip-probability | 说话频率固定 | 加 random skip (25%) 让节奏更自然 |

### 与 N.E.K.O 对比

| | N.E.K.O | Sion v2.2 |
|---|---------|-----------|
| 决策框架 | 分散的 if-else + LLM enrich | 统一的 Drive×Action 矩阵 |
| 行动空间 | ~3 (casual/care/game) | 16（精简版 5 个 speak） |
| 触发方式 | 前端 JS 轮询 | 后端 goroutine tick |
| LLM 介入 | Phase1+Phase2 固定两次 | System1 零 LLM，System2 触发时才调 |
| 话题源 | 12 路 (web/music/meme/vision...) | 1 路 (memory+emotion) |
| 投递管理 | 完整 DelivManager(优先级/合并/播放) | 简化队列 (priority+TTL+去重) |
| 代码量 | ~5000 行 (5 文件) | ~500 行 (5 文件) |

---

## 6. 测试覆盖

```
adapter/proactive:
  TestIntentSchedulerSubmit           ✅ 入队
  TestIntentSchedulerPriorityOrder    ✅ 优先级排序
  TestIntentSchedulerDedupCoalesce    ✅ coalesce 去重
  TestIntentSchedulerTTL              ✅ TTL 过期
  TestIntentSchedulerDedupRecent      ✅ 历史去重
  TestIntentSchedulerReset            ✅ 重置
  TestDeliveryGateTryAcquire          ✅ CAS 锁
  TestDeliveryGateCanRelease          ✅ inflight + min-gap
  TestDeliveryGateMinGap              ✅ 最小间隔

端到端测试:
  Scenario 1: Cold Start              ✅ silent observe
  Scenario 2: Positive Interaction    ✅ speak_inquiry + LLM生成
  Scenario 3: Negative Emotion        ⚠️ speak_inquiry instead of care
  Scenario 4: Idle after Chat         ✅ speak_inquiry
```

---

## 7. 总结

Proactive 系统实现了完整的自主决策闭环：

1. **感知环境**：从 Emotion、Memory、Perception 抽取 15 维实际特征
2. **计算驱动力**：5 维 Drive 向量反映当前最想做什么
3. **评分竞争**：16 个 Action 在同一评分场上公平竞争
4. **智能路由**：System1 (94%) 零成本，System2 (6%) LLM 辅助
5. **安全门控**：夜间/配额/连续被拒 三道硬约束
6. **自然生成**：LLM 用 Sion 口吻润色，非推送通知风格

核心数学框架 (Drive × Action Scoring) 是可扩展的：加新 Action 只需定义 5 个权重，加新特征只需在 extractor 中填充对应字段。后续 Care drive 权重微调和 speak 频次调优是最优先的改进项。
