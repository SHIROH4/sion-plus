# Sion v2.2 执行流程与分层依赖

---

## 分层总览

```
┌──────────────────────────────────────────────────────────────┐
│ Layer 1: Transport    HTTP + SSE + Static                    │
│ 依赖: AppRuntime                                            │
│ 提供: 外部通信接口                                           │
├──────────────────────────────────────────────────────────────┤
│ Layer 2: App          Runtime + ChatOrchestrator + Builder   │
│ 依赖: Port interfaces                                       │
│ 提供: 生命周期管理 + 对话管道编排                            │
├──────────────────────────────────────────────────────────────┤
│ Layer 3: Adapter      Emotion | Memory | Perception | Proactive│
│ 依赖: Domain types + Port interfaces                        │
│ 提供: 具体功能实现                                           │
├──────────────────────────────────────────────────────────────┤
│ Layer 4: Domain       types | cognition | emotion | memory   │
│ 依赖: 无 (纯函数/纯类型)                                     │
│ 提供: 数据类型 + 评分引擎 + 证据纯函数                       │
├──────────────────────────────────────────────────────────────┤
│ Layer 5: Port         接口定义                                │
│ 依赖: Domain types                                           │
│ 提供: 层间契约                                               │
└──────────────────────────────────────────────────────────────┘
```

---

## 一、启动流程

```
main.go
  │
  ├─ 读取环境变量 (SION_LLM_URL/KEY/MODEL, SION_ADDR, SION_DATA_DIR)
  │
  ├─ app.NewRuntime(cfg)           ← 创建所有适配器实例
  │   │
  │   ├─ LLMService                 ← ProviderRegistry + TokenTracker
  │   ├─ SQLiteStore               ← sion.db 打开/创建
  │   ├─ MemoryStack               ← buffer, recall, evidence, worker, comp
  │   ├─ EmotionService            ← EmotionStore + Evaluator
  │   ├─ ScreenObserver            ← macOS osascript/ioreg 绑定
  │   └─ PromptBuilder             ← personality text
  │
  ├─ runtime.Init(ctx)
  │   │
  │   ├─ Module.Init()             ← LLM/Emotion/Memory 各自的初始化
  │   ├─ SetExecutor()             ← 注入 LLM executor 到各模块
  │   ├─ LLMHooks.Install()        ← extractFacts, detectSignals, reflect+diary, compress
  │   ├─ ChatOrchestrator 创建     ← 注入所有 adapter 引用
  │   ├─ Proactive wiring          ← CognitionTick 创建
  │   └─ Identity wiring           ← SelfModelStore 加载 + IdentityBuilder 注入
  │
  ├─ runtime.Start(ctx)
  │   │
  │   ├─ LLMService.Start()        ← 健康检查启动
  │   ├─ EmotionStore.Start()      ← 5min decay goroutine
  │   ├─ MemoryWorker.Start()      ← pool_size=3 goroutines + maintenance loop
  │   └─ CognitionTick.Start()     ← 60s goroutine (30s first delay)
  │
  ├─ SSE Broker.Start()            ← topic fanout goroutine
  │
  └─ HTTP Server.ListenAndServe()  ← 阻塞，接收请求
```

**依赖方向**：`main → runtime → adapter → domain/port`  
**注入时机**：LLM executor 在 `Init()` 中注入，因为需要先注册 provider

---

## 二、对话执行流程 (POST /api/v1/chat/stream)

```
1. HTTP Handler 接收请求
   │  handlers.chatStream(w, r)
   │  解析 JSON body → {message: "..."}
   │  └─ 依赖: AppRuntime.Chat
   │
2. ChatOrchestrator.OnUserMessageStream(ctx, userMsg, onChunk)
   │
   ├─ STEP 1: 情绪评估 (同步, ~0.3s)
   │   │
   │   │  buildRecentTurns(userMsg)
   │   │  → EmotionEvaluator.Evaluate(ctx, input)
   │   │  → LLM: "你是Sion, 主人刚说: xxx. 你的情绪如何变化?"
   │   │  → 8D delta: {affection:0.7, worry:0, ...}
   │   │  → EmotionStore.ApplyDelta(delta)
   │   │    ├─ personality 调制 (+warmth, +sensitivity)
   │   │    ├─ recovery boost (负向恢复 ×2.5)
   │   │    ├─ neutral drift (零delta → 向中性微调)
   │   │    └─ EMA 平滑 (α=0.3)
   │   │  → PAD 投影: V/A/D
   │   │  → derivePrimary(8D): "joy"|"worried"|"anger"|...
   │   │
   │   │  依赖: EmotionStore (8D state)
   │   │        LLMExecutor.Chat (emotion model)
   │   │        PersonalityScale (调制参数)
   │   │
   │   │  输出: EmotionEvalResult {delta, state, vector, source}
   │   │
   │   ├─ SetMoodBias(state.Valence)      → recall 的 mood-congruent 检索
   │   └─ UpdateEmotionState(V, A)        → valence swing >0.3 → Wake worker
   │
   ├─ STEP 2: 记忆召回 (同步, ~0.05s)
   │   │
   │   │  Recall.HybridSearch(ctx, userMsg, 5)
   │   │  ├─ BM25 FTS5 → 100 candidates
   │   │  ├─ Vector cosine → 80 candidates (if embedding available)
   │   │  ├─ RRF fusion (k=60)
   │   │  └─ Exploration quota (20% cold facts)
   │   │
   │   │  Recall.SearchDiaries(ctx, userMsg, 2)
   │   │  Recall.SearchBoundaries(ctx)
   │   │
   │   │  依赖: SQLiteStore (facts + diaries + FTS5)
   │   │        EvidenceEngine (score computation + mood bias)
   │   │        EmbeddingService (可选，无则用 evidence score)
   │   │
   │   │  输出: []MemorySearchResult (facts + diaries + boundaries)
   │
   ├─ STEP 3: Prompt 构建 (同步, ~0.01s)
   │   │
   │   │  ScreenObserver.Observe() → app name + title
   │   │  PromptBuilder.Build(input)
   │   │  ├─ L0: 最近 5 轮对话
   │   │  ├─ L1: facts + diaries + boundaries + screen context
   │   │  ├─ L2: user model + self model (SelfModelStore)
   │   │  └─ 情绪: {primary, intensity}
   │   │  → <memory-context> 注入 user message
   │   │
   │   │  依赖: SessionBuffer (L0 messages)
   │   │        ScreenObserver (screen summary)
   │   │        SelfModelStore (user/self model text)
   │   │
   │   │  输出: BuildResult {systemPrompt, memoryContext, warnings}
   │
   ├─ STEP 4: LLM 调用 (流式)
   │   │
   │   │  TrackedExecutor.ChatStream(ctx, systemPrompt, messages, onChunk)
   │   │  ├─ OpenAIGateway.ChatStream → POST /v1/chat/completions (stream=true)
   │   │  ├─ SSE parsing → onChunk(token) → 前端逐字显示
   │   │  └─ TokenTracker.Record(promptTokens, completionTokens)
   │   │
   │   │  依赖: LLMExecutor (OpenAI-compatible gateway)
   │   │        TokenTracker (daily JSONL)
   │   │
   │   │  输出: full response text
   │
   └─ STEP 5: 写入记忆 (同步, ~0.01s)
       │
       │  MemoryWorker.OnAfterChat(ctx, userMsg, response)
       │  ├─ L0 buffer append (环形, 容量40, 30min TTL)
       │  ├─ SQLite history save (messages table)
       │  └─ turns++ → if turns>=3 → Wake() → async worker pool
       │
       └─ pushEmotion() → SSE Broker.Publish("emotion", PAD state)
```

---

## 三、异步记忆处理 (后台 Worker Pool)

```
MemoryWorker.wakeCh 被唤醒
  │
  │  触发条件:
  │  ├─ OnAfterChat: turns >= extractEveryN (每 3 轮)
  │  ├─ UpdateEmotionState: valence swing > 0.3
  │  └─ Idle timeout: 5 分钟无对话
  │
  ▼
processWake(ctx, cfg)
  │
  ├─ runFactExtraction
  │   │
  │   │  SQLiteStore.LoadHistory(50) → 最近 50 条消息
  │   │  → LLM: "从对话中提取原子化事实" → JSON
  │   │  → 过滤 importance<3, 去重(SHA-256 + FTS5)
  │   │  → SQLiteStore.SaveFact() × N
  │   │
  │   │  依赖: LLMExecutor (memory route)
  │   │        SQLiteStore (LoadHistory + SaveFact)
  │   │
  │   │  输出: []FactEntry
  │
  ├─ runSignalDetection (if new facts > 0)
  │   │
  │   │  SQLiteStore.ListActiveFacts() → existing facts
  │   │  → LLM: "新事实是否强化/否定已有事实?" → JSON
  │   │  → EvidenceEngine.ApplySignal(ID, reinforce/dispute)
  │   │    ├─ reinforcement += 0.5 (间接)
  │   │    ├─ disputation += 1.0 (矛盾)
  │   │    └─ evidence score 计算 (半衰期30天/180天)
  │   │
  │   │  依赖: LLMExecutor (signal route)
  │   │        EvidenceEngine
  │   │
  │   │  输出: evidence scores updated
  │
  ├─ runReflectionAndDiary (if >=5 unabsorbed facts)
  │   │
  │   │  SQLiteStore.ListUnabsorbedFacts(5, 50)
  │   │  → LLM: "从facts生成反思insights + 日记" → JSON
  │   │  → SQLiteStore.SaveReflection() (status=pending)
  │   │  → SQLiteStore.SaveDiary()
  │   │  → MarkFactsAbsorbed()
  │   │
  │   │  依赖: LLMExecutor (memory route)
  │   │        SQLiteStore
  │   │
  │   │  输出: []ReflectionEntry + DiaryEntry
  │
  └─ runCompression
      │
      │  SessionBuffer 消息超过阈值 → LLM 压缩为摘要
      │  → SessionBuffer.SetMemo(summary)
      │
      │  依赖: LLMExecutor (summary route)
      │        Compressor + SessionBuffer
      │
      │  输出: memo text
```

---

## 四、维护循环 (每 30 分钟)

```
MemoryWorker.maintenanceLoop
  │
  ├─ runMaintenance
  │   │
  │   ├─ runPromotionSweep
  │   │   SQLiteStore.ListReflectionsByStatus(pending + confirmed)
  │   │   → evidence score computation
  │   │   ├─ score >= 1.0 → confirmed
  │   │   ├─ score >= 2.0 → promoted
  │   │   └─ score <= -2.0 → denied
  │   │   → transitionReflection(old, new, reason)
  │   │   → onReflectionPromoted → MarkFactsAbsorbed
  │   │
  │   ├─ detectContradictions
  │   │   同 entity 的 promoted reflections
  │   │   → 正负矛盾 → 弱方 denied
  │   │
  │   ├─ IdentityBuilder.BuildIdentity
  │   │   promoted reflections → LLM叙事
  │   │   → SelfModelStore.Save(UserModel, SelfModel)
  │   │
  │   └─ RunForgetting (sub-zero sweep)
  │
  └─ archiveTicker (每 1 小时)
      └─ EvidenceEngine.ArchiveSweep
          sub_zero_days >= 14 → archive facts
```

---

## 五、认知 Tick (每 60 秒)

```
CognitionTick.loop
  │
  │  触发: goroutine + time.Ticker(60s)
  │  首次延迟: 30s (给系统预热)
  │
  ▼
run(ctx)
  │
  ├─ DeliveryGate.TryAcquire()     ← CAS: IDLE → RUNNING, 防重入
  │
  ├─ FeatureExtractor.Extract()
  │   ├─ EmotionStore.Current()    → 8D → A组特征 (affection/worry/...)
  │   ├─ ScreenObserver.Observe()  → app category
  │   ├─ 时钟                      → night_time, hour, cooldown
  │   └─ Memory outcomes           → accept_rate, rejections
  │   → QuantifiedFeatures (52维, 实际填充~15维)
  │
  ├─ ComputeDrives(features, needs) → DriveVector
  │   ├─ Social  = 0.30*loneliness + 0.25*playfulness + 0.25*affection + ...
  │   ├─ Care    = 0.40*worry + 0.25*(1-confidence) + 0.20*workMin/120 + ...
  │   ├─ Curious = 0.35*curiosity + 0.20*learningMomentum + ...
  │   ├─ Quiet   = 0.50*sleepiness + 0.30*(1-playfulness) + ...
  │   └─ Explore = 0.30*curiosity + 0.25*loneliness + 0.25*learningMomentum + ...
  │
  ├─ ScoreActions(drives, features, 16 actions) → [16 ScoredActions]
  │   │  baseScore = Σ(drive × weight)
  │   │  modulators:
  │   │  ├─ warmth:        social action × (1.0~1.5) when affection>0.55
  │   │  ├─ source_accept: speak action × (0.5~1.0) from outcomes
  │   │  ├─ time_window:   social action × (0.4~1.0)
  │   │  └─ engagement:    speak action × (0.6~1.0)
  │   │  排序: FinalScore 降序
  │
  ├─ Route(scored, features) → DecisionResult
  │   │  System1 (94%): top1-top2 gap > 0.03 → 选最高分
  │   │  System2 (6%): 7 triggers → LLM 决定
  │
  ├─ HardGate(action, features) → (allowed, reason)
  │   ├─ night + !NightSafe → block
  │   ├─ quota exhausted → block
  │   └─ 2次连续被拒 → block
  │
  ├─ buildIntent(action) → ProactiveIntent
  │
  ├─ IntentScheduler.Submit(intent)
  │   ├─ TTL check (120s default)
  │   ├─ CoalesceKey dedup (同 key 替换为新)
  │   ├─ Recent history dedup (10-deque ring)
  │   └─ Priority 排序 → 入队
  │
  ├─ DeliveryGate.Release()    ← 释放 tick 锁
  │
  └─ DeliveryGate.CanRelease()
      ├─ !inflight && minGap > 3s
      └─ IntentDeliverer.Deliver(intents)
          ├─ LLM: Sion voice rephrase
          └─ SSE Broker.Publish("proactive", {message, source})
```

---

## 六、层间依赖矩阵

| | Transport | App | Emotion | Memory | Perception | Proactive | Domain | Port |
|---|:---:|:---:|:---:|:---:|:---:|:---:|:---:|:---:|
| **Transport** | - | ✅ | | | | | ✅ | ✅ |
| **App** | | - | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **Emotion** | | | - | | | | ✅ | ✅ |
| **Memory** | | | | - | | | ✅ | ✅ |
| **Perception** | | | ✅ | | - | | ✅ | ✅ |
| **Proactive** | ✅(SSE) | | ✅ | ✅ | ✅ | - | ✅ | ✅ |
| **Domain** | | | | | | | - | |
| **Port** | | | | | | | ✅ | - |

**规则**：
- 上层依赖下层（Transport → App → Adapter → Domain）
- 同层不直接依赖（Emotion 不 import Memory）
- Port 是 Adapter 层的契约接口
- Domain 是纯函数/纯类型，不依赖任何其他层
- 跨层通信通过 Port 接口，不直接调用具体实现

---

## 七、关键设计模式

| 模式 | 位置 | 说明 |
|------|------|------|
| Port/Adapter | 全项目 | 接口定义在 port/，实现在 adapter/，app 只依赖 port |
| Module 生命周期 | app/runtime.go | Init → Start → Stop，统一管理 |
| Hook 注入 | LLMHooks.Install() | 延迟绑定：worker 创建时不依赖 LLM，Init 时注入 |
| 异步 Wake | MemoryWorker.wakeCh | buffer 64，非阻塞发送，pool 竞争消费 |
| CAS 并发控制 | DeliveryGate.TryAcquire | atomic CAS 保证一次只有一个 cognition tick |
| EMA 平滑 | EmotionStore.emaBlend | α=0.3，新信号占 30%，旧状态占 70% |
| Drive×Action 评分 | cognition/motivator.go | 5维驱动 × 16个行动权重 = 统一的决策数学框架 |
| System1/2 路由 | cognition/router.go | 快路径纯数学 (94%)，慢路径 LLM (6%) |
| RRF 融合 | memory/recall.go | BM25 + Cosine → Reciprocal Rank Fusion → 统一排序 |
| 证据引擎 | domain/memory/evidence.go | 半衰期衰减 + 强化/争议信号 + 状态自动推导 |
