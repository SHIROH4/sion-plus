# Sion v1.1 重构架构设计

> **Status (2026-09)**: Learner / DPO-style weight-update sections below are historical design material. The loop is disabled because it has neither valid chosen/rejected pairs nor a connection to the production scorer. The active implementation is documented in `docs/contextual-preference-policy.md`.

> **设计基准**: 对齐 N.E.K.O 的工程化深度 + 保留 Sion 的核心创新优势
> **核心原则**: 模块化单进程（Modular Monolith）— 写时如微服务般解耦，运行时如单体般简洁
>
> **v1.1 升级 (2026-06-09)**:
> - 🆕 P0: Token 用量追踪（`port/token_tracker.go` + `adapter/llm/token_tracker.go`）
> - 🆕 P1: SchemaVersion 数据迁移（所有持久化类型 + ConfigManager 迁移链）
> - 🆕 P1: 多 Provider LLM 配置（fallback 链 + 路由表 + 健康检查）

---

## 目录

1. [设计原则](#1-设计原则)
2. [四层架构](#2-四层架构)
3. [目录结构](#3-目录结构)
4. [Port 层 — 全部接口定义](#4-port-层--全部接口定义)
5. [Domain 层 — 纯领域模型](#5-domain-层--纯领域模型)
6. [Adapter 层 — 接口实现](#6-adapter-层--接口实现)
7. [Application 层 — 服务编排](#7-application-层--服务编排)
8. [Transport 层 — HTTP/SSE 传输](#8-transport-层--httpssE-传输)
9. [插件系统](#9-插件系统)
10. [生命周期管理](#10-生命周期管理)
11. [DI 注入方案](#11-di-注入方案)
12. [配置管理](#12-配置管理)
13. [迁移路径](#13-迁移路径)

---

## 1. 设计原则

### 1.1 铁律（不可违反）

| # | 规则 | 原因 |
|---|------|------|
| 1 | **模块间只通过 `port` 接口通信** | 禁止直接 import 其他模块的 adapter 包 |
| 2 | **Domain 层零外部依赖** | 纯 Go struct + 纯函数，不依赖 infra/adapter/transport |
| 3 | **所有模块实现统一生命周期接口** | `Init → Start → Stop`，由 Launcher 统一编排 |
| 4 | **编译期 DI（Wire），非运行时反射** | 依赖完整性在编译时验证 |
| 5 | **一个 Adapter 实现一个 Port 接口** | 替换实现只需改 Wire 绑定，不动业务代码 |

### 1.2 设计取舍

| 决策 | 选择 | 理由 |
|------|------|------|
| 进程模型 | 单进程多 goroutine | 桌面应用不需要跨进程通信开销 |
| 服务通信 | Go interface + EventBus | 同进程走函数调用，不走网络 |
| 记忆存储 | SQLite + sqlite-vec | 桌面应用不能要求用户装 Postgres |
| 嵌入模型 | Ollama 本地（默认） / 云端 API（可选） | 隐私优先，离线可用 |
| LLM 协议 | OpenAI-compatible 为主，预留多 provider 扩展 | 够用但可扩展 |
| 前端 | Vue 3 + Naive UI + Wails | 保持不变 |
| 插件加载 | 编译时内置（当前）/ 运行时 Go plugin（未来） | 先简化，后扩展 |

---

## 2. 四层架构

```
┌──────────────────────────────────────────────────────────────┐
│                    Transport 层                              │
│  HTTP handlers / SSE broker / Wails bindings                 │
│  职责：参数校验、序列化、错误码转换、SSE推送                    │
│  不写业务逻辑，不直接访问数据库                                 │
├──────────────────────────────────────────────────────────────┤
│                   Application 层                             │
│  模块编排：MemoryService / ChatService / CognitionService...  │
│  职责：调用 port 接口编排业务流程，管理模块生命周期               │
│  不写具体算法，不直接操作 IO                                    │
├──────────────────────────────────────────────────────────────┤
│                     Adapter 层                                │
│  实现 port 接口：SQLiteMemoryStore / OpenAILLMGateway...      │
│  职责：具体的 IO、外部 API 调用、OS 交互                       │
│  不含领域知识，不互相调用                                       │
├──────────────────────────────────────────────────────────────┤
│                      Port 层                                  │
│  Go interface 定义：MemoryStore / LLMExecutor / EventBus...   │
│  职责：定义模块间的"宪法"，声明输入输出                         │
│  零实现代码，零外部依赖                                         │
├──────────────────────────────────────────────────────────────┤
│                     Domain 层                                 │
│  纯领域模型：Cognition / Emotion / Memory / Identity / Care   │
│  职责：所有业务逻辑、算法、状态机、公式                          │
│  零外部依赖（不 import infra/adapter/transport）               │
└──────────────────────────────────────────────────────────────┘
```

**依赖方向**：

```
Transport → Application → Port ← Adapter
                ↓           ↓        ↓
              Domain  ←──────────────┘
                ↑
             (零依赖)
```

---

## 3. 目录结构

```
oyasumi-sion/
├── cmd/
│   └── sion/
│       └── main.go                    # 单一入口，解析参数 → Launcher
│
├── internal/
│   ├── domain/                        # === 领域层（零外部依赖）===
│   │   ├── cognition/                 # 决策引擎
│   │   │   ├── features.go            # 52维特征计算（纯数学）
│   │   │   ├── drives.go              # 5维驱力公式
│   │   │   ├── actions.go             # 16动作定义 + 权重矩阵
│   │   │   ├── motivator.go           # 动作评分 + 上下文调制
│   │   │   ├── interval.go            # 动态调度间距
│   │   │   └── needs.go               # 6维内源需求模型
│   │   ├── emotion/                   # 情绪模型
│   │   │   ├── state.go               # PAD三维 + 8维向量
│   │   │   ├── decay.go               # 衰减公式（纯函数）
│   │   │   ├── smoothing.go           # EMA平滑（纯函数）
│   │   │   ├── circadian.go           # 昼夜节律
│   │   │   └── personality.go         # 人格参数
│   │   ├── memory/                    # 记忆领域
│   │   │   ├── buffer.go              # L0 会话缓冲（环形队列）
│   │   │   ├── evidence.go            # 证据分计算（双信号半衰期）
│   │   │   ├── forget.go              # Ebbinghaus 遗忘曲线
│   │   │   ├── ontology.go            # 本体论类型约束
│   │   │   └── merge.go               # 策略合并逻辑
│   │   ├── identity/                  # 身份/自我
│   │   │   ├── self_model.go          # SelfModel
│   │   │   ├── graph.go               # IdentityGraph 纯逻辑
│   │   │   └── relation_types.go      # 关系类型枚举
│   │   ├── care/                      # 关怀引擎
│   │   │   ├── triggers.go            # 触发条件
│   │   │   ├── state.go               # 用户关怀状态
│   │   │   └── observation.go         # 屏幕观察结果处理
│   │   └── types/                     # 共享类型
│   │       ├── message.go             # Message, ChatContext
│   │       ├── memory_entry.go        # FactEntry, DiaryEntry, Episode...
│   │       └── outcome.go             # ActionOutcome, DecisionResult
│   │
│   ├── port/                          # === 端口层（纯接口）===
│   │   ├── memory.go                  # MemoryStore, SessionBuffer, DiaryRepo...
│   │   ├── cognition.go               # FeatureComputer, DriveComputer, ActionScorer...
│   │   ├── emotion.go                 # EmotionStateManager, EmotionEvaluator
│   │   ├── llm.go                     # LLMExecutor, ToolExecutor, EmbeddingService
│   │   ├── perception.go              # ScreenObserver, OCREngine, AppClassifier
│   │   ├── identity.go                # IdentityRepository, SelfModelStore
│   │   ├── event.go                   # EventBus, EventPublisher, EventSubscriber
│   │   ├── proactive.go               # IntentScheduler, DeliveryGate, IntentDeliverer
│   │   ├── care.go                    # CareEngine, CareActionRepo
│   │   ├── learning.go                # EvidenceEngine, Learner, StrategyAgent
│   │   ├── expression.go              # Renderer, SpeechSynthesizer, ExpressionMapper
│   │   └── config.go                  # ConfigManager, ConfigValidator
│   │
│   ├── adapter/                       # === 适配器层（concrete 实现）===
│   │   ├── memory/
│   │   │   ├── sqlite_store.go         # 实现 port.MemoryStore
│   │   │   ├── session_buffer.go       # 实现 port.SessionBuffer
│   │   │   ├── evidence_engine.go      # 实现 port.EvidenceEngine
│   │   │   ├── recall.go               # 实现 BM25+向量+RRF 混合检索
│   │   │   └── compressor.go           # 实现多级内联压缩
│   │   ├── llm/
│   │   │   ├── openai_gateway.go       # 实现 port.LLMExecutor（OpenAI协议）
│   │   │   ├── ollama_embed.go         # 实现 port.EmbeddingService（Ollama本地）
│   │   │   └── provider_registry.go    # 多Provider注册表（预留）
│   │   ├── perception/
│   │   │   ├── screen_darwin.go        # 实现 port.ScreenObserver (macOS)
│   │   │   ├── screen_windows.go       # 实现 port.ScreenObserver (Windows)
│   │   │   ├── ocr_darwin.go           # 实现 port.OCREngine (Vision框架)
│   │   │   └── app_classifier.go       # 实现 port.AppClassifier
│   │   ├── emotion/
│   │   │   ├── emotion_store.go        # 实现 port.EmotionStateManager (SQLite持久化)
│   │   │   ├── llm_evaluator.go        # 实现 port.EmotionEvaluator (Kardia-R1)
│   │   │   └── rule_evaluator.go       # 实现 port.EmotionEvaluator (正则回退)
│   │   ├── identity/
│   │   │   ├── identity_repo.go        # 实现 port.IdentityRepository
│   │   │   └── self_model_store.go     # 实现 port.SelfModelStore
│   │   ├── expression/
│   │   │   ├── live2d_controller.go    # 实现 port.Renderer
│   │   │   ├── emotion_mapper.go       # 实现 port.ExpressionMapper (8维→Live2D参数)
│   │   │   └── tts_client.go           # 实现 port.SpeechSynthesizer
│   │   └── event/
│   │       └── event_bus.go            # 实现 port.EventBus（channel-based）
│   │
│   ├── app/                           # === 应用层（组装 + 生命周期）===
│   │   ├── launcher.go                 # Launcher — 编排所有模块生命周期
│   │   ├── wire.go                     # Wire 依赖注入定义
│   │   ├── wire_gen.go                 # Wire 自动生成（gitignore）
│   │   └── modules/
│   │       ├── chat_svc.go             # ChatService — 对话编排
│   │       ├── cognition_svc.go        # CognitionService — 后台决策循环
│   │       ├── memory_svc.go           # MemoryService — 记忆管线
│   │       ├── emotion_svc.go          # EmotionService — 情绪生命周期
│   │       ├── perception_svc.go       # PerceptionService — 屏幕观察调度
│   │       ├── care_svc.go             # CareService — 关怀引擎
│   │       ├── identity_svc.go         # IdentityService — 身份图谱
│   │       └── proactive_svc.go        # ProactiveService — IntentScheduler+Delivery
│   │
│   └── transport/                     # === 传输层 ===
│       ├── http/
│       │   ├── router.go               # HTTP 路由注册
│       │   ├── middleware.go            # CORS, ReadyCheck, 日志
│       │   ├── config_handler.go        # GET/POST /api/config
│       │   ├── chat_handler.go          # POST /api/chat/send, GET /api/chat/history
│       │   ├── memory_handler.go        # GET /api/memories, DELETE /api/memories/:id
│       │   ├── cognition_handler.go     # GET /api/features/current, /api/strategies
│       │   ├── emotion_handler.go       # GET /api/emotion
│       │   ├── plugin_handler.go        # GET/POST /api/plugins, PATCH /api/plugins/:id
│       │   ├── identity_handler.go      # GET/POST /api/identity
│       │   ├── stats_handler.go         # GET /api/stats, /api/learning/overview
│       │   └── proactive_handler.go     # GET /api/proactive/poll
│       ├── sse/
│       │   └── broker.go                # SSE 事件代理
│       └── wails/
│           └── bindings.go              # Wails 前端绑定（thin proxy → HTTP API）
│
├── plugin/                            # === 插件系统 ===
│   ├── sdk/                            # 插件 SDK
│   │   ├── plugin.go                   # Plugin 接口定义
│   │   ├── lifecycle.go                # 生命周期（Init/Start/Stop）
│   │   ├── config.go                   # 插件配置接口
│   │   └── function.go                 # FunctionProvider 接口
│   └── builtin/                        # 内置插件
│       ├── chat/
│       ├── memory/
│       ├── vision/
│       ├── search/
│       ├── qq/
│       └── timer/                      # 定时提醒插件（新增）
│
├── frontend/                           # Vue 3 前端（保持不变）
│   ├── src/
│   │   ├── components/
│   │   ├── pages/
│   │   ├── stores/
│   │   └── ...
│   └── ...
│
├── docs/
│   ├── ARCHITECTURE_V2.md              # 本文档
│   ├── ARCHITECTURE.md                 # v0.3 旧架构文档（保留对照）
│   └── ...
│
├── go.mod
├── go.sum
├── wails.json
└── Makefile                            # 构建/测试/lint 命令
```

---

## 4. Port 层 — 全部接口定义

### 4.1 port/memory.go — 记忆系统

```go
package port

import (
    "context"
    "time"
    "desktop-pet/internal/domain/types"
)

// MemoryStore is the central abstraction for all memory persistence.
// Implementations: adapter/memory/sqlite_store.go
type MemoryStore interface {
    // ── L0 会话缓冲 ──
    SessionBuffer() SessionBuffer

    // ── L1 日记（Episodic）──
    SaveDiary(ctx context.Context, entry types.DiaryEntry) error
    ListDiaries(ctx context.Context, limit int) ([]types.DiaryEntry, error)
    SearchDiaries(ctx context.Context, vector []float32, topK int) ([]types.DiaryEntry, error)
    ArchiveDiary(ctx context.Context, id int64) error
    DiaryCount(ctx context.Context) int

    // ── L2 事实（Semantic）──
    SaveFact(ctx context.Context, fact types.FactEntry) error
    SearchFacts(ctx context.Context, query string, topK int) ([]types.FactEntry, error)
    SearchFactsByVector(ctx context.Context, vector []float32, topK int) ([]types.FactEntry, error)
    ListActiveFacts(ctx context.Context, minScore float64) ([]types.FactEntry, error)
    ArchiveFact(ctx context.Context, id int64) error
    GetFact(ctx context.Context, id int64) (*types.FactEntry, error)

    // ── L3 策略原则 ──
    SaveStrategy(ctx context.Context, s types.StrategyPrinciple) (int64, error)
    ListActiveStrategies(ctx context.Context) ([]types.StrategyPrinciple, error)
    DeactivateStrategy(ctx context.Context, id int64) error
    SearchStrategiesByVector(ctx context.Context, vector []float32, topK int) ([]types.StrategyPrinciple, error)

    // ── 对话历史 ──
    SaveHistory(ctx context.Context, msgs []types.Message) error
    LoadHistory(ctx context.Context, limit int) ([]types.Message, error)
    CleanOldHistory(ctx context.Context, olderThanDays int) error

    // ── 维护 ──
    RunForgetting(ctx context.Context) error   // Ebbinghaus 衰减扫描
    Close() error
}

// SessionBuffer is working memory (L0).
type SessionBuffer interface {
    Append(msg types.Message)
    Recent(n int) []types.Message
    All() []types.Message
    Len() int
    Clear()
    Snapshot() ([]types.Message, []time.Time)
    Restore(msgs []types.Message, timestamps []time.Time)
}

// EvidenceEngine manages evidence scores for memory entries.
// See docs: memory-evidence system (from N.E.K.O reference).
type EvidenceEngine interface {
    // ApplySignal applies a reinforcement or disputation delta.
    ApplySignal(ctx context.Context, entryID string, sig EvidenceSignal) (*EvidenceSnapshot, error)

    // Score computes current effective evidence (with half-life decay).
    Score(entry types.MemoryEvidenceEntry) float64

    // DeriveStatus maps score to a status tier.
    DeriveStatus(score float64, entry types.MemoryEvidenceEntry) EvidenceStatus

    // ArchiveSweep runs periodic archival (sub_zero_days tracking).
    ArchiveSweep(ctx context.Context) error
}

type EvidenceSignal struct {
    EntryID   string
    Type      SignalType // "user_confirm" | "user_deny" | "user_fact" | "behavior_match" | "contradiction"
    Strength  float64    // -1.0 ~ +1.0
    Source    string     // "chat" | "action_outcome" | "reflection"
    Timestamp time.Time
}

type SignalType string
const (
    SignalUserConfirm     SignalType = "user_confirm"
    SignalUserDeny        SignalType = "user_deny"
    SignalUserFact        SignalType = "user_fact"
    SignalBehaviorMatch   SignalType = "behavior_match"
    SignalContradiction   SignalType = "contradiction"
)

type EvidenceStatus string
const (
    StatusPending          EvidenceStatus = "pending"
    StatusConfirmed        EvidenceStatus = "confirmed"
    StatusPromoted         EvidenceStatus = "promoted"
    StatusArchiveCandidate EvidenceStatus = "archive_candidate"
)

type EvidenceSnapshot struct {
    Reinforcement    float64
    Disputation      float64
    EvidenceScore    float64
    Status           EvidenceStatus
    ComboCount       int
    ReinLastSignalAt time.Time
    DispLastSignalAt time.Time
}

// MemoryRecall performs hybrid retrieval.
type MemoryRecall interface {
    // HybridSearch combines BM25 keyword + cosine vector + RRF fusion.
    HybridSearch(ctx context.Context, query string, topK int) ([]MemorySearchResult, error)

    // VectorSearch is the fast path when embedding is available.
    VectorSearch(ctx context.Context, vector []float32, topK int) ([]MemorySearchResult, error)
}

type MemorySearchResult struct {
    ID       int64
    Content  string
    Source   string // "fact" | "diary" | "strategy"
    Score    float64
    Evidence *EvidenceSnapshot
}
```

### 4.2 port/cognition.go — 决策引擎

```go
package port

import (
    "context"
    "time"
    "desktop-pet/internal/domain/types"
)

// FeatureComputer computes the 52-dimension quantified features.
type FeatureComputer interface {
    ComputeFull(ctx context.Context, state *CognitionState) (*types.QuantifiedFeatures, error)
    // Tier 1: pure in-memory (~1ms), runs every tick.
    // Tier 2: SQL-backed (~50ms), TTL-cached for 5 minutes.
}

type CognitionState struct {
    Emotion      types.EmotionState
    EmotionVec   types.EmotionVector
    Needs        types.IntrinsicNeeds
    CareState    types.UserCareState
    LastChatAt   time.Time
    ActionCount  int
    ConsecutiveAction string
    ConsecutiveCount  int
}

// DriveComputer maps 52 features → 5 drives.
type DriveComputer interface {
    Compute(ctx context.Context, features *types.QuantifiedFeatures, needs *types.IntrinsicNeeds) (*types.DriveVector, error)
}

// ActionScorer maps 5 drives → 16 action scores.
type ActionScorer interface {
    Score(ctx context.Context, drives *types.DriveVector, features *types.QuantifiedFeatures) ([]ScoredAction, error)
    UpdateWeight(action string, drive string, delta float64) error
    LoadWeights() error
    SaveWeights() error
}

type ScoredAction struct {
    Action      types.ActionDef
    RawScore    float64
    FinalScore  float64  // after context modulation
    Modulators  map[string]float64  // which modulators applied
}

// DecisionRouter implements System 1 / System 2 routing.
type DecisionRouter interface {
    Route(ctx context.Context, scored []ScoredAction, features *types.QuantifiedFeatures) (*types.DecisionResult, error)
    // Returns: FastPath=true → use top scored action directly
    //          FastPath=false → need LLM fallback
}

// NeedModel manages the 6 intrinsic needs.
type NeedModel interface {
    Grow(elapsedHours float64) *types.IntrinsicNeeds
    Satisfy(action string, outcome types.OutcomeResult)
    Current() *types.IntrinsicNeeds
    Modulation() *types.NeedModulation
}
```

### 4.3 port/emotion.go — 情绪系统

```go
package port

import (
    "context"
    "desktop-pet/internal/domain/types"
)

// EmotionStateManager is the persistent emotion model.
type EmotionStateManager interface {
    // Current returns the current PAD state and 8D vector.
    Current() (types.EmotionState, types.EmotionVector)

    // Evaluate processes a new interaction and updates the emotional state.
    Evaluate(ctx context.Context, recentTurns string) error

    // NotifyActivity records user presence (mouse, keyboard, chat).
    NotifyActivity()

    // Load restores persisted state from storage.
    Load() error

    // Save persists current state.
    Save() error

    // SetPersonality overrides the personality baseline.
    SetPersonality(p types.PersonalityScale)

    // Personality returns the current learned personality.
    Personality() types.PersonalityScale

    // LearnPersonality adjusts personality from outcome history.
    LearnPersonality(ctx context.Context) error

    // SetNeedModulation injects need-driven decay modifiers.
    SetNeedModulation(mod *types.NeedModulation)

    // History returns emotion history (for visualization).
    History() ([]types.EmotionState, []types.EmotionVector)

    // Stop stops the background decay goroutine.
    Stop()
}

// EmotionEvaluator evaluates raw conversation into an emotion delta.
type EmotionEvaluator interface {
    Evaluate(ctx context.Context, recentTurns string) (*EmotionEvalResult, error)
}

type EmotionEvalResult struct {
    State  types.EmotionState
    Vector types.EmotionVector
    Source string // "llm" | "rule" | "cache"
}

// ExpressionMapper maps the 8D emotion vector to avatar parameters.
type ExpressionMapper interface {
    MapToParameters(vec types.EmotionVector) (*ExpressionParams, error)
}

type ExpressionParams struct {
    EyeOpen      float64  // 0~1
    MouthOpen    float64  // 0~1
    BrowAngle    float64  // -1~1
    BlushIntensity float64 // 0~1
    Motion       string   // "idle" | "happy" | "sad" | "angry" | "surprised"
    // Extensible for VRM/MMD later
}
```

### 4.4 port/llm.go — LLM 网关

```go
package port

import "context"

// LLMExecutor is the unified LLM calling interface.
type LLMExecutor interface {
    // Chat performs a synchronous chat completion.
    Chat(ctx context.Context, msgs []LLMMessage) (string, error)

    // ChatWithTools performs chat with function calling.
    ChatWithTools(
        ctx context.Context,
        msgs []LLMMessage,
        tools []ToolDef,
        onToolCall func(name, argsJSON string) string,
        maxRounds int,
        toolChoice string, // "auto" | "required" | "none"
    ) (string, error)

    // ChatStream performs streaming chat completion.
    ChatStream(ctx context.Context, msgs []LLMMessage, onChunk func(chunk string) error) error
}

type LLMMessage struct {
    Role    string // "system" | "user" | "assistant" | "tool"
    Content string
}

type ToolDef struct {
    Name        string
    Description string
    Parameters  map[string]any // JSON Schema
}

// EmbeddingService computes text embeddings.
type EmbeddingService interface {
    Vectorize(ctx context.Context, text string) ([]float32, error)
    BatchVectorize(ctx context.Context, texts []string) ([][]float32, error)
    IsAvailable() bool
    ModelName() string
}

// LLMProviderRegistry manages multiple LLM providers.
type LLMProviderRegistry interface {
    Register(name string, config LLMProviderConfig) error
    GetExecutor(provider string) (LLMExecutor, error)
    GetEmbedding(provider string) (EmbeddingService, error)
    ListProviders() []string
    SetDefault(provider string)
}

type LLMProviderConfig struct {
    Name      string // "openai" | "deepseek" | "qwen" | "ollama"...
    BaseURL   string
    APIKey    string
    ChatModel string
    EmbedModel string
}
```

### 4.5 port/perception.go — 感知系统

```go
package port

import "context"

// ScreenObserver captures and analyzes the user's screen.
type ScreenObserver interface {
    Observe(ctx context.Context) (*ScreenObservation, error)
    IsAvailable() bool
}

type ScreenObservation struct {
    AppName     string
    AppCategory string // "work" | "play" | "social" | "idle"
    WindowTitle string
    OCRText     string
    Screenshot  []byte // compressed JPEG
    Timestamp   int64
}

// OCREngine extracts text from images.
type OCREngine interface {
    ExtractText(ctx context.Context, imagePath string) (string, error)
    IsAvailable() bool
}

// AppClassifier categorizes the active window.
type AppClassifier interface {
    Classify(appName string, windowTitle string) AppCategory
    ClassifyWithLLM(ctx context.Context, obs *ScreenObservation) (AppCategory, error)
}

type AppCategory struct {
    Primary  string // "work" | "play" | "social" | "idle"
    Subtype  string // "coding" | "debugging" | "gaming" | "chatting"...
    IsWorking bool
}
```

### 4.6 port/proactive.go — 主动行为调度

```go
package port

import (
    "context"
    "time"
)

// ProactiveIntent represents a desire for the AI to act/speak.
type ProactiveIntent struct {
    ID          string
    Source      string // "cognition" | "plugin:qq" | "plugin:timer"...
    Action      string // "speak_casual" | "care_rest" | "search"...
    Message     string // the prompt/instruction for LLM
    Priority    int    // 0-10, higher = more urgent
    CoalesceKey string // same key → newer replaces older
    TTL         time.Duration
    MediaImages []string // optional attached images
    CreatedAt   time.Time
}

// IntentScheduler queues, prioritizes, and coalesces proactive intents.
// Reference: N.E.K.O ProactiveDeliveryManager
type IntentScheduler interface {
    Submit(ctx context.Context, intent ProactiveIntent) error
    Drain() []ProactiveIntent // returns all queued in priority order
    Stats() SchedulerStats
}

type SchedulerStats struct {
    Queued     int
    DroppedTTL int
    Coalesced  int
}

// DeliveryGate decides whether the AI can speak/act now.
type DeliveryGate interface {
    CanRelease(ctx context.Context) bool
    OnPlaybackStart(ctx context.Context)
    OnPlaybackEnd(ctx context.Context)
    Reset(ctx context.Context)
}

type DeliveryGateState struct {
    IsPlaying    bool
    IsInflight   bool
    LastReleaseAt time.Time
    MinGap       time.Duration
}

// IntentDeliverer actually executes the intent (LLM → TTS → display).
type IntentDeliverer interface {
    Deliver(ctx context.Context, intents []ProactiveIntent) (*DeliveryResult, error)
}

type DeliveryResult struct {
    Delivered int
    Output    string
    Source    string
}
```

### 4.7 port/event.go — 事件总线

```go
package port

// EventBus provides publish-subscribe for inter-module communication.
type EventBus interface {
    // Publish emits an event to all matching subscribers.
    Publish(topic string, payload any)

    // Subscribe registers a handler for a topic.
    // Returns an unsubscribe function.
    Subscribe(topic string, handler func(payload any)) (unsubscribe func())

    // SubscribePattern registers for topics matching a glob pattern.
    SubscribePattern(pattern string, handler func(topic string, payload any)) (unsubscribe func())

    // SubscriberCount returns the number of subscribers for a topic.
    SubscriberCount(topic string) int
}

// Standard topics:
const (
    TopicUserActive      = "user:active"
    TopicUserIdle        = "user:idle"
    TopicUserWorking     = "user:working"
    TopicChatSent        = "chat:sent"
    TopicChatReceived    = "chat:received"
    TopicMemoryUpdated   = "memory:updated"
    TopicMemoryForgotten = "memory:forgotten"
    TopicEmotionChanged  = "emotion:changed"
    TopicCareAction      = "care:action"
    TopicDecisionMade    = "cognition:decision"
    TopicPlaybackStart   = "playback:start"
    TopicPlaybackEnd     = "playback:end"
    TopicAppChanged      = "perception:app_changed"
    TopicPluginEvent     = "plugin:event"
)
```

### 4.8 port/config.go — 配置管理

```go
package port

import "context"

// ConfigManager manages application configuration with versioning.
type ConfigManager interface {
    Load() (*AppConfig, error)
    Save(cfg *AppConfig) error

    // GetPluginConfig returns the config subtree for a specific plugin.
    GetPluginConfig(pluginName string) (map[string]any, error)

    // SetPluginConfig updates a plugin's config subtree.
    SetPluginConfig(pluginName string, cfg map[string]any) error

    // Migrate runs pending config migrations (version-based).
    Migrate(ctx context.Context) error

    // Validate checks the loaded config for correctness.
    Validate(cfg *AppConfig) error

    // OnChange registers a callback for config hot-reload.
    OnChange(handler func(*AppConfig)) (unsubscribe func())
}

type AppConfig struct {
    Version    int              `yaml:"version"`
    LLM        LLMConfig        `yaml:"llm"`
    Vision     VisionConfig     `yaml:"vision"`
    Emotion    EmotionConfig    `yaml:"emotion"`
    Memory     MemoryConfig     `yaml:"memory"`
    Proactive  ProactiveConfig  `yaml:"proactive"`
    User       UserConfig       `yaml:"user"`
    WarmStart  WarmStartConfig  `yaml:"warm_start"`
    Plugins    map[string]map[string]any `yaml:"plugins"`
}
```

### 4.9 port/learning.go — 自学习

```go
package port

import "context"

// Learner performs DPO-style weight updates from action outcomes.
type Learner interface {
    RecordDrive(action string, drives *DriveSnapshot, reward float64) int
    UpdateReward(driveID int, reward float64)
    ShouldLearn() bool
    BatchLearn(ctx context.Context) int
    Audit(ctx context.Context) (*AuditResult, error)
}

type DriveSnapshot struct {
    Social  float64
    Care    float64
    Curious float64
    Quiet   float64
    Explore float64
}

type AuditResult struct {
    StuckActions []string
    DriftWarning bool
    Recommendation string
}

// StrategyAgent performs periodic reflection and strategy distillation.
type StrategyAgent interface {
    ShouldRun() bool
    Run(ctx context.Context) (*StrategyOutput, error)
    LastRun() int64
}

type StrategyOutput struct {
    SelfModelUpdate      string
    NewPrinciples        []StrategyRefinement
    DeactivatePrincipleIDs []int64
    ThreadRecommendations  []ThreadRec
    TacticalDirectives   []string
    NarrativeSummary     string
}

// CuriosityEngine scans for knowledge gaps and schedules exploration.
type CuriosityEngine interface {
    ScanGaps(ctx context.Context) ([]KnowledgeGap, error)
    ShouldExplore(gap KnowledgeGap) bool
    ExecuteExploration(ctx context.Context, gap KnowledgeGap) error
}

type KnowledgeGap struct {
    Topic        string
    Source       string // "fact_contradiction" | "thread_dormant" | "pattern_incomplete"
    Priority     float64
    LastExplored int64
}
```

---

## 5. Domain 层 — 纯领域模型

Domain 层是**零依赖**的纯 Go 代码。只包含 struct 定义、纯函数、常量。

### 5.1 domain/types/memory_entry.go

```go
package types

import "time"

// ── 状态定义 ──

type EvidenceStatus string
const (
    EvPending    EvidenceStatus = "pending"
    EvConfirmed  EvidenceStatus = "confirmed"
    EvPromoted   EvidenceStatus = "promoted"
    EvArchived   EvidenceStatus = "archived"
    EvMerged     EvidenceStatus = "merged"
)

// ── 本体论：关系类型 ──

type EntityKind string
const (
    KindUser         EntityKind = "user"
    KindCharacter    EntityKind = "character"
    KindRelationship EntityKind = "relationship"
)

type RelationType string
const (
    // user kind
    RelPreference RelationType = "preference"
    RelTrait      RelationType = "trait"
    RelHabit      RelationType = "habit"
    RelIdentity   RelationType = "identity"
    RelEmotional  RelationType = "emotional"
    RelBoundary   RelationType = "boundary"
    // character kind
    RelSelfAwareness RelationType = "self_awareness"
    RelLearned       RelationType = "learned"
    RelRoleNote      RelationType = "role_note"
    // relationship kind
    RelDynamic      RelationType = "dynamic"
    RelMilestone    RelationType = "milestone"
    RelTension      RelationType = "tension"
    RelSharedMemory RelationType = "shared_memory"
    RelAgreement    RelationType = "agreement"
)

// ── 记忆条目 ──

type FactEntry struct {
    ID             int64
    Entity         string        // "master" | "neko" | "relationship"
    RelationType   RelationType
    Content        string
    Importance     int           // 1-10
    Evidence       MemoryEvidenceEntry
    RecallCount    int
    LastRecalledAt time.Time
    Source         string        // "chat" | "reflection" | "warm_start"
    Archived       bool
    CreatedAt      time.Time
    UpdatedAt      time.Time
}

type MemoryEvidenceEntry struct {
    Reinforcement        float64
    Disputation          float64
    ReinLastSignalAt     time.Time
    DispLastSignalAt     time.Time
    ReinComboCount       int
    Protected            bool
    SubZeroDays          int
    SubZeroLastIncrDate  string
}

type DiaryEntry struct {
    ID              int64
    Title           string
    Summary         string
    EmotionValence  float64
    EmotionArousal  float64
    EmotionPrimary  string
    Vector          []float32
    Archived        bool
    CreatedAt       time.Time
}

type StrategyPrinciple struct {
    ID           int64
    Situation    string
    GoodStrategy string
    BadStrategy  string
    Reason       string
    Confidence   float64
    Source       string
    Vector       []float32
    Active       bool
    CreatedAt    int64
    UpdatedAt    int64
}
```

### 5.2 domain/cognition/features.go — 52维特征（核心保留）

```go
package cognition

// QuantifiedFeatures is the 52-dimension state vector.
// Same as current v0.3 implementation — this is your IP.
type QuantifiedFeatures struct {
    // A组: Agent 自身 (13维)
    A1_Affection   float64
    A1_Worry       float64
    A1_Curiosity   float64
    A1_Sleepiness  float64
    A1_Playfulness float64
    A1_Loneliness  float64
    A1_Confidence  float64
    A1_Annoyance   float64
    A2_PrimaryEmotion string
    A3_Intensity     float64
    A4_ValenceTrend  float64
    A5_AnnoySensitivity float64
    A5_AffectWarmth    float64
    A5_WorryTendency   float64
    A6_DailyActionCount  int
    A7_ActionSuccessRate map[string]float64
    A8_TimeBlockRate     map[string]float64
    A10_ActiveGoals      int
    A11_ActiveInquiries  int
    A12_KnowledgeGaps    int
    A13_LearningMomentum float64
    A14_ConsecutiveCount int

    // U组: User 状态 (14维)
    U1_AppCategory       string
    U2_WindowSubtype     string
    U3_IsWorking         float64
    U4_ContinuousWorkMins float64
    U5_AppSwitchCount    float64
    U7_LengthTrend       float64
    U8_EngagementNorm    float64
    U10_TimeWindowPref   float64
    U11_MealTime         float64
    U12_NightTime        float64
    U13_IsWeekend        float64
    U14_TimeSinceChatMins float64
    U15_FatigueMentionHrs float64
    U16_PrefDiversity    float64

    // E组: Environment (7维)
    E1_Hour              int
    E2_DayOfWeek         int
    E3_CooldownNorm      float64
    E4_QuotaRemaining    int
    E5_MinsSinceDecision float64
    E6_LLMAvailable      bool
    E7_ReflectionDue     float64

    // R组: Relationship (8维)
    R1_OverallAcceptRate  float64
    R1_SampleCount        float64
    R2_TimeWindowAccept   float64
    R3_SourceAcceptRate   map[string]float64
    R4_RecentRejections   float64
    R4_RejectionSeverity  float64
    R5_NeglectHours       float64
    R6_DepthTrend         float64
    R7_UserInitiative24h  float64
    R8_IntimacyTrend      float64

    // T组: Task Context (3维)
    T1_PrincipleCount    float64
    T2_PatternCount      float64
    T3_ReflexionLogCount float64
    T5_TodayActivityCount float64

    ComputedAt int64
}
```

### 5.3 domain/cognition/drives.go — 5维驱力公式

```go
// DriveVector is the 5-dimensional drive output.
type DriveVector struct {
    Social  float64 // [0,1] 社交驱力
    Care    float64 // [0,1] 关怀驱力
    Curious float64 // [0,1] 好奇驱力
    Quiet   float64 // [0,1] 静默驱力
    Explore float64 // [0,1] 探索驱力
}

// ComputeDrives is the PURE FUNCTION mapping 52 features → 5 drives.
// All weighted formulas from v0.3 ARCHITECTURE.md §4.2 preserved.
func ComputeDrives(f *QuantifiedFeatures, n *IntrinsicNeeds) *DriveVector { ... }

// interactionGate attenuates social/care drives based on acceptance rate.
func interactionGate(acceptRate float64) float64 { ... }
```

### 5.4 domain/memory/evidence.go — 证据分（从N.E.K.O移植）

```go
package memory

import (
    "math"
    "time"
    "desktop-pet/internal/domain/types"
)

// Constants (configurable via ConfigManager).
const (
    DefaultReinHalfLifeDays  = 60   // reinforcement half-life
    DefaultDispHalfLifeDays  = 14   // disputation half-life (shorter — bad memories fade faster)
    ConfirmedThreshold       = 1.0
    PromotedThreshold        = 2.0
    ArchiveThreshold         = 0.0
    ComboThreshold           = 3    // consecutive reinforces to trigger combo
    ComboBonus               = 0.5
    BaseReinDelta            = 0.5
)

// EffectiveReinforcement computes decayed reinforcement at now.
func EffectiveReinforcement(entry types.MemoryEvidenceEntry, now time.Time) float64 {
    r := entry.Reinforcement
    if r == 0 {
        return 0
    }
    age := daysSince(entry.ReinLastSignalAt, now)
    if age <= 0 {
        return r
    }
    return r * math.Pow(0.5, age/DefaultReinHalfLifeDays)
}

// EffectiveDisputation computes decayed disputation at now.
func EffectiveDisputation(entry types.MemoryEvidenceEntry, now time.Time) float64 {
    d := entry.Disputation
    if d == 0 {
        return 0
    }
    age := daysSince(entry.DispLastSignalAt, now)
    if age <= 0 {
        return d
    }
    return d * math.Pow(0.5, age/DefaultDispHalfLifeDays)
}

// EvidenceScore is the net evidence strength.
func EvidenceScore(entry types.MemoryEvidenceEntry, now time.Time) float64 {
    if entry.Protected {
        return math.Inf(1)
    }
    return EffectiveReinforcement(entry, now) - EffectiveDisputation(entry, now)
}

// DeriveStatus maps score to status tier.
func DeriveStatus(score float64) types.EvidenceStatus {
    if score >= PromotedThreshold {
        return types.EvPromoted
    }
    if score >= ConfirmedThreshold {
        return types.EvConfirmed
    }
    if score <= ArchiveThreshold {
        return types.EvArchived
    }
    return types.EvPending
}

// ApplySignal computes the new evidence snapshot after a signal delta.
func ApplySignal(entry types.MemoryEvidenceEntry, delta EvidenceDelta, now time.Time) types.MemoryEvidenceEntry {
    // reinforcement delta
    if delta.ReinDelta != 0 {
        entry.Reinforcement += delta.ReinDelta
        entry.ReinLastSignalAt = now
    }
    // disputation delta (non-negative)
    if delta.DispDelta != 0 {
        entry.Disputation = math.Max(0, entry.Disputation+delta.DispDelta)
        entry.DispLastSignalAt = now
    }
    // combo logic
    if delta.Source == "user_fact" && delta.ReinDelta > 0 {
        entry.ReinComboCount++
        if entry.ReinComboCount > ComboThreshold {
            entry.Reinforcement += ComboBonus
        }
    }
    return entry
}

type EvidenceDelta struct {
    ReinDelta float64
    DispDelta float64
    Source    string
}

func daysSince(ts time.Time, now time.Time) float64 {
    if ts.IsZero() {
        return 0
    }
    return now.Sub(ts).Hours() / 24
}
```

---

## 6. Adapter 层 — 接口实现

### 6.1 adapter/memory/recall.go — 混合检索（从N.E.K.O移植BM25+RRF）

```go
package memory

// HybridRecall implements port.MemoryRecall with BM25 + vector + RRF fusion.
type HybridRecall struct {
    embedSvc port.EmbeddingService
    store    port.MemoryStore
}

func (r *HybridRecall) HybridSearch(ctx context.Context, query string, topK int) ([]port.MemorySearchResult, error) {
    // 1. BM25 path — keyword-based retrieval
    bm25Results := r.bm25Search(query, topK*2)

    // 2. Cosine path — embedding-based retrieval
    var cosineResults []scoredDoc
    if r.embedSvc != nil && r.embedSvc.IsAvailable() {
        vec, err := r.embedSvc.Vectorize(ctx, query)
        if err == nil {
            cosineResults = r.cosineSearch(vec, topK*2)
        }
    }

    // 3. RRF fusion
    fused := rrfFuse(bm25Results, cosineResults, 60)

    // 4. Cap at topK
    if len(fused) > topK {
        fused = fused[:topK]
    }
    return fused, nil
}

// rrfFuse applies Reciprocal Rank Fusion with k=60.
func rrfFuse(a, b []scoredDoc, k float64) []port.MemorySearchResult { ... }
```

### 6.2 adapter/memory/evidence_engine.go

```go
package memory

type evidenceEngine struct {
    store port.MemoryStore
    mu    sync.Mutex
}

func NewEvidenceEngine(store port.MemoryStore) port.EvidenceEngine {
    return &evidenceEngine{store: store}
}

func (e *evidenceEngine) ApplySignal(ctx context.Context, entryID string, sig port.EvidenceSignal) (*port.EvidenceSnapshot, error) {
    e.mu.Lock()
    defer e.mu.Unlock()

    entry, err := e.store.GetFact(ctx, parseEntryID(entryID))
    if err != nil {
        return nil, fmt.Errorf("evidence: entry not found: %w", err)
    }

    now := time.Now()
    evidence := entry.Evidence

    switch sig.Type {
    case port.SignalUserConfirm, port.SignalUserFact, port.SignalBehaviorMatch:
        evidence = memory.ApplySignal(evidence, memory.EvidenceDelta{
            ReinDelta: memory.BaseReinDelta * sig.Strength,
            Source:    string(sig.Type),
        }, now)
    case port.SignalUserDeny, port.SignalContradiction:
        evidence = memory.ApplySignal(evidence, memory.EvidenceDelta{
            DispDelta: memory.BaseReinDelta * math.Abs(sig.Strength),
            Source:    string(sig.Type),
        }, now)
    }

    score := memory.EvidenceScore(evidence, now)
    status := memory.DeriveStatus(score)

    // persist
    entry.Evidence = evidence
    if err := e.store.SaveFact(ctx, entry); err != nil {
        return nil, err
    }

    return &port.EvidenceSnapshot{
        Reinforcement:    evidence.Reinforcement,
        Disputation:      evidence.Disputation,
        EvidenceScore:    score,
        Status:           status,
        ComboCount:       evidence.ReinComboCount,
        ReinLastSignalAt: evidence.ReinLastSignalAt,
        DispLastSignalAt: evidence.DispLastSignalAt,
    }, nil
}
```

---

## 7. Application 层 — 服务编排

### 7.1 app/modules/cognition_svc.go — 决策服务

```go
package modules

// CognitionService owns the background decision loop.
// It is the primary producer of ProactiveIntents from internal tick.
type CognitionService struct {
    features    port.FeatureComputer
    drives      port.DriveComputer
    scorer      port.ActionScorer
    router      port.DecisionRouter
    needs       port.NeedModel
    emotion     port.EmotionStateManager
    perception  port.ScreenObserver
    scheduler   port.IntentScheduler
    learner     port.Learner
    eventBus    port.EventBus
    interval    *cognition.IntervalCalculator
    stopCh      chan struct{}
}

func (s *CognitionService) Name() string { return "cognition" }

func (s *CognitionService) Start(ctx context.Context) error {
    go s.loop(ctx)
    return nil
}

func (s *CognitionService) Stop(ctx context.Context) error {
    close(s.stopCh)
    return nil
}

func (s *CognitionService) loop(ctx context.Context) {
    ticker := time.NewTicker(s.interval.BaseInterval())
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-s.stopCh:
            return
        case <-ticker.C:
            s.tick(ctx)
            // Adapt interval dynamically (your DynamicInterval from v0.3)
            nextInterval := s.interval.Calculate(s.currentState())
            ticker.Reset(nextInterval)
        }
    }
}

func (s *CognitionService) tick(ctx context.Context) {
    // 1. Observe screen
    obs, _ := s.perception.Observe(ctx)

    // 2. Read current state
    emotState, emotVec := s.emotion.Current()
    needs := s.needs.Current()

    state := &port.CognitionState{
        Emotion:    emotState,
        EmotionVec: emotVec,
        Needs:      *needs,
        // ... populate from obs and state
    }

    // 3. Compute features (Tier 1 + Tier 2)
    features, err := s.features.ComputeFull(ctx, state)
    if err != nil {
        return
    }

    // 4. Compute drives
    drives, err := s.drives.Compute(ctx, features, needs)
    if err != nil {
        return
    }

    // 5. Score actions
    scored, err := s.scorer.Score(ctx, drives, features)
    if err != nil {
        return
    }

    // 6. Route (System 1 / System 2)
    decision, err := s.router.Route(ctx, scored, features)
    if err != nil {
        return
    }

    // 7. If System 1 fast path → submit proactive intent
    if decision.FastPath && decision.Action != nil && decision.Action.Name != "none" {
        intent := port.ProactiveIntent{
            ID:        uuid.New().String(),
            Source:    "cognition",
            Action:    decision.Action.Name,
            Message:   decision.Action.SkillCard.GeneratePrompt(features),
            Priority:  s.mapScoreToPriority(decision.Action.FinalScore),
            TTL:       5 * time.Minute,
            CreatedAt: time.Now(),
        }
        s.scheduler.Submit(ctx, intent)

        // Record for learning
        driveID := s.learner.RecordDrive(
            decision.Action.Name,
            &port.DriveSnapshot{drives.Social, drives.Care, drives.Curious, drives.Quiet, drives.Explore},
            0, // reward pending
        )
        s.pendingDriveID = driveID
    }

    // 8. System 2 → trigger LLM fallback in delivery pipeline
    if !decision.FastPath {
        // ... handled by IntentDeliverer
    }

    // 9. Update needs (grow)
    s.needs.Grow(0)
    s.needs.Modulation() // → feed to emotion
}
```

---

## 8. Transport 层 — HTTP/SSE 传输

### 8.1 transport/http/router.go

```go
package http

func NewRouter(
    configHandler  *ConfigHandler,
    chatHandler    *ChatHandler,
    memoryHandler  *MemoryHandler,
    cognitionHandler *CognitionHandler,
    emotionHandler *EmotionHandler,
    pluginHandler  *PluginHandler,
    identityHandler *IdentityHandler,
    statsHandler   *StatsHandler,
    proactiveHandler *ProactiveHandler,
    sseBroker      *sse.Broker,
) http.Handler {
    mux := http.NewServeMux()

    // Config
    mux.HandleFunc("GET /api/config", configHandler.Get)
    mux.HandleFunc("POST /api/config", configHandler.Update)

    // Chat
    mux.HandleFunc("POST /api/chat/send", chatHandler.Send)
    mux.HandleFunc("GET /api/chat/history", chatHandler.History)

    // Memory
    mux.HandleFunc("GET /api/memories", memoryHandler.List)
    mux.HandleFunc("DELETE /api/memories/{id}", memoryHandler.Delete)
    mux.HandleFunc("GET /api/diaries", memoryHandler.ListDiaries)

    // Cognition
    mux.HandleFunc("GET /api/features/current", cognitionHandler.GetFeatures)
    mux.HandleFunc("GET /api/strategies", cognitionHandler.ListStrategies)

    // Emotion
    mux.HandleFunc("GET /api/emotion", emotionHandler.Get)

    // Plugins
    mux.HandleFunc("GET /api/plugins", pluginHandler.List)
    mux.HandleFunc("POST /api/plugins/{name}", pluginHandler.UpdateConfig)
    mux.HandleFunc("PATCH /api/plugins/{name}/toggle", pluginHandler.Toggle)

    // Identity
    mux.HandleFunc("GET /api/identity", identityHandler.List)
    mux.HandleFunc("POST /api/identity", identityHandler.Upsert)
    mux.HandleFunc("POST /api/identity/self-update", identityHandler.SelfUpdate)

    // Stats
    mux.HandleFunc("GET /api/stats", statsHandler.Dashboard)
    mux.HandleFunc("GET /api/learning/overview", statsHandler.LearningOverview)

    // Proactive
    mux.HandleFunc("GET /api/proactive/poll", proactiveHandler.Poll)

    // SSE
    mux.HandleFunc("GET /api/events", sseBroker.Handler())

    // Health
    mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(200)
        w.Write([]byte(`{"status":"ok"}`))
    })

    return withMiddleware(mux)
}
```

### 8.2 transport/http/middleware.go

```go
func withMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // 1. CORS (local origins only)
        origin := r.Header.Get("Origin")
        if origin == "" || isLocalOrigin(origin) {
            w.Header().Set("Access-Control-Allow-Origin", origin)
        }
        w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
        if r.Method == http.MethodOptions {
            w.WriteHeader(http.StatusNoContent)
            return
        }

        // 2. Request logging (structured)
        start := time.Now()
        defer func() {
            slog.Info("http",
                "method", r.Method,
                "path", r.URL.Path,
                "duration_ms", time.Since(start).Milliseconds(),
            )
        }()

        next.ServeHTTP(w, r)
    })
}
```

---

## 9. 插件系统

### 9.1 设计决策

| 当前 | 重构后 |
|------|--------|
| 插件直接 import 彼此 | 插件只依赖 `port` 接口 |
| 插件通过类型断言获取依赖 | 插件通过 `PluginContext` 注入依赖 |
| 插件注册在 `app.go domainReady()` | 插件在 `wire.go` 中声明式注入 |
| 插件代码散落 internal/plugins/ | 插件移到顶层 `plugin/builtin/` |

### 9.2 插件接口

```go
// plugin/sdk/plugin.go

// Plugin is the base interface all plugins must implement.
type Plugin interface {
    Info() PluginInfo
    Init(ctx PluginContext) error    // called once during startup
    Start() error                     // activate
    Stop() error                      // deactivate
    IsRunning() bool
}

type PluginInfo struct {
    Name        string
    Version     string
    Description string
    Priority    int
    Requires    []string
}

// PluginContext exposes shared services to plugins.
// Plugins CANNOT access other plugins directly — only via EventBus.
type PluginContext struct {
    Context           context.Context
    EventBus          port.EventBus
    Config            map[string]any
    Logger            *slog.Logger
    LLMExecutor       port.LLMExecutor
    MemoryStore       port.MemoryStore
    EvidenceEngine    port.EvidenceEngine
    EmotionManager    port.EmotionStateManager
    IntentScheduler   port.IntentScheduler
}
```

### 9.3 内置插件职责

| 插件 | 职责 | 输入 | 输出 |
|------|------|------|------|
| `chat` | 对话处理：system prompt构建、OnBeforeChat/OnAfterChat | port.LLMExecutor, MemoryStore | 回复文本 |
| `memory` | 记忆管线：提取、去重、检索、压缩 | LLMExecutor, MemoryStore, EvidenceEngine | 记忆检索结果 |
| `vision` | 视觉理解：截图OCR + 图像分析 | LLMExecutor, OCREngine | 分析文本 |
| `search` | 网页搜索 | HTTP Client | 搜索结果 |
| `qq` | QQ消息收发 | WebSocket Client | 消息事件 |
| `timer` | 定时提醒（新增） | IntentScheduler | ProactiveIntent |

---

## 10. 生命周期管理

### 10.1 模块接口

```go
// internal/app/launcher.go

type Module interface {
    Name() string
    Init(ctx context.Context) error   // allocate resources, connect DB
    Start(ctx context.Context) error  // spawn goroutines, start listeners
    Stop(ctx context.Context) error   // graceful shutdown with timeout
    Health(ctx context.Context) error // readiness check
}
```

### 10.2 启动顺序

```
Phase 0: Infrastructure (串行)
  ConfigManager → Logger → SQLite → LLMGateway → EmbeddingService

Phase 1: Core Services (并行)
  MemoryService  |  EmotionService  |  PerceptionService  |  IdentityService

Phase 2: Composite Services (依赖 Phase 1)
  ChatService (依赖 Memory + LLM + Emotion)
  CognitionService (依赖 Memory + Emotion + Perception + EventBus)
  CareService (依赖 Emotion + Perception)
  ProactiveService (依赖 IntentScheduler + DeliveryGate)

Phase 3: Transport
  HTTP Server → 开始接受请求

Phase 4: Plugins (并行，依赖 Phase 2)
  ChatPlugin / MemoryPlugin / VisionPlugin / SearchPlugin / QQPlugin / TimerPlugin
```

### 10.3 关闭顺序

```
Phase 0: HTTP Server (先停外部流量)
Phase 1: Plugins (逆序 Stop)
Phase 2: Composite Services (逆序 Stop，带超时)
Phase 3: Core Services (逆序 Stop)
Phase 4: Infrastructure (Close DB, flush logs)
```

### 10.4 Launcher 实现

```go
type SionLauncher struct {
    modules  []Module
    signalCh chan os.Signal
}

func (l *SionLauncher) Run() error {
    // Start phases 0-4
    if err := l.startPhases(); err != nil {
        return fmt.Errorf("startup: %w", err)
    }

    // Wait for signal
    sig := <-l.signalCh
    slog.Info("received signal, shutting down", "signal", sig.String())

    // Stop phases 0-4 (reverse order, each with timeout)
    return l.stopPhases()
}

func (l *SionLauncher) stopPhases() error {
    for i := len(l.modules) - 1; i >= 0; i-- {
        m := l.modules[i]
        ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
        defer cancel()

        slog.Info("stopping module", "name", m.Name())
        if err := m.Stop(ctx); err != nil {
            slog.Error("module stop failed", "name", m.Name(), "error", err)
        }
    }
    return nil
}
```

---

## 11. DI 注入方案

### 11.1 Wire 依赖集定义

```go
// internal/app/wire.go

//go:build wireinject
// +build wireinject

package app

import "github.com/google/wire"

// ── Infrastructure Set ──
var InfraSet = wire.NewSet(
    adapter.NewSQLiteStore,
    adapter.NewLLMGateway,
    adapter.NewOllamaEmbedding,
    adapter.NewScreenObserver,
    adapter.NewOCREngine,
    adapter.NewEvidenceEngine,
    adapter.NewEventBus,
    adapter.NewConfigManager,
    wire.Bind(new(port.MemoryStore), new(*adapter.SQLiteStore)),
    wire.Bind(new(port.LLMExecutor), new(*adapter.LLMGateway)),
    wire.Bind(new(port.EmbeddingService), new(*adapter.OllamaEmbedding)),
    wire.Bind(new(port.ScreenObserver), new(*adapter.ScreenObserverImpl)),
    wire.Bind(new(port.EvidenceEngine), new(*adapter.EvidenceEngineImpl)),
    wire.Bind(new(port.EventBus), new(*adapter.EventBusImpl)),
    wire.Bind(new(port.ConfigManager), new(*adapter.ConfigManagerImpl)),
)

// ── Domain Set ──
var DomainSet = wire.NewSet(
    // Pure domain objects — no dependencies
    wire.Value(cognition.DefaultFeatureComputer),
    wire.Value(cognition.DefaultDriveComputer),
    wire.Value(cognition.DefaultActionScorer),
    wire.Value(cognition.DefaultDecisionRouter),
    wire.Value(cognition.DefaultNeedModel),
    wire.Bind(new(port.FeatureComputer), new(cognition.FeatureComputerImpl)),
    wire.Bind(new(port.DriveComputer), new(cognition.DriveComputerImpl)),
    wire.Bind(new(port.ActionScorer), new(cognition.ActionScorerImpl)),
    wire.Bind(new(port.DecisionRouter), new(cognition.DecisionRouterImpl)),
    wire.Bind(new(port.NeedModel), new(cognition.NeedModelImpl)),
)

// ── Service Sets ──
var MemorySet = wire.NewSet(
    modules.NewMemoryService,
    modules.NewHybridRecall,
    wire.Bind(new(port.MemoryRecall), new(*modules.HybridRecall)),
)

var EmotionSet = wire.NewSet(
    modules.NewEmotionService,
    adapter.NewLLMEmotionEvaluator,
    adapter.NewRuleEmotionEvaluator,
    adapter.NewExpressionMapper,
    wire.Bind(new(port.EmotionStateManager), new(*modules.EmotionService)),
)

var CognitionSet = wire.NewSet(
    modules.NewCognitionService,
    modules.NewLearner,
    modules.NewStrategyAgent,
    modules.NewCuriosityEngine,
    wire.Bind(new(port.Learner), new(*modules.Learner)),
    wire.Bind(new(port.StrategyAgent), new(*modules.StrategyAgent)),
    wire.Bind(new(port.CuriosityEngine), new(*modules.CuriosityEngine)),
)

var ProactiveSet = wire.NewSet(
    modules.NewProactiveService,
    modules.NewIntentScheduler,
    modules.NewDeliveryGate,
    modules.NewIntentDeliverer,
    wire.Bind(new(port.IntentScheduler), new(*modules.IntentScheduler)),
    wire.Bind(new(port.DeliveryGate), new(*modules.DeliveryGate)),
    wire.Bind(new(port.IntentDeliverer), new(*modules.IntentDeliverer)),
)

var ChatSet = wire.NewSet(
    modules.NewChatService,
)

var TransportSet = wire.NewSet(
    http.NewRouter,
    http.NewConfigHandler,
    http.NewChatHandler,
    http.NewMemoryHandler,
    http.NewCognitionHandler,
    http.NewEmotionHandler,
    http.NewPluginHandler,
    http.NewIdentityHandler,
    http.NewStatsHandler,
    http.NewProactiveHandler,
    sse.NewBroker,
)

// ── Launcher ──
func InitializeLauncher() (*SionLauncher, error) {
    wire.Build(
        InfraSet,
        DomainSet,
        MemorySet,
        EmotionSet,
        CognitionSet,
        ProactiveSet,
        ChatSet,
        TransportSet,
        NewSionLauncher,
    )
    return nil, nil
}
```

### 11.2 Wire 生成

```bash
# 安装 Wire
go install github.com/google/wire/cmd/wire@latest

# 生成 wire_gen.go
cd internal/app && wire
```

---

## 12. 配置管理

### 12.1 配置结构

```yaml
# ~/.sion/config.yaml
version: 1

llm:
  provider: deepseek
  base_url: https://api.deepseek.com
  api_key: ${DEEPSEEK_API_KEY}
  chat_model: deepseek-chat
  embed_model: ""  # empty → use Ollama local

vision:
  provider: ""
  base_url: ""
  api_key: ""
  model: ""

emotion:
  provider: ""
  base_url: ""    # empty → fallback to llm config
  api_key: ""
  model: ""       # empty → fallback to llm config
  cloud_enabled: false  # false → use rule-based only

memory:
  evidence:
    rein_half_life_days: 60
    disp_half_life_days: 14
    confirmed_threshold: 1.0
    promoted_threshold: 2.0
    combo_threshold: 3
    combo_bonus: 0.5
  retrieval:
    bm25_budget: 10
    cosine_budget: 10
    rrf_k: 60
    final_budget: 5
  compression:
    l0_threshold: 20       # messages
    high_level_threshold: 3
    max_level: 3

proactive:
  chat_enabled: true
  vision_enabled: true
  min_interval_sec: 120
  max_daily_actions: 20
  modes:
    normal: {interval: 15min, all_sources: true}
    focus:  {interval: 60min, vision: false, news: false}
    frequent: {interval: 5min, all_sources: true}

user:
  name: ""
  tech_stack: []

warm_start:
  personality:
    annoyance_sensitivity: 0.5
    affection_warmth: 0.5
    worry_tendency: 0.5
  known_facts:
    - ""

plugins:
  memory:
    emotion_cloud_enabled: false
  search:
    bocha_api_key: ""
    bing_api_key: ""
```

### 12.2 ConfigManager 实现要点

- 从 `$HOME/.sion/config.yaml` 加载
- 环境变量覆盖（`${VAR_NAME}` 语法）
- 版本迁移（version 字段驱动）
- 配置校验（Validate 拒绝不合法值）
- 变更通知（OnChange callback → 热重载）

---

## 13. 迁移路径

### 13.1 渐进式迁移（不推倒重来）

```
Phase 1: 地基（第1-2周）
  ✅ 定义 port/ 层所有接口
  ✅ 实现 Launcher + Module 生命周期
  ✅ 实现 ConfigManager v2
  ✅ 引入 Wire，写 wire.go
  ❌ 不删旧代码，新旧并存

Phase 2: 迁移无依赖模块（第2-3周）
  ✅ domain/cognition/ → 直接搬，加 port 接口
  ✅ domain/emotion/ → 加 port 接口
  ✅ domain/memory/ → 加 types 定义
  ✅ domain/types/ → 整理共享类型

Phase 3: 迁移基础适配器（第3-5周）
  ✅ adapter/llm/ → 实现 port.LLMExecutor
  ✅ adapter/memory/sqlite_store → 实现 port.MemoryStore
  ✅ adapter/memory/evidence_engine → 实现 port.EvidenceEngine
  ✅ adapter/memory/recall → 实现 hybrid BM25+vector+RRF
  ✅ adapter/perception → 实现 port.ScreenObserver
  ✅ adapter/event → 实现 port.EventBus

Phase 4: 迁移组合服务（第5-7周）
  ✅ app/modules/chat_svc → 用 port 接口重写
  ✅ app/modules/cognition_svc → 用 port 接口重写 BackgroundLoop
  ✅ app/modules/memory_svc → 用 port 接口重写记忆管线
  ✅ app/modules/emotion_svc → 用 port 接口重写情绪生命周期
  ✅ app/modules/proactive_svc → 新增 IntentScheduler + DeliveryGate

Phase 5: 迁移传输层（第7-8周）
  ✅ transport/http/ → 拆分 handler_builder.go 的巨型闭包
  ✅ transport/sse/ → 独立 SSE broker
  ✅ transport/wails/ → 薄代理层 → HTTP API

Phase 6: 插件独立（第8-9周）
  ✅ 插件依赖从 concrete type → port 接口
  ✅ 插件移到 plugin/builtin/
  ✅ 插件通过 PluginContext 注入依赖

Phase 7: 清理（第9周）
  ✅ 删除旧 app.go domainReady() 方法
  ✅ 删除旧 handler_builder.go
  ✅ 统一日志规范
  ✅ 最终测试
```

### 13.2 测试策略

每个 Phase 的验收标准：
- Wire 编译通过（依赖完整性）
- 现有测试全部通过
- 新增 port 接口的 mock 测试
- 手动启动验证基础功能

---

## 附录A: 与旧架构的对应关系

| 旧文件 | 新位置 | 说明 |
|--------|--------|------|
| `app.go:domainReady()` | `internal/app/launcher.go` + `wire.go` | 拆分到Launcher+Wire |
| `handler_builder.go` | `internal/transport/http/*_handler.go` | 按资源拆分handler |
| `internal/service/cognition/` | `internal/domain/cognition/` | 保持算法，加port接口 |
| `internal/service/emotion/` | `internal/domain/emotion/` + `internal/adapter/emotion/` | 领域+适配器分离 |
| `internal/plugins/memory/` | `plugin/builtin/memory/` | 独立插件 |
| `internal/infra/storage/` | `internal/adapter/memory/` | 作为MemoryStore的实现 |
| `internal/infra/llm/` | `internal/adapter/llm/` | 作为LLMExecutor的实现 |

---

> **文档版本**: v1.0-rc1 | **作者**: 诗音开发团队 | **基于**: N.E.K.O + Sion v0.3 架构分析
